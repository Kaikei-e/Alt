"""RerunSection shares the run concurrency limit with pipeline runs.

A section rerun spends a writer LLM call, the same scarce resource the
pipeline semaphore protects. Like a queued pipeline run, a rerun that finds
every slot taken waits for one before generating, but requests that will be
rejected anyway are answered without queueing.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING
from uuid import uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.port.llm_provider import LLMResponse
from tests.conftest import make_request_ctx
from tests.unit.test_rerun_section_version_consistency import _LockingRepo, _service, _SlowLLM

if TYPE_CHECKING:
    from acolyte.handler.connect_service import AcolyteConnectService

_MAX_SLOTS = 4


class _FailingLLM(_SlowLLM):
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        msg = "model backend down"
        raise RuntimeError(msg)


class _BlockingLLM(_SlowLLM):
    def __init__(self) -> None:
        super().__init__()
        self.started = asyncio.Event()

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        self.started.set()
        await asyncio.Event().wait()
        raise AssertionError


def _request(report_id: object, section_key: str = "summary") -> acolyte_pb2.RerunSectionRequest:
    return acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key=section_key)


async def _saturate(service: AcolyteConnectService) -> list[None]:
    """Run real rerun-free slot holders until the limiter reports no free slot."""
    held = []
    for _ in range(_MAX_SLOTS):
        await service._run_semaphore.acquire()
        held.append(None)
    return held


async def _all_slots_can_be_acquired(service: AcolyteConnectService) -> bool:
    try:
        async with asyncio.timeout(1.0):
            for _ in range(_MAX_SLOTS):
                await service._run_semaphore.acquire()
    except TimeoutError:
        return False
    finally:
        for _ in range(_MAX_SLOTS):
            service._run_semaphore.release()
    return True


@pytest.mark.asyncio
async def test_rerun_waits_for_a_free_run_slot() -> None:
    report_id = uuid4()
    llm = _SlowLLM()
    service = _service(_LockingRepo(report_id), llm)
    held = await _saturate(service)

    task = asyncio.create_task(service.rerun_section(_request(report_id), ctx=make_request_ctx("RerunSection")))
    await asyncio.sleep(0.05)
    assert not task.done()
    assert llm.call_count == 0

    service._run_semaphore.release()
    await asyncio.wait_for(task, timeout=2.0)
    assert llm.call_count == 1

    for _ in held[1:]:
        service._run_semaphore.release()


@pytest.mark.asyncio
async def test_rejected_rerun_does_not_wait_for_a_slot() -> None:
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    repo.active_run = True
    llm = _SlowLLM()
    service = _service(repo, llm)
    held = await _saturate(service)

    with pytest.raises(ConnectError) as active:
        await asyncio.wait_for(
            service.rerun_section(_request(report_id), ctx=make_request_ctx("RerunSection")), timeout=1.0
        )
    assert active.value.code == Code.FAILED_PRECONDITION

    with pytest.raises(ConnectError) as unknown_section:
        await asyncio.wait_for(
            service.rerun_section(_request(report_id, "no-such-section"), ctx=make_request_ctx("RerunSection")),
            timeout=1.0,
        )
    assert unknown_section.value.code == Code.NOT_FOUND

    with pytest.raises(ConnectError) as other_owner:
        await asyncio.wait_for(
            service.rerun_section(_request(report_id), ctx=make_request_ctx("RerunSection", user_id=uuid4())),
            timeout=1.0,
        )
    assert other_owner.value.code == Code.NOT_FOUND

    assert llm.call_count == 0
    for _ in held:
        service._run_semaphore.release()


@pytest.mark.asyncio
async def test_slot_is_freed_when_the_model_call_raises() -> None:
    report_id = uuid4()
    llm = _FailingLLM()
    service = _service(_LockingRepo(report_id), llm)

    with pytest.raises(ConnectError) as excinfo:
        await service.rerun_section(_request(report_id), ctx=make_request_ctx("RerunSection"))

    assert excinfo.value.code == Code.INTERNAL
    assert llm.call_count == 1
    assert await _all_slots_can_be_acquired(service)


@pytest.mark.asyncio
async def test_slot_is_freed_when_the_rerun_is_cancelled_mid_generation() -> None:
    report_id = uuid4()
    llm = _BlockingLLM()
    service = _service(_LockingRepo(report_id), llm)

    task = asyncio.create_task(service.rerun_section(_request(report_id), ctx=make_request_ctx("RerunSection")))
    await asyncio.wait_for(llm.started.wait(), timeout=2.0)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    assert await _all_slots_can_be_acquired(service)
