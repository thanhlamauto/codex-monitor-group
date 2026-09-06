package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/example/codex-classroom-monitor/agent/codex-guard/internal/agent"
	"github.com/example/codex-classroom-monitor/agent/codex-guard/internal/config"
)

const defaultConfig = "/etc/codex-guard/config.json"

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
	fmt.Fprintln(os.Stderr, "Usage: codex-guard <enroll|run|once|status|configure-telemetry|version>")
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
	configPath := f.String("config", defaultConfig, "config path")
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
	path := f.String("config", defaultConfig, "config path")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.Run(ctx)
}

func status(args []string) error {
	a, err := load(args, "status")
	if err != nil {
		return err
	}
	_, telemetryOK, fpErr := agent.TelemetryStatus(a.Config.CodexHome, a.Config.ServerURL, a.Config.OTLPToken)
	fmt.Printf("Student      %s\nDevice       %s\nAgent        %s\nServer       %s\nCodex        %s\nTelemetry    %s\nLogs         %s\nccusage      %s\nQueue        %d pending\n", a.Config.StudentName, a.Config.DeviceLabel, localAgentStatus(), serverStatus(a.Config.ServerURL), agent.CodexVersion(a.Config.CodexPath), telemetryStatus(telemetryOK, fpErr), logs(a.Config.CodexHome), agent.CCUsageVersion(a.Config.CCUsagePath), a.Queue.Len())
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
	matches, _ := filepath.Glob(filepath.Join(home, "sessions", "*"))
	if len(matches) > 0 {
		return "found"
	}
	return "not found"
}

func configure(args []string) error {
	f := flag.NewFlagSet("configure-telemetry", flag.ContinueOnError)
	path := f.String("config", defaultConfig, "config path")
	if err := f.Parse(args); err != nil {
		return err
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	hash, err := agent.ConfigureTelemetry(c.CodexHome, c.ServerURL, c.OTLPToken)
	if err != nil {
		return err
	}
	c.ExpectedConfigHash = hash
	return config.Save(*path, c)
}
