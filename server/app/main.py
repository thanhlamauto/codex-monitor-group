from __future__ import annotations

import base64
import asyncio
import hashlib
import json
import secrets
import time
from collections import defaultdict, deque
from contextlib import asynccontextmanager
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from fastapi import Depends, FastAPI, HTTPException, Request
from pydantic import ValidationError
from fastapi.responses import FileResponse, HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from sqlalchemy import func, select
from sqlalchemy.orm import Session
from .config import settings
from .db import SessionLocal, ensure_schema, get_db
from .models import Device, Heartbeat, IntegritySnapshot, QuotaSnapshot, SecurityEvent, Student, UsageReport, utcnow
from .schemas import EnrollRequest, IntegrityPayload, QuotaPayload, SignedEnvelope, UsagePayload
from .security import canonical_payload, hash_secret, parse_client_timestamp, signature_message, verify_signature
from .services import add_security_event, artifact_hashes, classroom_today, device_state, maybe_usage_mismatch, reconcile_unreachable, usage_totals


BASE_DIR = Path(__file__).parent
templates = Jinja2Templates(directory=str(BASE_DIR / "templates"))
rate_windows: dict[str, deque[float]] = defaultdict(deque)


def format_tokens(value) -> str:
    number = int(value or 0)
    if number >= 1_000_000_000:
        return f"{number / 1_000_000_000:.1f}B"
    if number >= 1_000_000:
        return f"{number / 1_000_000:.1f}M"
    if number >= 1_000:
        return f"{number / 1_000:.1f}K"
    return str(number)


def format_ago(value) -> str:
    if not value:
        return "Never"
    if value.tzinfo is None:
        value = value.replace(tzinfo=timezone.utc)
    seconds = max(0, int((utcnow() - value).total_seconds()))
    if seconds < 60:
        return f"{seconds} sec"
    if seconds < 3600:
        return f"{seconds // 60} min"
    if seconds < 86400:
        return f"{seconds // 3600} hr"
    return f"{seconds // 86400} d"


def quota_window_label(minutes: int) -> str:
    if minutes % 10_080 == 0:
        return f"{minutes // 10_080} tuần"
    if minutes % 1_440 == 0:
        return f"{minutes // 1_440} ngày"
    if minutes % 60 == 0:
        return f"{minutes // 60} giờ"
    return f"{minutes} phút"


def quota_windows(snapshot: QuotaSnapshot | None) -> list[dict]:
    if not snapshot:
        return []
    windows = []
    for kind in ("primary", "secondary"):
        used = getattr(snapshot, f"{kind}_used_percent")
        minutes = getattr(snapshot, f"{kind}_window_minutes")
        resets_at = getattr(snapshot, f"{kind}_resets_at")
        if used is None or minutes is None:
            continue
        remaining = max(0.0, min(100.0, 100.0 - float(used)))
        tone = "good" if remaining > 50 else ("warn" if remaining > 20 else "low")
        windows.append({"kind": kind, "label": quota_window_label(minutes), "remaining": remaining, "used": float(used), "resets_at": resets_at, "tone": tone})
    return windows


templates.env.filters["tokens"] = format_tokens
templates.env.filters["ago"] = format_ago
templates.env.filters["dayminus"] = lambda value, days: value - timedelta(days=days)


@asynccontextmanager
async def lifespan(app: FastAPI):
    ensure_schema()
    if settings.serverless:
        yield
        return

    async def monitor_unreachable():
        while True:
            try:
                with SessionLocal() as db:
                    reconcile_unreachable(db)
                    db.commit()
            finally:
                await asyncio.sleep(60)
    task = asyncio.create_task(monitor_unreachable())
    try:
        yield
    finally:
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass


app = FastAPI(title="Codex Classroom Monitor", version="1.3.1", lifespan=lifespan)
app.mount("/static", StaticFiles(directory=str(BASE_DIR / "static")), name="static")


