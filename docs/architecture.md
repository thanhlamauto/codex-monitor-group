# Architecture

The deployable unit is three containers: Caddy, one FastAPI process, and PostgreSQL. The API serves the HTML dashboard and release artifacts. This is intentional: the target scale does not benefit from a separate frontend, message broker, or microservices.

On each participant device, one root service owns identity, queue, and checkpoints. Codex sends OTel only over loopback. ccusage and integrity scans read local files but return normalized counters or metadata. All outbound reports enter the same ordered signed queue. Registration and dashboard reads are public by design; telemetry writes remain bound to each device's Ed25519 key.

Windows support should implement the same `install service / start / stop / status` lifecycle behind a platform installer without changing the report protocol.
