import hashlib
import json

from app.models import Device, SecurityEvent, UsageReport
from app.services import classroom_today

from .helpers import enroll, signed


def usage_payload(total=2000):
    return {"source": "local", "days": [{"date": classroom_today().isoformat(), "input_tokens": total - 200, "cached_input_tokens": 100, "output_tokens": 200, "reasoning_output_tokens": 50, "total_tokens": total}]}


def test_normal_usage_import_duplicate_and_absolute_update(client, db):
    _, key, result = enroll(client, db)
    first = signed(key, result["device_id"], 1, usage_payload(2000))
    assert client.post("/api/v1/usage", json=first).status_code == 200
    assert client.post("/api/v1/usage", json=first).json()["duplicate"] is True
    second = signed(key, result["device_id"], 2, usage_payload(3000))
    assert client.post("/api/v1/usage", json=second).status_code == 200
    rows = db.query(UsageReport).filter_by(device_id=result["device_id"], source="local").all()
    assert len(rows) == 1
    assert rows[0].total_tokens == 3000


def test_empty_local_report_does_not_claim_logs_were_reconciled(client, db):
    student, key, result = enroll(client, db)
    empty = {"source": "local", "days": []}
    assert client.post("/api/v1/usage", json=signed(key, result["device_id"], 1, empty)).status_code == 200
    db.expire_all()
    device = db.get(Device, result["device_id"])
    assert device.last_local_usage_at is None
    assert device.last_otlp_at is None
    page = client.get(f"/students/{student.id}")
    assert "Session logs" in page.text
    assert "MISSING" in page.text


def test_signed_local_collector_snapshot_reconciles_with_ccusage(client, db):
    _, key, result = enroll(client, db)
    assert client.post("/api/v1/usage", json=signed(key, result["device_id"], 1, usage_payload(5000))).status_code == 200
    otel = usage_payload(600)
    otel["source"] = "otel"
    response = client.post("/api/v1/usage", json=signed(key, result["device_id"], 2, otel))
    assert response.status_code == 200
    report = db.query(UsageReport).filter_by(source="otel").one()
    assert report.total_tokens == 600
    assert db.query(SecurityEvent).filter_by(event_type="USAGE_SOURCE_MISMATCH").count() == 1


def test_integrity_accepts_allowlisted_metadata_and_rejects_paths(client, db):
    _, key, result = enroll(client, db)
    good = {"files_checked": 1, "status": "TAMPER", "findings": [{"event_type": "LOG_TRUNCATED", "severity": "CRITICAL", "opaque_file_id": "a" * 32, "old_size": 100, "new_size": 10}], "file_metadata": [{"opaque_file_id": "a" * 32, "size": 10, "prefix_size": 10, "prefix_sha256": "b" * 64, "archived": False, "mtime_unix": 1}]}
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, good)).status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="LOG_TRUNCATED").count() == 1
    bad = {"files_checked": 1, "status": "OK", "findings": [], "file_metadata": [{"opaque_file_id": "x", "size": 1, "path": "/private/student/session.jsonl"}]}
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 2, bad)).status_code == 422


def test_zero_checked_files_are_rendered_as_missing_not_ok(client, db):
    student, key, result = enroll(client, db)
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, integrity_payload())).status_code == 200
    page = client.get(f"/students/{student.id}")
    assert "Session logs" in page.text
    assert "MISSING" in page.text


def test_signed_auto_discovery_does_not_create_false_home_tamper(client, db):
    from .helpers import heartbeat_payload

    _, key, result = enroll(client, db)
    first = heartbeat_payload(codex_home="a" * 64)
    second = heartbeat_payload(codex_home="b" * 64, codex_home_auto_discovered=True)
    assert client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, first)).status_code == 200
    assert client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 2, second)).status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="CODEX_HOME_CHANGED").count() == 0


def test_student_detail_dashboard_renders(client, db):
    student, key, result = enroll(client, db)
    assert client.post("/api/v1/usage", json=signed(key, result["device_id"], 1, usage_payload(12345))).status_code == 200
    page = client.get(f"/students/{student.id}")
    assert page.status_code == 200
    assert "Daily token usage" in page.text
    assert "12.3K" in page.text