@app.middleware("http")
async def request_limits(request: Request, call_next):
    if request.method in {"POST", "PUT", "PATCH"}:
        try:
            length = int(request.headers.get("content-length", "0") or 0)
        except ValueError:
            return JSONResponse({"detail": "invalid content length"}, status_code=400)
        if length > 1_048_576:
            return JSONResponse({"detail": "request too large"}, status_code=413)
        if "content-length" not in request.headers:
            body = await request.body()
            if len(body) > 1_048_576:
                return JSONResponse({"detail": "request too large"}, status_code=413)
    if request.url.path.startswith("/api/") or request.url.path.startswith("/v1/"):
        key = request.client.host if request.client else "unknown"
        now = time.monotonic()
        q = rate_windows[key]
        while q and q[0] < now - 60:
            q.popleft()
        if len(q) >= 300:
            return JSONResponse({"detail": "rate limit exceeded"}, status_code=429)
        q.append(now)
    return await call_next(request)


@app.get("/healthz")
def healthz():
    return {"status": "ok", "version": app.version}


@app.get("/api/v1/cron/reconcile")
def cron_reconcile(request: Request, db: Session = Depends(get_db)):
    supplied = request.headers.get("authorization", "")
    expected = f"Bearer {settings.cron_secret}"
    if not settings.cron_secret or not secrets.compare_digest(supplied, expected):
        raise HTTPException(401, "invalid cron credential")
    reconcile_unreachable(db)
    db.commit()
    return {"status": "ok", "reconciled_at": utcnow().isoformat()}


@app.get("/", response_class=HTMLResponse)
def dashboard(request: Request, db: Session = Depends(get_db)):
    reconcile_unreachable(db)
    db.commit()
    today = classroom_today()
    rows = []
    for student in db.scalars(select(Student).order_by(Student.name)).all():
        device = db.scalar(select(Device).where(Device.student_id == student.id).order_by(Device.enrolled_at.desc()))
        open_alerts = db.scalar(select(func.count(SecurityEvent.id)).where(SecurityEvent.student_id == student.id, SecurityEvent.acknowledged_at.is_(None), SecurityEvent.event_type != "AGENT_UNREACHABLE")) or 0
        state = device_state(device) if device else "MISSING"
        latest_integrity = db.scalar(select(IntegritySnapshot).where(IntegritySnapshot.device_id == device.id).order_by(IntegritySnapshot.created_at.desc())) if device else None
        quota = db.get(QuotaSnapshot, device.id) if device else None
        integrity = "TAMPER" if open_alerts else ("UNKNOWN" if state in {"MISSING", "UNREACHABLE"} or latest_integrity is None else latest_integrity.status)
        rows.append({
            "student": student, "device": device, "state": state,
            "today": usage_totals(db, student.id, today, today)["total"],
            "week": usage_totals(db, student.id, today - timedelta(days=6), today)["total"],
            "alerts": open_alerts, "integrity": integrity,
            "telemetry": "OK" if device and device.last_otlp_at and state != "UNREACHABLE" else "MISSING",
            "logs": "OK" if latest_integrity and latest_integrity.files_checked > 0 and state != "UNREACHABLE" else "MISSING",
            "quota": quota, "quota_windows": quota_windows(quota),
        })
    class_today = sum(row["today"] for row in rows)
    class_week = sum(row["week"] for row in rows)
    top_students = sorted(rows, key=lambda row: row["week"], reverse=True)[:3]
    quota_values = [window["remaining"] for row in rows for window in row["quota_windows"]]
    quota_min = min(quota_values) if quota_values else None
    quota_devices = sum(bool(row["quota_windows"]) for row in rows)
    install_command = f"curl -fsSL {settings.public_base_url}/install.sh | sudo sh"
    windows_install_command = f"irm {settings.public_base_url}/install.ps1 | iex"
    unix_update_command = f"curl -fsSL {settings.public_base_url}/install.sh | sudo sh -s -- --upgrade"
    wsl_uninstall_command = f"curl -fsSL {settings.public_base_url}/downloads/uninstall.sh | sudo sh"
    return templates.TemplateResponse(request, "dashboard.html", {"rows": rows, "top_students": top_students, "class_today": class_today, "class_week": class_week, "timezone": settings.classroom_timezone, "install_command": install_command, "windows_install_command": windows_install_command, "unix_update_command": unix_update_command, "windows_update_command": windows_install_command, "wsl_uninstall_command": wsl_uninstall_command, "quota_min": quota_min, "quota_devices": quota_devices})


