# Codex Classroom Monitor

Public, shared token-usage and monitoring-integrity dashboard for a group using Codex CLI. Nobody needs a web account. Each person runs one installer command, enters a display name, and the device immediately registers itself on the dashboard with its own Ed25519 identity.

The web/API can also run on the free Vercel Hobby tier with CockroachDB Basic. Run `make vercel-release`, connect the repository to Vercel, create a Basic cluster, set its TLS connection string as `DATABASE_URL`, and deploy. See [Free Vercel deployment](docs/vercel.md).

The server never stores prompts, assistant responses, source code, commands, file contents, conversations, or raw Codex JSONL. Codex OTel is sent to a loopback-only collector inside `codex-guard`; only normalized counters leave the device.

## Architecture implemented

```text
Codex CLI ──OTLP/HTTP JSON──> codex-guard on 127.0.0.1
                                  │ filters response.completed counters
Codex JSONL ──metadata/hash chain─┤
Pinned ccusage 20.0.20 ──daily────┤ Ed25519-signed + durable queue
Service/ACL/watchdog posture ─────┤
                                  ▼
                         Caddy → FastAPI → PostgreSQL
                                      │
                              public shared dashboard
```

- **Agent:** static Go binary (`CGO_ENABLED=0`), Ed25519 device identity, monotonic sequence, chained integrity snapshots, 30-day durable JSON queue, exponential retry, local OTel filter, pinned native ccusage, automatic Codex-home recovery, append-only log verifier, service/watchdog posture checks, and privacy-filtered Codex quota reader.
- **Backend/UI:** FastAPI + SQLAlchemy + server-rendered Jinja UI. This avoids a separate Node/frontend container and keeps a 20–500 student deployment small.
- **Database:** PostgreSQL 17. Application-level append-only `security_events`; the UI exposes acknowledge but no delete.
- **Edge:** Caddy automatic HTTPS.
- **Distribution:** release builds cross-compile Linux, macOS, and Windows binaries for amd64/arm64, download and verify pinned ccusage `20.0.20`, publish artifacts and checksums, and serve both shell and PowerShell installers.

