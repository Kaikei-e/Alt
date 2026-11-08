"""Tests for RemoteHealthChecker rewritten around RemoteTarget and /v1/models."""

import asyncio
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
        reset_stream = getattr(self._response, "_reset_stream", None)
        if callable(reset_stream):
            reset_stream()
        return self._response

    async def __aexit__(self, exc_type, exc, tb):
        return False


def _make_response(
    status: int = 200,
    body: str | bytes = "",
    *,
    headers: dict[str, str] | None = None,
):
    raw_bytes = body.encode("utf-8") if isinstance(body, str) else bytes(body)
    resp = MagicMock()
    resp.status = status
    effective_headers = (
        dict(headers)
        if headers is not None
        else {"Content-Length": str(len(raw_bytes))}
    )
    resp.headers = effective_headers
    cl = effective_headers.get("Content-Length")
    resp.content_length = int(cl) if cl is not None else None

    async def _text(encoding: str | None = "utf-8", errors: str = "strict") -> str:
        return raw_bytes.decode(encoding or "utf-8", errors=errors)

    offset = 0

    def _reset_stream() -> None:
        nonlocal offset
        offset = 0

    async def _content_read(n: int = -1) -> bytes:
        nonlocal offset
        if n is None or n < 0:
            chunk = raw_bytes[offset:]
            offset = len(raw_bytes)
            return chunk
        chunk = raw_bytes[offset : offset + n]
        offset += len(chunk)
        return chunk

    async def _iter_chunked(n: int):
        for i in range(0, len(raw_bytes), n):
            yield raw_bytes[i : i + n]

    resp._reset_stream = _reset_stream
    resp.text = AsyncMock(side_effect=_text)
    resp.read = AsyncMock(return_value=raw_bytes)
    resp.content = MagicMock()
    resp.content.read = AsyncMock(side_effect=_content_read)
    resp.content.iter_chunked = _iter_chunked
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


@pytest.mark.asyncio
async def test_dispatch_failure_cooldown_escalates_across_4_failures_up_to_cap_and_resets_on_mark_success(
    monkeypatch,
):
    """4 consecutive mark_failure calls escalate cooldown up to the cap; probes do not clear it; mark_success resets it."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target = RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    hc = RemoteHealthChecker(
        targets=[target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
        max_cooldown_seconds=360,
    )
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200,
                json.dumps({"object": "list", "data": [{"id": target.model}]}),
            ),
        }
    )

    clock = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await hc._check_all()
    assert hc.status()[0]["healthy"] is True

    expected_cooldowns = [60.0, 120.0, 240.0, 360.0]
    observed_cooldowns: list[float] = []

    for expected_cd in expected_cooldowns:
        acquired = hc.acquire_idle_target(lambda _: True)
        assert acquired == target
        t_fail = clock
        hc.mark_failure(target)
        observed_cooldowns.append(hc._states[target]["cooldown_until"] - t_fail)

        # 1 second before cooldown_until: still in cooldown, not probed or acquirable
        clock = t_fail + expected_cd - 1.0
        await hc._check_all()
        assert hc.acquire_idle_target(lambda _: True) is None

        # After cooldown_until: probe succeeds and target becomes acquirable again
        clock = t_fail + expected_cd + 1.0
        await hc._check_all()

    assert observed_cooldowns == expected_cooldowns
    assert (
        observed_cooldowns[0]
        < observed_cooldowns[1]
        < observed_cooldowns[2]
        < observed_cooldowns[3]
    )
    assert observed_cooldowns[-1] == 360.0

    # mark_success resets the dispatch failure counter and cooldown back to base (60s)
    recovered = hc.acquire_idle_target(lambda _: True)
    assert recovered == target
    hc.mark_success(target)

    again = hc.acquire_idle_target(lambda _: True)
    assert again == target
    t_fail_after_success = clock
    hc.mark_failure(target)

    clock = t_fail_after_success + 59.0
    await hc._check_all()
    assert hc.acquire_idle_target(lambda _: True) is None

    clock = t_fail_after_success + 61.0
    await hc._check_all()
    assert hc.acquire_idle_target(lambda _: True) == target


@pytest.mark.asyncio
async def test_probe_success_during_cooldown_keeps_target_unacquirable(monkeypatch):
    """A /v1/models probe that lists the model during an active dispatch cooldown must not clear the cooldown."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target = RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    hc = RemoteHealthChecker(
        targets=[target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200,
                json.dumps({"object": "list", "data": [{"id": target.model}]}),
            ),
        }
    )

    clock = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await hc._check_all()
    acquired = hc.acquire_idle_target(lambda _: True)
    assert acquired == target

    # Dispatch failure at t=1000 sets cooldown_until = 1060
    hc.mark_failure(target)

    # Probe succeeds at t=1020 during active cooldown -> target must remain unacquirable
    clock = 1020.0
    await hc._probe_base_url(target.base_url, [target])
    assert hc.status()[0]["healthy"] is False
    assert hc.acquire_idle_target(lambda _: True) is None

    # Even if healthy were flipped, acquire_idle_target must still skip before cooldown_until
    clock = 1059.0
    hc._states[target]["healthy"] = True
    assert hc.acquire_idle_target(lambda _: True) is None
    hc._states[target]["healthy"] = False

    # After cooldown boundary (t=1060), probe succeeds and target can be acquired again
    clock = 1061.0
    await hc._check_all()
    assert hc.acquire_idle_target(lambda _: True) == target