@app.get("/students/{student_id}", response_class=HTMLResponse)
def student_detail(student_id: str, request: Request, db: Session = Depends(get_db), start: date | None = None, end: date | None = None):
    student = db.get(Student, student_id)
    if not student:
        raise HTTPException(404)
    today = classroom_today()
    start = start or today - timedelta(days=29)
    end = end or today
    device = db.scalar(select(Device).where(Device.student_id == student.id).order_by(Device.enrolled_at.desc()))
    state = device_state(device) if device else "MISSING"
    daily = []
    for day in db.execute(select(UsageReport.period_date, func.sum(UsageReport.input_tokens), func.sum(UsageReport.cached_input_tokens), func.sum(UsageReport.output_tokens), func.sum(UsageReport.reasoning_output_tokens), func.sum(UsageReport.total_tokens)).where(UsageReport.student_id == student.id, UsageReport.source == "local", UsageReport.period_date >= start, UsageReport.period_date <= end).group_by(UsageReport.period_date).order_by(UsageReport.period_date)).all():
        daily.append({"date": day[0], "input": int(day[1]), "cached": int(day[2]), "output": int(day[3]), "reasoning": int(day[4]), "total": int(day[5])})
    events = db.scalars(select(SecurityEvent).where(SecurityEvent.student_id == student.id).order_by(SecurityEvent.created_at.desc()).limit(100)).all()
    max_daily = max((row["total"] for row in daily), default=1)
    alert_types = {event.event_type for event in events if event.acknowledged_at is None}
    latest_integrity = db.scalar(select(IntegritySnapshot).where(IntegritySnapshot.device_id == device.id).order_by(IntegritySnapshot.created_at.desc())) if device else None
    quota = db.get(QuotaSnapshot, device.id) if device else None
    default_integrity = "OK" if latest_integrity and state != "UNREACHABLE" else "UNKNOWN"
    session_logs_status = "TAMPER" if alert_types.intersection({"LOG_PREFIX_MODIFIED", "LOG_TRUNCATED", "LOG_DELETED", "ARCHIVED_LOG_MODIFIED"}) else ("OK" if latest_integrity and latest_integrity.files_checked > 0 and state != "UNREACHABLE" else "MISSING")
    integrity_status = {
        "Agent binary": "TAMPER" if "AGENT_BINARY_MODIFIED" in alert_types else default_integrity,
        "ccusage": "TAMPER" if "CCUSAGE_BINARY_MODIFIED" in alert_types else default_integrity,
        "Codex config": "TAMPER" if "TELEMETRY_CONFIG_CHANGED" in alert_types or "CODEX_HOME_CHANGED" in alert_types else default_integrity,
        "Session logs": session_logs_status,
        "Monitor service": "TAMPER" if alert_types.intersection({"SERVICE_STOPPED_OR_MODIFIED", "SERVICE_RESTART_FAILED", "SERVICE_CONFIGURATION_CHANGED", "SERVICE_AUTOSTART_DISABLED", "SERVICE_RECOVERY_DISABLED", "WATCHDOG_DISABLED"}) else default_integrity,
        "Local protection": "TAMPER" if alert_types.intersection({"AGENT_PERMISSIONS_WEAKENED", "CONFIG_PERMISSIONS_WEAKENED", "KEY_PROTECTION_WEAK", "INTEGRITY_STATE_RESET", "INTEGRITY_STATE_ROLLBACK", "INTEGRITY_CHAIN_BROKEN", "INTEGRITY_SNAPSHOT_HASH_INVALID"}) else default_integrity,
    }
    return templates.TemplateResponse(request, "student.html", {"student": student, "device": device, "state": state, "totals": usage_totals(db, student.id, start, end), "daily": daily, "max_daily": max_daily, "integrity_status": integrity_status, "events": events, "start": start, "end": end, "today": today, "current_version": app.version, "quota": quota, "quota_windows": quota_windows(quota)})


