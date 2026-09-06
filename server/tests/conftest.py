import os
from pathlib import Path

TEST_DB = Path(__file__).parent / "test.db"
if TEST_DB.exists():
    TEST_DB.unlink()
os.environ["DATABASE_URL"] = f"sqlite:///{TEST_DB}"
os.environ["PUBLIC_BASE_URL"] = "https://meter.test"
os.environ["ONLINE_SECONDS"] = "180"
os.environ["LATE_SECONDS"] = "600"
os.environ["ARTIFACTS_DIR"] = str(Path(__file__).parent / "artifacts")

import pytest
from fastapi.testclient import TestClient

from app.db import Base, SessionLocal, engine
from app.main import app, rate_windows


@pytest.fixture(autouse=True)
def clean_db():
    Base.metadata.drop_all(engine)
    Base.metadata.create_all(engine)
    rate_windows.clear()
    yield


@pytest.fixture
def client():
    with TestClient(app) as value:
        yield value


@pytest.fixture
def db():
    session = SessionLocal()
    try:
        yield session
    finally:
        session.close()
