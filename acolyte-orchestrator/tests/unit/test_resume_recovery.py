"""Regression tests for the operator resume path (scripts/resume_run.py).

Two defects that only surface when a run is resumed rather than started:

- "error"/"failure_code" are LastValue channels, so a failed attempt's values
  are restored from the checkpoint. connect_service's "terminal checkpoint
  without final_version_no, re-running pipeline" branch feeds the initial
  state back in untouched, so _finalize_guard saw the *previous* attempt's
  error and aborted with no_evidence — every resume failed identically, even
  after the upstream that caused the original failure had recovered (and the
  10-minute circuit breaker then blocked the ordinary retry too).

- Article bodies only ever live in the process-local MemoryContentStore LRU,
  filled as a side effect of the gatherer's search. A run resumed in a fresh
  process hydrates 0 of N curated articles; that used to be an INFO log plus
  a generic end-of-pipeline "no_content", indistinguishable from articles
  that genuinely carry no body.
"""

from __future__ import annotations

import json
from types import SimpleNamespace
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import httpx
import pytest
from langgraph.checkpoint.memory import MemorySaver
from structlog.testing import capture_logs

from acolyte.config.settings import Settings
from acolyte.domain.brief import ReportBrief
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.memory_job_gw import MemoryJobGateway
from acolyte.gateway.memory_report_gw import MemoryReportGateway
from acolyte.handler.connect_service import AcolyteConnectService
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.report_graph import build_report_graph
from scripts.resume_run import _build_pipeline_deps

_TOPIC = "AI trends 2026"


class FakeLLM:
    """Writer gets generic prose, critic always accepts, curator selects nothing
    (the deterministic <= max_evidence path keeps everything anyway)."""

    def __init__(self) -> None:
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        if "critic" in prompt.lower() or "evaluate" in prompt.lower():
            return LLMResponse(
                text=json.dumps({"reasoning": "ok", "verdict": "accept", "revise_sections": [], "feedback": {}}),
                model="fake-model",
            )
        if "curator" in prompt.lower() or "select" in prompt.lower():
            return LLMResponse(text=json.dumps([]), model="fake-model")
        return LLMResponse(text="Generated section content.", model="fake-model")


class RecoveringEvidence:
    """Every search fails until `recovered` flips — the upstream outage that
    produced the failed checkpoint, then healed before the operator resumed."""

    def __init__(self) -> None:
        self.recovered = False

    async def search_articles(self, query: str, **kwargs: object) -> list[ArticleHit]:
        if not self.recovered:
            raise httpx.HTTPError("simulated upstream failure")  # noqa: TRY003 — test fake, message is the assertion fixture
        return [ArticleHit(article_id="art-1", title="Article One", score=1.0, language="en")]

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        if not self.recovered:
            raise httpx.HTTPError("simulated upstream failure")  # noqa: TRY003 — test fake, message is the assertion fixture
        return []


class ArticleOnlyEvidence:
    """Search always returns one article; bodies live only in the content store."""

    async def search_articles(self, query: str, **kwargs: object) -> list[ArticleHit]:
        return [ArticleHit(article_id="art-1", title="Article One", score=1.0, language="en")]

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return []


class RecapOnlyEvidence:
    """No articles at all — nothing for the hydrator to request a body for."""

    async def search_articles(self, query: str, **kwargs: object) -> list[ArticleHit]:
        return []

    async def fetch_article_metadata(self, article_ids: list[str]) -> list:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return ""

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return [RecapHit(recap_id="recap-1", title="Recap One", score=1.0)]


def _service(graph: object, repo: MemoryReportGateway, jobs: MemoryJobGateway) -> AcolyteConnectService:
    settings = SimpleNamespace(checkpoint_enabled=True, default_model="fake-model")
    return AcolyteConnectService(settings, repo, job_queue=jobs, graph=graph)  # type: ignore[bad-argument-type]


# --- Finding 038: the checkpoint's stale error must not abort the re-run ---


@pytest.mark.asyncio
async def test_resume_after_upstream_recovery_clears_the_previous_attempts_error() -> None:
    """scripts/resume_run.py re-runs a terminal, version-less checkpoint. The
    failed attempt's error must not survive into the new attempt's
    finalize_guard, or the resume can only ever reproduce no_evidence."""
    llm = FakeLLM()
    evidence = RecoveringEvidence()
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    report = await repo.create_report("Test Report", "weekly_briefing")
    run = await jobs.create_run(report.report_id, 1)
    graph = build_report_graph(llm, evidence, repo, checkpointer=MemorySaver())
    service = _service(graph, repo, jobs)

    # Attempt 1: upstream is down — no_evidence, terminal checkpoint, no version.
    await service.resume_pipeline(str(report.report_id), str(run.run_id), {"topic": _TOPIC})
    failed = await jobs.get_run(run.run_id)
    assert failed is not None
    assert failed.run_status == "failed"
    assert failed.failure_code == "no_evidence"

    # Attempt 2: operator resumes once search-indexer is back.
    evidence.recovered = True
    await service.resume_pipeline(str(report.report_id), str(run.run_id), {"topic": _TOPIC})

    resumed = await jobs.get_run(run.run_id)
    assert resumed is not None
    assert resumed.failure_code is None
    assert resumed.run_status == "succeeded"
    assert (await repo.get_report(report.report_id)).current_version == 1  # type: ignore[optional-member-access]


