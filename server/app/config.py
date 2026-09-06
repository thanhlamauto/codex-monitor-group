from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from zoneinfo import ZoneInfo


def database_url() -> str:
    value = os.getenv("DATABASE_URL") or os.getenv("POSTGRES_URL")
    if not value:
        if os.getenv("VERCEL"):
            raise RuntimeError("DATABASE_URL is required on Vercel; connect a Neon Postgres integration first")
        return "sqlite:///./codex-monitor.db"
    if value.startswith("postgres://"):
        return "postgresql+psycopg://" + value.removeprefix("postgres://")
    if value.startswith("postgresql://"):
        return "postgresql+psycopg://" + value.removeprefix("postgresql://")
    return value


def public_base_url() -> str:
    explicit = os.getenv("PUBLIC_BASE_URL")
    if explicit:
        return explicit.rstrip("/")
    vercel_host = os.getenv("VERCEL_PROJECT_PRODUCTION_URL") or os.getenv("VERCEL_URL")
    return f"https://{vercel_host}" if vercel_host else "http://localhost:8000"


def artifacts_dir() -> Path:
    explicit = os.getenv("ARTIFACTS_DIR")
    if explicit:
        return Path(explicit)
    if os.getenv("VERCEL"):
        return Path(__file__).resolve().parents[2] / "public" / "downloads"
    return Path("/app/artifacts")


@dataclass(frozen=True)
class Settings:
    database_url: str = database_url()
    public_base_url: str = public_base_url()
    classroom_timezone: str = os.getenv("CLASSROOM_TIMEZONE", "UTC")
    online_seconds: int = int(os.getenv("ONLINE_SECONDS", "180"))
    late_seconds: int = int(os.getenv("LATE_SECONDS", "600"))
    mismatch_percent: float = float(os.getenv("USAGE_MISMATCH_PERCENT", "10"))
    mismatch_min_tokens: int = int(os.getenv("USAGE_MISMATCH_MIN_TOKENS", "1000"))
    artifacts_dir: Path = artifacts_dir()
    ccusage_version: str = os.getenv("CCUSAGE_VERSION", "20.0.20")
    cron_secret: str = os.getenv("CRON_SECRET", "")
    serverless: bool = bool(os.getenv("VERCEL"))

    def timezone(self) -> ZoneInfo:
        return ZoneInfo(self.classroom_timezone)


settings = Settings()