@app.post("/api/v1/register")
@app.post("/api/v1/enroll", include_in_schema=False)
def enroll(body: EnrollRequest, db: Session = Depends(get_db)):
    try:
        if len(base64.b64decode(body.public_key, validate=True)) != 32:
            raise ValueError
    except Exception as exc:
        raise HTTPException(422, "public_key must be a base64 Ed25519 public key") from exc
    otlp_token = secrets.token_urlsafe(32)
    student = Student(name=body.name, device_label=body.device_label)
    db.add(student)
    db.flush()
    device = Device(student_id=student.id, label=body.device_label, public_key=body.public_key, otlp_token_hash=hash_secret(otlp_token))
    db.add(device)
    db.commit()
    return {"device_id": device.id, "student_name": student.name, "device_label": device.label, "otlp_token": otlp_token, "heartbeat_seconds": 60, "classroom_timezone": settings.classroom_timezone, "server_time": utcnow().isoformat()}


def verified(envelope: SignedEnvelope, db: Session) -> tuple[Device, bool]:
    device = db.get(Device, envelope.device_id)
    if not device:
        raise HTTPException(401, "unknown device")
    message = signature_message(envelope.device_id, envelope.sequence, envelope.timestamp, envelope.event_id, envelope.payload)
    if not verify_signature(device.public_key, envelope.signature, message):
        add_security_event(db, device, "INVALID_SIGNATURE", f"invalid-signature:{envelope.event_id}", {"event_id": envelope.event_id})
        db.commit()
        raise HTTPException(401, "invalid signature")
    duplicate = any([
        db.scalar(select(Heartbeat.id).where(Heartbeat.device_id == device.id, Heartbeat.event_id == envelope.event_id)),
        db.scalar(select(UsageReport.id).where(UsageReport.device_id == device.id, UsageReport.event_id.like(f"{envelope.event_id}%"))),
        db.scalar(select(IntegritySnapshot.id).where(IntegritySnapshot.device_id == device.id, IntegritySnapshot.event_id == envelope.event_id)),
        db.scalar(select(SecurityEvent.id).where(SecurityEvent.device_id == device.id, SecurityEvent.event_key == f"agent-event:{envelope.event_id}")),
    ])
    if duplicate:
        return device, True
    if envelope.sequence <= device.last_sequence:
        add_security_event(db, device, "REPLAY_ATTEMPT", f"replay:{envelope.event_id}", {"sequence": envelope.sequence, "last_sequence": device.last_sequence})
        db.commit()
        raise HTTPException(409, "replayed sequence")
    device.last_sequence = envelope.sequence
    return device, False


