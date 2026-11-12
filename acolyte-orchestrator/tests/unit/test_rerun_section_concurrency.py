"""RerunSection shares the run concurrency limit with pipeline runs.

A section rerun spends a writer LLM call, the same scarce resource the
pipeline semaphore protects. Like a queued pipeline run, a rerun that finds
every slot taken waits for one instead of starting anyway.
"""

from __future__ import annotations

import asyncio
from uuid import uuid4

import pytest
from connectrpc.errors import ConnectError

from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from tests.conftest import make_request_ctx
from tests.unit.test_rerun_section_version_consistency import _LockingRepo, _service, _SlowLLM


@pytest.mark.asyncio
async def test_rerun_waits_for_a_free_run_slot() -> None:
    report_id = uuid4()
    llm = _SlowLLM()
    service = _service(_LockingRepo(report_id), llm)
    semaphore = service._run_semaphore
    held = 0
    while not semaphore.locked():
        await semaphore.acquire()
        held += 1

    task = asyncio.create_task(
        service.rerun_section(
            acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
            ctx=make_request_ctx("RerunSection"),
        )
    )
    await asyncio.sleep(0.05)
    assert not task.done()
    assert llm.call_count == 0

    semaphore.release()
    held -= 1
    await asyncio.wait_for(task, timeout=2.0)
    assert llm.call_count == 1

    for _ in range(held):
        semaphore.release()


@pytest.mark.asyncio
async def test_rerun_releases_its_slot_when_it_fails() -> None:
    report_id = uuid4()
    service = _service(_LockingRepo(report_id), _SlowLLM())
    free_before = service._run_semaphore._value

    with pytest.raises(ConnectError):
        await service.rerun_section(
            acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="no-such-section"),
            ctx=make_request_ctx("RerunSection"),
        )

    assert service._run_semaphore._value == free_before
