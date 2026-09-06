from __future__ import annotations

import hashlib
from datetime import date, datetime, timezone

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from .config import settings
from .models import Device, SecurityEvent, UsageReport


SEVERITY = {
    "AGENT_UNREACHABLE": "WARNING", "AGENT_BINARY_MODIFIED": "CRITICAL",
    "CCUSAGE_BINARY_MODIFIED": "CRITICAL", "TELEMETRY_CONFIG_CHANGED": "CRITICAL",
    "CODEX_HOME_CHANGED": "WARNING", "LOG_PREFIX_MODIFIED": "CRITICAL",
    "LOG_TRUNCATED": "CRITICAL", "LOG_DELETED": "CRITICAL",
    "ARCHIVED_LOG_MODIFIED": "CRITICAL", "USAGE_SOURCE_MISMATCH": "WARNING",
    "INVALID_SIGNATURE": "CRITICAL", "REPLAY_ATTEMPT": "CRITICAL",
    "AGENT_UNINSTALL_REQUESTED": "INFO",
}


def artifact_hashes() -> tuple[set[str], set[str]]:
    agents: set[str] = set()
    ccusage: set[str] = set()
    checksums = settings.artifacts_dir / "checksums.txt"
    if not checksums.exists():
        return agents, ccusage
    for line in checksums.read_text().splitlines():
        parts = line.split()
        if len(parts) != 2:
            continue
        digest, name = parts
        if name.startswith("codex-guard-"):
            agents.add(digest.lower())
        elif name.startswith("ccusage-"):
            ccusage.add(digest.lower())
    return agents, ccusage


def add_security_event(db: Session, device: Device, event_type: str, event_key: str, details: dict | None = None, severity: str | None = None):
    existing = db.scalar(select(SecurityEvent).where(SecurityEvent.device_id == device.id, SecurityEvent.event_key == event_key))
    if existing:
        return existing
    event = SecurityEvent(
        student_id=device.student_id, device_id=device.id, event_type=event_type,
        event_key=event_key, severity=severity or SEVERITY.get(event_type, "WARNING"),
        details_json=details or {},
    )
    db.add(event)
    return event


def device_state(device: Device, now: datetime | None = None) -> str:
    if not device.last_seen:
        return "UNKNOWN"
    now = now or datetime.now(timezone.utc)
    last = device.last_seen
    if last.tzinfo is None:
        last = last.replace(tzinfo=timezone.utc)
    age = (now - last).total_seconds()
    if age <= settings.online_seconds:
        return "ONLINE"
    if age <= settings.late_seconds:
        return "LATE"
    return "UNREACHABLE"


def reconcile_unreachable(db: Session, now: datetime | None = None):
    now = now or datetime.now(timezone.utc)
    for device in db.scalars(select(Device)).all():
        if device_state(device, now) == "UNREACHABLE":
            incident = device.last_seen.isoformat() if device.last_seen else "never"
            add_security_event(db, device, "AGENT_UNREACHABLE", f"unreachable:{incident}", {"last_seen": device.last_seen.isoformat() if device.last_seen else None})


def usage_totals(db: Session, student_id: str, start: date, end: date, source: str = "local") -> dict:
    row = db.execute(
        select(
            func.coalesce(func.sum(UsageReport.input_tokens), 0),
            func.coalesce(func.sum(UsageReport.cached_input_tokens), 0),
            func.coalesce(func.sum(UsageReport.output_tokens), 0),
            func.coalesce(func.sum(UsageReport.reasoning_output_tokens), 0),
            func.coalesce(func.sum(UsageReport.total_tokens), 0),
        ).where(
            UsageReport.student_id == student_id, UsageReport.source == source,
            UsageReport.period_date >= start, UsageReport.period_date <= end,
        )
    ).one()
    return dict(zip(("input", "cached", "output", "reasoning", "total"), map(int, row)))


def classroom_today() -> date:
    return datetime.now(timezone.utc).astimezone(settings.timezone()).date()


def maybe_usage_mismatch(db: Session, device: Device, period: date):
    values = {}
    for source in ("local", "otel"):
        values[source] = int(db.scalar(select(func.coalesce(func.sum(UsageReport.total_tokens), 0)).where(
            UsageReport.device_id == device.id, UsageReport.source == source, UsageReport.period_date == period
        )) or 0)
    if not values["local"] or not values["otel"]:
        return
    delta = abs(values["local"] - values["otel"])
    denominator = max(values.values())
    percent = delta * 100.0 / denominator
    if delta >= settings.mismatch_min_tokens and percent > settings.mismatch_percent:
        add_security_event(db, device, "USAGE_SOURCE_MISMATCH", f"mismatch:{period.isoformat()}", {**values, "difference_percent": round(percent, 2)})


def opaque(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