@app.post("/api/v1/heartbeat")
def heartbeat(envelope: SignedEnvelope, db: Session = Depends(get_db)):
    device, duplicate = verified(envelope, db)
    if duplicate:
        return {"accepted": True, "duplicate": True}
    p = envelope.payload
    required = ("agent_version", "agent_sha256", "ccusage_version", "ccusage_sha256", "codex_home", "uptime_seconds")
    if any(k not in p for k in required):
        raise HTTPException(422, "missing heartbeat field")
    device.last_seen = utcnow()
    device.agent_version = str(p["agent_version"])[:64]
    device.agent_sha256 = str(p["agent_sha256"])[:64].lower()
    device.ccusage_version = str(p["ccusage_version"])[:64]
    device.ccusage_sha256 = str(p["ccusage_sha256"])[:64].lower()
    device.codex_version = str(p.get("codex_version", ""))[:64]
    codex_home = str(p["codex_home"])[:64]
    if device.codex_home_id and device.codex_home_id != codex_home and p.get("codex_home_auto_discovered") is not True:
        add_security_event(db, device, "CODEX_HOME_CHANGED", f"codex-home:{envelope.event_id}", {"previous": device.codex_home_id, "current": codex_home})
    device.codex_home_id = codex_home
    config_fp = str(p.get("config_fingerprint", ""))[:64]
    if device.config_fingerprint and config_fp and device.config_fingerprint != config_fp:
        add_security_event(db, device, "TELEMETRY_CONFIG_CHANGED", f"config:{envelope.event_id}", {})
    if config_fp:
        device.config_fingerprint = config_fp
    if p.get("telemetry_config_ok") is False:
        add_security_event(db, device, "TELEMETRY_CONFIG_CHANGED", f"config-invalid:{envelope.event_id}", {"reason": "required OTel routing or prompt-redaction setting is missing"})
    if p.get("quota") is not None:
        try:
            quota = QuotaPayload.model_validate(p["quota"])
            observed_at = parse_client_timestamp(quota.observed_at)
        except (ValidationError, ValueError) as exc:
            raise HTTPException(422, "invalid quota payload") from exc
        snapshot = db.get(QuotaSnapshot, device.id) or QuotaSnapshot(device_id=device.id)
        snapshot.plan_type = quota.plan_type
        snapshot.primary_used_percent = quota.primary.used_percent if quota.primary else None
        snapshot.primary_window_minutes = quota.primary.window_minutes if quota.primary else None
        snapshot.primary_resets_at = datetime.fromtimestamp(quota.primary.resets_at, tz=timezone.utc) if quota.primary and quota.primary.resets_at else None
        snapshot.secondary_used_percent = quota.secondary.used_percent if quota.secondary else None
        snapshot.secondary_window_minutes = quota.secondary.window_minutes if quota.secondary else None
        snapshot.secondary_resets_at = datetime.fromtimestamp(quota.secondary.resets_at, tz=timezone.utc) if quota.secondary and quota.secondary.resets_at else None
        snapshot.observed_at = observed_at
        db.add(snapshot)
    agent_allowed, cc_allowed = artifact_hashes()
    if agent_allowed and device.agent_sha256 not in agent_allowed:
        add_security_event(db, device, "AGENT_BINARY_MODIFIED", f"agent-hash:{device.agent_sha256}", {"version": device.agent_version})
    if cc_allowed and device.ccusage_sha256 not in cc_allowed:
        add_security_event(db, device, "CCUSAGE_BINARY_MODIFIED", f"ccusage-hash:{device.ccusage_sha256}", {"version": device.ccusage_version})
    if device.ccusage_version != settings.ccusage_version:
        add_security_event(db, device, "CCUSAGE_BINARY_MODIFIED", f"ccusage-version:{device.ccusage_version}", {"expected_version": settings.ccusage_version})
    db.add(Heartbeat(device_id=device.id, event_id=envelope.event_id, sequence=envelope.sequence, client_timestamp=parse_client_timestamp(envelope.timestamp), uptime_seconds=max(0, int(p["uptime_seconds"]))))
    db.commit()
    return {"accepted": True, "duplicate": False, "server_time": utcnow().isoformat()}


@app.post("/api/v1/usage")
def usage(envelope: SignedEnvelope, db: Session = Depends(get_db)):
    device, duplicate = verified(envelope, db)
    if duplicate:
        return {"accepted": True, "duplicate": True}
    try:
        payload = UsagePayload.model_validate(envelope.payload)
    except ValidationError as exc:
        raise HTTPException(422, json.loads(exc.json())) from exc
    occurred = parse_client_timestamp(envelope.timestamp)
    for index, day in enumerate(payload.days):
        period = date.fromisoformat(day.date)
        report = db.scalar(select(UsageReport).where(UsageReport.device_id == device.id, UsageReport.source == payload.source, UsageReport.period_date == period))
        if report is None:
            report = UsageReport(device_id=device.id, student_id=device.student_id, event_id=f"{envelope.event_id}:{index}", source=payload.source, period_date=period, occurred_at=occurred)
            db.add(report)
        else:
            report.event_id = f"{envelope.event_id}:{index}"
            report.occurred_at = occurred
            report.received_at = utcnow()
        report.input_tokens = day.input_tokens
        report.cached_input_tokens = day.cached_input_tokens
        report.output_tokens = day.output_tokens
        report.reasoning_output_tokens = day.reasoning_output_tokens
        report.total_tokens = day.total_tokens
    if payload.days:
        if payload.source == "local":
            device.last_local_usage_at = utcnow()
        else:
            device.last_otlp_at = utcnow()
    db.commit()
    for day in payload.days:
        maybe_usage_mismatch(db, device, date.fromisoformat(day.date))
    db.commit()
    return {"accepted": True, "duplicate": False}


