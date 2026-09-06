//go:build !windows

package config

import "fmt"

func secureSecretStorage() bool { return false }
func protectionName() string    { return "root-owned-file" }

func protectSecret(string) (string, error) {
	return "", fmt.Errorf("platform secret sealing is unavailable")
}

func unprotectSecret(string) (string, error) {
	return "", fmt.Errorf("protected Windows secret cannot be opened on this platform")
}
