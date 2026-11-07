"""Tests for RemoteHealthChecker rewritten around RemoteTarget and /v1/models."""

import json
import logging
from unittest.mock import AsyncMock, MagicMock

import aiohttp
import pytest

from news_creator.gateway.remote_health_checker import RemoteHealthChecker


class _StubGetContext:
    """Async context manager for aiohttp.ClientSession.get."""

    def __init__(self, response):
        self._response = response

    async def __aenter__(self):
        return self._response

    async def __aexit__(self, exc_type, exc, tb):
        return False


def _make_response(status: int = 200, body: str = ""):
    resp = MagicMock()
    resp.status = status
    resp.text = AsyncMock(return_value=body)
    return resp


def _build_session_by_url(url_to_response: dict[str, object]):
    """Build a ClientSession mock that dispatches GET by URL."""
    session = MagicMock()
    session.closed = False
    session.headers = {}

    def _get(url: str, **kwargs):
        entry = url_to_response.get(url)
        if isinstance(entry, BaseException):
            raise entry
        return _StubGetContext(entry)

    session.get = MagicMock(side_effect=_get)
    return session


def _make_targets():
    from news_creator.config.distributed_be_config import RemoteTarget

    return [
        RemoteTarget(
            base_url="http://router-a.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        ),
        RemoteTarget(
            base_url="http://router-a.example:8080",
            model="provider-b/model-b:latest",
            num_ctx=16384,
        ),
        RemoteTarget(
            base_url="http://router-b.example:8080",
            model="provider-c/model-c:latest",
            num_ctx=65536,
        ),
    ]


def _make_checker():
    targets = _make_targets()
    checker = RemoteHealthChecker(
        targets=targets,
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    return checker, targets


async def _make_all_healthy(checker: RemoteHealthChecker, targets) -> None:
    models_by_base: dict[str, list[dict[str, str]]] = {}
    for t in targets:
        models_by_base.setdefault(t.base_url, []).append({"id": t.model})
    url_map = {
        f"{base_url}/v1/models": _make_response(
            200, json.dumps({"object": "list", "data": data})
        )
        for base_url, data in models_by_base.items()
    }
    checker._session = _build_session_by_url(url_map)
    await checker._check_all()


@pytest.mark.asyncio
async def test_one_get_v1_models_per_distinct_base_url_evaluates_all_targets():
    """One GET {base_url}/v1/models per distinct base_url evaluates every target on that host."""
    checker, _targets = _make_checker()
    router_a_models = {
        "object": "list",
        "data": [{"id": "provider-a/model-a:latest"}],
    }
    router_b_models = {
        "object": "list",
        "data": [{"id": "provider-c/model-c:latest"}],
    }
    session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200, json.dumps(router_a_models)
            ),
            "http://router-b.example:8080/v1/models": _make_response(
                200, json.dumps(router_b_models)
            ),
        }
    )
    checker._session = session

    await checker._check_all()

    assert session.get.call_count == 2
    called_urls = [call.args[0] for call in session.get.call_args_list]
    assert called_urls == [
        "http://router-a.example:8080/v1/models",
        "http://router-b.example:8080/v1/models",
    ]
    for call in session.get.call_args_list:
        assert call.kwargs.get("allow_redirects") is False

    status = checker.status()
    assert len(status) == 3
    assert status[0]["healthy"] is True
    assert status[1]["healthy"] is False
    assert status[2]["healthy"] is True


