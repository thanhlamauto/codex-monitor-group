from __future__ import annotations

import hashlib
from datetime import date, datetime, timedelta, timezone

from sqlalchemy import delete, func, select
from sqlalchemy.orm import Session

from .config import settings
from .models import Device, Heartbeat, IntegritySnapshot, ProcessedEvent, SecurityEvent, UsageReport


SEVERITY = {
    "AGENT_UNREACHABLE": "WARNING", "AGENT_BINARY_MODIFIED": "CRITICAL",
    "CCUSAGE_BINARY_MODIFIED": "CRITICAL", "TELEMETRY_CONFIG_CHANGED": "CRITICAL",
    "CODEX_HOME_CHANGED": "WARNING", "LOG_PREFIX_MODIFIED": "CRITICAL",
    "LOG_TRUNCATED": "CRITICAL", "LOG_DELETED": "CRITICAL",
    "ARCHIVED_LOG_MODIFIED": "CRITICAL", "USAGE_SOURCE_MISMATCH": "WARNING",
    "INVALID_SIGNATURE": "CRITICAL", "REPLAY_ATTEMPT": "CRITICAL",
    "AGENT_UNINSTALL_REQUESTED": "INFO",
    "SERVICE_STOPPED_OR_MODIFIED": "CRITICAL", "SERVICE_RESTART_FAILED": "CRITICAL",
    "SERVICE_CONFIGURATION_CHANGED": "CRITICAL", "SERVICE_AUTOSTART_DISABLED": "CRITICAL",
    "SERVICE_RECOVERY_DISABLED": "WARNING", "WATCHDOG_DISABLED": "WARNING",
    "AGENT_PERMISSIONS_WEAKENED": "CRITICAL", "CONFIG_PERMISSIONS_WEAKENED": "CRITICAL",
    "KEY_PROTECTION_WEAK": "WARNING", "INTEGRITY_STATE_RESET": "CRITICAL",
    "INTEGRITY_STATE_ROLLBACK": "CRITICAL", "INTEGRITY_CHAIN_BROKEN": "CRITICAL",
    "INTEGRITY_SNAPSHOT_HASH_INVALID": "CRITICAL",
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


def purge_expired_telemetry(db: Session, now: datetime | None = None) -> dict[str, int]:
    """Delete bounded raw telemetry while preserving usage and audit history."""
    now = now or datetime.now(timezone.utc)
    policies = (
        ("heartbeats", Heartbeat, Heartbeat.received_at, settings.heartbeat_retention_days),
        ("integrity_snapshots", IntegritySnapshot, IntegritySnapshot.created_at, settings.integrity_retention_days),
        ("processed_events", ProcessedEvent, ProcessedEvent.received_at, settings.receipt_retention_days),
    )
    deleted: dict[str, int] = {}
    for name, model, timestamp, days in policies:
        result = db.execute(delete(model).where(timestamp < now - timedelta(days=max(1, days))))
        deleted[name] = max(0, result.rowcount or 0)
    return deleted


def usage_totals(db: Session, student_id: str, start: date, end: date, source: str = "local") -> dict:
    row = db.execute(
        select(
            func.sum(UsageReport.input_tokens),
            func.sum(UsageReport.cached_input_tokens),
            func.sum(UsageReport.output_tokens),
            func.sum(UsageReport.reasoning_output_tokens),
            func.sum(UsageReport.total_tokens),
        ).where(
            UsageReport.student_id == student_id, UsageReport.source == source,
            UsageReport.period_date >= start, UsageReport.period_date <= end,
        )
    ).one()
    # CockroachDB returns DECIMAL for SUM(INT8), while a bare SQL COALESCE(...,
    # 0) binds the fallback as INT4. Normalize nullable aggregates in Python so
    # the same query works on CockroachDB, PostgreSQL, and SQLite.
    return dict(zip(("input", "cached", "output", "reasoning", "total"), (int(value or 0) for value in row)))


def classroom_today() -> date:
    return datetime.now(timezone.utc).astimezone(settings.timezone()).date()


def maybe_usage_mismatch(db: Session, device: Device, period: date, now: datetime | None = None):
    """Compare sources only once OTel has covered at least one complete day.

    Enrollment and OTel configuration happen together in the installers. The
    enrollment day is therefore a partial OTel day while the local collector can
    immediately backfill the whole day (and older JSONL history). Comparing
    those values would create a permanent false-positive alert. Waiting for a
    completed calendar day also avoids comparing two in-flight daily snapshots.
    """
    now = now or datetime.now(timezone.utc)
    enrolled_at = device.enrolled_at
    last_otlp_at = device.last_otlp_at
    if not enrolled_at or not last_otlp_at:
        return
    if enrolled_at.tzinfo is None:
        enrolled_at = enrolled_at.replace(tzinfo=timezone.utc)
    if last_otlp_at.tzinfo is None:
        last_otlp_at = last_otlp_at.replace(tzinfo=timezone.utc)
    if last_otlp_at - enrolled_at < timedelta(days=1):
        return
    classroom_timezone = settings.timezone()
    enrollment_day = enrolled_at.astimezone(classroom_timezone).date()
    today = now.astimezone(classroom_timezone).date()
    if period <= enrollment_day or period >= today:
        return

    values = {}
    for source in ("local", "otel"):
        total = db.scalar(select(func.sum(UsageReport.total_tokens)).where(
            UsageReport.device_id == device.id, UsageReport.source == source, UsageReport.period_date == period
        ))
        values[source] = int(total or 0)
    if not values["local"] or not values["otel"]:
        return
    delta = abs(values["local"] - values["otel"])
    denominator = max(values.values())
    percent = delta * 100.0 / denominator
    if delta >= settings.mismatch_min_tokens and percent > settings.mismatch_percent:
        add_security_event(db, device, "USAGE_SOURCE_MISMATCH", f"mismatch:{period.isoformat()}", {**values, "difference_percent": round(percent, 2)})


def opaque(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
