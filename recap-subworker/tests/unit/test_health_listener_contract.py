"""Pin recap-subworker cheap/deep listener placement; PKI ops stays /health+/metrics."""

from __future__ import annotations

import re
import urllib.error
import urllib.request
from pathlib import Path

from prometheus_client import CollectorRegistry

from recap_subworker.app.infra.pki.ops import start_ops

REPO_ROOT = Path(__file__).resolve().parents[3]
COMPOSE_RECAP = REPO_ROOT / "compose" / "recap.yaml"
OPS_PY = REPO_ROOT / "recap-subworker" / "recap_subworker" / "app" / "infra" / "pki" / "ops.py"
MAIN_PY = REPO_ROOT / "recap-subworker" / "recap_subworker" / "app" / "main.py"
HEALTH_PY = REPO_ROOT / "recap-subworker" / "recap_subworker" / "app" / "routers" / "health.py"


def _service_block(compose: str, name: str) -> str:
    marker = f"  {name}:\n"
    start = compose.index(marker)
    rest = compose[start + len(marker) :]
    nxt = re.search(r"\n  [a-z0-9-]+:\n", rest)
    return rest[: nxt.start()] if nxt else rest


def test_compose_publishes_cheap_health_on_loopback_8002() -> None:
    compose = COMPOSE_RECAP.read_text(encoding="utf-8")
    block = _service_block(compose, "recap-subworker")
    assert '"127.0.0.1:8002:8002"' in block
    assert "localhost:8002/health" in block
    assert "/health/deep" not in block
    assert "OPS_LISTEN=:9110" in block


def test_pki_ops_source_does_not_mount_health_deep() -> None:
    src = OPS_PY.read_text(encoding="utf-8")
    assert "/health/deep" not in src
    assert "send_error(404)" in src
    main = MAIN_PY.read_text(encoding="utf-8")
    assert "health.router" in main


def test_pki_ops_listener_returns_404_for_deep_health() -> None:
    handle = start_ops("recap-subworker", CollectorRegistry(), listen="127.0.0.1:0")
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


def test_health_deep_handler_does_not_construct_runner_per_request() -> None:
    src = HEALTH_PY.read_text(encoding="utf-8")
    _, _, rest = src.partition("async def health_deep")
    handler, _, _ = rest.partition("\nasync def ")
    assert "DeepHealthRunner(" not in handler, (
        "M6: constructing DeepHealthRunner inside health_deep drops cache/singleflight"
    )
