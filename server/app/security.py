from __future__ import annotations

import base64
import hashlib
import json
from datetime import datetime, timezone

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey


def hash_secret(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def canonical_payload(payload: dict) -> bytes:
    return json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def signature_message(device_id: str, sequence: int, timestamp: str, event_id: str, payload: dict) -> bytes:
    digest = hashlib.sha256(canonical_payload(payload)).hexdigest()
    return f"{device_id}\n{sequence}\n{timestamp}\n{event_id}\n{digest}".encode()


def verify_signature(public_key_b64: str, signature_b64: str, message: bytes) -> bool:
    try:
        key = Ed25519PublicKey.from_public_bytes(base64.b64decode(public_key_b64, validate=True))
        key.verify(base64.b64decode(signature_b64, validate=True), message)
        return True
    except Exception:
        return False


def parse_client_timestamp(value: str) -> datetime:
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)
