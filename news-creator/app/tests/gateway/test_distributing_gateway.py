"""Tests for DistributingGateway with deferred BE slot and RemoteChatDriver."""

import asyncio
import logging
from contextlib import asynccontextmanager
from unittest.mock import AsyncMock, MagicMock

import aiohttp
import pytest
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader

from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.distributing_gateway import DistributingGateway


def _target_a():
    from news_creator.config.distributed_be_config import RemoteTarget

    return RemoteTarget(
        base_url="http://router-a.example:8080",
        model="provider-a/model-a:latest",
        num_ctx=32768,
    )


def _target_b():
    from news_creator.config.distributed_be_config import RemoteTarget

    return RemoteTarget(
        base_url="http://router-b.example:8080",
        model="provider-b/model-b:latest",
        num_ctx=32768,
    )


def _small_target():
    from news_creator.config.distributed_be_config import RemoteTarget

    return RemoteTarget(
        base_url="http://router-small.example:8080",
        model="provider-s/model-small:latest",
        num_ctx=1500,
    )


def _make_local_gateway():
    """Create a mock local OllamaGateway tracking hold_slot acquisitions."""
    gw = MagicMock()
    gw.config = MagicMock()
    gw.config.model_name = "gemma4-e4b-12k"
    gw.config.llm_keep_alive = -1
    gw.config.get_keep_alive_for_model.return_value = "24h"
    gw.config.get_llm_options.return_value = {
        "num_ctx": 8192,
        "num_predict": 1200,
        "num_batch": 1024,
        "num_gpu": 99,
        "temperature": 0.7,
        "top_p": 0.85,
        "top_k": 40,
        "repeat_penalty": 1.15,
        "num_keep": 0,
        "stop": ["<turn|>"],
    }
    gw.initialize = AsyncMock()
    gw.cleanup = AsyncMock()
    gw.list_models = AsyncMock(return_value=[{"name": "gemma4-e4b-12k"}])
    gw.chat_generate = AsyncMock(return_value={"message": {"content": "local chat"}})

    gw.hold_slot_calls = []
    gw.local_cancel_event = asyncio.Event()
    gw.local_task_id = "local-slot-task-id"

    @asynccontextmanager
    async def _hold_slot(is_high_priority=False):
        gw.hold_slot_calls.append(is_high_priority)
        yield 0.1, gw.local_cancel_event, gw.local_task_id

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
    gw.queue_status = MagicMock(
        return_value={
            "rt_queue": 0,
            "be_queue": 0,
            "total_slots": 2,
            "available_slots": 2,
            "accepting": True,
            "max_queue_depth": 10,
        }
    )
    return gw


def _make_health_checker(targets=None):
    """Create a mock RemoteHealthChecker that evaluates fits(target)."""
    candidate_pool = list(targets) if targets is not None else [_target_a()]
    in_flight = set()
    unhealthy = set()

    hc = MagicMock()

    def _acquire_idle_target(fits):
        for t in candidate_pool:
            if t not in in_flight and t not in unhealthy and fits(t):
                in_flight.add(t)
                return t
        return None

    def _release_target(t):
        in_flight.discard(t)

    def _mark_success(t):
        in_flight.discard(t)

    def _mark_failure(t):
        in_flight.discard(t)
        unhealthy.add(t)

    hc.acquire_idle_target = MagicMock(side_effect=_acquire_idle_target)
    hc.release_target = MagicMock(side_effect=_release_target)
    hc.mark_success = MagicMock(side_effect=_mark_success)
    hc.mark_failure = MagicMock(side_effect=_mark_failure)
    hc.start = AsyncMock()
    hc.stop = AsyncMock()
    hc.status = MagicMock(return_value=[])
    return hc


def _make_remote_driver():
    """Create a mock RemoteChatDriver."""
    driver = MagicMock()
    driver.initialize = AsyncMock()
    driver.cleanup = AsyncMock()
    driver.chat = AsyncMock(
        return_value=LLMGenerateResponse(
            response="remote chat response",
            model="served-model-a:latest",
            done=True,
            done_reason="stop",
            prompt_eval_count=100,
            eval_count=50,
        )
    )
    return driver


