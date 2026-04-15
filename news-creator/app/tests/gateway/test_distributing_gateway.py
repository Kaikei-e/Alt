"""Tests for DistributingGateway."""

import asyncio
import pytest
from contextlib import asynccontextmanager
from unittest.mock import AsyncMock, MagicMock

from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.distributing_gateway import DistributingGateway


# --- Helpers ---


def _make_local_gateway():
    """Create a mock OllamaGateway (local)."""
    gw = MagicMock()
    gw.config = MagicMock()
    gw.config.model_name = "gemma4-e4b-12k"
    gw.config.llm_keep_alive = -1
    gw.config.get_keep_alive_for_model.return_value = "24h"
    gw.config.get_llm_options.return_value = {
        "num_ctx": 8192,
        "num_predict": 1200,
    }
    gw.initialize = AsyncMock()
    gw.cleanup = AsyncMock()
    gw.list_models = AsyncMock(return_value=[])

    # hold_slot as async context manager yielding (wait_time, cancel_event, task_id)
    @asynccontextmanager
    async def _hold_slot(is_high_priority=False):
        yield 0.1, asyncio.Event(), "local-task-id"

    gw.hold_slot = _hold_slot

    gw.generate = AsyncMock(
        return_value=LLMGenerateResponse(
            response="local response",
            model="gemma4-e4b-12k",
            done=True,
        )
    )
    gw.generate_raw = AsyncMock(
        return_value=LLMGenerateResponse(
            response="local raw response",
            model="gemma4-e4b-12k",
            done=True,
        )
    )

    # Expose _semaphore for health handler compatibility
    gw._semaphore = MagicMock()
    gw._semaphore.queue_status.return_value = {
        "rt_queue": 0,
        "be_queue": 0,
        "total_slots": 2,
        "available_slots": 2,
        "accepting": True,
        "max_queue_depth": 10,
    }

    return gw


def _make_health_checker(healthy_url=None):
    """Create a mock RemoteHealthChecker."""
    hc = MagicMock()
    hc.acquire_idle_remote = MagicMock(return_value=healthy_url)
    hc.release_remote = MagicMock()
    hc.mark_success = MagicMock()
    hc.mark_failure = MagicMock()
    hc.get_healthy_remotes = MagicMock(return_value=[])
    hc.start = AsyncMock()
    hc.stop = AsyncMock()
    hc.status.return_value = []
    return hc


def _make_remote_driver():
    """Create a mock RemoteOllamaDriver."""
    driver = MagicMock()
    driver.initialize = AsyncMock()
    driver.cleanup = AsyncMock()
    driver.generate = AsyncMock(
        return_value=LLMGenerateResponse(
            response="remote response",
            model="gemma4-e4b-q4km",
            done=True,
        )
    )
    return driver


def _make_gateway(
    enabled=True,
    healthy_url="http://remote:11434",
    remote_model="gemma4-e4b-q4km",
    model_overrides=None,
):
    """Assemble a DistributingGateway with mocked dependencies."""
    local = _make_local_gateway()
    hc = _make_health_checker(healthy_url)
    driver = _make_remote_driver()

    gw = DistributingGateway(
        local_gateway=local,
        health_checker=hc,
        remote_driver=driver,
        enabled=enabled,
        remote_model=remote_model,
        model_overrides=model_overrides,
    )
    return gw, local, hc, driver


# --- Tests ---


@pytest.mark.asyncio
async def test_rt_always_uses_local():
    """RT (high priority) requests always use local gateway."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    async with gw.hold_slot(is_high_priority=True) as (
        wait_time,
        cancel_event,
        task_id,
    ):
        await gw.generate_raw("prompt")

    local.generate_raw.assert_awaited_once()
    driver.generate.assert_not_awaited()


@pytest.mark.asyncio
async def test_be_dispatched_to_remote_when_healthy():
    """BE request dispatched to remote when a healthy remote exists."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    async with gw.hold_slot(is_high_priority=False) as (
        wait_time,
        cancel_event,
        task_id,
    ):
        result = await gw.generate_raw("prompt")

    assert result.response == "remote response"
    driver.generate.assert_awaited_once()
    assert driver.generate.await_args.kwargs["payload"]["model"] == "gemma4-e4b-q4km"
    # Local generate_raw should NOT have been called
    local.generate_raw.assert_not_awaited()