@pytest.mark.asyncio
async def test_sibling_probe_on_same_base_url_does_not_revive_cooled_down_target(
    monkeypatch,
):
    """A /v1/models probe triggered by a sibling target on the same base_url must not revive a cooled-down target."""
    checker, targets = _make_checker()
    clock = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await _make_all_healthy(checker, targets)

    # Acquire targets[0] and mark it failed at t=1010 (cooldown 60s -> until 1070)
    clock = 1010.0
    acquired = checker.acquire_idle_target(lambda t: t == targets[0])
    assert acquired == targets[0]
    checker.mark_failure(targets[0])
    assert checker.acquire_idle_target(lambda t: t == targets[0]) is None

    # At t=1035, sibling targets[1] on the same base_url is due for its 30s health probe
    clock = 1035.0
    await checker._check_all()

    # Cooled-down targets[0] must remain unacquirable and unhealthy; sibling targets[1] stays acquirable
    assert checker.acquire_idle_target(lambda t: t == targets[0]) is None
    assert checker.status()[0]["healthy"] is False
    assert checker.acquire_idle_target(lambda t: t == targets[1]) == targets[1]


@pytest.mark.asyncio
async def test_probe_non_utf8_200_body_marks_targets_unhealthy_without_raising():
    """A 200 /v1/models body with invalid UTF-8 bytes marks targets unhealthy without leaking UnicodeDecodeError."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    checker._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200, b"\xff\xfe\xfdnot-valid-utf8"
            ),
            "http://router-b.example:8080/v1/models": _make_response(
                200, b"\x80\x81\x82"
            ),
        }
    )

    await checker._probe_base_url("http://router-a.example:8080", targets[:2])
    await checker._probe_base_url("http://router-b.example:8080", targets[2:])

    status = checker.status()
    assert all(s["healthy"] is False for s in status)


@pytest.mark.asyncio
async def test_background_loop_logs_and_continues_on_unexpected_exception(caplog):
    """The background loop catches and logs any Exception from a probe cycle and continues until CancelledError."""
    checker, _targets = _make_checker()
    checker._interval_seconds = 0.01
    second_cycle_reached = asyncio.Event()
    calls = 0

    async def _flaky_check_all():
        nonlocal calls
        calls += 1
        if calls == 1:
            raise RuntimeError("unexpected probe cycle failure")
        second_cycle_reached.set()

    checker._check_all = _flaky_check_all  # type: ignore[method-assign]

    with caplog.at_level(
        logging.ERROR, logger="news_creator.gateway.remote_health_checker"
    ):
        task = asyncio.create_task(checker._loop())
        try:
            await asyncio.wait_for(second_cycle_reached.wait(), timeout=1.0)
        finally:
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass

    assert calls >= 2


@pytest.mark.asyncio
async def test_start_leaves_targets_unhealthy_when_initial_probe_raises_unexpected_exception(
    monkeypatch,
):
    """A failing probe during start() leaves targets unhealthy instead of failing startup."""
    checker, _targets = _make_checker()

    def _get_raising(url: str, **kwargs):
        raise RuntimeError("unexpected error during initial probe")

    fake_session = MagicMock()
    fake_session.closed = False
    fake_session.headers = {}
    fake_session.get = MagicMock(side_effect=_get_raising)
    fake_session.close = AsyncMock()

    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.aiohttp.ClientSession",
        lambda *args, **kwargs: fake_session,
    )

    await checker.start()
    try:
        status = checker.status()
        assert all(s["healthy"] is False for s in status)
        assert checker._task is not None
    finally:
        await checker.stop()


@pytest.mark.parametrize("with_content_length", [True, False])
@pytest.mark.asyncio
async def test_probe_response_body_over_1_mib_marks_targets_unhealthy(
    with_content_length: bool,
):
    """A 200 /v1/models body larger than 1 MiB marks targets unhealthy."""
    checker, targets = _make_checker()
    await _make_all_healthy(checker, targets)

    one_mib = 1 * 1024 * 1024
    padding = "p" * (one_mib + 64)
    oversized_body = json.dumps(
        {
            "object": "list",
            "data": [
                {"id": targets[0].model},
                {"id": targets[1].model},
                {"id": padding},
            ],
        }
    ).encode("utf-8")
    assert len(oversized_body) > one_mib
    headers = (
        {"Content-Length": str(len(oversized_body))} if with_content_length else {}
    )

    checker._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200, oversized_body, headers=headers
            ),
        }
    )

    await checker._probe_base_url("http://router-a.example:8080", targets[:2])

    status = checker.status()
    assert status[0]["healthy"] is False
    assert status[1]["healthy"] is False


@pytest.mark.asyncio
async def test_probe_backoff_after_dispatch_cooldown_depends_only_on_consecutive_probe_failures(
    monkeypatch,
):
    """L3: After 4 dispatch failures and cooldown expiry, 1 probe failure schedules the next probe after base cooldown, not escalated."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target = RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    hc = RemoteHealthChecker(
        targets=[target],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200,
                json.dumps({"object": "list", "data": [{"id": target.model}]}),
            ),
        }
    )

    clock = 1000.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await hc._check_all()
    assert hc.status()[0]["healthy"] is True

    # 4 consecutive dispatch failures at t=1000 -> dispatch cooldown_until = 1000 + 480 = 1480
    for _ in range(4):
        hc.mark_failure(target)
    assert hc._states[target]["cooldown_until"] == 1480.0

    # Now /v1/models returns 503 when probed after the dispatch cooldown expires
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                503, '{"error": "temporarily unavailable"}'
            ),
        }
    )

    # First probe after dispatch cooldown expires at t=1481 fails (1 consecutive probe failure)
    clock = 1481.0
    await hc._check_all()
    assert hc._session.get.call_count == 1

    # Next probe must be scheduled after base cooldown (60s), not escalated (480s/900s)
    clock = 1481.0 + 59.0
    await hc._check_all()
    assert hc._session.get.call_count == 1

    clock = 1481.0 + 61.0
    await hc._check_all()
    assert hc._session.get.call_count == 2


