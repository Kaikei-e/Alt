"""Pin news-creator cheap/deep listener placement without touching ops PKI mux."""

from __future__ import annotations

import re
import urllib.error
import urllib.request
from pathlib import Path

from unittest.mock import AsyncMock, Mock

from fastapi import FastAPI
from fastapi.testclient import TestClient
from prometheus_client import REGISTRY, CollectorRegistry, generate_latest

from news_creator.handler.health_handler import create_health_router
from news_creator.infra.pki.ops import start_ops

REPO_ROOT = Path(__file__).resolve().parents[4]
COMPOSE_AI = REPO_ROOT / "compose" / "ai.yaml"
OPS_PY = (
    REPO_ROOT / "news-creator" / "app" / "news_creator" / "infra" / "pki" / "ops.py"
)
MAIN_PY = REPO_ROOT / "news-creator" / "app" / "main.py"


def _service_block(compose: str, name: str) -> str:
    marker = f"  {name}:\n"
    start = compose.index(marker)
    rest = compose[start + len(marker) :]
    nxt = re.search(r"\n  [a-z0-9-]+:\n", rest)
    return rest[: nxt.start()] if nxt else rest


def test_compose_publishes_plaintext_health_on_loopback_11434() -> None:
    compose = COMPOSE_AI.read_text(encoding="utf-8")
    block = _service_block(compose, "news-creator")
    assert '"127.0.0.1:11434:11434"' in block
    assert "0.0.0.0:11434" not in block
    assert "localhost:11434/health" in block
    assert "/health/deep" not in block
    assert "OPS_LISTEN=:9110" in block
    timeout = re.search(r"healthcheck:.*?timeout:\s*(\S+)", block, re.DOTALL)
    assert timeout is not None
    assert timeout.group(1).startswith("5")


def test_pki_ops_source_does_not_mount_health_deep() -> None:
    src = OPS_PY.read_text(encoding="utf-8")
    assert "/health/deep" not in src
    assert "send_error(404)" in src
    main = MAIN_PY.read_text(encoding="utf-8")
    assert "create_health_router" in main
    assert 'app.mount("/metrics", make_asgi_app())' in main


def test_pki_ops_listener_returns_404_for_deep_health() -> None:
    handle = start_ops("news-creator", CollectorRegistry(), listen="127.0.0.1:0")
    try:
        with urllib.request.urlopen(f"http://{handle.addr}/health", timeout=2) as resp:
            assert resp.status == 200
        try:
            urllib.request.urlopen(f"http://{handle.addr}/health/deep", timeout=2)
        except urllib.error.HTTPError as exc:
            assert exc.code == 404
        else:
            raise AssertionError("PKI ops must not serve /health/deep")
    finally:
        handle.aclose_sync()


def test_deep_health_never_returns_429_and_exports_app_metrics() -> None:
    mock = Mock()
    mock.list_models = AsyncMock(return_value=[{"name": "gemma4-e4b-q4km"}])
    mock.queue_status.return_value = {}
    app = FastAPI()
    app.include_router(create_health_router(mock))
    client = TestClient(app)
    statuses = [client.get("/health/deep").status_code for _ in range(12)]
    assert 429 not in statuses
    bodies = [client.get("/health/deep").json() for _ in range(3)]
    assert all(b["status"] in {"pass", "warn", "fail"} for b in bodies)
    assert mock.list_models.call_count == 1
    scraped = generate_latest(REGISTRY).decode()
    assert 'health_deep_status{service="news-creator"}' in scraped
    assert "health_deep_latency_seconds" in scraped
