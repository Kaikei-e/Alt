"""Unit tests for LangGraph report generation pipeline."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.port.evidence_provider import ArticleHit, RecapHit
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import should_revise
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.quote_selector_node import QuoteSelectorNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.report_graph import build_report_graph

# --- Fakes ---


class FakeLLM:
    def __init__(self, responses: dict[str, str] | None = None) -> None:
        self._responses = responses or {}
        self._call_count = 0
        self._calls: list[dict] = []

    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        temperature: float | None = None,
        top_p: float | None = None,
        top_k: int | None = None,
        format: dict | None = None,
        think: bool | None = None,
        mode: object = None,
        system_prompt: str | None = None,
    ) -> LLMResponse:
        self._call_count += 1
        self._calls.append(
            {"prompt": prompt, "num_predict": num_predict, "temperature": temperature, "format": format, "mode": mode}
        )

        # Check for keyword matches in prompt
        for key, response in self._responses.items():
            if key in prompt:
                return LLMResponse(text=response, model="fake-model")

        # Default: return valid JSON based on prompt content.
        # Order matters: specific patterns before general ones.
        if "summary planner" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Summarizing key findings for ES",
                        "claims": [
                            {
                                "claim": "Key finding: AI trends are consolidating rapidly",
                                "claim_type": "synthesis",
                                "evidence_ids": ["art-1"],
                                "supporting_quotes": ["Article body text."],
                                "numeric_facts": [],
                                "novelty_against": ["analysis", "conclusion"],
                                "must_cite": True,
                            },
                        ],
                    }
                ),
                model="fake-model",
            )
        if "synthesis planner" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Synthesizing analysis claims",
                        "claims": [
                            {
                                "claim": "Overall, AI trends point to consolidation",
                                "claim_type": "synthesis",
                                "evidence_ids": ["art-1"],
                                "supporting_quotes": ["Article body text."],
                                "numeric_facts": [],
                                "novelty_against": ["analysis"],
                                "must_cite": True,
                            },
                        ],
                    }
                ),
                model="fake-model",
            )
        if "claim planner" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Planning claims from extracted facts",
                        "claims": [
                            {
                                "claim": "Test claim from evidence",
                                "claim_type": "factual",
                                "evidence_ids": ["art-1"],
                                "supporting_quotes": ["Article body text."],
                                "numeric_facts": [],
                                "novelty_against": [],
                                "must_cite": True,
                            },
                        ],
                    }
                ),
                model="fake-model",
            )
        if "select" in prompt.lower() and "quote" in prompt.lower() and "verbatim" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Selecting key quotes.",
                        "quotes": [
                            {
                                "text": "Article body text.",
                                "source_id": "art-1",
                                "source_title": "Test Article",
                            },
                        ],
                    }
                ),
                model="fake-model",
            )
        if "normalize" in prompt.lower() and "quote" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Normalizing quote into fact.",
                        "claim": "AI trends are accelerating",
                        "confidence": 0.9,
                        "data_type": "quote",
                    }
                ),
                model="fake-model",
            )
        if "planner" in prompt.lower() or "plan" in prompt.lower() or "query" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Need ES, analysis, and conclusion",
                        "queries": {
                            "executive_summary": ["AI trends overview"],
                            "analysis": ["AI trends analysis"],
                            "conclusion": ["AI trends conclusion"],
                        },
                    }
                ),
                model="fake-model",
            )
        if "critic" in prompt.lower() or "evaluate" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Quality is acceptable",
                        "verdict": "accept",
                        "revise_sections": [],
                        "feedback": {},
                    }
                ),
                model="fake-model",
            )
        if "curator" in prompt.lower() or ("select" in prompt.lower() and "quote" not in prompt.lower()):
            return LLMResponse(
                text=json.dumps(["art-1"]),
                model="fake-model",
            )
        return LLMResponse(text="Generated section content.", model="fake-model")


class FakeEvidence:
    async def search_articles(self, query: str, *, limit: int = 20) -> list[ArticleHit]:
        return [
            ArticleHit(article_id="art-1", title="Test Article", tags=["AI"], score=0.9),
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
        self.briefs: dict[UUID, ReportBrief] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.versions: list[ReportVersion] = []
        self.section_versions: list[SectionVersion] = []

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

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
        self.section_versions.append(
            SectionVersion(
                report_id=report_id,
                section_key=section_key,
                version_no=new_v,
                body=body,
                citations=citations or [],
            )
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
        self.briefs.pop(report_id, None)
        self.sections.pop(report_id, None)


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
            "brief": {"topic": "AI"},
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
    state = {"critique": {"verdict": "revise", "revise_sections": ["summary"]}, "revision_count": 3}
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
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    assert repo.reports[report.report_id].current_version == 1
    assert len(repo.sections[report.report_id]) > 0


@pytest.mark.asyncio
async def test_full_pipeline_with_content_store_hydrates_evidence() -> None:
    """Pipeline with content_store wires hydrator, extractor, and section_planner nodes."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()
    content_store = MemoryContentStore()

    # Pre-populate content store (simulating search_indexer storing content)
    await content_store.store("art-1", "Full article body about AI trends.")

    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo, content_store=content_store)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    # Hydrator should have produced hydrated_evidence
    assert "hydrated_evidence" in result
    assert result["hydrated_evidence"].get("art-1") == "Full article body about AI trends."
    # Section planner should have produced claim_plans
    assert "claim_plans" in result


