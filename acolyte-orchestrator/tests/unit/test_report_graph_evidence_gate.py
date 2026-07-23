"""Unit tests for the finalize_guard gate in report_graph.py.

Regression coverage for the audited run 0c835c9f defect: a gatherer
failure (state["error"]) or empty curated evidence must never reach
FinalizerNode.bump_version — no hollow version may be persisted
(CLAUDE.md Rule 8 — no silent fallback).

Also covers run 2a4787e8: curated evidence existed (total_curated=10) but
the content-store pipeline (hydrator→compressor→quote_selector→
fact_normalizer) hydrated 0 of them, producing an empty report body that
was persisted as a real version anyway. finalize_guard must abort that
"curated but nothing groundable survived" case too.
"""

from __future__ import annotations

import json
from datetime import UTC, datetime
from typing import TYPE_CHECKING
from uuid import UUID, uuid4

import httpx
import pytest

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.report_graph import (
    NO_CONTENT_FAILURE_CODE,
    NO_EVIDENCE_FAILURE_CODE,
    _finalize_guard,
    _route_finalize_guard,
    build_report_graph,
)

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief
    from acolyte.usecase.graph.state import ReportGenerationState

# --- Direct unit tests of the guard node/router ---


@pytest.mark.asyncio
async def test_finalize_guard_sets_failure_code_when_error_present() -> None:
    result = await _finalize_guard({"error": "All evidence searches failed", "curated": [{"id": "art-1"}]})
    assert result["failure_code"] == NO_EVIDENCE_FAILURE_CODE


@pytest.mark.asyncio
async def test_finalize_guard_sets_failure_code_when_curated_empty() -> None:
    result = await _finalize_guard({"curated": []})
    assert result["failure_code"] == NO_EVIDENCE_FAILURE_CODE
    assert result["error"]


@pytest.mark.asyncio
async def test_finalize_guard_sets_failure_code_when_curated_missing() -> None:
    result = await _finalize_guard({})
    assert result["failure_code"] == NO_EVIDENCE_FAILURE_CODE


@pytest.mark.asyncio
async def test_finalize_guard_passes_through_when_evidence_present() -> None:
    result = await _finalize_guard({"curated": [{"id": "art-1"}]})
    assert result == {}


def test_route_finalize_guard_aborts_on_failure_code() -> None:
    assert _route_finalize_guard({"failure_code": NO_EVIDENCE_FAILURE_CODE}) == "abort"


def test_route_finalize_guard_finalizes_without_failure_code() -> None:
    assert _route_finalize_guard({}) == "finalize"


# --- content-store total wipeout (run 2a4787e8): curated non-empty but
# hydrator→compressor→quote_selector→fact_normalizer produced nothing ---


@pytest.mark.asyncio
async def test_finalize_guard_aborts_with_no_content_when_content_store_pipeline_is_fully_hollow() -> None:
    """Run 2a4787e8 regression: curated=10, hydrated=0/10, 0 chars, 0 facts."""
    state: ReportGenerationState = {
        "curated": [{"id": "art-1", "type": "article"}],
        "hydrated_evidence": {},
        "compressed_evidence": {},
        "extracted_facts": [],
        "selected_quotes": [],
    }
    result = await _finalize_guard(state)
    assert result["failure_code"] == NO_CONTENT_FAILURE_CODE
    assert result["error"]


@pytest.mark.asyncio
async def test_finalize_guard_passes_through_when_hydrated_evidence_non_empty() -> None:
    """Partial content (hydrator succeeded) must not abort, even if nothing
    downstream produced facts/quotes yet."""
    state: ReportGenerationState = {
        "curated": [{"id": "art-1", "type": "article"}],
        "hydrated_evidence": {"art-1": "Full body text."},
        "compressed_evidence": {},
        "extracted_facts": [],
        "selected_quotes": [],
    }
    result = await _finalize_guard(state)
    assert result == {}


@pytest.mark.asyncio
async def test_finalize_guard_passes_through_when_compressed_chars_present() -> None:
    state: ReportGenerationState = {
        "curated": [{"id": "art-1", "type": "article"}],
        "hydrated_evidence": {},
        "compressed_evidence": {"art-1": [{"text": "relevant span", "char_offset": 0, "relevance_score": 0.5}]},
        "extracted_facts": [],
        "selected_quotes": [],
    }
    result = await _finalize_guard(state)
    assert result == {}


@pytest.mark.asyncio
async def test_finalize_guard_passes_through_when_extracted_facts_present() -> None:
    state: ReportGenerationState = {
        "curated": [{"id": "art-1", "type": "article"}],
        "hydrated_evidence": {},
        "compressed_evidence": {},
        "extracted_facts": [{"claim": "X grew 10%", "source_id": "art-1"}],
        "selected_quotes": [],
    }
    result = await _finalize_guard(state)
    assert result == {}


@pytest.mark.asyncio
async def test_finalize_guard_passes_through_when_selected_quotes_present() -> None:
    state: ReportGenerationState = {
        "curated": [{"id": "art-1", "type": "article"}],
        "hydrated_evidence": {},
        "compressed_evidence": {},
        "extracted_facts": [],
        "selected_quotes": [{"text": "a quote", "source_id": "art-1"}],
    }
    result = await _finalize_guard(state)
    assert result == {}


