# Signed report protocol

Registration sends the user's chosen display name, detected device label, and a 32-byte Ed25519 public key. The server atomically creates the user/device and returns a device ID plus a scoped OTel secret. The private key remains local; no web account, password, or pre-created enrollment token is required.

Signed heartbeats may include a `quota` snapshot taken from Codex `token_count` events. A snapshot contains `plan_type`, primary/secondary `used_percent`, `window_minutes`, `resets_at`, and `observed_at`. The server validates percentages and renders `100 - used_percent` as the remaining quota. It does not receive credit balance or raw log content.

Each report signs this UTF-8 message:

```text
device_id + "\n" + decimal(sequence) + "\n" + RFC3339Nano(timestamp) + "\n" + event_id + "\n" + hex(SHA256(canonical_json(payload)))
```

Canonical JSON recursively sorts object keys, removes insignificant whitespace, and uses JSON scalar encodings. The server verifies the signature, checks `(device_id,event_id)` idempotency, then requires `sequence > last_sequence`. A reused sequence with a new event ID creates `REPLAY_ATTEMPT`.
