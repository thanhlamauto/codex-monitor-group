//go:build windows

package agent

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
)

func windowsOutput(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return string(out)
}

func protectedWindowsACL(path string) bool {
	sddl := windowsOutput("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "$p=$args[0]; (Get-Acl -LiteralPath $p).Sddl", path)
	upper := strings.ToUpper(sddl)
	if !strings.Contains(upper, ";;;SY)") || !strings.Contains(upper, ";;;BA)") {
		return false
	}
	for _, broad := range []string{";;;BU)", ";;;WD)", ";;;AU)"} {
		if strings.Contains(upper, broad) {
			return false
		}
	}
	return true
}

func inspectMonitorPosture(configPath string, c *config.Config) MonitorPosture {
	query := windowsOutput("sc.exe", "query", "CodexGuard")
	qc := windowsOutput("sc.exe", "qc", "CodexGuard")
	failure := windowsOutput("sc.exe", "qfailure", "CodexGuard")
	watchdog := windowsOutput("schtasks.exe", "/Query", "/TN", "CodexGuardWatchdog", "/XML")
	upperQuery, upperQC, upperFailure := strings.ToUpper(query), strings.ToUpper(qc), strings.ToUpper(failure)
	return MonitorPosture{
		OS:                  runtime.GOOS,
		ServiceManager:      "windows-scm",
		ServiceInstalled:    strings.Contains(upperQC, "SERVICE_NAME") && strings.Contains(upperQC, "CODEXGUARD"),
		ServiceRunning:      strings.Contains(upperQuery, "RUNNING"),
		AutoStartOK:         strings.Contains(upperQC, "AUTO_START"),
		RestartPolicyOK:     strings.Count(upperFailure, "RESTART") >= 2,
		WatchdogExpected:    true,
		WatchdogOK:          strings.Contains(strings.ToUpper(watchdog), "CODEX-GUARD.EXE") && strings.Contains(strings.ToUpper(watchdog), "WATCHDOG"),
		AgentPermissionsOK:  protectedWindowsACL(c.AgentPath),
		ConfigPermissionsOK: protectedWindowsACL(configPath),
		KeyProtection:       c.KeyProtection,
		ServiceFingerprint:  postureFingerprint(qc, failure, watchdog),
	}
}
