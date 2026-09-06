from __future__ import annotations

import uuid
from datetime import date, datetime, timezone

from sqlalchemy import BigInteger, Boolean, Date, DateTime, Float, ForeignKey, Integer, JSON, String, Text, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column, relationship

from .db import Base


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


def uid() -> str:
    return str(uuid.uuid4())


class Student(Base):
    __tablename__ = "students"
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    name: Mapped[str] = mapped_column(String(200), nullable=False)
    device_label: Mapped[str] = mapped_column(String(200), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    devices: Mapped[list["Device"]] = relationship(back_populates="student")


class Device(Base):
    __tablename__ = "devices"
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    student_id: Mapped[str] = mapped_column(ForeignKey("students.id"), nullable=False, index=True)
    label: Mapped[str] = mapped_column(String(200), nullable=False)
    public_key: Mapped[str] = mapped_column(Text, nullable=False)
    otlp_token_hash: Mapped[str] = mapped_column(String(64), nullable=False)
    enrolled_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    last_seen: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), index=True)
    last_sequence: Mapped[int] = mapped_column(BigInteger, default=0)
    agent_version: Mapped[str | None] = mapped_column(String(64))
    agent_sha256: Mapped[str | None] = mapped_column(String(64))
    ccusage_version: Mapped[str | None] = mapped_column(String(64))
    ccusage_sha256: Mapped[str | None] = mapped_column(String(64))
    codex_version: Mapped[str | None] = mapped_column(String(64))
    codex_home_id: Mapped[str | None] = mapped_column(String(64))
    config_fingerprint: Mapped[str | None] = mapped_column(String(64))
    last_otlp_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    last_local_usage_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    student: Mapped[Student] = relationship(back_populates="devices")


class QuotaSnapshot(Base):
    __tablename__ = "quota_snapshots"
    device_id: Mapped[str] = mapped_column(ForeignKey("devices.id"), primary_key=True)
    plan_type: Mapped[str | None] = mapped_column(String(64))
    primary_used_percent: Mapped[float | None] = mapped_column(Float)
    primary_window_minutes: Mapped[int | None] = mapped_column(Integer)
    primary_resets_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    secondary_used_percent: Mapped[float | None] = mapped_column(Float)
    secondary_window_minutes: Mapped[int | None] = mapped_column(Integer)
    secondary_resets_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    observed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)


class UsageReport(Base):
    __tablename__ = "usage_reports"
    __table_args__ = (
        UniqueConstraint("device_id", "event_id", name="uq_usage_event"),
    )
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    device_id: Mapped[str] = mapped_column(ForeignKey("devices.id"), nullable=False, index=True)
    student_id: Mapped[str] = mapped_column(ForeignKey("students.id"), nullable=False, index=True)
    event_id: Mapped[str] = mapped_column(String(128), nullable=False)
    source: Mapped[str] = mapped_column(String(16), nullable=False, index=True)
    period_date: Mapped[date] = mapped_column(Date, nullable=False, index=True)
    input_tokens: Mapped[int] = mapped_column(BigInteger, default=0)
    cached_input_tokens: Mapped[int] = mapped_column(BigInteger, default=0)
    output_tokens: Mapped[int] = mapped_column(BigInteger, default=0)
    reasoning_output_tokens: Mapped[int] = mapped_column(BigInteger, default=0)
    total_tokens: Mapped[int] = mapped_column(BigInteger, default=0)
    occurred_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)


class IntegritySnapshot(Base):
    __tablename__ = "integrity_snapshots"
    __table_args__ = (UniqueConstraint("device_id", "event_id", name="uq_integrity_event"),)
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    device_id: Mapped[str] = mapped_column(ForeignKey("devices.id"), nullable=False, index=True)
    event_id: Mapped[str] = mapped_column(String(128), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    files_checked: Mapped[int] = mapped_column(Integer, default=0)
    status: Mapped[str] = mapped_column(String(32), default="OK")
    metadata_json: Mapped[dict] = mapped_column(JSON, default=dict)


class SecurityEvent(Base):
    __tablename__ = "security_events"
    __table_args__ = (UniqueConstraint("device_id", "event_key", name="uq_security_event_key"),)
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    student_id: Mapped[str] = mapped_column(ForeignKey("students.id"), nullable=False, index=True)
    device_id: Mapped[str] = mapped_column(ForeignKey("devices.id"), nullable=False, index=True)
    event_type: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    event_key: Mapped[str] = mapped_column(String(160), nullable=False)
    severity: Mapped[str] = mapped_column(String(16), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow, index=True)
    details_json: Mapped[dict] = mapped_column(JSON, default=dict)
    acknowledged_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))


class Heartbeat(Base):
    __tablename__ = "heartbeats"
    __table_args__ = (UniqueConstraint("device_id", "event_id", name="uq_heartbeat_event"),)
    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=uid)
    device_id: Mapped[str] = mapped_column(ForeignKey("devices.id"), nullable=False, index=True)
    event_id: Mapped[str] = mapped_column(String(128), nullable=False)
    sequence: Mapped[int] = mapped_column(BigInteger, nullable=False)
    client_timestamp: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    uptime_seconds: Mapped[int] = mapped_column(BigInteger, default=0)
