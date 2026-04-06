"""Provider verification tests for tts-speaker.

Verifies that tts-speaker fulfills the contracts defined by its consumers:
- alt-butterfly-facade (via /alt.tts.v1.TTSService/Synthesize)

Uses a mock TTSPipeline (no GPU/model required) and verifies against pact
contracts. Contracts are loaded from either:
- Pact Broker (when PACT_BROKER_BASE_URL is set) — used in CI
- Local filesystem (fallback) — used in local development
"""

import logging
import os
import socket
import threading
import time
import urllib.request
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock

import numpy as np
import pytest
import uvicorn
from pact import Verifier
from starlette.applications import Starlette
from starlette.routing import Mount, Route
from starlette.responses import JSONResponse

from tts_speaker.app.connect_service import TTSConnectService
from tts_speaker.gen.proto.alt.tts.v1.tts_connect import TTSServiceASGIApplication
from tts_speaker.infra.config import Settings

logger = logging.getLogger(__name__)

# Pact Broker configuration
PACT_BROKER_URL = os.environ.get("PACT_BROKER_BASE_URL")
PACT_BROKER_USERNAME = os.environ.get("PACT_BROKER_USERNAME")
PACT_BROKER_PASSWORD = os.environ.get("PACT_BROKER_PASSWORD")
PACT_PROVIDER_VERSION = os.environ.get("PACT_PROVIDER_VERSION")
PACT_PROVIDER_BRANCH = os.environ.get("PACT_PROVIDER_BRANCH")

# Local pact file
PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent / "pacts"
BFF_PACT = PACT_DIR / "alt-butterfly-facade-tts-speaker.json"


def _get_free_port() -> int:
    """Get a free port from the OS."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _create_mock_pipeline():
    """Create a mock TTSPipeline that returns silent audio."""
    pipeline = MagicMock()
    pipeline.is_ready = True

    # generate() returns (audio_array, sample_rate)
    async def mock_generate(text, voice=None, speed=None):
        return np.zeros(24000, dtype=np.float32), 24000

    pipeline.generate = AsyncMock(side_effect=mock_generate)
    return pipeline


def _create_provider_app():
    """Create a Starlette app with a mocked TTS pipeline for provider verification."""
    pipeline = _create_mock_pipeline()
    settings = Settings()

    tts_service = TTSConnectService(pipeline, settings)
    tts_asgi = TTSServiceASGIApplication(tts_service)

    async def health(request):
        return JSONResponse({"status": "ok"})

    async def provider_states(request):
        """Handle provider state setup for Pact verification."""
        body = await request.json()
        logger.info("Provider state: %s", body.get("state", ""))
        return JSONResponse({"status": "ok"})

    app = Starlette(
        routes=[
            Route("/health", health),
            Route("/_pact/provider-states", provider_states, methods=["POST"]),
            Mount(tts_asgi.path, app=tts_asgi),
        ],
    )
    app.state.pipeline = pipeline
    app.state.settings = settings

    return app


@pytest.fixture(scope="module")
def provider_url():
    """Start the provider server on a free port."""
    port = _get_free_port()
    app = _create_provider_app()

    config = uvicorn.Config(app, host="127.0.0.1", port=port, log_level="warning")
    server = uvicorn.Server(config)

    thread = threading.Thread(target=server.run, daemon=True)
    thread.start()

    base_url = f"http://127.0.0.1:{port}"
    for _ in range(100):
        try:
            urllib.request.urlopen(f"{base_url}/health")
            break
        except Exception:
            time.sleep(0.05)
    else:
        pytest.fail("Provider server did not start")

    yield base_url, port


@pytest.mark.skipif(
    not PACT_BROKER_URL and not BFF_PACT.exists(),
    reason=f"No Broker URL and pact file not found: {BFF_PACT}",
)
def test_verify_bff_contract(provider_url: tuple[str, int]):
    """Verify tts-speaker satisfies alt-butterfly-facade's contract."""
    base_url, port = provider_url

    verifier = Verifier("tts-speaker")
    verifier.add_transport(protocol="http", port=port)

    if PACT_BROKER_URL:
        builder = verifier.broker_source(
            url=PACT_BROKER_URL,
            username=PACT_BROKER_USERNAME,
            password=PACT_BROKER_PASSWORD,
            selector=True,
        )
        builder = builder.consumer_version(consumer="alt-butterfly-facade", latest=True)
        builder.build()

        if PACT_PROVIDER_VERSION:
            verifier.set_publish_options(
                version=PACT_PROVIDER_VERSION,
                url=PACT_BROKER_URL,
                branch=PACT_PROVIDER_BRANCH,
            )
    else:
        verifier.add_source(str(BFF_PACT))

    verifier.state_handler(
        f"{base_url}/_pact/provider-states",
        teardown=False,
        body=True,
    )

    verifier.verify()