@pytest.mark.asyncio
async def test_be_falls_back_to_local_when_all_remotes_down():
    """BE request falls back to local when no healthy remotes."""
    gw, local, hc, driver = _make_gateway(enabled=True, healthy_url=None)

    async with gw.hold_slot(is_high_priority=False) as (
        wait_time,
        cancel_event,
        task_id,
    ):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    local.generate_raw.assert_awaited_once()
    driver.generate.assert_not_awaited()


@pytest.mark.asyncio
async def test_feature_off_always_uses_local():
    """When feature is OFF, all requests use local gateway."""
    gw, local, hc, driver = _make_gateway(
        enabled=False, healthy_url="http://remote:11434"
    )

    # BE request
    async with gw.hold_slot(is_high_priority=False) as (
        wait_time,
        cancel_event,
        task_id,
    ):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    local.generate_raw.assert_awaited_once()
    driver.generate.assert_not_awaited()
    # Health checker should not be queried
    hc.acquire_idle_remote.assert_not_called()


@pytest.mark.asyncio
async def test_generate_delegates_to_local():
    """generate() always delegates to local gateway (used for streaming/hierarchical)."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    result = await gw.generate("prompt", stream=False)

    assert isinstance(result, LLMGenerateResponse)
    assert result.response == "local response"
    local.generate.assert_awaited_once()


@pytest.mark.asyncio
async def test_concurrent_be_requests_use_contextvars():
    """Concurrent BE requests maintain per-coroutine remote tracking."""
    local = _make_local_gateway()
    driver = _make_remote_driver()

    # Health checker returns different remotes on successive calls
    urls = ["http://remote-a:11434", "http://remote-b:11434"]
    call_count = 0

    def acquire_idle():
        nonlocal call_count
        url = urls[call_count % len(urls)]
        call_count += 1
        return url

    hc = _make_health_checker()
    hc.acquire_idle_remote = MagicMock(side_effect=acquire_idle)

    gw = DistributingGateway(
        local_gateway=local,
        health_checker=hc,
        remote_driver=driver,
        enabled=True,
        remote_model="gemma4-e4b-q4km",
    )

    results = []

    async def be_task():
        async with gw.hold_slot(is_high_priority=False):
            # Small delay to ensure both are in-flight
            await asyncio.sleep(0.01)
            r = await gw.generate_raw("prompt")
            results.append(r)

    await asyncio.gather(be_task(), be_task())

    assert len(results) == 2
    assert driver.generate.await_count == 2


@pytest.mark.asyncio
async def test_initialize_starts_health_checker():
    """initialize() starts the health checker."""
    gw, local, hc, driver = _make_gateway(enabled=True)

    await gw.initialize()

    local.initialize.assert_awaited_once()
    hc.start.assert_awaited_once()
    driver.initialize.assert_awaited_once()


@pytest.mark.asyncio
async def test_cleanup_stops_health_checker():
    """cleanup() stops the health checker."""
    gw, local, hc, driver = _make_gateway(enabled=True)

    await gw.cleanup()

    local.cleanup.assert_awaited_once()
    hc.stop.assert_awaited_once()
    driver.cleanup.assert_awaited_once()


@pytest.mark.asyncio
async def test_initialize_skips_remote_when_disabled():
    """When disabled, initialize skips health checker and remote driver."""
    gw, local, hc, driver = _make_gateway(enabled=False)

    await gw.initialize()

    local.initialize.assert_awaited_once()
    hc.start.assert_not_awaited()


@pytest.mark.asyncio
async def test_remote_failure_retries_next_healthy_remote_then_succeeds():
    """A failed remote should be marked unhealthy and the next healthy remote should be tried."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote-a:11434"
    )
    hc.get_healthy_remotes = MagicMock(
        side_effect=[
            ["http://remote-b:11434"],
        ]
    )
    driver.generate = AsyncMock(
        side_effect=[
            RuntimeError("remote-a failed"),
            LLMGenerateResponse(
                response="remote-b response", model="gemma4-e4b-q4km", done=True
            ),
        ]
    )

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "remote-b response"
    assert driver.generate.await_count == 2
    first_call = driver.generate.await_args_list[0].kwargs
    second_call = driver.generate.await_args_list[1].kwargs
    assert first_call["base_url"] == "http://remote-a:11434"
    assert second_call["base_url"] == "http://remote-b:11434"
    hc.mark_failure.assert_called_once_with("http://remote-a:11434")
    hc.mark_success.assert_called_once_with("http://remote-b:11434")