@pytest.mark.asyncio
async def test_full_pipeline_with_content_store_produces_citations() -> None:
    """Pipeline with content_store produces section_citations via claim_plans."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()
    content_store = MemoryContentStore()

    await content_store.store("art-1", "Full article body about AI trends.")

    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo, content_store=content_store)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    # section_citations should be populated
    section_citations = result.get("section_citations", {})
    assert len(section_citations) > 0
    # Each section with claims should have citations
    for key, cites in section_citations.items():
        if result.get("claim_plans", {}).get(key):
            assert len(cites) > 0, f"Section {key} has claims but no citations"
            for cite in cites:
                assert "claim_id" in cite
                assert "source_id" in cite
                assert "source_type" in cite
    # Verify citations were persisted via bump_section_version
    assert len(repo.section_versions) > 0
    sv_with_cites = [sv for sv in repo.section_versions if sv.citations]
    assert len(sv_with_cites) > 0


@pytest.mark.asyncio
async def test_full_pipeline_without_content_store_still_works() -> None:
    """Pipeline without content_store should still work (backward compat)."""
    llm = FakeLLM()
    evidence = FakeEvidence()
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

    assert result.get("final_version_no") == 1
    assert result.get("error") is None


@pytest.mark.asyncio
async def test_full_pipeline_conclusion_uses_analysis_claims() -> None:
    """With content_store, conclusion section_planner uses analysis claims, not raw facts."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()
    content_store = MemoryContentStore()

    await content_store.store("art-1", "Full article body about AI trends.")

    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo, content_store=content_store)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    claim_plans = result.get("claim_plans", {})
    # Check that conclusion claims exist and are synthesis type
    conclusion_keys = [s.get("key") for s in result.get("outline", []) if s.get("section_role") == "conclusion"]
    for key in conclusion_keys:
        claims = claim_plans.get(key, [])
        for claim in claims:
            assert claim.get("claim_type") == "synthesis", (
                f"Conclusion claim should be synthesis, got {claim.get('claim_type')}"
            )


@pytest.mark.asyncio
async def test_full_pipeline_es_uses_accepted_claims() -> None:
    """ES is generated from accepted section claims, not raw evidence. Citations must not be empty."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()
    content_store = MemoryContentStore()

    await content_store.store("art-1", "Full article body about AI trends.")

    report = await repo.create_report("Test Report", "weekly_briefing")

    graph = build_report_graph(llm, evidence, repo, content_store=content_store)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    claim_plans = result.get("claim_plans", {})
    # ES should have claim_plans
    es_keys = [s.get("key") for s in result.get("outline", []) if s.get("section_role") == "executive_summary"]
    for key in es_keys:
        es_claims = claim_plans.get(key, [])
        assert len(es_claims) > 0, f"ES section '{key}' should have claims from accepted sections"
        # ES claims should be synthesis type
        for claim in es_claims:
            assert claim.get("claim_type") == "synthesis", (
                f"ES claim should be synthesis, got {claim.get('claim_type')}"
            )
    # ES citations should not be empty
    section_citations = result.get("section_citations", {})
    for key in es_keys:
        es_citations = section_citations.get(key, [])
        assert len(es_citations) > 0, f"ES section '{key}' must have citations"


@pytest.mark.asyncio
async def test_planner_uses_num_predict_1024() -> None:
    """Planner must request num_predict=1024 (skeleton only needs query expansion)."""
    llm = FakeLLM()
    node = PlannerNode(llm)

    await node({"scope": {"topic": "AI trends"}})

    planner_calls = [c for c in llm._calls if "plan" in c["prompt"].lower() or "query" in c["prompt"].lower()]
    assert len(planner_calls) >= 1
    assert planner_calls[0]["num_predict"] == 1024


@pytest.mark.asyncio
async def test_quote_selector_uses_heuristic_primary() -> None:
    """QuoteSelector uses heuristic primary and does NOT call LLM when body matches queries."""
    llm = FakeLLM()
    node = QuoteSelectorNode(llm)

    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test Article"}]},
        "hydrated_evidence": {"art-1": "Article body text about AI trends."},
        "compressed_evidence": {},
        "outline": [{"key": "analysis", "search_queries": ["AI trends"]}],
    }

    result = await node(state)

    # Heuristic primary means no LLM calls when body matches
    assert len(llm._calls) == 0
    assert len(result["selected_quotes"]) >= 1


@pytest.mark.asyncio
async def test_full_pipeline_compresses_evidence_before_extraction() -> None:
    """CompressorNode reduces article body before quote selection."""
    llm = FakeLLM()
    evidence = FakeEvidence()
    repo = FakeReportRepo()
    content_store = MemoryContentStore()

    # Long body with repeated relevant content — should be compressed
    long_body = "Important AI trend: spending hit $100B in 2026. " * 50  # ~2450 chars
    await content_store.store("art-1", long_body)

    report = await repo.create_report("Test Report", "weekly_briefing")
    graph = build_report_graph(llm, evidence, repo, content_store=content_store)

    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": {"topic": "AI trends 2026"},
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    # Compressor should have produced compressed_evidence
    compressed = result.get("compressed_evidence", {})
    assert "art-1" in compressed
    total_chars = sum(len(s["text"]) for s in compressed["art-1"])
    assert total_chars < len(long_body), "Compression should reduce body size"
    # Pipeline should produce selected_quotes
    assert "selected_quotes" in result


def test_quote_selector_output_has_reasoning_field() -> None:
    """QuoteSelectorOutput must have 'reasoning' field (ADR-632)."""
    from acolyte.domain.quote_selection import QuoteSelectorOutput

    schema = QuoteSelectorOutput.model_json_schema()
    assert "reasoning" in schema["properties"]


def test_fact_normalizer_output_uses_tiny_schema() -> None:
    """FactNormalizerOutput should stay small and omit reasoning."""
    from acolyte.domain.quote_selection import FactNormalizerOutput

    schema = FactNormalizerOutput.model_json_schema()
    assert "reasoning" not in schema["properties"]
    assert {"claim", "confidence", "data_type"}.issubset(schema["properties"])
