"""Shared test fixtures for tts-speaker tests."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest
from httpx import ASGITransport, AsyncClient

from tts_speaker.app.main import create_app
from tts_speaker.core.pipeline import TTSPipeline
from tts_speaker.infra.config import get_settings


@pytest.fixture(autouse=True)
def _clear_settings_cache():
    """Clear the lru_cache on get_settings between tests."""
    get_settings.cache_clear()
    yield
    get_settings.cache_clear()


@pytest.fixture
def mock_pipeline() -> MagicMock:
    """Create a mock TTSPipeline that returns 1 second of silence (24kHz)."""
    pipeline = MagicMock(spec=TTSPipeline)
    pipeline.is_ready = True
    pipeline.synthesize = AsyncMock(return_value=np.zeros(24000, dtype=np.float32))
    pipeline.voices = [
        {"id": "jf_alpha", "name": "Alpha", "gender": "female"},
        {"id": "jf_gongitsune", "name": "Gongitsune", "gender": "female"},
        {"id": "jf_nezumi", "name": "Nezumi", "gender": "female"},
        {"id": "jf_tebukuro", "name": "Tebukuro", "gender": "female"},
        {"id": "jm_kumo", "name": "Kumo", "gender": "male"},
    ]
    pipeline._device = "cpu"
    return pipeline


@pytest.fixture
def app(mock_pipeline: MagicMock):
    """Create a Starlette app with mocked pipeline (no auth by default)."""
    with patch.dict("os.environ", {"SERVICE_SECRET": ""}, clear=False):
        application = create_app(pipeline_override=mock_pipeline)
    return application


@pytest.fixture
async def client(app) -> AsyncClient:
    """Create an async test client."""
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
