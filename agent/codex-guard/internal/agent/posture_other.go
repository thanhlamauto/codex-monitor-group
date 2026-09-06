//go:build !windows

package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
)

func rootOwnedNotWritableByUsers(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

func commandOK(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func inspectMonitorPosture(configPath string, c *config.Config) MonitorPosture {
	posture := MonitorPosture{
		OS:                  runtime.GOOS,
		AgentPermissionsOK:  rootOwnedNotWritableByUsers(c.AgentPath),
		ConfigPermissionsOK: rootOwnedNotWritableByUsers(configPath),
		KeyProtection:       c.KeyProtection,
	}
	var definition string
	if runtime.GOOS == "linux" {
		posture.ServiceManager = "systemd"
		data, err := os.ReadFile("/etc/systemd/system/codex-guard.service")
		definition = string(data)
		posture.ServiceInstalled = err == nil
		posture.ServiceRunning = commandOK("systemctl", "is-active", "--quiet", "codex-guard.service")
		posture.AutoStartOK = commandOK("systemctl", "is-enabled", "--quiet", "codex-guard.service")
		posture.RestartPolicyOK = strings.Contains(definition, "Restart=always") && strings.Contains(definition, "RestartSec=5") && strings.Contains(definition, "NoNewPrivileges=true")
		watchdog, watchdogErr := os.ReadFile("/etc/systemd/system/codex-guard-watchdog.timer")
		posture.WatchdogExpected = true
		posture.WatchdogOK = watchdogErr == nil && strings.Contains(string(watchdog), "OnUnitActiveSec=2min") && commandOK("systemctl", "is-enabled", "--quiet", "codex-guard-watchdog.timer")
		definition += "\n" + string(watchdog)
	} else if runtime.GOOS == "darwin" {
		posture.ServiceManager = "launchd"
		data, err := os.ReadFile("/Library/LaunchDaemons/com.openai.codex-guard.plist")
		definition = string(data)
		posture.ServiceInstalled = err == nil
		posture.ServiceRunning = commandOK("launchctl", "print", "system/com.openai.codex-guard")
		posture.AutoStartOK = strings.Contains(definition, "<key>RunAtLoad</key><true/>")
		posture.RestartPolicyOK = strings.Contains(definition, "<key>KeepAlive</key><true/>")
		watchdog, watchdogErr := os.ReadFile("/Library/LaunchDaemons/com.openai.codex-guard-watchdog.plist")
		posture.WatchdogExpected = true
		posture.WatchdogOK = watchdogErr == nil && strings.Contains(string(watchdog), "<key>StartInterval</key><integer>120</integer>") && commandOK("launchctl", "print", "system/com.openai.codex-guard-watchdog")
		definition += "\n" + string(watchdog)
	} else {
		posture.ServiceManager = "unsupported"
		posture.WatchdogExpected = false
		posture.WatchdogOK = true
	}
	posture.ServiceFingerprint = postureFingerprint(runtime.GOOS, definition)
	return posture
}
