package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/agent"
	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
)

func defaultConfigPath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "CodexGuard", "config.json")
	}
	return "/etc/codex-guard/config.json"
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "enroll":
		err = enroll(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	case "once":
		err = once(os.Args[2:])
	case "status":
		err = status(os.Args[2:])
	case "configure-telemetry":
		err = configure(os.Args[2:])
	case "watchdog":
		err = watchdog(os.Args[2:])
	case "uninstall-notice":
		err = uninstallNotice(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("codex-guard %s\n", agent.Version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "codex-guard: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: codex-guard <enroll|run|once|status|configure-telemetry|watchdog|uninstall-notice|version>")
}

func enroll(args []string) error {
	f := flag.NewFlagSet("enroll", flag.ContinueOnError)
	server := f.String("server", "", "server URL")
	name := f.String("name", "", "display name")
	label := f.String("device-label", "", "device label")
	home := f.String("codex-home", "", "Codex home")
	cc := f.String("ccusage", "/usr/local/lib/codex-guard/ccusage", "ccusage binary")
	codex := f.String("codex", "codex", "Codex binary")
	binary := f.String("agent", "/usr/local/bin/codex-guard", "agent binary")
	state := f.String("state-dir", "/var/lib/codex-guard", "state directory")
	configPath := f.String("config", defaultConfigPath(), "config path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *server == "" || strings.TrimSpace(*name) == "" || strings.TrimSpace(*label) == "" || *home == "" {
		return fmt.Errorf("--server, --name, --device-label, and --codex-home are required")
	}
	_, err := agent.Enroll(*server, *name, *label, *home, *codex, *cc, *binary, *state, *configPath)
	return err
}

func load(args []string, name string) (*agent.Client, error) {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	path := f.String("config", defaultConfigPath(), "config path")
	if err := f.Parse(args); err != nil {
		return nil, err
	}
	return agent.New(*path)
}
func once(args []string) error {
	a, err := load(args, "once")
	if err != nil {
		return err
	}
	return a.Heartbeat()
}
func run(args []string) error {
	a, err := load(args, "run")
	if err != nil {
		return err
	}
	handled, serviceErr := agent.RunAsPlatformService(a)
	if serviceErr != nil {
		return serviceErr
	}
	if handled {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.Run(ctx)
}

func watchdog(args []string) error {
	a, err := load(args, "watchdog")
	if err != nil {
		return err
	}
	return a.Watchdog()
}

func uninstallNotice(args []string) error {
	a, err := load(args, "uninstall-notice")
	if err != nil {
		return err
	}
	return a.SecurityEvent("AGENT_UNINSTALL_REQUESTED", map[string]any{"requested": true})
}

func status(args []string) error {
	a, err := load(args, "status")
	if err != nil {
		return err
	}
	resolution := agent.ResolveCodexHome(a.Config.CodexHome)
	_, telemetryOK, fpErr := agent.TelemetryStatus(resolution.Path, a.Config.ServerURL, a.Config.OTLPToken)
	fmt.Printf("Student      %s\nDevice       %s\nAgent        %s\nServer       %s\nCodex        %s\nCodex home   %s (%s)\nTelemetry    %s\nLogs         %s\nccusage      %s\nQueue        %d pending\n", a.Config.StudentName, a.Config.DeviceLabel, localAgentStatus(), serverStatus(a.Config.ServerURL), agent.CodexVersion(a.Config.CodexPath), resolution.Path, resolution.Source, telemetryStatus(telemetryOK, fpErr), logs(a.Config.CodexHome), agent.CCUsageVersion(a.Config.CCUsagePath), a.Queue.Len())
	return nil
}
func telemetryStatus(valid bool, err error) string {
	if err == nil && valid {
		return "enabled"
	}
	return "changed/not configured"
}
func localAgentStatus() string {
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://127.0.0.1:9464/v1/logs")
	if err != nil {
		return "not running"
	}
	response.Body.Close()
	return "running"
}
func serverStatus(server string) string {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(strings.TrimRight(server, "/") + "/healthz")
	if err != nil {
		return "unreachable"
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return "connected"
	}
	return fmt.Sprintf("HTTP %d", response.StatusCode)
}
func logs(home string) string {
	resolution := agent.ResolveCodexHome(home)
	if resolution.LogCount > 0 {
		return fmt.Sprintf("found (%d JSONL)", resolution.LogCount)
	}
	return "not found"
}

func configure(args []string) error {
	f := flag.NewFlagSet("configure-telemetry", flag.ContinueOnError)
	path := f.String("config", defaultConfigPath(), "config path")
	if err := f.Parse(args); err != nil {
		return err
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	resolution := agent.ResolveCodexHome(c.CodexHome)
	if resolution.Path != "" {
		c.CodexHome = resolution.Path
	}
	hash, err := agent.ConfigureTelemetry(c.CodexHome, c.ServerURL, c.OTLPToken)
	if err != nil {
		return err
	}
	c.ExpectedConfigHash = hash
	if err := config.Save(*path, c); err != nil {
		return err
	}
	fmt.Printf("Codex home   %s (%s, %d JSONL)\n", c.CodexHome, resolution.Source, resolution.LogCount)
	return nil
}
