from __future__ import annotations

from datetime import timedelta

import pytest
from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session

from app.config import normalize_database_url
from app.db import Base
from app.migration import migrate_database
from app.models import Device, Heartbeat, IntegritySnapshot, ProcessedEvent, Student, utcnow
from app.services import purge_expired_telemetry

from .helpers import enroll, heartbeat_payload, signed


def test_cockroach_cloud_url_uses_official_sqlalchemy_dialect():
    url = "postgresql://monitor:secret@cluster.aws-ap-southeast-1.cockroachlabs.cloud:26257/defaultdb?sslmode=verify-full"
    normalized = normalize_database_url(url)
    assert normalized.startswith("cockroachdb+psycopg://")
    assert "secret" in normalized


def test_heartbeat_updates_presence_but_samples_history(client, db):
    _, key, result = enroll(client, db)
    first = client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, heartbeat_payload()))
    assert first.status_code == 200
    assert first.json()["history_recorded"] is True
    db.expire_all()
    first_seen = db.get(Device, result["device_id"]).last_seen

    second = client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 2, heartbeat_payload()))
    assert second.status_code == 200
    assert second.json()["history_recorded"] is False
    db.expire_all()
    assert db.get(Device, result["device_id"]).last_seen >= first_seen
    assert db.query(Heartbeat).count() == 1
    assert db.query(ProcessedEvent).count() == 2


def test_one_receipt_table_handles_duplicates_across_routes(client, db):
    _, key, result = enroll(client, db)
    heartbeat = signed(key, result["device_id"], 1, heartbeat_payload())
    assert client.post("/api/v1/heartbeat", json=heartbeat).json()["duplicate"] is False
    assert client.post("/api/v1/heartbeat", json=heartbeat).json()["duplicate"] is True
    receipt = db.get(ProcessedEvent, (result["device_id"], heartbeat["event_id"]))
    assert receipt.event_kind == "heartbeat"
    assert db.query(ProcessedEvent).count() == 1


def test_retention_prunes_raw_telemetry_but_keeps_recent_rows(client, db):
    _, key, result = enroll(client, db)
    assert client.post("/api/v1/heartbeat", json=signed(key, result["device_id"], 1, heartbeat_payload())).status_code == 200
    device_id = result["device_id"]
    old = utcnow() - timedelta(days=90)
    db.query(Heartbeat).update({Heartbeat.received_at: old})
    db.query(ProcessedEvent).update({ProcessedEvent.received_at: old})
    db.add(IntegritySnapshot(device_id=device_id, event_id="old-integrity", created_at=old, files_checked=1, status="OK", metadata_json={}))
    db.add(IntegritySnapshot(device_id=device_id, event_id="recent-integrity", files_checked=1, status="OK", metadata_json={}))
    db.commit()

    deleted = purge_expired_telemetry(db)
    db.commit()
    assert deleted == {"heartbeats": 1, "integrity_snapshots": 1, "processed_events": 1}
    assert db.query(Heartbeat).count() == 0
    assert db.query(IntegritySnapshot).filter_by(event_id="recent-integrity").count() == 1
    assert db.get(Device, device_id) is not None


def test_migration_preserves_ids_keys_and_history_from_legacy_schema(tmp_path):
    source_path = tmp_path / "source.db"
    target_path = tmp_path / "target.db"
    source_url = f"sqlite:///{source_path}"
    target_url = f"sqlite:///{target_path}"
    source_engine = create_engine(source_url)
    Base.metadata.create_all(source_engine)
    with Session(source_engine) as session:
        student = Student(name="Migrated Student", device_label="Laptop")
        session.add(student)
        session.flush()
        device = Device(student_id=student.id, label="Laptop", public_key="public-key", otlp_token_hash="a" * 64, last_sequence=42)
        session.add(device)
        session.flush()
        session.add(Heartbeat(device_id=device.id, event_id="heartbeat-1", sequence=42, client_timestamp=utcnow(), uptime_seconds=100))
        session.add(IntegritySnapshot(device_id=device.id, event_id="integrity-1", files_checked=3, status="OK", metadata_json={"snapshot_hash": "b" * 64}))
        session.commit()
        student_id = student.id
        device_id = device.id
    # Match the pre-1.4 schema: no receipt table and no sampling marker.
    with source_engine.begin() as connection:
        connection.execute(text("DROP TABLE processed_events"))
        connection.execute(text("ALTER TABLE devices DROP COLUMN last_heartbeat_recorded_at"))
    source_engine.dispose()

    copied = migrate_database(source_url, target_url)
    assert copied["students"] == 1
    assert copied["devices"] == 1
    assert copied["heartbeats"] == 1
    assert copied["integrity_snapshots"] == 1
    assert copied["processed_events"] == 0

    target_engine = create_engine(target_url)
    with Session(target_engine) as session:
        assert session.get(Student, student_id).name == "Migrated Student"
        migrated = session.get(Device, device_id)
        assert migrated.public_key == "public-key"
        assert migrated.last_sequence == 42
        assert migrated.last_heartbeat_recorded_at is None
    target_engine.dispose()

    with pytest.raises(RuntimeError, match="target database is not empty"):
        migrate_database(source_url, target_url)