@app.post("/api/v1/integrity")
def integrity(envelope: SignedEnvelope, db: Session = Depends(get_db)):
    device, duplicate = verified(envelope, db)
    if duplicate:
        return {"accepted": True, "duplicate": True}
    try:
        payload = IntegrityPayload.model_validate(envelope.payload)
    except ValidationError as exc:
        raise HTTPException(422, json.loads(exc.json())) from exc
    previous = db.scalar(select(IntegritySnapshot).where(IntegritySnapshot.device_id == device.id).order_by(IntegritySnapshot.created_at.desc()))
    previous_metadata = previous.metadata_json if previous and isinstance(previous.metadata_json, dict) else {}
    previous_state_id = previous_metadata.get("state_id")
    previous_sequence = previous_metadata.get("scan_sequence")
    previous_hash = previous_metadata.get("snapshot_hash")
    validated_snapshot_hash = payload.snapshot_hash
    if payload.state_id and payload.scan_sequence and payload.snapshot_hash:
        chain_document = {
            "state_id": payload.state_id,
            "sequence": payload.scan_sequence,
            "previous_hash": payload.previous_snapshot_hash or "",
            "files": [row.model_dump(exclude_none=True) for row in payload.file_metadata],
            "findings": [row.model_dump(exclude_none=True) for row in payload.findings],
        }
        calculated_snapshot_hash = hashlib.sha256(canonical_payload(chain_document)).hexdigest()
        validated_snapshot_hash = calculated_snapshot_hash
        if calculated_snapshot_hash != payload.snapshot_hash:
            add_security_event(db, device, "INTEGRITY_SNAPSHOT_HASH_INVALID", f"state-hash:{envelope.event_id}", {"reported_snapshot_hash": payload.snapshot_hash, "calculated_snapshot_hash": calculated_snapshot_hash})
        if previous_state_id and previous_state_id != payload.state_id:
            add_security_event(db, device, "INTEGRITY_STATE_RESET", f"state-reset:{payload.state_id}", {"previous_state_id": previous_state_id, "current_state_id": payload.state_id})
        elif previous_sequence is not None and payload.scan_sequence <= int(previous_sequence):
            add_security_event(db, device, "INTEGRITY_STATE_ROLLBACK", f"state-rollback:{payload.state_id}:{payload.scan_sequence}", {"previous_sequence": previous_sequence, "current_sequence": payload.scan_sequence})
        elif previous_hash and payload.previous_snapshot_hash != previous_hash:
            add_security_event(db, device, "INTEGRITY_CHAIN_BROKEN", f"state-chain:{payload.snapshot_hash}", {"expected_previous_hash": previous_hash, "reported_previous_hash": payload.previous_snapshot_hash})
    posture_dict = payload.monitor_posture.model_dump() if payload.monitor_posture else None
    previous_posture = previous_metadata.get("monitor_posture") if isinstance(previous_metadata.get("monitor_posture"), dict) else {}
    if posture_dict:
        fingerprint = posture_dict["service_fingerprint"]
        previous_fingerprint = previous_posture.get("service_fingerprint")
        if previous_fingerprint and previous_fingerprint != fingerprint:
            add_security_event(db, device, "SERVICE_CONFIGURATION_CHANGED", f"service-config:{fingerprint}", {"previous_fingerprint": previous_fingerprint, "current_fingerprint": fingerprint})
        posture_checks = {
            "service_installed": "SERVICE_STOPPED_OR_MODIFIED",
            "service_running": "SERVICE_STOPPED_OR_MODIFIED",
            "auto_start_ok": "SERVICE_AUTOSTART_DISABLED",
            "restart_policy_ok": "SERVICE_RECOVERY_DISABLED",
            "agent_permissions_ok": "AGENT_PERMISSIONS_WEAKENED",
            "config_permissions_ok": "CONFIG_PERMISSIONS_WEAKENED",
        }
        if posture_dict["watchdog_expected"]:
            posture_checks["watchdog_ok"] = "WATCHDOG_DISABLED"
        for field, event_type in posture_checks.items():
            if not posture_dict[field]:
                add_security_event(db, device, event_type, f"posture:{event_type}:{field}:{fingerprint}", {"failed_check": field, "service_manager": posture_dict["service_manager"]})
        expected_key_protection = "windows-dpapi-machine" if posture_dict["os"] == "windows" else "root-owned-file"
        if posture_dict["key_protection"] != expected_key_protection:
            add_security_event(db, device, "KEY_PROTECTION_WEAK", f"key-protection:{posture_dict['key_protection']}", {"expected": expected_key_protection, "reported": posture_dict["key_protection"]})
    metadata = {
        "files": [row.model_dump() for row in payload.file_metadata],
        "state_id": payload.state_id,
        "scan_sequence": payload.scan_sequence,
        "previous_snapshot_hash": payload.previous_snapshot_hash,
        "snapshot_hash": validated_snapshot_hash,
        "reported_snapshot_hash": payload.snapshot_hash,
        "monitor_posture": posture_dict,
    }
    db.add(IntegritySnapshot(device_id=device.id, event_id=envelope.event_id, files_checked=payload.files_checked, status=payload.status, metadata_json=metadata))
    for index, finding in enumerate(payload.findings):
        details = {k: v for k, v in finding.model_dump().items() if k not in {"event_type", "severity"} and v is not None}
        add_security_event(db, device, finding.event_type, f"integrity:{envelope.event_id}:{index}", details, finding.severity)
    db.commit()
    return {"accepted": True, "duplicate": False}


