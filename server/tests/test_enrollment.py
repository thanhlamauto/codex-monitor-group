import base64

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

from app.models import Device, Student

def public_key():
    key = Ed25519PrivateKey.generate().public_key()
    return base64.b64encode(key.public_bytes(Encoding.Raw, PublicFormat.Raw)).decode()


def test_public_registration_creates_student_and_device(client, db):
    response = client.post("/api/v1/register", json={"name": "Nguyen Van An", "device_label": "An MacBook", "public_key": public_key()})
    assert response.status_code == 200
    assert response.json()["student_name"] == "Nguyen Van An"
    assert response.json()["device_label"] == "An MacBook"
    assert response.json()["device_id"]
    assert response.json()["otlp_token"]
    assert db.query(Student).count() == 1
    assert db.query(Device).count() == 1


def test_registration_normalizes_display_values(client):
    response = client.post("/api/v1/register", json={"name": "  Nguyen   Van An  ", "device_label": "  Lab   Mac  ", "public_key": public_key()})
    assert response.status_code == 200
    assert response.json()["student_name"] == "Nguyen Van An"
    assert response.json()["device_label"] == "Lab Mac"


def test_registration_requires_name_and_label(client):
    response = client.post("/api/v1/register", json={"public_key": public_key()})
    assert response.status_code == 422


def test_dashboard_is_public_and_shows_one_line_installer(client):
    page = client.get("/")
    assert page.status_code == 200
    assert "Không cần tài khoản" in page.text
    assert "curl -fsSL https://meter.test/install.sh | sudo sh" in page.text
    assert client.get("/login").status_code == 404


def test_legacy_enroll_alias_needs_no_token(client):
    response = client.post("/api/v1/enroll", json={"name": "Student A", "device_label": "Laptop", "public_key": public_key()})
    assert response.status_code == 200


def test_invalid_public_key_rejected_without_creating_rows(client, db):
    response = client.post("/api/v1/register", json={"name": "Student A", "device_label": "Laptop", "public_key": "a" * 44})
    assert response.status_code == 422
    assert db.query(Student).count() == 0
