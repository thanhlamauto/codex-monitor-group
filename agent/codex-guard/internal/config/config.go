package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/atomicfile"
)

type Config struct {
	ServerURL           string `json:"server_url"`
	DeviceID            string `json:"device_id"`
	StudentName         string `json:"student_name"`
	DeviceLabel         string `json:"device_label"`
	PrivateKey          string `json:"private_key,omitempty"`
	ProtectedPrivateKey string `json:"private_key_protected,omitempty"`
	PublicKey           string `json:"public_key"`
	OTLPToken           string `json:"otlp_token,omitempty"`
	ProtectedOTLPToken  string `json:"otlp_token_protected,omitempty"`
	KeyProtection       string `json:"key_protection"`
	CodexHome           string `json:"codex_home"`
	CodexPath           string `json:"codex_path"`
	CCUsagePath         string `json:"ccusage_path"`
	AgentPath           string `json:"agent_path"`
	StateDir            string `json:"state_dir"`
	Timezone            string `json:"timezone"`
	HeartbeatSeconds    int    `json:"heartbeat_seconds"`
	UsageSeconds        int    `json:"usage_seconds"`
	IntegritySeconds    int    `json:"integrity_seconds"`
	RetentionDays       int    `json:"retention_days"`
	NextSequence        uint64 `json:"next_sequence"`
	ExpectedConfigHash  string `json:"expected_config_hash"`
	OTelListen          string `json:"otel_listen"`
}

func (c *Config) Defaults() {
	if c.HeartbeatSeconds == 0 {
		c.HeartbeatSeconds = 60
	}
	if c.UsageSeconds == 0 {
		c.UsageSeconds = 300
	}
	if c.IntegritySeconds == 0 {
		c.IntegritySeconds = 60
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = 30
	}
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if c.CodexPath == "" {
		c.CodexPath = "codex"
	}
	if c.NextSequence == 0 {
		c.NextSequence = 1
	}
	if c.OTelListen == "" {
		c.OTelListen = "127.0.0.1:9464"
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.PrivateKey == "" && c.ProtectedPrivateKey != "" {
		c.PrivateKey, err = unprotectSecret(c.ProtectedPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("unprotect device private key: %w", err)
		}
	}
	if c.OTLPToken == "" && c.ProtectedOTLPToken != "" {
		c.OTLPToken, err = unprotectSecret(c.ProtectedOTLPToken)
		if err != nil {
			return nil, fmt.Errorf("unprotect OTel token: %w", err)
		}
	}
	c.Defaults()
	if c.ServerURL == "" || c.DeviceID == "" || c.PrivateKey == "" {
		return nil, errors.New("config is incomplete")
	}
	return &c, nil
}

func Save(path string, c *Config) error {
	c.Defaults()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	disk := *c
	if secureSecretStorage() {
		var err error
		if c.PrivateKey != "" {
			disk.ProtectedPrivateKey, err = protectSecret(c.PrivateKey)
			if err != nil {
				return fmt.Errorf("protect device private key: %w", err)
			}
		}
		if c.OTLPToken != "" {
			disk.ProtectedOTLPToken, err = protectSecret(c.OTLPToken)
			if err != nil {
				return fmt.Errorf("protect OTel token: %w", err)
			}
		}
		disk.PrivateKey = ""
		disk.OTLPToken = ""
		disk.KeyProtection = protectionName()
		c.KeyProtection = disk.KeyProtection
	} else {
		disk.ProtectedPrivateKey = ""
		disk.ProtectedOTLPToken = ""
		disk.KeyProtection = protectionName()
		c.KeyProtection = disk.KeyProtection
	}
	data, err := json.MarshalIndent(&disk, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		return err
	}
	return atomicfile.Replace(tmp, path)
}
