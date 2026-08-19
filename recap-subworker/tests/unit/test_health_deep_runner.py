"""DeepHealthRunner cache, singleflight, and cancellation (process-scoped use)."""

from __future__ import annotations

import asyncio

import pytest

from recap_subworker.infra.health_deep import Check, DeepHealthRunner


@pytest.mark.asyncio
async def test_cache_and_singleflight_share_one_probe() -> None:
    calls = 0
    started = asyncio.Event()
    release = asyncio.Event()

    async def probe() -> None:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()

    runner = DeepHealthRunner(
        "recap-sf",
        [Check(name="classifier_artefacts", critical=True, probe=probe)],
        cache_ttl_s=2.0,
        per_check_s=0.5,
        budget_s=0.8,
    )
    first = asyncio.create_task(runner.run())
    second = asyncio.create_task(runner.run())
    await started.wait()
    release.set()
    await asyncio.gather(first, second)
    assert calls == 1
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
        "recap-cancel",
        [Check(name="classifier_artefacts", critical=True, probe=probe)],
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
