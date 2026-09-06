# Operations

Back up the `postgres_data` Docker volume and Caddy data. Audit rows have no delete route but database administrators remain trusted. Monitor `/healthz`, container health, disk, and PostgreSQL backups.

To upgrade, pull a reviewed release and run `docker compose up -d --build`. Existing database tables are preserved. For the MVP, review model changes before deployment because automatic schema migration is not yet included.

## Device protection checks

- Linux: inspect `codex-guard.service`, `codex-guard-watchdog.timer`, and their journal entries with `systemctl`/`journalctl`.
- macOS: inspect `system/com.openai.codex-guard` and `system/com.openai.codex-guard-watchdog` with `launchctl print`.
- Windows: inspect `Get-Service CodexGuard`, `sc.exe qc CodexGuard`, `sc.exe qfailure CodexGuard`, and `schtasks.exe /Query /TN CodexGuardWatchdog /XML` from Administrator PowerShell.

The primary service and watchdog are deliberately independent. If the service is stopped, the watchdog attempts a restart and queues a signed audit event. If recovery/autostart/permissions are weakened while the service remains running, its next signed integrity snapshot reports that drift. If both execution paths stop, the server changes the device from ONLINE to LATE and then UNREACHABLE. Treat these signals as evidence that telemetry became unavailable, not as an automatic cheating verdict.

Re-running the installer with the upgrade option restores checksummed binaries, permissions, service recovery settings, watchdog configuration, and managed Codex OTel settings. Explicit uninstall sends a signed notice when the server is reachable and preserves local credentials/checkpoints for incident review or recovery.
