from __future__ import annotations

import threading

from sqlalchemy import create_engine
from sqlalchemy.orm import DeclarativeBase, sessionmaker

from .config import database_connect_args, settings


class Base(DeclarativeBase):
    pass


connect_args = database_connect_args(settings.database_url)
engine_options = {"pool_pre_ping": True, "connect_args": connect_args}
if not settings.database_url.startswith("sqlite"):
    # A tiny per-instance pool reuses TLS/database handshakes across warm Vercel
    # invocations while keeping the fleet-wide connection ceiling bounded.
    engine_options.update({
        "pool_size": max(1, settings.db_pool_size),
        "max_overflow": max(0, settings.db_max_overflow),
        "pool_recycle": max(30, settings.db_pool_recycle_seconds),
        "pool_timeout": 5,
        "pool_use_lifo": True,
    })
    if settings.database_url.startswith("cockroachdb"):
        # Row locks and unique receipts provide the ordering guarantees this
        # workload needs; READ COMMITTED lets CockroachDB retry contention
        # server-side instead of surfacing avoidable 40001 responses to agents.
        engine_options["isolation_level"] = "READ COMMITTED"
engine = create_engine(settings.database_url, **engine_options)
SessionLocal = sessionmaker(bind=engine, expire_on_commit=False)
_schema_ready = False
_schema_lock = threading.Lock()


def ensure_schema():
    global _schema_ready
    if _schema_ready:
        return
    with _schema_lock:
        if not _schema_ready:
            Base.metadata.create_all(engine)
            _schema_ready = True


def get_db():
    ensure_schema()
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
