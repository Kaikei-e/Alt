"""Formal acceptance tests for Planner fixed skeleton + XML DSL query expansion.

Acceptance criteria:
  1. market_analysis で固定 3 セクション生成が保証される
  2. Planner failure 時でも deterministic fallback で同じ skeleton が得られる
  3. Planner が format kwarg を使わない (XML DSL モード)
  4. unit test に「LLM 出力なし」「LLM malformed」「fallback query generation」が追加される
"""

from __future__ import annotations

import pytest
from structlog.testing import capture_logs

from acolyte.port.llm_provider import LLMMode, LLMResponse
from acolyte.usecase.graph.nodes.planner_node import (
    PlannerNode,
    _get_skeleton,
)

EXPECTED_KEYS = ["executive_summary", "analysis", "conclusion"]


class FakeLLM:
    """Returns pre-configured response text."""

    def __init__(self, response_text: str) -> None:
        self._response_text = response_text
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        return LLMResponse(text=self._response_text, model="fake")


def _success_response(**extra_queries: list[str]) -> str:
    queries = {
        "executive_summary": ["AI chip market overview 2026"],
        "analysis": ["NVIDIA Blackwell GPU", "AMD MI400"],
        "conclusion": ["AI chip market outlook"],
    }
    queries.update(extra_queries)
    sections = "\n".join(
        f"  <section><key>{k}</key>" + "".join(f"<query>{q}</query>" for q in v) + "</section>"
        for k, v in queries.items()
    )
    return f"<plan><reasoning>Need market and trend data.</reasoning>\n{sections}\n</plan>"


# ---------------------------------------------------------------------------
# AC-1: market_analysis で固定 3 セクション生成が保証される
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_market_analysis_exact_keys_on_success() -> None:
    """market_analysis produces exactly 3 sections in fixed key order on LLM success."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})

    keys = [s["key"] for s in result["outline"]]
    assert keys == EXPECTED_KEYS


@pytest.mark.asyncio
async def test_market_analysis_exact_keys_on_failure() -> None:
    """market_analysis produces exactly 3 sections in fixed key order on LLM failure."""
    llm = FakeLLM("not valid json at all")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})

    keys = [s["key"] for s in result["outline"]]
    assert keys == EXPECTED_KEYS


@pytest.mark.asyncio
async def test_market_analysis_exact_keys_on_empty_queries() -> None:
    """market_analysis produces exactly 3 sections when LLM returns empty queries."""
    llm = FakeLLM("<plan><reasoning>nothing</reasoning></plan>")
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI chips", "report_type": "market_analysis"}})

    keys = [s["key"] for s in result["outline"]]
    assert keys == EXPECTED_KEYS


# ---------------------------------------------------------------------------
# AC-2: Planner failure 時でも deterministic fallback で同じ skeleton が得られる
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_success_and_failure_produce_identical_key_order() -> None:
    """Success and failure paths produce identical section key order and contract fields."""
    success_node = PlannerNode(FakeLLM(_success_response()))
    fail_node = PlannerNode(FakeLLM("broken"))

    brief = {"topic": "AI chips", "report_type": "market_analysis"}
    success_outline = (await success_node({"brief": brief}))["outline"]
    fail_outline = (await fail_node({"brief": brief}))["outline"]

    assert len(success_outline) == len(fail_outline)
    for s, f in zip(success_outline, fail_outline, strict=True):
        assert s["key"] == f["key"]
        assert s["section_role"] == f["section_role"]
        assert s.get("synthesis_only") == f.get("synthesis_only")
        assert s.get("min_citations") == f.get("min_citations")
        assert s.get("max_claims") == f.get("max_claims")


# ---------------------------------------------------------------------------
# Skeleton variation: trend_report
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_trend_report_has_correct_titles() -> None:
    """trend_report skeleton uses distinct titles: 'Trend Analysis' and 'Outlook'."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "AI trends", "report_type": "trend_report"}})

    titles = {s["key"]: s["title"] for s in result["outline"]}
    assert titles["analysis"] == "Trend Analysis"
    assert titles["conclusion"] == "Outlook"


# ---------------------------------------------------------------------------
# Unknown report_type → default skeleton
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_unknown_report_type_uses_default_skeleton() -> None:
    """Unknown report_type produces 3-section default skeleton."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test", "report_type": "custom_report"}})

    keys = [s["key"] for s in result["outline"]]
    assert keys == EXPECTED_KEYS


def test_default_skeleton_has_no_prebaked_queries() -> None:
    """_get_skeleton for unknown report_type returns sections without search_queries."""
    skeleton = _get_skeleton({"report_type": "nonexistent", "topic": "test"})
    for section in skeleton:
        assert "search_queries" not in section, f"Section '{section['key']}' has pre-baked search_queries in skeleton"


# ---------------------------------------------------------------------------
# Orphan LLM queries safely dropped
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_orphan_llm_queries_safely_dropped() -> None:
    """LLM queries for non-skeleton keys do not appear in the outline."""
    response = _success_response(nonexistent_section=["orphan query"])
    llm = FakeLLM(response)
    node = PlannerNode(llm)

    result = await node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    keys = [s["key"] for s in result["outline"]]
    assert "nonexistent_section" not in keys
    assert len(result["outline"]) == 3


# ---------------------------------------------------------------------------
# AC-3: JSON schema が平坦で小さい
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_planner_passes_structured_mode() -> None:
    """Planner must use mode=LLMMode.STRUCTURED for generate_validated."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    await node({"brief": {"topic": "test"}})

    assert llm.calls[0]["mode"] == LLMMode.STRUCTURED


@pytest.mark.asyncio
async def test_prompt_contains_xml_example() -> None:
    """The prompt must contain XML DSL example tags."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    await node({"brief": {"topic": "test"}})

    prompt = llm.calls[0]["prompt"]
    assert "<plan>" in prompt
    assert "<section>" in prompt
    assert "<query>" in prompt


# ---------------------------------------------------------------------------
# Prompt quality
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_prompt_contains_topic_not_full_brief() -> None:
    """Prompt should contain the topic string, not the full brief dict repr."""
    llm = FakeLLM(_success_response())
    node = PlannerNode(llm)

    await node({"brief": {"topic": "AI semiconductor supply chain", "report_type": "market_analysis"}})

    prompt = llm.calls[0]["prompt"]
    assert "AI semiconductor supply chain" in prompt
    # Full dict repr should NOT be in the prompt
    assert "report_type" not in prompt


# ---------------------------------------------------------------------------
# Fallback logging
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_fallback_emits_structured_log() -> None:
    """When LLM fails, planner must emit a 'skeleton fallback activated' log with report_type."""
    llm = FakeLLM("invalid json garbage")
    node = PlannerNode(llm)

    with capture_logs() as logs:
        await node({"brief": {"topic": "test", "report_type": "market_analysis"}})

    planner_fallback_logs = [entry for entry in logs if entry.get("event") == "Planner skeleton fallback activated"]
    assert len(planner_fallback_logs) == 1, f"Expected planner fallback log but got: {logs}"
    assert planner_fallback_logs[0]["report_type"] == "market_analysis"
