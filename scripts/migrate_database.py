#!/usr/bin/env python3
from __future__ import annotations

import os
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "server"))

from app.migration import migrate_database  # noqa: E402


def main() -> int:
    source = os.getenv("SOURCE_DATABASE_URL", "")
    target = os.getenv("TARGET_DATABASE_URL", "")
    if not source or not target:
        print("Set SOURCE_DATABASE_URL and TARGET_DATABASE_URL before running migration.", file=sys.stderr)
        return 2
    try:
        copied = migrate_database(source, target)
    except Exception as exc:
        print(f"Migration failed: {exc}", file=sys.stderr)
        return 1
    print("Migration completed atomically.")
    for table, count in copied.items():
        print(f"  {table}: {count}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
