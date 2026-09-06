# Free Vercel deployment

The dashboard and API run as one Python FastAPI Function on Vercel Hobby. Durable relational data lives in a Neon Postgres database connected through Vercel Marketplace. Release binaries are static files under `public/` and are delivered by Vercel's CDN.

## Constraints

- Vercel Hobby is intended for personal/non-commercial projects and is subject to its included usage limits.
- Hobby cron jobs can run only once per day. ONLINE/LATE/UNREACHABLE is therefore computed from `last_seen` whenever the dashboard is opened; opening the dashboard also appends any newly detected unreachable event.
- For proactive ten-minute alerts without opening the dashboard, use an external scheduler to call `GET /api/v1/cron/reconcile` with `Authorization: Bearer $CRON_SECRET`, or move to a plan that permits frequent cron execution.
- Do not deploy without a persistent Postgres integration. The application deliberately refuses to use ephemeral SQLite when `VERCEL=1`.

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
npx --yes vercel@latest integration guide neon
npx --yes vercel@latest integration add neon
```

During Neon provisioning, select its free plan. Stop rather than approving any paid plan. The Marketplace integration should connect the resource and inject `DATABASE_URL` or `POSTGRES_URL`.

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
```

`PUBLIC_BASE_URL` is optional on Vercel: the application derives it from `VERCEL_PROJECT_PRODUCTION_URL`. Set it explicitly when using a custom domain.

## Deploy and verify

```bash
npx --yes vercel@latest --prod
npx --yes vercel@latest inspect <deployment-url>
curl -fsS https://<production-domain>/healthz
```

Then open the production URL and run the one-line installer shown on the dashboard. It asks for a display name, detects the device label, self-registers through `/api/v1/register`, and appears on the public dashboard without any pre-created token.