@pytest.mark.asyncio
async def test_remote_failure_falls_back_to_local_when_no_other_healthy_remote():
    """If no other healthy remote remains, generate_raw should fall back to local."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote-a:11434"
    )
    hc.get_healthy_remotes = MagicMock(return_value=[])
    driver.generate = AsyncMock(side_effect=RuntimeError("remote-a failed"))

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    hc.mark_failure.assert_called_once_with("http://remote-a:11434")
    local.generate_raw.assert_awaited_once()
    driver.initialize.assert_not_awaited()


def test_queue_status_includes_local_state():
    """queue_status() returns local semaphore state."""
    gw, local, hc, driver = _make_gateway(enabled=True)

    status = gw.queue_status()

    assert status["total_slots"] == 2
    assert "remotes" in status


def test_queue_status_includes_remote_state():
    """queue_status() includes remote health states."""
    gw, local, hc, driver = _make_gateway(enabled=True)
    hc.status.return_value = [
        {"url": "http://remote:11434", "healthy": True, "consecutive_failures": 0},
    ]

    status = gw.queue_status()

    assert len(status["remotes"]) == 1
    assert status["remotes"][0]["healthy"] is True


@pytest.mark.asyncio
async def test_remote_generate_failure_falls_back_to_local_when_no_backup_remote():
    """If the selected remote fails and no backup remote exists, local fallback is used."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )
    hc.get_healthy_remotes = MagicMock(return_value=[])
    driver.generate = AsyncMock(side_effect=RuntimeError("Remote timeout"))

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    hc.mark_failure.assert_called_once_with("http://remote:11434")
    local.generate_raw.assert_awaited_once()


@pytest.mark.asyncio
async def test_be_falls_back_to_local_when_all_remotes_busy():
    """If all healthy remotes are busy, BE should use the local fallback path."""
    gw, local, hc, driver = _make_gateway(enabled=True, healthy_url=None)

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    hc.acquire_idle_remote.assert_called_once()
    driver.generate.assert_not_awaited()
    local.generate_raw.assert_awaited_once()