def _make_gateway(
    enabled: bool = True,
    targets=None,
):
    local = _make_local_gateway()
    hc = _make_health_checker(targets=targets)
    driver = _make_remote_driver()
    gw = DistributingGateway(
        local_gateway=local,
        health_checker=hc,
        remote_driver=driver,
        enabled=enabled,
    )
    return gw, local, hc, driver


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
async def test_remote_success_takes_no_local_slot_and_logs_served_model(
    metrics_reader, caplog
):
    """BE request inside deferred hold_slot dispatches to remote without taking a local slot and logs served_model."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[target_a])
    prompt = "<|turn>user\nSummarize this English article.<turn|>\n<|turn>model\n"

    with caplog.at_level(
        logging.INFO, logger="news_creator.gateway.distributing_gateway"
    ):
        async with gw.hold_slot(is_high_priority=False) as (
            wait_time,
            cancel_event,
            task_id,
        ):
            assert (wait_time, cancel_event, task_id) == (0.0, None, None)
            hc.acquire_idle_target.assert_not_called()
            assert local.hold_slot_calls == []

            result = await gw.generate_raw(prompt)

    assert result.response == "remote chat response"
    assert local.hold_slot_calls == []
    local.generate_raw.assert_not_awaited()
    driver.chat.assert_awaited_once()
    hc.mark_success.assert_called_once_with(target_a)

    assert any(
        getattr(r, "served_model", None) == "served-model-a:latest"
        or "served-model-a:latest" in r.getMessage()
        for r in caplog.records
    )

    dispatches = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.dispatches"),
        outcome="success",
    )
    assert len(dispatches) == 1
    assert dispatches[0].value == 1


@pytest.mark.asyncio
async def test_prompt_too_large_for_every_target_falls_back_to_local_with_local_slot():
    """When estimated_prompt_tokens + max_tokens + 256 > target.num_ctx for all targets, run locally with a local BE slot."""
    small_target = _small_target()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[small_target])
    oversized_body = "a" * 733
    prompt = f"<|turn>user\n{oversized_body}<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw(prompt, num_predict=1000)

    assert result.response == "local raw response"
    hc.acquire_idle_target.assert_called_once()
    driver.chat.assert_not_awaited()
    assert local.hold_slot_calls == [False]
    local.generate_raw.assert_awaited_once()
    assert (
        local.generate_raw.await_args.kwargs["cancel_event"] is local.local_cancel_event
    )
    assert local.generate_raw.await_args.kwargs["task_id"] == local.local_task_id


@pytest.mark.asyncio
async def test_prompt_fitting_exact_num_ctx_boundary_dispatches_to_remote():
    """When estimated_prompt_tokens + max_tokens + 256 == target.num_ctx, the target fits."""
    small_target = _small_target()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[small_target])
    exact_body = "a" * 732
    prompt = f"<|turn>user\n{exact_body}<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw(prompt, num_predict=1000)

    assert result.response == "remote chat response"
    driver.chat.assert_awaited_once()
    assert local.hold_slot_calls == []
    local.generate_raw.assert_not_awaited()


@pytest.mark.asyncio
async def test_first_target_runtime_error_falls_over_to_second_target(metrics_reader):
    """If the first target raises RuntimeError, mark_failure is called and the next fitting target is used."""
    target_a = _target_a()
    target_b = _target_b()
    gw, local, hc, driver = _make_gateway(
        enabled=True,
        targets=[target_a, target_b],
    )
    driver.chat = AsyncMock(
        side_effect=[
            RuntimeError("target A HTTP 502"),
            LLMGenerateResponse(
                response="from target B",
                model=target_b.model,
                done=True,
            ),
        ]
    )
    prompt = "<|turn>user\nSummarize article.<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw(prompt)

    assert result.response == "from target B"
    assert driver.chat.await_count == 2
    assert driver.chat.await_args_list[0].args[0] == target_a
    assert driver.chat.await_args_list[1].args[0] == target_b
    hc.mark_failure.assert_called_once_with(target_a)
    hc.mark_success.assert_called_once_with(target_b)
    assert local.hold_slot_calls == []
    local.generate_raw.assert_not_awaited()

    fallbacks = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.fallbacks"),
        to="next_remote",
        reason="error",
    )
    assert len(fallbacks) == 1
    assert fallbacks[0].value == 1


@pytest.mark.asyncio
async def test_all_targets_fail_falls_back_to_local_with_local_slot(metrics_reader):
    """When every fitting target raises RuntimeError, fall back to local while holding a local BE slot."""
    target_a = _target_a()
    target_b = _target_b()
    gw, local, hc, driver = _make_gateway(
        enabled=True,
        targets=[target_a, target_b],
    )
    driver.chat = AsyncMock(
        side_effect=[
            RuntimeError("target A failed"),
            RuntimeError("target B failed"),
        ]
    )
    prompt = "<|turn>user\nSummarize article.<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw(prompt, num_predict=900)

    assert result.response == "local raw response"
    assert driver.chat.await_count == 2
    assert hc.mark_failure.call_count == 2
    assert local.hold_slot_calls == [False]
    local.generate_raw.assert_awaited_once()
    assert (
        local.generate_raw.await_args.kwargs["cancel_event"] is local.local_cancel_event
    )
    assert local.generate_raw.await_args.kwargs["task_id"] == local.local_task_id
    assert local.generate_raw.await_args.kwargs["num_predict"] == 900

    fallbacks = _match(
        _metric_points(metrics_reader, "newscreator.distributed_be.fallbacks"),
        to="local",
        reason="exhausted",
    )
    assert len(fallbacks) == 1
    assert fallbacks[0].value == 1


@pytest.mark.asyncio
async def test_connection_refused_on_every_target_still_produces_summary_locally():
    """ADR-000943 degraded contract: connection refused on every remote target still produces summary locally."""
    from news_creator.gateway.remote_chat_driver import RemoteChatDriver

    target_a = _target_a()
    target_b = _target_b()
    local = _make_local_gateway()
    hc = _make_health_checker(targets=[target_a, target_b])
    real_driver = RemoteChatDriver(timeout_seconds=5, reasoning_effort=None)
    mock_session = MagicMock()
    mock_session.closed = False
    mock_session.post = MagicMock(
        side_effect=aiohttp.ClientConnectorError(
            connection_key=MagicMock(),
            os_error=ConnectionRefusedError("Connection refused"),
        )
    )
    real_driver._session = mock_session

    gw = DistributingGateway(
        local_gateway=local,
        health_checker=hc,
        remote_driver=real_driver,
        enabled=True,
    )
    prompt = (
        "<|turn>user\nArticle content for degraded mode test.<turn|>\n<|turn>model\n"
    )

    async with gw.hold_slot(is_high_priority=False):
        result = await gw.generate_raw(prompt)

    assert result.response == "local raw response"
    assert hc.mark_failure.call_count == 2
    assert local.hold_slot_calls == [False]
    local.generate_raw.assert_awaited_once()


@pytest.mark.asyncio
async def test_rt_passthrough_holds_real_local_slot_and_delegates_unchanged():
    """RT (is_high_priority=True) acquires a real local RT slot in hold_slot and delegates generate_raw unchanged."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[target_a])
    prompt = "<|turn>user\nRT request.<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=True) as (
        wait_time,
        cancel_event,
        task_id,
    ):
        assert cancel_event is local.local_cancel_event
        assert task_id == local.local_task_id
        result = await gw.generate_raw(
            prompt, cancel_event=cancel_event, task_id=task_id
        )

    assert result.response == "local raw response"
    assert local.hold_slot_calls == [True]
    local.generate_raw.assert_awaited_once()
    assert (
        local.generate_raw.await_args.kwargs["cancel_event"] is local.local_cancel_event
    )
    assert local.generate_raw.await_args.kwargs["task_id"] == local.local_task_id
    hc.acquire_idle_target.assert_not_called()
    driver.chat.assert_not_awaited()


