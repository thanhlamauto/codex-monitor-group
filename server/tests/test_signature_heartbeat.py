from datetime import datetime, timedelta, timezone

from app.models import Device, QuotaSnapshot, SecurityEvent
from app.services import device_state

from .helpers import enroll, heartbeat_payload, signed


def test_valid_signature_and_heartbeat_online(client, db):
    _, key, result = enroll(client, db)
    response = client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, heartbeat_payload()))
    assert response.status_code == 200
    db.expire_all()
    assert device_state(db.get(Device, result["device_id"])) == "ONLINE"


def test_modified_payload_is_rejected_and_audited(client, db):
    _, key, result = enroll(client, db)
    envelope = signed(key, result["device_id"], 1, heartbeat_payload())
    envelope["payload"]["uptime_seconds"] = 999
    assert client.post("/api/v1/heartbeat", json=envelope).status_code == 401
    assert db.query(SecurityEvent).filter_by(event_type="INVALID_SIGNATURE").count() == 1


def test_replayed_sequence_is_rejected(client, db):
    _, key, result = enroll(client, db)
    assert client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 5, heartbeat_payload())).status_code == 200
    assert client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 5, heartbeat_payload())).status_code == 409
    assert db.query(SecurityEvent).filter_by(event_type="REPLAY_ATTEMPT").count() == 1


def test_duplicate_event_is_idempotent(client, db):
    _, key, result = enroll(client, db)
    envelope = signed(key, result["device_id"], 1, heartbeat_payload())
    assert client.post("/api/v1/heartbeat", json=envelope).json()["duplicate"] is False
    assert client.post("/api/v1/heartbeat", json=envelope).json()["duplicate"] is True


def test_heartbeat_state_boundaries(client, db):
    _, _, result = enroll(client, db)
    device = db.get(Device, result["device_id"])
    now = datetime.now(timezone.utc)
    device.last_seen = now - timedelta(seconds=60)
    assert device_state(device, now) == "ONLINE"
    device.last_seen = now - timedelta(seconds=300)
    assert device_state(device, now) == "LATE"
    device.last_seen = now - timedelta(seconds=601)
    assert device_state(device, now) == "UNREACHABLE"


def test_signed_heartbeat_stores_and_renders_quota(client, db):
    student, key, result = enroll(client, db)
    quota = {
        "plan_type": "pro",
        "primary": {"used_percent": 23.5, "window_minutes": 300, "resets_at": 1788667200},
        "secondary": {"used_percent": 61, "window_minutes": 10080, "resets_at": 1789272000},
        "observed_at": "2026-09-06T02:00:00Z",
    }
    response = client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, heartbeat_payload(quota=quota)))
    assert response.status_code == 200, response.text
    db.expire_all()
    snapshot = db.get(QuotaSnapshot, result["device_id"])
    assert snapshot.plan_type == "pro"
    assert snapshot.primary_used_percent == 23.5
    assert snapshot.secondary_window_minutes == 10080
    dashboard = client.get("/")
    assert "76%" in dashboard.text
    assert "39%" in dashboard.text
    detail = client.get(f"/students/{student.id}")
    assert "Gói Codex" in detail.text
    assert "5 giờ" in detail.text
    assert "1 tuần" in detail.text


def test_invalid_quota_is_rejected(client, db):
    _, key, result = enroll(client, db)
    quota = {
        "primary": {"used_percent": 101, "window_minutes": 300, "resets_at": 1788667200},
        "observed_at": "2026-09-06T02:00:00Z",
    }
    response = client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, heartbeat_payload(quota=quota)))
    assert response.status_code == 422
    assert db.get(QuotaSnapshot, result["device_id"]) is None
