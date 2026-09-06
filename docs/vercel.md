# Free Vercel deployment

The dashboard and API run as one Python FastAPI Function on Vercel Hobby. Durable relational data lives in CockroachDB Basic over its PostgreSQL-compatible TLS endpoint. Release binaries are static files under `public/` and are delivered by Vercel's CDN.

## Constraints

- Vercel Hobby is intended for personal/non-commercial projects and is subject to its included usage limits.
- Hobby cron jobs can run only once per day. ONLINE/LATE/UNREACHABLE is therefore computed from `last_seen` whenever the dashboard is opened; the authenticated daily cron also appends newly detected unreachable events and applies telemetry retention.
- For proactive ten-minute alerts without opening the dashboard, use an external scheduler to call `GET /api/v1/cron/reconcile` with `Authorization: Bearer $CRON_SECRET`, or move to a plan that permits frequent cron execution.
- Do not deploy without a persistent SQL database. The application deliberately refuses to use ephemeral SQLite when `VERCEL=1`.
- One warm Vercel function keeps a pool of one database connection with one overflow connection. Do not raise these defaults without checking CockroachDB connection and Vercel concurrency metrics.

## Prepare release files

Run this after every agent or installer change:

```bash
make vercel-release
```

This generates four static Go agent binaries, four pinned ccusage binaries, checksums, and installer scripts under `public/`. The total is below the Vercel Hobby source-upload limit.

## Link and provision

Use Vercel CLI 48.1.8 or newer. The commands below intentionally use the current CLI without installing it globally.

```bash
npx --yes vercel@latest login
npx --yes vercel@latest whoami
npx --yes vercel@latest link
```

Create a CockroachDB Cloud **Basic** cluster in a region close to the Vercel function, create a `codex_monitor` database, and copy its TLS `postgresql://` connection string. Do not select Standard or Advanced. The app recognizes `*.cockroachlabs.cloud` and converts the URL to the official `cockroachdb+psycopg` SQLAlchemy dialect.

Add the connection string as a sensitive Vercel variable without putting it in shell history or source control:

```bash
npx --yes vercel@latest env add DATABASE_URL production --sensitive
```

Use a separate database/credential for Preview and Development. Do not point untrusted preview deployments at the production database.

Confirm variable names without printing their values:

```bash
npx --yes vercel@latest env ls
```

Generate and add the optional reconciliation secret plus timezone to Production, Preview, and Development without printing the secret:

```bash
./scripts/provision-vercel-env.sh
```

The dashboard and device detail pages are public by design: there is no web account, password, session secret, or login page. The script configures:

```text
CRON_SECRET=<independent random secret>
CLASSROOM_TIMEZONE=Asia/Ho_Chi_Minh
DB_POOL_SIZE=1
DB_MAX_OVERFLOW=1
DB_POOL_RECYCLE_SECONDS=300
HEARTBEAT_HISTORY_SECONDS=600
HEARTBEAT_RETENTION_DAYS=30
INTEGRITY_RETENTION_DAYS=30
RECEIPT_RETENTION_DAYS=35
```

`PUBLIC_BASE_URL` is optional on Vercel: the application derives it from `VERCEL_PROJECT_PRODUCTION_URL`. Set it explicitly when using a custom domain.

## Migrate an existing database

Use secret environment variables in the local process; never commit connection strings:

```bash
export SOURCE_DATABASE_URL='<old PostgreSQL URL>'
export TARGET_DATABASE_URL='<empty CockroachDB URL>'
PYTHONPATH=server python3 scripts/migrate_database.py
```

The target must be empty. Migration creates the current schema and copies all available legacy columns atomically in foreign-key order. Stable student/device IDs, device public keys, sequence counters, usage reports, audit events, heartbeats, integrity snapshots, and quota snapshots are preserved. New sampling and receipt fields start with safe defaults.

After migration succeeds, replace Production `DATABASE_URL`, redeploy, verify `/healthz`, the public dashboard, and one signed agent heartbeat, then disconnect the old database integration. If the old provider has suspended all queries, export cannot proceed until it restores read access; do not switch an empty database into production unless you intentionally accept re-enrollment and loss of old history.

## Deploy and verify

```bash
npx --yes vercel@latest --prod
npx --yes vercel@latest inspect <deployment-url>
curl -fsS https://<production-domain>/healthz
```

Then open the production URL and run the one-line installer shown on the dashboard. It asks for a display name, detects the device label, self-registers through `/api/v1/register`, and appears on the public dashboard without any pre-created token.