@pytest.mark.asyncio
async def test_format_parameter_inside_deferred_be_slot_raises_value_error():
    """format != None inside a deferred BE slot raises ValueError; structured output is never routed remotely."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[target_a])
    prompt = "<|turn>user\nReturn JSON.<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False):
        with pytest.raises(ValueError, match="format"):
            await gw.generate_raw(prompt, format={"type": "object"})

    driver.chat.assert_not_awaited()
    local.generate_raw.assert_not_awaited()


@pytest.mark.asyncio
async def test_options_mapping_sends_only_max_tokens_temperature_and_top_p():
    """Options merged from config + caller options + num_predict send only max_tokens, temperature, top_p."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[target_a])
    prompt = "<|turn>user\nSummarize this.<turn|>\n<|turn>model\n"
    caller_options = {
        "temperature": 0.35,
        "repeat_penalty": 1.2,
        "top_k": 20,
        "num_ctx": 16384,
    }

    async with gw.hold_slot(is_high_priority=False):
        await gw.generate_raw(
            prompt,
            num_predict=950,
            keep_alive="15m",
            options=caller_options,
        )

    driver.chat.assert_awaited_once()
    call = driver.chat.await_args
    assert call.args[0] == target_a
    assert call.args[1] == [{"role": "user", "content": "Summarize this."}]
    assert call.kwargs == {
        "max_tokens": 950,
        "temperature": 0.35,
        "top_p": 0.85,
    }


@pytest.mark.asyncio
async def test_disabled_gateway_passes_through_to_local():
    """When enabled=False, hold_slot(False) and generate_raw pass through to local gateway."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=False, targets=[target_a])
    prompt = "<|turn>user\nSummarize.<turn|>\n<|turn>model\n"

    async with gw.hold_slot(is_high_priority=False) as (
        _wait_time,
        cancel_event,
        task_id,
    ):
        result = await gw.generate_raw(
            prompt, cancel_event=cancel_event, task_id=task_id
        )

    assert result.response == "local raw response"
    assert local.hold_slot_calls == [False]
    local.generate_raw.assert_awaited_once()
    hc.acquire_idle_target.assert_not_called()
    driver.chat.assert_not_awaited()


@pytest.mark.asyncio
async def test_passthrough_methods_and_queue_status():
    """generate(), chat_generate(), list_models() delegate to local; queue_status() includes remotes and flag."""
    target_a = _target_a()
    gw, local, hc, driver = _make_gateway(enabled=True, targets=[target_a])
    hc.status.return_value = [
        {"url": target_a.base_url, "model": target_a.model, "healthy": True}
    ]

    gen_res = await gw.generate("prompt", stream=False, priority="low")
    chat_res = await gw.chat_generate({"messages": []}, priority="high")
    models_res = await gw.list_models()
    q_status = gw.queue_status()

    assert gen_res.response == "local response"
    assert chat_res == {"message": {"content": "local chat"}}
    assert models_res == [{"name": "gemma4-e4b-12k"}]
    assert q_status["distributed_be_enabled"] is True
    assert q_status["remotes"] == [
        {"url": target_a.base_url, "model": target_a.model, "healthy": True}
    ]
    driver.chat.assert_not_awaited()
