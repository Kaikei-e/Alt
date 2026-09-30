"""Unit tests for health check handler."""

from fastapi.testclient import TestClient

from tts_speaker.main import create_app


def test_health_check_returns_ok() -> None:
    app = create_app()
    client = TestClient(app, raise_server_exceptions=False)
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
