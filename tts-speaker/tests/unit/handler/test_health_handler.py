"""Unit tests for health check handler."""

from unittest.mock import AsyncMock

from fastapi.testclient import TestClient

from tts_speaker.app import create_app
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase


def test_health_check_returns_ok() -> None:
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    app = create_app(usecase=mock_usecase)
    client = TestClient(app, raise_server_exceptions=False)
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