@pytest.mark.asyncio
async def test_finalize_guard_ignores_no_content_check_for_simple_pipeline() -> None:
    """No content_store wired (no 'hydrated_evidence' key at all) — the
    simple planner→gatherer→curator→writer pipeline must be unaffected."""
    result = await _finalize_guard({"curated": [{"id": "art-1"}]})
    assert result == {}


def test_route_finalize_guard_aborts_on_no_content_failure_code() -> None:
    assert _route_finalize_guard({"failure_code": NO_CONTENT_FAILURE_CODE}) == "abort"


# --- Full-pipeline integration: gatherer failure must never reach finalizer ---


class FakeLLM:
    """Minimal LLM stub: writer gets generic content, critic always accepts,
    planner/curator fall back to their deterministic paths."""

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        if "critic" in prompt.lower() or "evaluate" in prompt.lower():
            return LLMResponse(
                text=json.dumps({"reasoning": "ok", "verdict": "accept", "revise_sections": [], "feedback": {}}),
                model="fake-model",
            )
        if "curator" in prompt.lower() or "select" in prompt.lower():
            return LLMResponse(text=json.dumps([]), model="fake-model")
        return LLMResponse(text="Generated section content.", model="fake-model")


class AllSearchesFailEvidence:
    """Every search call raises httpx.HTTPError — mirrors run 0c835c9f."""

    async def search_articles(
        self,
        query: str,
        *,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]:
        raise httpx.HTTPError("simulated upstream failure")  # noqa: TRY003 — test fake, message is the assertion fixture

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        raise httpx.HTTPError("simulated upstream failure")  # noqa: TRY003 — test fake, message is the assertion fixture


class ZeroHitEvidence:
    """Searches succeed but return nothing — no error, but no evidence either."""

    async def search_articles(
        self,
        query: str,
        *,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]:
        return []

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return []


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.section_versions: list[SectionVersion] = []
        self.bump_version_calls = 0

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

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        pass

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return None

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
        self.bump_version_calls += 1
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
        self.section_versions.append(
            SectionVersion(report_id=report_id, section_key=section_key, version_no=new_v, body=body)
        )
        return new_v

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return list(self.reports.values()), None

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        return None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return [], None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return []

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        self.reports.pop(report_id, None)


@pytest.mark.asyncio
async def test_full_pipeline_skips_finalizer_when_all_searches_fail() -> None:
    """Run 0c835c9f regression: gatherer error must abort before finalizer."""
    llm = FakeLLM()
    evidence = AllSearchesFailEvidence()
    repo = FakeReportRepo()
    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo)
    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") is None
    assert result.get("failure_code") == "no_evidence"
    assert result.get("error")
    assert repo.bump_version_calls == 0
    assert repo.reports[report.report_id].current_version == 0
    assert repo.section_versions == []


@pytest.mark.asyncio
async def test_full_pipeline_skips_finalizer_when_curated_evidence_empty() -> None:
    """Zero search hits (no exception) must also abort before finalizer."""
    llm = FakeLLM()
    evidence = ZeroHitEvidence()
    repo = FakeReportRepo()
    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo)
    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") is None
    assert result.get("failure_code") == "no_evidence"
    assert repo.bump_version_calls == 0
    assert repo.reports[report.report_id].current_version == 0


class ArticlesFoundButUnhydratableEvidence:
    """Search finds an article, but the content_store never cached its body —
    mirrors run 2a4787e8: total_curated=10, hydrated=0/10."""

    async def search_articles(
        self,
        query: str,
        *,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]:
        return [ArticleHit(article_id="art-1", title="Article One", score=1.0, language="en")]

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return []


@pytest.mark.asyncio
async def test_full_pipeline_skips_finalizer_when_content_store_pipeline_is_hollow() -> None:
    """Run 2a4787e8 regression: curated evidence exists but the content_store
    (empty MemoryContentStore — nothing was ever cached for these IDs)
    hydrates 0 articles, so compressor/quote_selector/fact_normalizer all
    produce nothing groundable. Must abort with 'no_content' before the
    finalizer persists a hollow version."""
    llm = FakeLLM()
    evidence = ArticlesFoundButUnhydratableEvidence()
    repo = FakeReportRepo()
    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo, content_store=MemoryContentStore())
    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") is None
    assert result.get("failure_code") == "no_content"
    assert result.get("error")
    assert result.get("curated")  # curator did select evidence — the guard isn't just re-checking no_evidence
    assert repo.bump_version_calls == 0
    assert repo.reports[report.report_id].current_version == 0
    assert repo.section_versions == []


@pytest.mark.asyncio
async def test_full_pipeline_finalizes_when_content_store_pipeline_hydrates_successfully() -> None:
    """Sanity check: the new no_content guard must not block a healthy
    content_store run where the article actually hydrates."""
    llm = FakeLLM()
    evidence = ArticlesFoundButUnhydratableEvidence()
    repo = FakeReportRepo()
    report = await repo.create_report("Test Report", "weekly_briefing")

    content_store = MemoryContentStore()
    await content_store.store("art-1", "Article One has a full body with plenty of relevant sentences to cite.")

    graph = build_report_graph(llm, evidence, repo, content_store=content_store)
    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("failure_code") is None
    assert result.get("final_version_no") is not None
    assert repo.bump_version_calls == 1