def integrity_payload(state_id="a" * 32, sequence=1, previous_hash=None, snapshot_hash=None, **posture_overrides):
    posture = {
        "os": "windows", "service_manager": "windows-scm", "service_installed": True,
        "service_running": True, "auto_start_ok": True, "restart_policy_ok": True,
        "watchdog_expected": True, "watchdog_ok": True, "agent_permissions_ok": True,
        "config_permissions_ok": True, "key_protection": "windows-dpapi-machine",
        "service_fingerprint": "c" * 64,
    }
    posture.update(posture_overrides)
    payload = {
        "files_checked": 0, "status": "OK", "findings": [], "file_metadata": [],
        "state_id": state_id, "scan_sequence": sequence,
        "previous_snapshot_hash": previous_hash,
        "monitor_posture": posture,
    }
    chain = {"state_id": state_id, "sequence": sequence, "previous_hash": previous_hash or "", "files": [], "findings": []}
    payload["snapshot_hash"] = snapshot_hash or hashlib.sha256(json.dumps(chain, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    return payload


def test_integrity_chain_break_and_state_reset_are_audited(client, db):
    _, key, result = enroll(client, db)
    first = integrity_payload()
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, first)).status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="INTEGRITY_SNAPSHOT_HASH_INVALID").count() == 0
    broken = integrity_payload(sequence=2, previous_hash="d" * 64)
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 2, broken)).status_code == 200
    reset = integrity_payload(state_id="f" * 32, sequence=1)
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 3, reset)).status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="INTEGRITY_CHAIN_BROKEN").count() == 1
    assert db.query(SecurityEvent).filter_by(event_type="INTEGRITY_STATE_RESET").count() == 1


def test_invalid_integrity_snapshot_hash_is_recomputed_and_audited(client, db):
    _, key, result = enroll(client, db)
    payload = integrity_payload(snapshot_hash="0" * 64)
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, payload)).status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="INTEGRITY_SNAPSHOT_HASH_INVALID").count() == 1


def test_partial_integrity_chain_is_rejected(client, db):
    _, key, result = enroll(client, db)
    payload = {"files_checked": 0, "status": "OK", "findings": [], "file_metadata": [], "state_id": "a" * 32}
    assert client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, payload)).status_code == 422


def test_weakened_windows_service_and_key_protection_are_audited(client, db):
    _, key, result = enroll(client, db)
    payload = integrity_payload(auto_start_ok=False, watchdog_ok=False, config_permissions_ok=False, key_protection="plaintext")
    response = client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, payload))
    assert response.status_code == 200, response.text
    event_types = {event.event_type for event in db.query(SecurityEvent).all()}
    assert {"SERVICE_AUTOSTART_DISABLED", "WATCHDOG_DISABLED", "CONFIG_PERMISSIONS_WEAKENED", "KEY_PROTECTION_WEAK"} <= event_types


def test_missing_service_records_both_failed_posture_checks(client, db):
    _, key, result = enroll(client, db)
    payload = integrity_payload(service_installed=False, service_running=False)
    response = client.post("/api/v1/integrity", json=signed(key, result["device_id"], 1, payload))
    assert response.status_code == 200, response.text
    events = db.query(SecurityEvent).filter_by(event_type="SERVICE_STOPPED_OR_MODIFIED").all()
    assert {event.details_json["failed_check"] for event in events} == {"service_installed", "service_running"}


def test_signed_watchdog_event_is_accepted(client, db):
    _, key, result = enroll(client, db)
    payload = {"event_type": "SERVICE_STOPPED_OR_MODIFIED", "details": {"service_running": False}}
    response = client.post("/api/v1/events", json=signed(key, result["device_id"], 1, payload))
    assert response.status_code == 200
    assert db.query(SecurityEvent).filter_by(event_type="SERVICE_STOPPED_OR_MODIFIED").count() == 1
