package agent

import (
	"context"
	"fmt"
	"time"
)

func (a *Client) Run(ctx context.Context) error {
	if err := a.StartOTelServer(ctx); err != nil {
		return err
	}
	backoff := time.Second
	lastUsage := time.Time{}
	lastIntegrity := time.Time{}
	for {
		now := time.Now()
		if err := a.OTelUsage(); err != nil {
			fmt.Printf("OTel usage queued/failed: %v\n", err)
		}
		if now.Sub(lastUsage) >= time.Duration(a.Config.UsageSeconds)*time.Second {
			if err := a.Usage(); err != nil {
				fmt.Printf("usage queued/failed: %v\n", err)
			}
			lastUsage = now
		}
		if now.Sub(lastIntegrity) >= time.Duration(a.Config.IntegritySeconds)*time.Second {
			if err := a.Integrity(); err != nil {
				fmt.Printf("integrity queued/failed: %v\n", err)
			}
			lastIntegrity = now
		}
		if err := a.Heartbeat(); err != nil {
			fmt.Printf("heartbeat queued: %v\n", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		} else {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(a.Config.HeartbeatSeconds) * time.Second):
		}
	}
}

func (a *Client) Cycle() error {
	var first error
	if err := a.Usage(); err != nil {
		first = err
	}
	if err := a.OTelUsage(); err != nil && first == nil {
		first = err
	}
	if err := a.Integrity(); err != nil && first == nil {
		first = err
	}
	if err := a.Heartbeat(); err != nil && first == nil {
		first = err
	}
	return first
}
