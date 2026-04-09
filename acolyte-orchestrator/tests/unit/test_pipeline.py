"""Unit tests for LangGraph report generation pipeline."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import should_revise
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.report_graph import build_report_graph

# --- Fakes ---


class FakeLLM:
    def __init__(self, responses: dict[str, str] | None = None) -> None:
        self._responses = responses or {}
        self._call_count = 0

    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
        format: dict | None = None,
    ) -> LLMResponse:
        self._call_count += 1

        # Check for keyword matches in prompt
        for key, response in self._responses.items():
            if key in prompt:
                return LLMResponse(text=response, model="fake-model")

        # Default: return valid JSON for planner, accept for critic
        if "planner" in prompt.lower() or "plan" in prompt.lower() or "outline" in prompt.lower():
            return LLMResponse(
                text=json.dumps({
                    "reasoning": "Need a summary section",
                    "sections": [{"key": "summary", "title": "Summary"}],
                }),
                model="fake-model",
            )
        if "critic" in prompt.lower() or "evaluate" in prompt.lower():
            return LLMResponse(
                text=json.dumps({
                    "reasoning": "Quality is acceptable",
                    "verdict": "accept",
                    "revise_sections": [],
                    "feedback": {},
                }),
                model="fake-model",
            )
        if "curator" in prompt.lower() or "select" in prompt.lower():
            return LLMResponse(
                text=json.dumps(["art-1"]),
                model="fake-model",
            )
        return LLMResponse(text="Generated section content.", model="fake-model")


class FakeEvidence:
    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]:
        return [
            ArticleHit(article_id="art-1", title="Test Article", url="https://example.com", score=0.9),
        ]

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return "Article body text."

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return [RecapHit(recap_id="recap-1", title="Test Recap", score=0.8)]


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.versions: list[ReportVersion] = []
        self.section_versions: list[SectionVersion] = []

    async def create_report(self, title: str, report_type: str) -> Report:
        rid = uuid4()
        report = Report(
            report_id=rid,
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.reports[rid] = report
        self.sections[rid] = []
        return report

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        report = self.reports[report_id]
        new_v = expected_version + 1
        self.reports[report_id] = Report(
            report_id=report.report_id,
            title=report.title,
            report_type=report.report_type,
            current_version=new_v,
            latest_successful_run_id=report.latest_successful_run_id,
            created_at=report.created_at,
        )
        return new_v

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        sec = ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )
        self.sections.setdefault(report_id, []).append(sec)
        return sec

    async def bump_section_version(
        self, report_id: UUID, section_key: str, expected_version: int, body: str, citations: list[dict] | None = None
    ) -> int:
        new_v = expected_version + 1
        sections = self.sections.get(report_id, [])
        for i, s in enumerate(sections):
            if s.section_key == section_key:
                sections[i] = ReportSection(
                    report_id=report_id, section_key=section_key, current_version=new_v, display_order=s.display_order
                )
                break
        return new_v

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None


# --- Tests ---


@pytest.mark.asyncio
async def test_planner_node_generates_outline() -> None:
    llm = FakeLLM()
    node = PlannerNode(llm)

    result = await node({"scope": {"topic": "AI trends"}})

    assert "outline" in result
    assert len(result["outline"]) > 0
    assert "key" in result["outline"][0]


@pytest.mark.asyncio
async def test_writer_node_generates_sections() -> None:
    llm = FakeLLM()
    node = WriterNode(llm)

    result = await node(
        {
            "outline": [{"key": "summary", "title": "Summary"}],
            "curated": [{"type": "article", "id": "art-1", "title": "Test", "score": 0.9}],
            "scope": {"topic": "AI"},
            "sections": {},
        }
    )

    assert "sections" in result
    assert "summary" in result["sections"]
    assert result["revision_count"] == 1


def test_should_revise_accepts_when_no_critique() -> None:
    assert should_revise({"critique": None}) == "accept"


def test_should_revise_returns_revise_when_verdict_is_revise() -> None:
    state = {"critique": {"verdict": "revise", "revise_sections": ["summary"]}, "revision_count": 0}
    assert should_revise(state) == "revise"


def test_should_revise_accepts_at_max_revisions() -> None:
    state = {"critique": {"verdict": "revise", "revise_sections": ["summary"]}, "revision_count": 2}
    assert should_revise(state) == "accept"


@pytest.mark.asyncio
async def test_full_pipeline_produces_version() -> None:
    """Integration test: full pipeline with fakes produces a versioned report."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()

    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "scope": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    assert repo.reports[report.report_id].current_version == 1
    assert len(repo.sections[report.report_id]) > 0
