package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ServerURL          string `json:"server_url"`
	DeviceID           string `json:"device_id"`
	StudentName        string `json:"student_name"`
	DeviceLabel        string `json:"device_label"`
	PrivateKey         string `json:"private_key"`
	PublicKey          string `json:"public_key"`
	OTLPToken          string `json:"otlp_token"`
	CodexHome          string `json:"codex_home"`
	CodexPath          string `json:"codex_path"`
	CCUsagePath        string `json:"ccusage_path"`
	AgentPath          string `json:"agent_path"`
	StateDir           string `json:"state_dir"`
	Timezone           string `json:"timezone"`
	HeartbeatSeconds   int    `json:"heartbeat_seconds"`
	UsageSeconds       int    `json:"usage_seconds"`
	IntegritySeconds   int    `json:"integrity_seconds"`
	RetentionDays      int    `json:"retention_days"`
	NextSequence       uint64 `json:"next_sequence"`
	ExpectedConfigHash string `json:"expected_config_hash"`
	OTelListen         string `json:"otel_listen"`
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
	data, err := json.MarshalIndent(c, "", "  ")
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
	return os.Rename(tmp, path)
}
