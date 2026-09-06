from __future__ import annotations

from datetime import timedelta

from sqlalchemy import select

from .db import Base, SessionLocal, engine
from .models import Device, IntegritySnapshot, QuotaSnapshot, SecurityEvent, Student, UsageReport, utcnow
from .services import classroom_today


def seed_student(db, name: str, label: str, today_total: int, week_total: int, last_seen_minutes: int, quota_used: float, tamper: bool = False):
    if db.scalar(select(Student).where(Student.name == name)):
        return
    student = Student(name=name, device_label=label)
    db.add(student)
    db.flush()
    now = utcnow()
    device = Device(
        student_id=student.id, label=label, public_key="demo-public-key", otlp_token_hash="demo",
        last_seen=now - timedelta(minutes=last_seen_minutes), agent_version="1.3.1", agent_sha256="demo",
        ccusage_version="20.0.20", ccusage_sha256="demo", codex_version="codex-cli 0.133.0",
        codex_home_id="demo", last_otlp_at=now - timedelta(minutes=last_seen_minutes),
        last_local_usage_at=now - timedelta(minutes=last_seen_minutes),
    )
    db.add(device)
    db.flush()
    db.add(QuotaSnapshot(
        device_id=device.id, plan_type="pro", primary_used_percent=quota_used,
        primary_window_minutes=300, primary_resets_at=now + timedelta(hours=3),
        secondary_used_percent=min(99, quota_used + 12), secondary_window_minutes=10080,
        secondary_resets_at=now + timedelta(days=4), observed_at=now,
    ))
    db.add(IntegritySnapshot(device_id=device.id, event_id=f"demo-integrity:{student.id}", files_checked=3, status="TAMPER" if tamper else "OK", metadata_json={"files": []}))
    today = classroom_today()
    remaining = max(0, week_total - today_total)
    per_day = remaining // 6
    for offset in range(7):
        total = today_total if offset == 0 else per_day
        db.add(UsageReport(
            device_id=device.id, student_id=student.id, event_id=f"demo:{student.id}:{offset}",
            source="local", period_date=today - timedelta(days=offset), input_tokens=int(total * .55),
            cached_input_tokens=int(total * .30), output_tokens=int(total * .15),
            reasoning_output_tokens=int(total * .04), total_tokens=total, occurred_at=now - timedelta(days=offset),
        ))
    if tamper:
        db.add(SecurityEvent(
            student_id=student.id, device_id=device.id, event_type="LOG_PREFIX_MODIFIED",
            event_key=f"demo-tamper:{student.id}", severity="CRITICAL",
            details_json={"opaque_file_id": "demo-7d21", "old_size": 48210, "new_size": 49218},
        ))


def main():
    Base.metadata.create_all(engine)
    with SessionLocal() as db:
        seed_student(db, "Student A · Healthy", "MacBook A", 12_400_000, 81_200_000, 0, 28)
        seed_student(db, "Student B · High usage", "Linux B", 31_800_000, 201_400_000, 1, 82)
        seed_student(db, "Student C · Missing heartbeat", "MacBook C", 8_100_000, 51_700_000, 18, 55)
        seed_student(db, "Student D · Log tamper", "Linux D", 4_600_000, 28_900_000, 1, 15, tamper=True)
        db.commit()
    print("Demo classroom ready: four students seeded.")


if __name__ == "__main__":
    main()
