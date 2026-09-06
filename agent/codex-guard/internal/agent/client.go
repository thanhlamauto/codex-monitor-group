package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/example/codex-classroom-monitor/agent/codex-guard/internal/config"
	"github.com/example/codex-classroom-monitor/agent/codex-guard/internal/protocol"
	queuepkg "github.com/example/codex-classroom-monitor/agent/codex-guard/internal/queue"
)

const Version = "1.2.0"

type Client struct {
	ConfigPath string
	Config     *config.Config
	HTTP       *http.Client
	Queue      *queuepkg.Queue
	started    time.Time
	mu         sync.Mutex
	otelMu     sync.Mutex
	quotaMu    sync.Mutex
	quota      *QuotaSnapshot
	quotaRead  time.Time
}

type EnrollResponse struct {
	DeviceID          string `json:"device_id"`
	StudentName       string `json:"student_name"`
	DeviceLabel       string `json:"device_label"`
	OTLPToken         string `json:"otlp_token"`
	HeartbeatSeconds  int    `json:"heartbeat_seconds"`
	ClassroomTimezone string `json:"classroom_timezone"`
}

func New(configPath string) (*Client, error) {
	c, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return &Client{ConfigPath: configPath, Config: c, HTTP: &http.Client{Timeout: 20 * time.Second}, Queue: queuepkg.New(filepath.Join(c.StateDir, "queue.json"), c.RetentionDays), started: time.Now()}, nil
}

func Enroll(server, name, label, codexHome, codexPath, ccusagePath, agentPath, stateDir, configPath string) (*config.Config, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{"name": strings.TrimSpace(name), "public_key": base64.StdEncoding.EncodeToString(public), "device_label": strings.TrimSpace(label)})
	response, err := (&http.Client{Timeout: 20 * time.Second}).Post(strings.TrimRight(server, "/")+"/api/v1/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connect to server: %w", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("enrollment failed (%s): %s", response.Status, strings.TrimSpace(string(data)))
	}
	var enrolled EnrollResponse
	if err := json.Unmarshal(data, &enrolled); err != nil {
		return nil, err
	}
	c := &config.Config{ServerURL: strings.TrimRight(server, "/"), DeviceID: enrolled.DeviceID, StudentName: enrolled.StudentName, DeviceLabel: enrolled.DeviceLabel, PrivateKey: base64.StdEncoding.EncodeToString(private), PublicKey: base64.StdEncoding.EncodeToString(public), OTLPToken: enrolled.OTLPToken, CodexHome: codexHome, CodexPath: codexPath, CCUsagePath: ccusagePath, AgentPath: agentPath, StateDir: stateDir, Timezone: enrolled.ClassroomTimezone, HeartbeatSeconds: enrolled.HeartbeatSeconds}
	c.Defaults()
	if err := config.Save(configPath, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (a *Client) enqueue(endpoint string, payload any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	keyBytes, err := base64.StdEncoding.DecodeString(a.Config.PrivateKey)
	if err != nil {
		return err
	}
	eventID, err := randomID()
	if err != nil {
		return err
	}
	envelope, err := protocol.Sign(a.Config.DeviceID, a.Config.NextSequence, eventID, payload, ed25519.PrivateKey(keyBytes), time.Now())
	if err != nil {
		return err
	}
	a.Config.NextSequence++
	if err := config.Save(a.ConfigPath, a.Config); err != nil {
		return err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return a.Queue.Push(queuepkg.Item{Endpoint: endpoint, CreatedAt: time.Now().UTC(), Envelope: data})
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (a *Client) Flush() error {
	for {
		item, err := a.Queue.Peek()
		if err != nil {
			return err
		}
		if item == nil {
			return nil
		}
		request, err := http.NewRequest(http.MethodPost, a.Config.ServerURL+item.Endpoint, bytes.NewReader(item.Envelope))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := a.HTTP.Do(request)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("server returned %s: %s", response.Status, strings.TrimSpace(string(data)))
		}
		if err := a.Queue.Pop(); err != nil {
			return err
		}
	}
}

func shaFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unavailable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(h.Sum(nil))
}
func opaquePath(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

func (a *Client) Heartbeat() error {
	configHash, configOK, _ := TelemetryStatus(a.Config.CodexHome, a.Config.ServerURL, a.Config.OTLPToken)
	payload := map[string]any{"agent_version": Version, "agent_sha256": shaFile(a.Config.AgentPath), "ccusage_version": CCUsageVersion(a.Config.CCUsagePath), "ccusage_sha256": shaFile(a.Config.CCUsagePath), "codex_version": CodexVersion(a.Config.CodexPath), "codex_home": opaquePath(a.Config.CodexHome), "config_fingerprint": configHash, "telemetry_config_ok": configOK, "uptime_seconds": int64(time.Since(a.started).Seconds()), "os": runtime.GOOS, "arch": runtime.GOARCH}
	if quota := a.cachedQuota(); quota != nil {
		payload["quota"] = quota
	}
	if err := a.enqueue("/api/v1/heartbeat", payload); err != nil {
		return err
	}
	return a.Flush()
}
