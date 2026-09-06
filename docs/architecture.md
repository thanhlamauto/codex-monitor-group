# Architecture

The deployable unit is three containers: Caddy, one FastAPI process, and PostgreSQL. The API serves the HTML dashboard and release artifacts. This is intentional: the target scale does not benefit from a separate frontend, message broker, or microservices.

On each participant device, one privileged service owns identity, queue, and checkpoints. Codex sends OTel only over loopback. ccusage and integrity scans read local files but return normalized counters or metadata. All outbound reports enter the same ordered signed queue. Registration and dashboard reads are public by design; telemetry writes remain bound to each device's Ed25519 key.

Linux uses a hardened systemd unit and root timer; macOS uses a root launch daemon plus watchdog launch daemon; Windows uses a native delayed-auto LocalSystem Service with SCM recovery plus a SYSTEM scheduled watchdog. The platform implementations share the same signed report protocol. Windows secrets are DPAPI-encrypted at rest and install/data ACLs allow only SYSTEM and Administrators; Unix secrets remain in a root-owned `0600` file.

Integrity snapshots form a device-local SHA-256 chain over the random state ID, scan sequence, previous snapshot hash, sorted opaque file metadata, and findings. The server remembers the last accepted chain point and emits immutable audit events for state reset, rollback, or a broken link. Monitor posture is included in the signed snapshot: service lifecycle settings, watchdog state, binary/config permissions, key-protection mode, and a fingerprint of service definitions. A separate privileged scheduler reports primary-service failure and attempts restart.

This is defense in depth for detection, not prevention against a fully privileged administrator. Root/admin can ultimately stop both schedulers, read or replace local state, and forge observations after obtaining the device identity. Server-side missing-heartbeat detection remains the independent signal when all local reporting stops.