# --- Finding 039: an empty content store must be its own, explicit failure ---


@pytest.mark.asyncio
async def test_resume_with_empty_content_store_fails_with_content_store_miss() -> None:
    """The gatherer's search populated the LRU in the *previous* process; the
    resumed run hydrates 0/N and must say so, not report a generic
    end-of-pipeline no_content."""
    llm = FakeLLM()
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    report = await repo.create_report("Test Report", "weekly_briefing")
    run = await jobs.create_run(report.report_id, 1)
    graph = build_report_graph(
        llm,
        ArticleOnlyEvidence(),
        repo,
        content_store=MemoryContentStore(),  # fresh process — nothing cached
        checkpointer=MemorySaver(),
    )
    service = _service(graph, repo, jobs)

    await service.resume_pipeline(str(report.report_id), str(run.run_id), {"topic": _TOPIC})

    failed = await jobs.get_run(run.run_id)
    assert failed is not None
    assert failed.run_status == "failed"
    # The literal is what an operator reads out of report_runs.failure_code,
    # and what start_run_uc's circuit breaker matches on — pin the string.
    assert failed.failure_code == "content_store_miss"
    assert (await repo.get_report(report.report_id)).current_version == 0  # type: ignore[optional-member-access]


@pytest.mark.asyncio
async def test_content_store_miss_aborts_before_the_writer_runs() -> None:
    """Fail at the hydration boundary, not 70 minutes of LLM calls later."""
    llm = FakeLLM()
    repo = MemoryReportGateway()
    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, ArticleOnlyEvidence(), repo, content_store=MemoryContentStore())
    result = await graph.ainvoke(
        {"report_id": str(report.report_id), "run_id": str(uuid4()), "brief": {"topic": _TOPIC}, "revision_count": 0}
    )

    assert result.get("curated")  # the curator did select the article
    assert not result.get("sections")  # …but the writer never ran
    assert not result.get("critique")


@pytest.mark.asyncio
async def test_recap_only_run_is_not_a_content_store_miss() -> None:
    """Zero articles requested means zero hydrated is correct, not a miss."""
    llm = FakeLLM()
    repo = MemoryReportGateway()
    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, RecapOnlyEvidence(), repo, content_store=MemoryContentStore())
    result = await graph.ainvoke(
        {"report_id": str(report.report_id), "run_id": str(uuid4()), "brief": {"topic": _TOPIC}, "revision_count": 0}
    )

    assert result.get("failure_code") != "content_store_miss"


def test_resume_run_announces_that_its_content_store_starts_empty() -> None:
    """scripts/resume_run.py builds a brand-new LRU. Article bodies only enter
    it as a side effect of the gatherer's search, so a resume past the gatherer
    hydrates 0/N — the operator must be told that up front instead of inferring
    it from the failure code much later."""
    with capture_logs() as logs:
        _hyde_generator, content_store = _build_pipeline_deps(Settings(content_store_max_size=42), MagicMock())

    assert isinstance(content_store, MemoryContentStore)
    assert len(content_store) == 0
    warnings = [e for e in logs if e.get("log_level") == "warning"]
    assert warnings, "an empty content store on resume must be logged, not silent"
    assert warnings[0].get("hydration_failure_code") == "content_store_miss"
    assert warnings[0].get("max_size") == 42


# --- Sanity: the brief round-trip resume_run.py performs stays intact ---


@pytest.mark.asyncio
async def test_resume_pipeline_accepts_a_brief_dict_from_the_repository() -> None:
    """resume_run.py resolves run -> report -> brief.to_dict() before calling
    resume_pipeline; the recovery fixes must not change that contract."""
    llm = FakeLLM()
    evidence = ArticleOnlyEvidence()
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    report = await repo.create_report("Test Report", "weekly_briefing")
    await repo.create_brief(report.report_id, ReportBrief(topic=_TOPIC, report_type="weekly_briefing"))
    run = await jobs.create_run(report.report_id, 1)

    content_store = MemoryContentStore()
    await content_store.store("art-1", "Article One has a full body with plenty of relevant sentences to cite.")
    graph = build_report_graph(llm, evidence, repo, content_store=content_store, checkpointer=MemorySaver())
    service = _service(graph, repo, jobs)

    brief = await repo.get_brief(report.report_id)
    assert brief is not None
    await service.resume_pipeline(str(report.report_id), str(run.run_id), brief.to_dict())

    completed = await jobs.get_run(run.run_id)
    assert completed is not None
    assert completed.run_status == "succeeded"
    assert completed.run_id == UUID(str(run.run_id))
