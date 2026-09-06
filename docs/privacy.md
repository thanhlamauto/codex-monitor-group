# Privacy boundary

The local collector is the boundary. It parses OTLP in memory, selects five numeric token fields and a timestamp, and discards all other attributes. For quota display, codex-guard reads only the tail of local JSONL files and decodes `token_count.info.rate_limits`; it returns only plan type, used percentage, window length, reset timestamp, and observation time. Prompt text, model output, credit balance, and all unknown fields are discarded in memory. ccusage performs daily token parsing in a separate pinned executable.

Server schemas reject integrity metadata keys outside the explicit allowlist. There is no raw-payload column for OTel or JSONL and no API for uploading either.

Integrity reports additionally contain a random local state ID, monotonic scan number, previous/current snapshot hashes, service posture booleans, key-protection label, and a hash of service definitions. These values detect reset/rollback and monitor weakening without sending unit files, task XML, registry values, ACL text, usernames, or filesystem paths.