The implementation tracks the current [official Codex OTel configuration](https://developers.openai.com/codex/config-advanced) and pins the [ccusage v20.0.20 source/schema](https://github.com/ccusage/ccusage/tree/v20.0.20). The current Codex fields used are `event.name=codex.sse_event`, `event.kind=response.completed`, `input_token_count`, `cached_token_count`, `output_token_count`, `reasoning_token_count`, and `tool_token_count`.

## Self-hosted setup

Prerequisites: a Linux VPS, Docker Engine with Compose v2, a DNS A/AAAA record pointing the chosen domain to the VPS, and inbound TCP 80/443 plus UDP 443.

```bash
git clone https://github.com/thanhlamauto/codex-monitor-group.git codex-classroom-monitor
cd codex-classroom-monitor
cp .env.example .env
```

Edit `.env` for production:

```dotenv
POSTGRES_PASSWORD=<long-random-value>
CRON_SECRET=<independent-random-value>
SITE_ADDRESS=meter.example.com
PUBLIC_BASE_URL=https://meter.example.com
CLASSROOM_TIMEZONE=Asia/Ho_Chi_Minh
```

Generate secrets with `openssl rand -hex 32`. Then deploy:

```bash
docker compose up -d
docker compose ps
curl -fsS https://meter.example.com/healthz
```

The first build cross-compiles the four release artifacts and fetches the pinned ccusage binaries, so it takes longer than later starts. Open `https://meter.example.com`; the dashboard is immediately visible without an account or password.

For local-only evaluation, keep `SITE_ADDRESS=http://localhost` and `PUBLIC_BASE_URL=http://localhost`.

### Free Vercel + CockroachDB Basic deployment

```bash
make vercel-release
npx --yes vercel@latest login
npx --yes vercel@latest link
# Create a CockroachDB Basic database, then add its TLS URL as DATABASE_URL.
# Add CRON_SECRET, retention, pooling, and CLASSROOM_TIMEZONE.
./scripts/provision-vercel-env.sh
npx --yes vercel@latest --prod
```

The Vercel app automatically selects the official CockroachDB SQLAlchemy dialect when `DATABASE_URL` points to `*.cockroachlabs.cloud`, uses a bounded warm-instance connection pool, automatic HTTPS, and static CDN delivery for installer artifacts. It refuses to start on ephemeral SQLite. See [docs/vercel.md](docs/vercel.md) for provisioning and migration.

## One-line installation

Open the dashboard and run the command shown at the top:

```bash
curl -fsSL https://codex-classroom-monitor.vercel.app/install.sh | sudo sh
```

The installer asks only for the display name. It detects the computer name, OS, amd64/arm64, Codex home, and Codex executable automatically. On Windows it resolves the interactive user's profile even when PowerShell is elevated with a different administrator account. On every platform the agent rechecks `CODEX_HOME`, the configured location, and local OS profiles every five minutes; if the stored location has no session logs, it safely switches to the candidate that has real Codex JSONL data. It downloads `checksums.txt`, `codex-guard`, and pinned ccusage; fails closed on any checksum mismatch; creates a per-device Ed25519 identity; registers the person and device; safely replaces only the Codex `[otel]` configuration (saving `config.toml.codex-guard.bak`); and installs one of:

- Linux: a hardened systemd service plus a two-minute root watchdog timer.
- macOS: a root launch daemon with `RunAtLoad`/`KeepAlive` plus a two-minute watchdog launch daemon.
- Windows: a delayed-auto LocalSystem Windows Service with three recovery restarts plus a two-minute SYSTEM Task Scheduler watchdog. Credentials are encrypted at rest with machine-scoped DPAPI and the install/data trees are restricted to SYSTEM and Administrators.

On Windows, open PowerShell as Administrator and run:

```powershell
irm https://codex-classroom-monitor.vercel.app/install.ps1 | iex
```

PowerShell options are `-Name "Display name"`, `-Server https://your-domain`, `-CodexHome C:\absolute\path\.codex`, and `-Upgrade` when running a downloaded installer script directly.

For a nonstandard Codex home, use `curl -fsSL URL/install.sh | sudo sh -s -- --codex-home /absolute/path/to/.codex`. Noninteractive installation can pass `--name "Display name"`; a custom deployment can pass `--server https://your-domain`. Re-running the command is idempotent when the local registration config already exists. For an update, re-run it with `--upgrade`; only artifacts present in the server's checksummed release are accepted. There is no silent arbitrary auto-update. Agent 1.5.0 also recovers from an explicitly reset server database: only an exact signed-API response of `401 unknown device` triggers re-registration with the existing local Ed25519 public key, a new server device ID and OTel token, a cleared stale queue, and rewritten telemetry configuration. Other authentication failures never trigger re-registration.

## Verify installation

```bash
sudo codex-guard status
```

Example:

```text
Agent        running
Server       connected
Codex        codex-cli 0.133.0
Telemetry    enabled
Logs         found
ccusage      20.0.20
Queue        0 pending
```

Linux service checks:

```bash
sudo systemctl status codex-guard
sudo journalctl -u codex-guard -n 100
```

macOS service checks:

```bash
sudo launchctl print system/com.openai.codex-guard
sudo tail -100 /var/log/codex-guard.err.log
```

Windows service checks (Administrator PowerShell):

```powershell
Get-Service CodexGuard
sc.exe qfailure CodexGuard
schtasks.exe /Query /TN CodexGuardWatchdog
& "$env:ProgramFiles\CodexGuard\codex-guard.exe" status
```

## Usage and reconciliation

Source A is current Codex native OTel. Codex exports OTLP/HTTP JSON to `127.0.0.1:9464`; the agent accepts only authenticated local requests, selects only `response.completed` counters, discards everything else in memory, and sends an absolute per-day signed snapshot.

Source B runs exactly:

```bash
ccusage codex daily --json --no-cost --offline --timezone <CLASSROOM_TIMEZONE> --since ... --until ...
```

with ccusage pinned to `20.0.20` and `CODEX_HOME` explicitly set. It normalizes `inputTokens`, `cacheReadTokens`, `outputTokens`, `reasoningOutputTokens`, and `totalTokens`. The server stores UTC receipt times and assigns day boundaries in the classroom timezone. A configurable difference above both `USAGE_MISMATCH_PERCENT` and `USAGE_MISMATCH_MIN_TOKENS` creates `USAGE_SOURCE_MISMATCH`; it is an anomaly, not an accusation.

To bound database traffic, presence heartbeats still arrive every minute but only one history row is retained per ten-minute window. Integrity scans run every five minutes. Every signed route shares one short-lived `processed_events` receipt table instead of probing four event tables. The daily Vercel cron removes heartbeat and integrity rows older than 30 days and receipts older than 35 days; token usage and security events are retained.

The dashboard provides Today, Yesterday, 7d, 30d, and custom ranges, input/cache/output/reasoning breakdowns, daily graph/table, class totals, health, integrity, and an audit timeline. A sticky quota taskbar and per-device progress bars show the remaining percentage for every primary/secondary Codex limit window plus its reset time.

## Integrity detection

- **Agent/ccusage:** every heartbeat reports version and SHA-256. The server reads its own `checksums.txt` as the allowlist. A mismatch creates `AGENT_BINARY_MODIFIED` or `CCUSAGE_BINARY_MODIFIED`.
- **Config:** only the managed OTel endpoint, JSON protocol, auth, prompt-redaction value, and opaque `CODEX_HOME` fingerprint are monitored. Unrelated Codex settings do not affect the fingerprint.
- **Active JSONL:** checkpoint is `(opaque id, N, SHA256(bytes[0:N]))`. Growth must preserve the old prefix; shrink becomes `LOG_TRUNCATED`, changed prefix becomes `LOG_PREFIX_MODIFIED`.
- **Delete/move:** a missing active file is matched against newly archived files using the old size and prefix. A valid move keeps its opaque ID. Otherwise it becomes `LOG_DELETED`.
- **Archive:** closed files get a full SHA-256 and any later size/hash change becomes `ARCHIVED_LOG_MODIFIED`.
- **Checkpoint rollback/reset:** each integrity scan advances a random-state-ID canonical SHA-256 chain. The server independently recomputes every snapshot hash and compares the scan number and previous hash; deletion/re-creation, rollback, broken links, or invalid hashes become explicit audit events.
- **Monitor posture:** every integrity report covers service installation/running/autostart/restart policy, watchdog presence, protected agent/config permissions, expected key protection, and a service-definition fingerprint. Weakening or changing those controls creates an audit event.
- **Watchdog:** a second privileged scheduler checks the primary service every two minutes, submits a signed `SERVICE_STOPPED_OR_MODIFIED` event when possible, and attempts a restart. Explicit uninstall submits `AGENT_UNINSTALL_REQUESTED` before removal.
- **Availability:** 0–3 min is ONLINE, 3–10 min LATE, and over 10 min UNREACHABLE by default. `AGENT_UNREACHABLE` means monitoring is unavailable; it is deliberately distinct from tamper.

Only opaque file IDs, sizes, prefix sizes/hashes, archive flags, and modification timestamps are uploaded. Raw JSONL is never uploaded or stored.

## Simulated classroom demo

After the stack is running:

```bash
make demo
```

This idempotently creates Student A healthy, Student B with high usage, Student C with missing heartbeat, and Student D with a log-prefix alert.

## Tests and development

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r server/requirements.txt
make test
make release
```

`make test` covers public self-registration and validation, Ed25519 mutation and replay, shared idempotency receipts, sampled heartbeat history, telemetry retention, database migration, duplicate usage and absolute snapshot update, OTel privacy filtering, source mismatch, ONLINE/LATE/UNREACHABLE, FIFO offline replay, log mutation/move cases, integrity-chain rollback/reset/break detection, Windows posture alerts, watchdog events, and installer static checks. CI also runs Go tests and parses both PowerShell installers on Windows. `make release` creates local artifacts in `dist/`; `make vercel-release` creates the same release under `public/` for Vercel CDN delivery.

API endpoints are versioned: `/api/v1/register`, `/api/v1/heartbeat`, `/api/v1/usage`, `/api/v1/integrity`, and `/api/v1/events`. The server deliberately has no raw OTLP ingestion endpoint: Codex exports only to the agent's loopback collector, which filters events before signing normalized counters for the API.

## Uninstall

```bash
curl -fsSL https://meter.example.com/downloads/uninstall.sh | sudo sh
```

Windows (Administrator PowerShell):

```powershell
irm https://meter.example.com/downloads/uninstall.ps1 | iex
```

The service and binaries are removed. Credentials/checkpoints remain for recoverability; remove `/etc/codex-guard` and `/var/lib/codex-guard` manually only if intentionally purging the device. The server naturally emits `AGENT_UNREACHABLE` after the heartbeat timeout and never deletes its audit history.

## Troubleshooting

- **No Codex installed:** installation still succeeds; status says `not-detected`, logs remain `not found`, and heartbeats continue. Install Codex, then rerun the installer so its executable path is captured.
- **Server unreachable:** reports remain in `/var/lib/codex-guard/queue.json` for 30 days and retry with backoff. Check DNS, TLS, firewall, and `PUBLIC_BASE_URL`.
- **Permission denied:** run the enrollment command through `sudo`; confirm `/etc/codex-guard/config.json` is root-owned `0600` and the student's Codex config is student-readable.
- **Custom `CODEX_HOME`:** rerun with `--codex-home /absolute/path`.
- **Telemetry changed:** current implementation expects Codex OTLP/HTTP JSON and the documented `response.completed` fields. Upgrade this parser and its fixtures when the official schema changes.
- **ccusage missing/modified:** rerun the checksummed installer; never use `npx ccusage@latest` in production.
- **Caddy certificate failure:** verify public DNS and ports 80/443, then inspect `docker compose logs caddy`.

## Privacy and security assumptions

Collected: token counters, event/day timestamps, versions, heartbeat/uptime, opaque home/file IDs, binary/config/file hashes, integrity-chain state/sequence/hashes, service posture booleans/fingerprint, OS/architecture, sizes, anomaly codes, Codex plan type, rate-limit percentages, window lengths, and reset times.

Never collected: prompt text, assistant output, source code, file content, terminal commands, conversation content, raw JSONL, raw OTel records, or filesystem paths. `otel.log_user_prompt=false` is explicitly configured, and the loopback filter provides a second boundary before network transmission.

Registration is intentionally public so anybody with the installer can join the shared dashboard. Reports still use standard Ed25519, per-device keys, signed canonical JSON, unique event IDs, and strictly increasing sequences, so one device cannot submit telemetry as another device without its private key. Server receipt time is always stored separately from client time. API requests are schema/size/rate limited.

**A user with full root/admin control can disable any local monitoring software. The system is designed to detect loss of monitoring, not to guarantee prevention against a fully privileged adversary.** A privileged user can also forge local observations after extracting the device key. DPAPI, restricted ACLs, root ownership, restart policies, watchdogs, and hash chains raise the effort and make ordinary tampering visible; they do not create a hardware-backed root of trust. This is detection-oriented classroom telemetry, not a root-of-trust or cheating verdict system.

Known MVP limitations: macOS/Linux use a root-owned `0600` key file rather than Keychain/TPM; Windows DPAPI uses machine scope so the LocalSystem service and watchdog can share the key; the JSON queue is durable/atomic but not SQLite; schema creation is automatic rather than migration-managed; rate limiting is per server process; and horizontally scaling the API requires shared rate limiting plus an unreachable-event scheduler. One modest single-process VPS is the intended 20–500 student deployment.

## Contributing

The project is public and accepts pull requests from everyone. See [CONTRIBUTING.md](CONTRIBUTING.md) for the local test workflow, privacy rules, and fair-review policy. The protected `main` branch requires a pull request and passing CI for owner and collaborators alike; collaborators may merge their own pull requests once those checks pass.

See [architecture](docs/architecture.md), [privacy](docs/privacy.md), [protocol](docs/protocol.md), and [operations](docs/operations.md).
