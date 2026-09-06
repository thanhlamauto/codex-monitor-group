from __future__ import annotations

from collections.abc import Iterator

from sqlalchemy import MetaData, create_engine, func, select
from sqlalchemy.engine import Engine

from .config import database_connect_args, normalize_database_url
from .db import Base
from . import models as _models  # noqa: F401 - registers every mapped table


def _batches(rows: Iterator[dict], size: int = 500) -> Iterator[list[dict]]:
    batch: list[dict] = []
    for row in rows:
        batch.append(row)
        if len(batch) == size:
            yield batch
            batch = []
    if batch:
        yield batch


def _engine(url: str) -> Engine:
    normalized = normalize_database_url(url)
    return create_engine(normalized, pool_pre_ping=True, connect_args=database_connect_args(normalized))


def migrate_database(source_url: str, target_url: str) -> dict[str, int]:
    """Atomically copy the legacy database into an empty current schema.

    Columns and tables introduced after the source was created are deliberately
    omitted and receive their target defaults. Stable IDs, device public keys,
    audit events, usage, and telemetry history are preserved byte-for-byte.
    """
    source_engine = _engine(source_url)
    target_engine = _engine(target_url)
    if source_engine.url.render_as_string(hide_password=True) == target_engine.url.render_as_string(hide_password=True):
        raise ValueError("source and target databases must be different")

    try:
        source_metadata = MetaData()
        source_metadata.reflect(bind=source_engine)
        Base.metadata.create_all(target_engine)

        with target_engine.connect() as connection:
            occupied = [
                table.name
                for table in Base.metadata.sorted_tables
                if connection.scalar(select(func.count()).select_from(table))
            ]
        if occupied:
            raise RuntimeError(f"target database is not empty: {', '.join(occupied)}")

        copied: dict[str, int] = {}
        with source_engine.connect() as source, target_engine.begin() as target:
            for target_table in Base.metadata.sorted_tables:
                source_table = source_metadata.tables.get(target_table.name)
                if source_table is None:
                    copied[target_table.name] = 0
                    continue
                common_names = [column.name for column in target_table.columns if column.name in source_table.c]
                result = source.execute(select(*(source_table.c[name] for name in common_names)))
                count = 0
                rows = (dict(row) for row in result.mappings())
                for batch in _batches(rows):
                    target.execute(target_table.insert(), batch)
                    count += len(batch)
                copied[target_table.name] = count
        return copied
    finally:
        source_engine.dispose()
        target_engine.dispose()
