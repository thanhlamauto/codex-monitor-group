from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


class EnrollRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str = Field(min_length=1, max_length=200)
    public_key: str = Field(min_length=40, max_length=128)
    device_label: str = Field(min_length=1, max_length=200)

    @field_validator("name", "device_label")
    @classmethod
    def clean_label(cls, value: str) -> str:
        value = " ".join(value.split())
        if not value or any(ord(char) < 32 for char in value):
            raise ValueError("must contain visible text")
        return value


class SignedEnvelope(BaseModel):
    device_id: str = Field(min_length=36, max_length=36)
    sequence: int = Field(gt=0)
    timestamp: str = Field(min_length=20, max_length=40)
    event_id: str = Field(min_length=8, max_length=128)
    payload: dict[str, Any]
    signature: str = Field(min_length=40, max_length=256)


class RateLimitWindow(BaseModel):
    model_config = ConfigDict(extra="forbid")

    used_percent: float = Field(ge=0, le=100)
    window_minutes: int = Field(ge=1, le=525_600)
    resets_at: int = Field(ge=0, le=4_102_444_800)


class QuotaPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    plan_type: str | None = Field(default=None, max_length=64)
    primary: RateLimitWindow | None = None
    secondary: RateLimitWindow | None = None
    observed_at: str = Field(min_length=20, max_length=40)


class UsageDay(BaseModel):
    model_config = ConfigDict(extra="forbid")

    date: str
    input_tokens: int = Field(ge=0)
    cached_input_tokens: int = Field(ge=0)
    output_tokens: int = Field(ge=0)
    reasoning_output_tokens: int = Field(default=0, ge=0)
    total_tokens: int = Field(ge=0)


class UsagePayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    source: Literal["local", "otel"] = "local"
    days: list[UsageDay] = Field(max_length=400)


class IntegrityFinding(BaseModel):
    model_config = ConfigDict(extra="forbid")

    event_type: Literal[
        "AGENT_BINARY_MODIFIED", "CCUSAGE_BINARY_MODIFIED", "TELEMETRY_CONFIG_CHANGED",
        "CODEX_HOME_CHANGED", "LOG_PREFIX_MODIFIED", "LOG_TRUNCATED", "LOG_DELETED",
        "ARCHIVED_LOG_MODIFIED", "USAGE_SOURCE_MISMATCH", "AGENT_UNINSTALL_REQUESTED",
        "SERVICE_STOPPED_OR_MODIFIED", "SERVICE_RESTART_FAILED", "SERVICE_CONFIGURATION_CHANGED",
        "SERVICE_AUTOSTART_DISABLED", "SERVICE_RECOVERY_DISABLED", "WATCHDOG_DISABLED",
        "AGENT_PERMISSIONS_WEAKENED", "CONFIG_PERMISSIONS_WEAKENED", "KEY_PROTECTION_WEAK",
        "INTEGRITY_STATE_RESET", "INTEGRITY_STATE_ROLLBACK", "INTEGRITY_CHAIN_BROKEN",
        "INTEGRITY_SNAPSHOT_HASH_INVALID"
    ]
    severity: Literal["INFO", "WARNING", "CRITICAL"]
    opaque_file_id: str | None = Field(default=None, max_length=64)
    old_size: int | None = Field(default=None, ge=0)
    new_size: int | None = Field(default=None, ge=0)


class FileMetadata(BaseModel):
    model_config = ConfigDict(extra="forbid")

    opaque_file_id: str = Field(min_length=16, max_length=64)
    size: int = Field(ge=0)
    prefix_size: int = Field(ge=0)
    prefix_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    full_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    archived: bool
    mtime_unix: int


class MonitorPosture(BaseModel):
    model_config = ConfigDict(extra="forbid")

    os: Literal["linux", "darwin", "windows"]
    service_manager: Literal["systemd", "launchd", "windows-scm", "unsupported"]
    service_installed: bool
    service_running: bool
    auto_start_ok: bool
    restart_policy_ok: bool
    watchdog_expected: bool
    watchdog_ok: bool
    agent_permissions_ok: bool
    config_permissions_ok: bool
    key_protection: str = Field(min_length=1, max_length=64)
    service_fingerprint: str = Field(pattern=r"^[0-9a-f]{64}$")


class IntegrityPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    files_checked: int = Field(ge=0, le=1_000_000)
    status: Literal["OK", "TAMPER", "UNKNOWN"]
    findings: list[IntegrityFinding] = Field(default_factory=list, max_length=1000)
    file_metadata: list[FileMetadata] = Field(default_factory=list, max_length=10000)
    state_id: str | None = Field(default=None, pattern=r"^[0-9a-f]{32}$")
    scan_sequence: int | None = Field(default=None, ge=1)
    previous_snapshot_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    snapshot_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    monitor_posture: MonitorPosture | None = None

    @model_validator(mode="after")
    def complete_chain(self):
        present = (self.state_id is not None, self.scan_sequence is not None, self.snapshot_hash is not None)
        if any(present) and not all(present):
            raise ValueError("state_id, scan_sequence, and snapshot_hash must be supplied together")
        if self.previous_snapshot_hash is not None and not all(present):
            raise ValueError("previous_snapshot_hash requires a complete integrity chain")
        return self