@pytest.mark.asyncio
async def test_200_non_json_html_marks_all_targets_of_base_url_unhealthy_with_warning(
    caplog,
):
    """A 200 response that is not JSON (e.g. HTML admin page) marks all targets of that base_url unhealthy with WARNING."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200, "<html><body>Admin Login</body></html>"
            ),
            "http://router-b.example:8080/v1/models": _make_response(
                200,
                json.dumps(
                    {
                        "object": "list",
                        "data": [{"id": "provider-c/model-c:latest"}],
                    }
                ),
            ),
        }
    )
    checker._session = session

    import time

    base_now = time.monotonic() + 120.0
    with (
        pytest.MonkeyPatch.context() as mp,
        caplog.at_level(
            logging.WARNING, logger="news_creator.gateway.remote_health_checker"
        ),
    ):
        mp.setattr(
            "news_creator.gateway.remote_health_checker.time.monotonic",
            lambda: base_now,
        )
        await checker._check_all()

    status = checker.status()
    assert status[0]["healthy"] is False
    assert status[1]["healthy"] is False
    assert status[2]["healthy"] is True
    assert any(r.levelno == logging.WARNING for r in caplog.records)


@pytest.mark.asyncio
async def test_200_json_without_data_list_marks_targets_unhealthy(caplog):
    """A 200 JSON response without a 'data' list marks targets unhealthy with WARNING."""
    checker, _targets = _make_checker()
    session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200, json.dumps({"models": [{"name": "provider-a/model-a:latest"}]})
            ),
            "http://router-b.example:8080/v1/models": _make_response(
                200, json.dumps({"data": "not-a-list"})
            ),
        }
    )
    checker._session = session

    with caplog.at_level(
        logging.WARNING, logger="news_creator.gateway.remote_health_checker"
    ):
        await checker._check_all()

    status = checker.status()
    assert all(s["healthy"] is False for s in status)
    assert any(r.levelno == logging.WARNING for r in caplog.records)


@pytest.mark.asyncio
async def test_probe_marks_unhealthy_on_http_error_and_connection_error():
    """Non-200 status and aiohttp ClientError mark targets on that base_url unhealthy."""
    checker, _targets = _make_checker()
    session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(503, "busy"),
            "http://router-b.example:8080/v1/models": aiohttp.ClientConnectorError(
                connection_key=MagicMock(), os_error=OSError("Connection refused")
            ),
        }
    )
    checker._session = session

    await checker._check_all()

    status = checker.status()
    assert all(s["healthy"] is False for s in status)
    assert all(s["consecutive_failures"] == 1 for s in status)


@pytest.mark.asyncio
async def test_acquire_idle_target_filters_by_healthy_idle_and_fits_predicate():
    """acquire_idle_target returns only healthy, non-in-flight targets where fits(target) is True."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    first = checker.acquire_idle_target(lambda t: t.num_ctx >= 32768)
    assert first == targets[0]

    second = checker.acquire_idle_target(lambda t: t.num_ctx >= 32768)
    assert second == targets[2]

    third = checker.acquire_idle_target(lambda t: t.num_ctx >= 32768)
    assert third is None

    small = checker.acquire_idle_target(lambda t: t.num_ctx >= 8192)
    assert small == targets[1]


@pytest.mark.asyncio
async def test_release_target_and_mark_success_and_longest_idle_order(monkeypatch):
    """release_target frees in-flight reservation; mark_success records completion time for longest-idle selection."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    t0 = checker.acquire_idle_target(lambda _: True)
    assert t0 == targets[0]
    checker.release_target(t0)

    now = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: now
    )

    a0 = checker.acquire_idle_target(lambda _: True)
    now = 1030.0
    checker.mark_success(a0)

    a1 = checker.acquire_idle_target(lambda _: True)
    now = 1010.0
    checker.mark_success(a1)

    a2 = checker.acquire_idle_target(lambda _: True)
    now = 1020.0
    checker.mark_success(a2)

    chosen = checker.acquire_idle_target(lambda _: True)
    assert chosen == targets[1]


@pytest.mark.asyncio
async def test_mark_failure_immediately_marks_target_unhealthy_and_releases_slot():
    """mark_failure releases the target and marks only that target unhealthy."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    acquired = checker.acquire_idle_target(lambda _: True)
    assert acquired == targets[0]

    checker.mark_failure(targets[0])

    status = checker.status()
    assert status[0]["healthy"] is False
    assert status[0]["busy"] is False
    assert status[0]["in_flight_count"] == 0
    assert status[0]["consecutive_failures"] == 1
    assert status[1]["healthy"] is True


@pytest.mark.asyncio
async def test_cooldown_and_exponential_backoff_semantics(monkeypatch):
    """Unhealthy targets back off exponentially (60s -> 120s -> 240s -> 480s -> 900s cap)."""
    from news_creator.config.distributed_be_config import RemoteTarget

    single_target = [
        RemoteTarget(
            base_url="http://router-a.example:8080",
            model="provider-a/model-a:latest",
            num_ctx=32768,
        )
    ]
    hc = RemoteHealthChecker(
        targets=single_target,
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(503, "down"),
        }
    )
    hc._session = session

    clock = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await hc._check_all()
    assert session.get.call_count == 1

    clock = 1059.0
    await hc._check_all()
    assert session.get.call_count == 1

    clock = 1061.0
    await hc._check_all()
    assert session.get.call_count == 2

    clock = 1180.0
    await hc._check_all()
    assert session.get.call_count == 2

    clock = 1182.0
    await hc._check_all()
    assert session.get.call_count == 3


@pytest.mark.asyncio
async def test_start_creates_session_without_authorization_header():
    """start() initializes ClientSession with no Authorization header and runs an initial probe."""
    checker, _targets = _make_checker()
    checker._check_all = AsyncMock()

    await checker.start()
    try:
        assert checker._session is not None
        assert "Authorization" not in checker._session.headers
        checker._check_all.assert_awaited_once()
    finally:
        await checker.stop()
