# Signed report protocol

Registration sends the user's chosen display name, detected device label, and a 32-byte Ed25519 public key. The server atomically creates the user/device and returns a device ID plus a scoped OTel secret. The private key remains local; no web account, password, or pre-created enrollment token is required.

Signed heartbeats may include a `quota` snapshot taken from Codex `token_count` events. A snapshot contains `plan_type`, primary/secondary `used_percent`, `window_minutes`, `resets_at`, and `observed_at`. The server validates percentages and renders `100 - used_percent` as the remaining quota. It does not receive credit balance or raw log content.

Each report signs this UTF-8 message:

```text
device_id + "\n" + decimal(sequence) + "\n" + RFC3339Nano(timestamp) + "\n" + event_id + "\n" + hex(SHA256(canonical_json(payload)))
```

Canonical JSON recursively sorts object keys, removes insignificant whitespace, and uses JSON scalar encodings. The server verifies the signature, checks `(device_id,event_id)` idempotency, then requires `sequence > last_sequence`. A reused sequence with a new event ID creates `REPLAY_ATTEMPT`.

Integrity reports also carry `state_id`, `scan_sequence`, `previous_snapshot_hash`, and `snapshot_hash`. The snapshot hash is SHA-256 over canonical JSON containing those chain inputs plus sorted opaque file metadata and findings. The server independently recomputes the hash, compares each report with the last accepted device snapshot, and records invalid-hash, reset, rollback, or link-break events. Signed `monitor_posture` fields cover the service manager and boolean protection results; only a SHA-256 fingerprint of service definitions is transmitted.

The watchdog uses the same device signature protocol for `SERVICE_STOPPED_OR_MODIFIED` and `SERVICE_RESTART_FAILED`. A normal installer-driven removal first sends `AGENT_UNINSTALL_REQUESTED`. If local reporting is completely disabled, no fabricated local event is required: server receipt-time aging independently produces the LATE/UNREACHABLE state.
