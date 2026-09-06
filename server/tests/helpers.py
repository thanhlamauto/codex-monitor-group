import base64
import uuid
from datetime import datetime, timezone

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

from app.models import Student
from app.security import signature_message


def enroll(client, db):
    key = Ed25519PrivateKey.generate()
    public = base64.b64encode(key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)).decode()
    response = client.post("/api/v1/register", json={"name": "Student A", "device_label": "Laptop", "public_key": public})
    assert response.status_code == 200, response.text
    student = db.query(Student).filter(Student.name == "Student A").one()
    return student, key, response.json()


def signed(key, device_id, sequence, payload, event_id=None, timestamp=None):
    event_id = event_id or str(uuid.uuid4())
    timestamp = timestamp or datetime.now(timezone.utc).isoformat()
    message = signature_message(device_id, sequence, timestamp, event_id, payload)
    signature = base64.b64encode(key.sign(message)).decode()
    return {"device_id": device_id, "sequence": sequence, "timestamp": timestamp, "event_id": event_id, "payload": payload, "signature": signature}


def heartbeat_payload(**overrides):
    payload = {
        "agent_version": "1.0.0", "agent_sha256": "a" * 64,
        "ccusage_version": "20.0.20", "ccusage_sha256": "b" * 64,
        "codex_version": "codex-cli 0.133.0", "codex_home": "c" * 64,
        "config_fingerprint": "d" * 64, "uptime_seconds": 10,
    }
    payload.update(overrides)
    return payload
