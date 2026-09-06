package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type MonitorPosture struct {
	OS                  string `json:"os"`
	ServiceManager      string `json:"service_manager"`
	ServiceInstalled    bool   `json:"service_installed"`
	ServiceRunning      bool   `json:"service_running"`
	AutoStartOK         bool   `json:"auto_start_ok"`
	RestartPolicyOK     bool   `json:"restart_policy_ok"`
	WatchdogExpected    bool   `json:"watchdog_expected"`
	WatchdogOK          bool   `json:"watchdog_ok"`
	AgentPermissionsOK  bool   `json:"agent_permissions_ok"`
	ConfigPermissionsOK bool   `json:"config_permissions_ok"`
	KeyProtection       string `json:"key_protection"`
	ServiceFingerprint  string `json:"service_fingerprint"`
}

func postureFingerprint(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		clean = append(clean, strings.TrimSpace(strings.ReplaceAll(part, "\r\n", "\n")))
	}
	sum := sha256.Sum256([]byte(strings.Join(clean, "\n---\n")))
	return hex.EncodeToString(sum[:])
}

func (a *Client) MonitorPosture() MonitorPosture {
	return inspectMonitorPosture(a.ConfigPath, a.Config)
}
