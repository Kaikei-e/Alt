"""Unit tests for CriticNode's forced_accept flag.

should_revise() force-accepts once revision_count reaches MAX_REVISIONS,
even if the critic's own verdict is still "revise". CriticNode must stamp
critique["forced_accept"] = True in that situation so downstream nodes
(FinalizerNode) can mark the persisted version instead of recording it
identically to a genuine accept.
"""

from __future__ import annotations

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import MAX_REVISIONS, CriticNode
from acolyte.usecase.graph.state import ReportGenerationState

_ACCEPT_XML = "<critic><reasoning>ok</reasoning><verdict>accept</verdict></critic>"


class FakeLLM:
    def __init__(self, text: str = _ACCEPT_XML) -> None:
        self._text = text
        self.call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        return LLMResponse(text=self._text, model="fake")


async def test_forced_accept_stamped_when_max_revisions_reached_with_blocking_issue() -> None:
    """Heuristic blocking issue (empty body) at MAX_REVISIONS → forced_accept=True."""
    llm = FakeLLM()
    node = CriticNode(llm)

    state: ReportGenerationState = {
        "sections": {"analysis": ""},  # FM4 empty body → verdict stays "revise"
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "revision_count": MAX_REVISIONS,
    }

    result = await node(state)

    assert result["critique"]["verdict"] == "revise"
    assert result["critique"]["forced_accept"] is True


async def test_forced_accept_not_stamped_below_max_revisions() -> None:
    """Same blocking issue, but revision budget remains — no forced_accept."""
    llm = FakeLLM()
    node = CriticNode(llm)

    state: ReportGenerationState = {
        "sections": {"analysis": ""},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "revision_count": 0,
    }

    result = await node(state)

    assert result["critique"]["verdict"] == "revise"
    assert "forced_accept" not in result["critique"]


async def test_forced_accept_not_stamped_when_verdict_already_accept() -> None:
    """Hitting MAX_REVISIONS on a genuinely accepted report is not a forced accept."""
    llm = FakeLLM()
    node = CriticNode(llm)

    state: ReportGenerationState = {
        "sections": {"analysis": "Solid, sufficiently long analysis content about AI trends." * 3},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "revision_count": MAX_REVISIONS,
    }

    result = await node(state)

    assert result["critique"]["verdict"] == "accept"
    assert "forced_accept" not in result["critique"]
