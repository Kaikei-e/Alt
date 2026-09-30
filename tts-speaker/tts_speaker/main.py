"""Composition root and module-level application for tts-speaker."""

import logging
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI
from pydantic import ValidationError

from tts_speaker.app import create_app
from tts_speaker.config.settings import Settings
from tts_speaker.driver.irodori_http_driver import IrodoriHttpDriver
from tts_speaker.gateway.irodori_gateway import IrodoriGateway
from tts_speaker.infra.peer_identity import (
    PeerIdentityMiddleware,
    allowed_peers_from_env,
)
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase

logger = logging.getLogger(__name__)

try:
    settings = Settings()
except ValidationError as e:
    logging.basicConfig(level=logging.INFO)
    logger.error("Configuration validation failed: %s", e)
    sys.exit(1)

logging.basicConfig(
    level=settings.log_level,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logging.getLogger("httpx").setLevel(logging.WARNING)

logger.info(
    "Starting tts-speaker: irodori_host=%s, "
    "model=%s, voice_id=%s, max_chunk_chars=%d, max_text_chars=%d, chunk_gap_ms=%d",
    settings.irodori_base_url.host,
    settings.irodori_model_name,
    settings.tts_voice_id,
    settings.tts_max_chunk_chars,
    settings.tts_max_text_chars,
    settings.tts_chunk_gap_ms,
)

allowed_peers = allowed_peers_from_env()
if settings.peer_identity_strict and not allowed_peers:
    logger.error("PEER_IDENTITY_STRICT is enabled but MTLS_ALLOWED_PEERS is empty")
    sys.exit(1)

if settings.peer_identity_strict:
    logger.info("peer_identity_strict_enabled allowed=%s", ",".join(allowed_peers))
else:
    logger.info("peer_identity_strict_disabled allowed=%s", ",".join(allowed_peers))

client = httpx.AsyncClient(
    base_url=str(settings.irodori_base_url).rstrip("/"),
    timeout=httpx.Timeout(settings.irodori_request_timeout_seconds, connect=5.0),
)

driver = IrodoriHttpDriver(
    client=client,
    api_key=settings.irodori_api_key.get_secret_value(),
)
gateway = IrodoriGateway(
    driver=driver,
    model_name=settings.irodori_model_name,
    voice_id=settings.tts_voice_id,
    max_attempts=settings.irodori_max_attempts,
    backoff_seconds=settings.irodori_retry_backoff_seconds,
)
usecase = SynthesizeUsecase(
    synthesizer=gateway,
    max_chunk_chars=settings.tts_max_chunk_chars,
    max_text_chars=settings.tts_max_text_chars,
    chunk_gap_ms=settings.tts_chunk_gap_ms,
    queue_timeout_seconds=settings.tts_queue_timeout_seconds,
)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    try:
        yield
    finally:
        await client.aclose()


app = create_app(usecase=usecase, lifespan=lifespan)

app.add_middleware(
    PeerIdentityMiddleware,
    allowed=allowed_peers,
    strict=settings.peer_identity_strict,
)
