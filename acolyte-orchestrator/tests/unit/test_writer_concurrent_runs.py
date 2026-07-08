"""Concurrency regression test for WriterNode — shared instance across runs.

The compiled LangGraph graph constructs each node once (see
report_graph.build_report_graph) and reuses the instance for every run
launched via connect_service.py's `asyncio.create_task`. WriterNode used to
stash the per-run SourceMap on `self._source_map`, so two runs executing
concurrently on the same instance could read each other's SourceMap
mid-generation. This test forces that interleaving deterministically and
asserts each run only ever resolves evidence via its own SourceMap.
"""

from __future__ import annotations

import asyncio

import pytest

from acolyte.domain.source_map import SourceMap
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import PlannedClaimDict, ReportGenerationState


class SteppingLLM:
    """Fake LLM whose generate() blocks until the test explicitly releases it.

    Lets the test control the exact interleaving of two concurrent
    WriterNode.__call__() invocations sharing one instance.
    """

    def __init__(self) -> None:
        self.calls: list[str] = []
        self._entered: dict[int, asyncio.Event] = {}
        self._release: dict[int, asyncio.Event] = {}
        self._next_id = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        call_id = self._next_id
        self._next_id += 1
        self.calls.append(prompt)
        self._entered.setdefault(call_id, asyncio.Event()).set()
        await self._release.setdefault(call_id, asyncio.Event()).wait()
        return LLMResponse(text=f"Paragraph {call_id}.", model="fake")

    async def wait_entered(self, call_id: int) -> None:
        await self._entered.setdefault(call_id, asyncio.Event()).wait()

    def release(self, call_id: int) -> None:
        self._release.setdefault(call_id, asyncio.Event()).set()


def _claim(claim_id: str, evidence_id: str) -> PlannedClaimDict:
    return {
        "claim_id": claim_id,
        "claim": f"Claim {claim_id}",
        "claim_type": "factual",
        "evidence_ids": [evidence_id],
        "supporting_quotes": [f"quote for {claim_id}"],
        "numeric_facts": [],
        "novelty_against": [],
        "must_cite": True,
    }


def _state(claims: list[PlannedClaimDict], source_map: SourceMap) -> ReportGenerationState:
    outline = [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}]
    return {
        "outline": outline,
        "curated": [],
        "claim_plans": {"analysis": claims},
        "brief": {"topic": "AI trends"},
        "sections": {},
        "revision_count": 0,
        "source_map": source_map.to_dict(),
    }


@pytest.mark.asyncio
async def test_writer_shared_instance_does_not_leak_source_map_across_runs() -> None:
    """Two concurrent runs on one shared WriterNode must not cross-read SourceMap."""
    llm = SteppingLLM()
    node = WriterNode(llm)

    map_a = SourceMap()
    map_a.register("src-A", "Source A", language="en")

    map_b = SourceMap()
    map_b.register("src-B", "Source B", language="ja")

    state_a = _state([_claim("analysis-1", "src-A"), _claim("analysis-2", "src-A")], map_a)
    state_b = _state([_claim("analysis-1", "src-B")], map_b)

    task_a = asyncio.create_task(node(state_a))
    await llm.wait_entered(0)  # run A's 1st paragraph is blocked mid-LLM-call

    task_b = asyncio.create_task(node(state_b))
    await llm.wait_entered(1)  # run B's (only) paragraph is blocked mid-LLM-call
    # By now, under the buggy implementation, self._source_map has been
    # overwritten with run B's map while run A is still in flight.

    llm.release(0)  # let A's 1st paragraph finish; A moves on to its 2nd claim
    await llm.wait_entered(2)  # A's 2nd paragraph reached its LLM call

    llm.release(1)
    llm.release(2)

    result_a, result_b = await asyncio.gather(task_a, task_b)

    # Run A's 2nd paragraph must still resolve its evidence via run A's OWN
    # SourceMap (map_a: "src-A" -> "S1"), never run B's map_b (which has no
    # entry for "src-A" and would leave the raw UUID unresolved).
    prompt_a2 = llm.calls[2]
    assert "S1" in prompt_a2
    assert "src-A" not in prompt_a2

    assert result_a["sections"]["analysis"]
    assert result_b["sections"]["analysis"]
