"""DeepHealthRunner contract: budget, cache, singleflight, cancel, metrics."""

from __future__ import annotations

import asyncio

import pytest
from prometheus_client import REGISTRY, generate_latest

from news_creator.infra.health_deep import (
    DEFAULT_BUDGET_S,
    Check,
    DeepHealthRunner,
)


async def _ok() -> None:
    return None


@pytest.mark.asyncio
async def test_per_check_timeout_fails_critical_without_hanging() -> None:
    async def hang() -> None:
        await asyncio.Event().wait()

    runner = DeepHealthRunner(
        "news-creator-timeout",
        [Check(name="ollama", critical=True, probe=hang)],
        per_check_s=0.05,
        budget_s=0.4,
        cache_ttl_s=2.0,
    )
    start = asyncio.get_running_loop().time()
    report = await runner.run()
    elapsed = asyncio.get_running_loop().time() - start
    assert elapsed < 0.25
    assert report.status == "fail"
    assert report.checks[0].reason == "timeout"
    assert report.http_status == 503


@pytest.mark.asyncio
async def test_budget_is_clamped_under_one_second() -> None:
    runner = DeepHealthRunner(
        "news-creator-budget",
        [Check(name="ollama", critical=True, probe=_ok)],
        budget_s=5.0,
    )
    assert runner.budget_s <= DEFAULT_BUDGET_S
    assert runner.budget_s < 1.0


@pytest.mark.asyncio
async def test_cache_and_singleflight_do_not_amplify_probes() -> None:
    calls = 0
    started = asyncio.Event()
    release = asyncio.Event()

    async def probe() -> None:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()

    runner = DeepHealthRunner(
        "news-creator-sf",
        [Check(name="ollama", critical=True, probe=probe)],
        cache_ttl_s=2.0,
        per_check_s=0.5,
        budget_s=0.8,
    )
    first = asyncio.create_task(runner.run())
    second = asyncio.create_task(runner.run())
    await started.wait()
    release.set()
    reports = await asyncio.gather(first, second)
    assert calls == 1
    assert all(r.status == "pass" for r in reports)

    cached = await runner.run()
    assert cached.cached is True
    assert calls == 1


@pytest.mark.asyncio
async def test_cancelled_waiter_does_not_abort_inflight() -> None:
    calls = 0
    started = asyncio.Event()
    release = asyncio.Event()

    async def probe() -> None:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()

    runner = DeepHealthRunner(
        "news-creator-cancel",
        [Check(name="ollama", critical=True, probe=probe)],
        cache_ttl_s=2.0,
        per_check_s=0.5,
        budget_s=0.8,
    )
    waiter = asyncio.create_task(runner.run())
    await started.wait()
    survivor = asyncio.create_task(runner.run())
    waiter.cancel()
    with pytest.raises(asyncio.CancelledError):
        await waiter
    release.set()
    report = await survivor
    assert report.status == "pass"
    assert calls == 1
    cached = await runner.run()
    assert cached.cached is True
    assert calls == 1


@pytest.mark.asyncio
async def test_observe_publishes_default_registry_gauges() -> None:
    runner = DeepHealthRunner(
        "news-creator-metrics",
        [Check(name="ollama", critical=True, probe=_ok)],
        cache_ttl_s=2.0,
    )
    await runner.run()
    body = generate_latest(REGISTRY).decode()
    assert 'health_deep_status{service="news-creator-metrics"}' in body
    assert 'health_deep_latency_seconds{service="news-creator-metrics"}' in body
    assert (
        " 2.0"
        in body.split('health_deep_status{service="news-creator-metrics"}')[1][:12]
    )