@pytest.mark.asyncio
async def test_sibling_probe_failure_does_not_reset_last_checked_or_inflate_backoff(
    monkeypatch,
):
    """A failing /v1/models probe triggered by a due sibling must not update a target that is not due."""
    from news_creator.config.distributed_be_config import RemoteTarget

    target_a = RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )
    target_b = RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-b/model-b:latest",
        num_ctx=16384,
    )
    hc = RemoteHealthChecker(
        targets=[target_a, target_b],
        interval_seconds=30,
        cooldown_seconds=60,
        timeout_seconds=10,
    )
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(
                200,
                json.dumps(
                    {
                        "object": "list",
                        "data": [{"id": target_a.model}, {"id": target_b.model}],
                    }
                ),
            ),
        }
    )

    clock = 100.0
    monkeypatch.setattr(
        "news_creator.gateway.remote_health_checker.time.monotonic", lambda: clock
    )

    await hc._check_all()
    assert hc.status()[0]["healthy"] is True
    assert hc.status()[1]["healthy"] is True

    # target_a fails dispatch at t=100 -> cooldown_until = 160
    hc.mark_failure(target_a)
    assert hc._states[target_a]["cooldown_until"] == 160.0
    assert hc._states[target_a]["probe_failures"] == 0

    # At t=155, target_b is due (155 - 100 >= 30) while target_a is in cooldown; /v1/models returns 503
    hc._session = _build_session_by_url(
        {
            "http://router-a.example:8080/v1/models": _make_response(503, "busy"),
        }
    )
    clock = 155.0
    await hc._check_all()

    assert hc._should_probe_target(target_a, 161.0) is True
    assert hc._states[target_a]["probe_failures"] == 0
    assert hc._states[target_a]["last_checked"] == 100.0
