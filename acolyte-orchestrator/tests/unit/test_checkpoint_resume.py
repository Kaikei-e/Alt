"""Integration test for checkpoint save/restore cycle (Issue 6).

Verifies that LangGraph checkpointer persists state at node boundaries
and can resume from the last successful super-step after failure.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TypedDict
from uuid import uuid4

import pytest
from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, StateGraph

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.fact_normalizer_node import FactNormalizerNode, should_continue_fact_normalization
from acolyte.usecase.graph.nodes.quote_selector_node import QuoteSelectorNode, should_continue_quote_selection
from acolyte.usecase.graph.state import ReportGenerationState

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
    graph = StateGraph(ResumeState)  # type: ignore[bad-specialization]
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
        await graph.ainvoke(initial_state, config=config)  # type: ignore[missing-attribute]

    assert _fail_once_tracker["node_a"] == 1
    assert _fail_once_tracker["node_b"] == 1

    # Verify checkpoint saved node_a's output via state inspection
    saved = await graph.aget_state(config)  # type: ignore[missing-attribute]
    assert saved is not None
    assert saved.values.get("evidence") == [{"id": "art-1"}]

    # Second invocation (resume): node_a should NOT re-run, node_b retries
    await graph.ainvoke(None, config=config)  # type: ignore[missing-attribute]

    # node_a was NOT called again (resumed from checkpoint)
    assert _fail_once_tracker["node_a"] == 1  # still 1, not 2
    # node_b was called again (retry after failure)
    assert _fail_once_tracker["node_b"] == 2

    # Verify final state has node_b's output (completed successfully)
    final_state = await graph.aget_state(config)  # type: ignore[missing-attribute]
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
    graph1 = StateGraph(ResumeState)  # type: ignore[bad-specialization]
    graph1.add_node("node", counting_node_1)
    graph1.set_entry_point("node")
    graph1.add_edge("node", END)
    compiled1 = graph1.compile(checkpointer=checkpointer)

    graph2 = StateGraph(ResumeState)  # type: ignore[bad-specialization]
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


class _NoopLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(text='{"claim":"normalized","confidence":0.9,"data_type":"quote"}', model="fake")


@pytest.mark.asyncio
async def test_incremental_quote_selector_resume_preserves_processed_articles() -> None:
    """Per-article quote selection should survive a crash after n processed articles."""

    class FlakyQuoteSelector(QuoteSelectorNode):
        def __init__(self) -> None:
            super().__init__(_NoopLLM(), incremental=True)
            self.failed = False
            self.calls: list[str] = []

        async def _select_quotes(self, *args, **kwargs):  # type: ignore[override]
            source_id = args[0]
            self.calls.append(source_id)
            if source_id == "art-2" and not self.failed:
                self.failed = True
                raise RuntimeError("boom")
            return await super()._select_quotes(*args, **kwargs)

    async def route_quote_selector(state: ReportGenerationState) -> str:
        return should_continue_quote_selection(state)

    graph = StateGraph(ReportGenerationState)  # type: ignore[bad-specialization]
    node = FlakyQuoteSelector()
    graph.add_node("quote_selector", node)
    graph.set_entry_point("quote_selector")
    graph.add_conditional_edges(
        "quote_selector",
        route_quote_selector,
        {"more": "quote_selector", "done": END},
    )
    compiled = graph.compile(checkpointer=MemorySaver())

    config = {"configurable": {"thread_id": f"acolyte-run:{uuid4()}"}}  # noqa: DTZ005
    initial_state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "A1"}, {"id": "art-2", "title": "A2"}]},
        "hydrated_evidence": {
            "art-1": "AI market grew 20%.",
            "art-2": "GPU demand surged 30%.",
        },
        "compressed_evidence": {},
        "outline": [{"key": "analysis", "search_queries": ["AI market GPU demand"]}],
    }

    with pytest.raises(RuntimeError, match="boom"):
        await compiled.ainvoke(initial_state, config=config)

    saved = await compiled.aget_state(config)
    assert saved.values["quote_selector_cursor"] == 1
    assert [q["source_id"] for q in saved.values["selected_quotes"]] == ["art-1"]

    await compiled.ainvoke(None, config=config)

    final_state = await compiled.aget_state(config)
    assert [q["source_id"] for q in final_state.values["selected_quotes"]] == ["art-1", "art-2"]
    assert node.calls.count("art-1") == 1


@dataclass
class _FactSettings:
    fact_num_predict: int = 128
    max_facts_total: int = 20


@pytest.mark.asyncio
async def test_incremental_fact_normalizer_resume_preserves_processed_quotes() -> None:
    """Per-quote normalization should survive a crash after n processed quotes."""

    class FlakyFactNormalizer(FactNormalizerNode):
        def __init__(self) -> None:
            super().__init__(_NoopLLM(), _FactSettings(), incremental=True)
            self.failed = False
            self.calls: list[str] = []

        async def _normalize_quote(self, quote: dict) -> dict:  # type: ignore[override]
            source_id = quote.get("source_id", "")
            self.calls.append(source_id)
            if source_id == "art-2" and not self.failed:
                self.failed = True
                raise RuntimeError("boom")
            return await super()._normalize_quote(quote)

    async def route_fact_normalizer(state: ReportGenerationState) -> str:
        return should_continue_fact_normalization(state)

    graph = StateGraph(ReportGenerationState)  # type: ignore[bad-specialization]
    node = FlakyFactNormalizer()
    graph.add_node("fact_normalizer", node)
    graph.set_entry_point("fact_normalizer")
    graph.add_conditional_edges(
        "fact_normalizer",
        route_fact_normalizer,
        {"more": "fact_normalizer", "done": END},
    )
    compiled = graph.compile(checkpointer=MemorySaver())

    config = {"configurable": {"thread_id": f"acolyte-run:{uuid4()}"}}  # noqa: DTZ005
    initial_state = {
        "selected_quotes": [
            {"text": "AI market grew 20%", "source_id": "art-1", "section_key": "analysis"},
            {"text": "GPU demand surged 30%", "source_id": "art-2", "section_key": "analysis"},
        ],
    }

    with pytest.raises(RuntimeError, match="boom"):
        await compiled.ainvoke(initial_state, config=config)

    saved = await compiled.aget_state(config)
    assert saved.values["fact_normalizer_cursor"] == 1
    assert [f["source_id"] for f in saved.values["extracted_facts"]] == ["art-1"]

    await compiled.ainvoke(None, config=config)

    final_state = await compiled.aget_state(config)
    assert [f["source_id"] for f in final_state.values["extracted_facts"]] == ["art-1", "art-2"]
    assert node.calls.count("art-1") == 1
