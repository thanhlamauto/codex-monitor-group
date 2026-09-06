from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator


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
        "ARCHIVED_LOG_MODIFIED", "USAGE_SOURCE_MISMATCH", "AGENT_UNINSTALL_REQUESTED"
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


class IntegrityPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    files_checked: int = Field(ge=0, le=1_000_000)
    status: Literal["OK", "TAMPER", "UNKNOWN"]
    findings: list[IntegrityFinding] = Field(default_factory=list, max_length=1000)
    file_metadata: list[FileMetadata] = Field(default_factory=list, max_length=10000)
