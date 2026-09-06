//go:build !windows

package agent

import (
	"fmt"
	"os/exec"
	"runtime"
)

func RunAsPlatformService(*Client) (bool, error) { return false, nil }

func (a *Client) Watchdog() error {
	posture := a.MonitorPosture()
	// The running agent reports configuration/permission drift in its integrity
	// snapshots. Avoid touching its shared sequence and queue from a second
	// process; the watchdog owns only the stopped-service recovery path.
	if posture.ServiceRunning {
		return nil
	}
	details := map[string]any{
		"service_installed":   posture.ServiceInstalled,
		"service_running":     posture.ServiceRunning,
		"auto_start_ok":       posture.AutoStartOK,
		"restart_policy_ok":   posture.RestartPolicyOK,
		"service_fingerprint": posture.ServiceFingerprint,
	}
	eventErr := a.SecurityEvent("SERVICE_STOPPED_OR_MODIFIED", details)
	var command *exec.Cmd
	if runtime.GOOS == "linux" {
		command = exec.Command("systemctl", "start", "codex-guard.service")
	} else if runtime.GOOS == "darwin" {
		command = exec.Command("launchctl", "kickstart", "-k", "system/com.openai.codex-guard")
	} else {
		return fmt.Errorf("watchdog is unsupported on %s", runtime.GOOS)
	}
	if err := command.Run(); err != nil {
		_ = a.SecurityEvent("SERVICE_RESTART_FAILED", map[string]any{"service_fingerprint": posture.ServiceFingerprint})
		return err
	}
	return eventErr
}
