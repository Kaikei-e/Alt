"""Shared test fixtures for tts-speaker tests."""

from __future__ import annotations

from collections.abc import AsyncGenerator
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
    """TTSPipeline mock with a fake engine pre-wired for the 3 Qwen JA voices."""
    pipeline = MagicMock(spec=TTSPipeline)
    pipeline.is_ready = True
    pipeline.synthesize = AsyncMock(
        return_value=(np.zeros(24000, dtype=np.float32), 24000),
    )
    pipeline.voices = [
        {"id": "qwen-ja-1", "name": "JA Voice 1", "gender": "female"},
        {"id": "qwen-ja-2", "name": "JA Voice 2", "gender": "female"},
        {"id": "qwen-ja-3", "name": "JA Voice 3", "gender": "female"},
    ]
    pipeline.voice_ids = {"qwen-ja-1", "qwen-ja-2", "qwen-ja-3"}
    pipeline.device = "cpu"
    pipeline.gpu_name = None
    pipeline.keepalive_tick = AsyncMock(return_value=None)

    # The /health endpoint reads pipeline.engine.name.
    fake_engine = MagicMock()
    fake_engine.name = "qwen3-tts-12hz-0.6b-customvoice"
    pipeline.engine = fake_engine
    return pipeline


@pytest.fixture
def app(mock_pipeline: MagicMock):
    """Create a Starlette app with mocked pipeline (no auth by default)."""
    with patch.dict("os.environ", {"SERVICE_SECRET": ""}, clear=False):
        application = create_app(pipeline_override=mock_pipeline)
    return application


@pytest.fixture
async def client(app) -> AsyncGenerator[AsyncClient]:
    """Create an async test client."""
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
