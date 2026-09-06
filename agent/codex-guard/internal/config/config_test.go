package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := &Config{ServerURL: "https://meter.example", DeviceID: "device", PrivateKey: "private", OTLPToken: "otel", StateDir: t.TempDir()}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateKey != "private" || got.OTLPToken != "otel" {
		t.Fatalf("secret round trip failed: %#v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if secureSecretStorage() && (string(data) == "" || got.KeyProtection != protectionName()) {
		t.Fatal("secure storage metadata missing")
	}
	if secureSecretStorage() && strings.Contains(string(data), `"private_key": "private"`) {
		t.Fatal("private key was written in plaintext")
	}
}