@app.post("/api/v1/events")
def events(envelope: SignedEnvelope, db: Session = Depends(get_db)):
    device, duplicate = verified(envelope, db)
    if duplicate:
        return {"accepted": True, "duplicate": True}
    kind = str(envelope.payload.get("event_type", ""))
    if kind not in {"AGENT_UNINSTALL_REQUESTED", "SERVICE_STOPPED_OR_MODIFIED", "SERVICE_RESTART_FAILED"}:
        raise HTTPException(422, "unsupported event")
    details = envelope.payload.get("details", {})
    if not isinstance(details, dict) or len(json.dumps(details)) > 4096:
        raise HTTPException(422, "invalid event details")
    add_security_event(db, device, kind, f"agent-event:{envelope.event_id}", details)
    device.last_sequence = envelope.sequence
    db.commit()
    return {"accepted": True, "duplicate": False}


@app.get("/install.sh")
def install_script():
    path = settings.artifacts_dir.parent / "install.sh" if settings.serverless else settings.artifacts_dir / "install.sh"
    if not path.exists():
        raise HTTPException(503, "release artifacts not built")
    return FileResponse(path, media_type="text/x-shellscript")


@app.get("/install.ps1")
def install_powershell():
    path = settings.artifacts_dir.parent / "install.ps1" if settings.serverless else settings.artifacts_dir / "install.ps1"
    if not path.exists():
        raise HTTPException(503, "release artifacts not built")
    return FileResponse(path, media_type="text/plain")


@app.get("/downloads/{filename}")
def download(filename: str):
    if "/" in filename or ".." in filename:
        raise HTTPException(404)
    path = settings.artifacts_dir / filename
    if not path.is_file():
        raise HTTPException(404)
    return FileResponse(path, media_type="application/octet-stream")
