from app.models import SecurityEvent, UsageReport
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


def test_student_detail_dashboard_renders(client, db):
    student, key, result = enroll(client, db)
    assert client.post("/api/v1/usage", json=signed(key, result["device_id"], 1, usage_payload(12345))).status_code == 200
    page = client.get(f"/students/{student.id}")
    assert page.status_code == 200
    assert "Daily token usage" in page.text
    assert "12.3K" in page.text