@pytest.mark.asyncio
async def test_remote_success_marks_remote_available():
    """Successful remote execution should mark the remote as available again."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "remote response"
    hc.mark_success.assert_called_once_with("http://remote:11434")
    hc.release_remote.assert_not_called()


@pytest.mark.asyncio
async def test_reserved_remote_is_released_if_context_exits_without_generate():
    """A reserved remote should not remain busy if generate_raw is never called."""
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    async with gw.hold_slot(is_high_priority=False):
        pass

    hc.release_remote.assert_called_once_with("http://remote:11434")
    hc.mark_success.assert_not_called()


@pytest.mark.asyncio
async def test_model_override_uses_per_remote_model():
    """Per-remote model override is applied in the payload."""
    gw, local, hc, driver = _make_gateway(
        enabled=True,
        healthy_url="http://remote-rag:11434",
        model_overrides={"http://remote-rag:11434": "gemma4-e4b-rag"},
    )

    async with gw.hold_slot(is_high_priority=False):
        await gw.generate_raw("prompt")

    payload = driver.generate.await_args.kwargs["payload"]
    assert payload["model"] == "gemma4-e4b-rag"


@pytest.mark.asyncio
async def test_no_model_override_uses_default():
    """Remote without override uses the default remote model."""
    gw, local, hc, driver = _make_gateway(
        enabled=True,
        healthy_url="http://remote-gpu:11434",
        model_overrides={"http://other:11434": "gemma4-e4b-rag"},
    )

    async with gw.hold_slot(is_high_priority=False):
        await gw.generate_raw("prompt")

    payload = driver.generate.await_args.kwargs["payload"]
    assert payload["model"] == "gemma4-e4b-q4km"


# ============================================================================
# Contract: generate() is local-only, BE distribution is via hold_slot+generate_raw
# ============================================================================


@pytest.mark.asyncio
async def test_generate_is_local_only_contract():
    """generate() MUST always delegate to local gateway, even when distributed BE is enabled.

    This is a contract test — generate() handles streaming, model routing, and
    local semaphore/cancel semantics. BE distribution is done exclusively through
    hold_slot() + generate_raw(). Do not add remote dispatch to generate().
    """
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote:11434"
    )

    # Call generate with various options
    await gw.generate("prompt", stream=False, priority="low")
    await gw.generate("prompt", stream=True, priority="high")

    # All calls must go to local, never to remote driver
    assert local.generate.await_count == 2
    driver.generate.assert_not_awaited()
    # Health checker should never be consulted for generate()
    hc.acquire_idle_remote.assert_not_called()


# --- Metrics integration ---


from opentelemetry.sdk.metrics import MeterProvider  # noqa: E402
from opentelemetry.sdk.metrics.export import InMemoryMetricReader  # noqa: E402


@pytest.fixture
def metrics_reader():
    from news_creator.gateway import dispatch_metrics

    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    meter = provider.get_meter("news_creator.distributed_be")
    dispatch_metrics.reset_metrics_for_tests(meter)
    yield reader
    dispatch_metrics.reset_metrics_for_tests(None)
    provider.shutdown()


def _metric_points(reader, name):
    data = reader.get_metrics_data()
    if data is None:
        return []
    points = []
    for rm in data.resource_metrics:
        for sm in rm.scope_metrics:
            for metric in sm.metrics:
                if metric.name == name:
                    points.extend(metric.data.data_points)
    return points


def _match(points, **attrs):
    return [
        p for p in points if all(p.attributes.get(k) == v for k, v in attrs.items())
    ]


@pytest.mark.asyncio
async def test_successful_remote_dispatch_records_success_metric(metrics_reader):
    gw, _, _, _ = _make_gateway(enabled=True, healthy_url="http://remote-a:11434")

    async with gw.hold_slot(is_high_priority=False):
        await gw.generate_raw("prompt")

    dispatches = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.dispatches"),
        remote_url="http://remote-a:11434",
        outcome="success",
    )
    assert len(dispatches) == 1
    assert dispatches[0].value == 1


@pytest.mark.asyncio
async def test_cascade_retry_records_failure_and_fallback(metrics_reader):
    gw, _, hc, driver = _make_gateway(enabled=True, healthy_url="http://remote-a:11434")
    # First call fails, second succeeds on the next healthy remote
    hc.get_healthy_remotes.return_value = ["http://remote-b:11434"]
    driver.generate = AsyncMock(
        side_effect=[
            RuntimeError("connection refused"),
            LLMGenerateResponse(response="ok", model="gemma4-e4b-q4km", done=True),
        ]
    )

    async with gw.hold_slot(is_high_priority=False):
        await gw.generate_raw("prompt")

    dispatches = _metric_points(metrics_reader, "newscreator.distributed_be.dispatches")
    failure_a = _match(
        dispatches, remote_url="http://remote-a:11434", outcome="failure"
    )
    success_b = _match(
        dispatches, remote_url="http://remote-b:11434", outcome="success"
    )
    assert failure_a[0].value == 1
    assert success_b[0].value == 1

    fallbacks = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.fallbacks"),
        from_remote_url="http://remote-a:11434",
        to="next_remote",
        reason="error",
    )
    assert fallbacks[0].value == 1


@pytest.mark.asyncio
async def test_all_remotes_fail_falls_back_to_local(metrics_reader):
    gw, local, hc, driver = _make_gateway(
        enabled=True, healthy_url="http://remote-a:11434"
    )
    hc.get_healthy_remotes.return_value = []
    driver.generate = AsyncMock(side_effect=RuntimeError("boom"))

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw("prompt")

    assert result.response == "local raw response"
    local.generate_raw.assert_awaited_once()

    fallbacks = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.fallbacks"),
        from_remote_url="http://remote-a:11434",
        to="local",
        reason="exhausted",
    )
    assert fallbacks[0].value == 1
