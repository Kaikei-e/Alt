"""Integration test for checkpoint save/restore cycle (Issue 6).

Verifies that LangGraph checkpointer persists state at node boundaries
and can resume from the last successful super-step after failure.
"""

from __future__ import annotations

from typing import TypedDict
from uuid import uuid4

import pytest
from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, StateGraph

_fail_once_tracker: dict[str, int] = {"node_a": 0, "node_b": 0}


class ResumeState(TypedDict, total=False):
    report_id: str
    evidence: list[dict[str, str]]
    sections: dict[str, str]
    marker: str


def _reset_tracker() -> None:
    _fail_once_tracker["node_a"] = 0
    _fail_once_tracker["node_b"] = 0


async def node_a(state: ResumeState) -> dict:
    """Simulates a node that always succeeds."""
    _fail_once_tracker["node_a"] += 1
    return {"evidence": [{"id": "art-1"}]}


async def node_b_fail_once(state: ResumeState) -> dict:
    """Simulates a node that fails on first call, succeeds on retry."""
    _fail_once_tracker["node_b"] += 1
    if _fail_once_tracker["node_b"] == 1:
        raise RuntimeError("Simulated mid-pipeline failure")
    return {"sections": {"analysis": "Generated content."}}


def _build_test_graph(checkpointer: MemorySaver) -> object:
    """Build a minimal 2-node graph with checkpointer for resume testing."""
    graph = StateGraph(ResumeState)
    graph.add_node("node_a", node_a)
    graph.add_node("node_b", node_b_fail_once)
    graph.set_entry_point("node_a")
    graph.add_edge("node_a", "node_b")
    graph.add_edge("node_b", END)
    return graph.compile(checkpointer=checkpointer)


@pytest.mark.asyncio
async def test_pipeline_resumes_from_checkpoint_after_failure() -> None:
    """Pipeline fails at node_b, resumes from node_a's checkpoint on retry.

    This verifies the core Issue 6 guarantee: state is saved at super-step
    boundaries (between nodes), and resume skips already-completed nodes.
    """
    _reset_tracker()

    checkpointer = MemorySaver()
    graph = _build_test_graph(checkpointer)

    thread_id = f"acolyte-run:{uuid4()}"
    config = {"configurable": {"thread_id": thread_id}}
    initial_state = {"report_id": "test-report"}

    # First invocation: node_a succeeds, node_b fails
    with pytest.raises(RuntimeError, match="Simulated mid-pipeline failure"):
        await graph.ainvoke(initial_state, config=config)

    assert _fail_once_tracker["node_a"] == 1
    assert _fail_once_tracker["node_b"] == 1

    # Verify checkpoint saved node_a's output via state inspection
    saved = await graph.aget_state(config)
    assert saved is not None
    assert saved.values.get("evidence") == [{"id": "art-1"}]

    # Second invocation (resume): node_a should NOT re-run, node_b retries
    await graph.ainvoke(None, config=config)

    # node_a was NOT called again (resumed from checkpoint)
    assert _fail_once_tracker["node_a"] == 1  # still 1, not 2
    # node_b was called again (retry after failure)
    assert _fail_once_tracker["node_b"] == 2

    # Verify final state has node_b's output (completed successfully)
    final_state = await graph.aget_state(config)
    assert final_state.values.get("sections") == {"analysis": "Generated content."}


@pytest.mark.asyncio
async def test_checkpoint_thread_id_isolation() -> None:
    """Different thread_ids produce independent checkpoint histories."""
    checkpointer = MemorySaver()

    call_counts: dict[str, int] = {"run1": 0, "run2": 0}

    async def counting_node_1(state: ResumeState) -> dict:
        call_counts["run1"] += 1
        return {"marker": "run1"}

    async def counting_node_2(state: ResumeState) -> dict:
        call_counts["run2"] += 1
        return {"marker": "run2"}

    # Build two identical graphs with same checkpointer
    graph1 = StateGraph(ResumeState)
    graph1.add_node("node", counting_node_1)
    graph1.set_entry_point("node")
    graph1.add_edge("node", END)
    compiled1 = graph1.compile(checkpointer=checkpointer)

    graph2 = StateGraph(ResumeState)
    graph2.add_node("node", counting_node_2)
    graph2.set_entry_point("node")
    graph2.add_edge("node", END)
    compiled2 = graph2.compile(checkpointer=checkpointer)

    config_1 = {"configurable": {"thread_id": "acolyte-run:run-1"}}
    config_2 = {"configurable": {"thread_id": "acolyte-run:run-2"}}

    await compiled1.ainvoke({}, config=config_1)
    await compiled2.ainvoke({}, config=config_2)

    state_1 = await compiled1.aget_state(config_1)
    state_2 = await compiled2.aget_state(config_2)

    # Thread IDs are isolated — each has its own state
    assert state_1.values["marker"] == "run1"
    assert state_2.values["marker"] == "run2"
