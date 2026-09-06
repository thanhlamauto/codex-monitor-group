# Operations

Back up the `postgres_data` Docker volume and Caddy data. Audit rows have no delete route but database administrators remain trusted. Monitor `/healthz`, container health, disk, and PostgreSQL backups.

On Vercel, the authenticated daily cron samples reachability and deletes only bounded raw telemetry: heartbeat and integrity snapshots default to 30 days, while idempotency receipts default to 35 days to outlive the agent's 30-day offline queue. Usage reports and security events are not deleted. Override the documented `*_RETENTION_DAYS` variables only after preserving that queue/receipt relationship.

To move providers, set `SOURCE_DATABASE_URL` and `TARGET_DATABASE_URL` in the local process and run `PYTHONPATH=server python3 scripts/migrate_database.py`. The target must be empty. The script creates the current schema and copies all legacy tables in one target transaction while preserving stable student/device IDs, device public keys, sequence counters, usage, and audit history. It never prints either connection string.

To upgrade, pull a reviewed release and run `docker compose up -d --build`. Existing database tables are preserved. For the MVP, review model changes before deployment because automatic schema migration is not yet included.

## Device protection checks

- Linux: inspect `codex-guard.service`, `codex-guard-watchdog.timer`, and their journal entries with `systemctl`/`journalctl`.
- macOS: inspect `system/com.openai.codex-guard` and `system/com.openai.codex-guard-watchdog` with `launchctl print`.
- Windows: inspect `Get-Service CodexGuard`, `sc.exe qc CodexGuard`, `sc.exe qfailure CodexGuard`, and `schtasks.exe /Query /TN CodexGuardWatchdog /XML` from Administrator PowerShell.

The primary service and watchdog are deliberately independent. If the service is stopped, the watchdog attempts a restart and queues a signed audit event. If recovery/autostart/permissions are weakened while the service remains running, its next signed integrity snapshot reports that drift. If both execution paths stop, the server changes the device from ONLINE to LATE and then UNREACHABLE. Treat these signals as evidence that telemetry became unavailable, not as an automatic cheating verdict.

Re-running the installer with the upgrade option restores checksummed binaries, permissions, service recovery settings, watchdog configuration, and managed Codex OTel settings. Explicit uninstall sends a signed notice when the server is reachable and preserves local credentials/checkpoints for incident review or recovery.
