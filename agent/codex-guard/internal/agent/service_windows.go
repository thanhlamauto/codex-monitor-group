//go:build windows

package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "CodexGuard"

type windowsService struct{ client *Client }

func (service *windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() { errors <- service.client.Run(ctx) }()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-errors:
			if err != nil {
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-errors; err != nil {
					return true, 1
				}
				return false, 0
			}
		}
	}
}

func RunAsPlatformService(client *Client) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, err
	}
	return true, svc.Run(windowsServiceName, &windowsService{client: client})
}

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
	if !posture.ServiceInstalled {
		return fmt.Errorf("Windows service %s is missing", windowsServiceName)
	}
	out, err := exec.Command("sc.exe", "start", windowsServiceName).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToUpper(string(out)), "RUNNING") {
		_ = a.SecurityEvent("SERVICE_RESTART_FAILED", map[string]any{"service_fingerprint": posture.ServiceFingerprint})
		return fmt.Errorf("restart Windows service: %w", err)
	}
	return eventErr
}
