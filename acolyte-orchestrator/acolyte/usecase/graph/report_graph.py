"""LangGraph report generation pipeline."""

from __future__ import annotations

from typing import TYPE_CHECKING

from langgraph.graph import END, StateGraph
from langgraph.graph.state import CompiledStateGraph

from acolyte.config.settings import Settings
from acolyte.usecase.graph.nodes.compressor_node import CompressorNode
from acolyte.usecase.graph.nodes.critic_node import CriticNode, should_revise
from acolyte.usecase.graph.nodes.curator_node import CuratorNode
from acolyte.usecase.graph.nodes.fact_normalizer_node import FactNormalizerNode, should_continue_fact_normalization
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.nodes.hydrator_node import HydratorNode
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.quote_selector_node import QuoteSelectorNode, should_continue_quote_selection
from acolyte.usecase.graph.nodes.section_planner_node import SectionPlannerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.domain.fusion import FusionStrategy
    from acolyte.port.content_store import ContentStorePort
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort

# Surfaced via state["failure_code"] when the finalize guard aborts a run.
# connect_service._run_pipeline_locked forwards this to JobQueuePort.fail_run
# instead of the generic "pipeline_error" code (CLAUDE.md Rule 8).
NO_EVIDENCE_FAILURE_CODE = "no_evidence"

# Curated evidence existed, but the content_store pipeline (hydrator→
# compressor→quote_selector→fact_normalizer) produced zero groundable text
# for the writer to cite — a distinct failure mode from NO_EVIDENCE_FAILURE_CODE
# (run 2a4787e8: gatherer evidence_count=50, curator total_curated=10,
# hydrator hydrated=0/10, 0 facts, an empty report persisted anyway).
# The hydrated=0/N half of that run is now caught earlier and more precisely by
# CONTENT_STORE_MISS_FAILURE_CODE; this code remains the verdict for runs that
# requested no article body at all (recap-only evidence) or hydrated some and
# still ended up with nothing citable.
NO_CONTENT_FAILURE_CODE = "no_content"

# Every curated article missed the content store, so the hydrator handed the
# rest of the pipeline nothing. Distinct from NO_CONTENT_FAILURE_CODE, which is
# the end-of-pipeline verdict: article bodies live only in the process-local
# MemoryContentStore LRU, filled as a side effect of the gatherer's search, so a
# run resumed in a fresh process (scripts/resume_run.py) hydrates 0/N and used
# to be indistinguishable from "these articles genuinely have no body".
# Deliberately absent from start_run_uc's circuit-breaker set — unlike
# no_evidence / no_content this failure is transient: a full re-run re-populates
# the store from search.
CONTENT_STORE_MISS_FAILURE_CODE = "content_store_miss"


async def _reset_failure_state(state: ReportGenerationState) -> dict:
    """Clear the previous attempt's failure at the start of every re-run.

    "error" and "failure_code" are LastValue channels and no node ever writes
    them back to None, so a checkpointed run that already failed restores them.
    connect_service's "terminal checkpoint without final_version_no, re-running
    pipeline" branch — the one scripts/resume_run.py drives — hands the initial
    state straight to ainvoke, so without this reset _finalize_guard reads the
    *previous* attempt's error and aborts with no_evidence before the re-run has
    gathered anything; that fresh no_evidence then trips start_run_uc's
    10-minute circuit breaker, blocking the ordinary retry as well.

    Only entry-point re-runs pass through here. A mid-pipeline resume
    (ainvoke(None) with pending nodes) continues from where it stopped by
    design, and its channels are the ones that attempt actually produced.
    """
    return {"error": None, "failure_code": None}


async def _route_quote_selector(state: ReportGenerationState) -> str:
    return should_continue_quote_selection(state)


async def _route_fact_normalizer(state: ReportGenerationState) -> str:
    return should_continue_fact_normalization(state)


async def _route_critic(state: ReportGenerationState) -> str:
    return should_revise(state)


def _compressed_char_count(compressed_evidence: dict[str, list[dict]]) -> int:
    """Total character count across every CompressedSpan the compressor kept."""
    return sum(len(span.get("text", "")) for spans in compressed_evidence.values() for span in spans)


def _curated_article_count(state: ReportGenerationState) -> int:
    """How many distinct article bodies HydratorNode was asked to fetch.

    Mirrors HydratorNode's own selection — curated_by_section when the curator
    produced one, else the flat curated list — because the guard below has to
    count exactly the set the hydrator requested. Counting curated items instead
    would make a recap-only run, which legitimately hydrates nothing, look like
    a total content-store miss.
    """
    curated_by_section = state.get("curated_by_section")
    if curated_by_section:
        items = [item for section_items in curated_by_section.values() for item in section_items]
    else:
        items = state.get("curated", [])
    return len({item.get("id", "") for item in items if item.get("type") == "article"})


def _content_store_missed(state: ReportGenerationState) -> bool:
    """True when the hydrator asked for article bodies and got none of them."""
    return _curated_article_count(state) > 0 and not state.get("hydrated_evidence")


async def _hydration_guard(state: ReportGenerationState) -> dict:
    """Abort at the hydration boundary when the content store held nothing.

    HydratorNode reports `hydrated=0/N` at INFO and returns an empty dict, so
    the compressor, quote_selector and fact_normalizer then spend the run's
    whole LLM budget on nothing before _finalize_guard finally calls it
    no_content. Failing here instead names the actual cause (an empty
    content store, typically a resume in a fresh process) and costs no
    inference (CLAUDE.md Rule 8: no silent fallback).
    """
    if not _content_store_missed(state):
        return {}
    return {
        "failure_code": CONTENT_STORE_MISS_FAILURE_CODE,
        "error": (
            f"Content store held none of the {_curated_article_count(state)} curated article "
            "bodies (hydrated 0) — aborting before the extraction pipeline runs on nothing"
        ),
    }


def _route_hydration_guard(state: ReportGenerationState) -> str:
    # Recomputed rather than read off failure_code: on a mid-pipeline resume the
    # checkpoint still carries the previous attempt's failure_code (see
    # _reset_failure_state), which must not divert a healthy hydration.
    return "abort" if _content_store_missed(state) else "hydrated"


async def _finalize_guard(state: ReportGenerationState) -> dict:
    """Abort before finalizer instead of persisting a hollow version.

    Runs on every path into "accept" (including the MAX_REVISIONS forced
    accept in should_revise), so a gatherer failure or empty curated evidence
    can never reach FinalizerNode.bump_version — no version is stamped for a
    run that has nothing to report. Sets failure_code so connect_service's
    fail_run path records why, instead of the generic "pipeline_error".
    """
    if state.get("error"):
        return {"failure_code": NO_EVIDENCE_FAILURE_CODE}
    if not state.get("curated"):
        return {
            "failure_code": NO_EVIDENCE_FAILURE_CODE,
            "error": "No curated evidence available — aborting before persisting a hollow version",
        }

    # "hydrated_evidence" only exists once HydratorNode has run — absent
    # means the simple pipeline (no content_store) is in play, which this
    # check doesn't apply to.
    hydrated = state.get("hydrated_evidence")
    if hydrated is not None:
        no_hydrated_articles = not hydrated
        no_compressed_chars = _compressed_char_count(state.get("compressed_evidence", {})) == 0
        no_facts = not state.get("extracted_facts")
        no_quotes = not state.get("selected_quotes")
        if no_hydrated_articles and no_compressed_chars and no_facts and no_quotes:
            return {
                "failure_code": NO_CONTENT_FAILURE_CODE,
                "error": (
                    "Content-store pipeline produced zero groundable content "
                    "(0 hydrated articles, 0 compressed chars, 0 facts, 0 quotes) "
                    "despite curated evidence — aborting before persisting a hollow version"
                ),
            }

    return {}


def _route_finalize_guard(state: ReportGenerationState) -> str:
    return "abort" if state.get("failure_code") else "finalize"


def build_report_graph(  # noqa: PLR0913 — top-level graph factory wires every node's dependency, each param independently optional
    llm: LLMProviderPort,
    evidence: EvidenceProviderPort,
    report_repo: ReportRepositoryPort,
    *,
    content_store: ContentStorePort | None = None,
    fusion: FusionStrategy | None = None,
    checkpointer: object | None = None,
    settings: Settings | None = None,
    hyde_generator: object | None = None,
) -> CompiledStateGraph:
    """Build the report generation StateGraph.

    Pipeline:
      Without content_store: reset_failure_state → planner → gatherer → curator → writer → critic → finalize_guard → finalizer
      With content_store:    reset_failure_state → planner → gatherer → curator → hydrator → hydration_guard → compressor → quote_selector → fact_normalizer → section_planner → writer → critic → finalize_guard → finalizer

    Note: ExtractorNode (usecase/graph/nodes/extractor_node.py) implements an
    older single-pass extraction strategy and is not wired into this graph —
    quote_selector + fact_normalizer replaced it with a two-phase approach.

    Revision loop: critic → writer (section_planner is NOT re-run on revision;
    claim_plans persist in state and writer re-uses them with revision feedback).

    reset_failure_state: clears the error / failure_code channels a failed
    checkpoint restores, so an operator re-run (scripts/resume_run.py) isn't
    aborted by the previous attempt's failure.

    hydration_guard: aborts straight to END when the curator selected articles
    but the content store held none of their bodies — the resume-in-a-fresh-
    process case, which otherwise burns the whole LLM budget on empty input.

    finalize_guard: runs on every critic "accept" route (including the
    MAX_REVISIONS forced accept) and aborts straight to END — without
    persisting a version — when the gatherer reported an error, curated
    evidence is empty, or (content_store pipeline only) curated evidence
    existed but hydrator/compressor/quote_selector/fact_normalizer produced
    zero groundable content (CLAUDE.md Rule 8: no silent fallback to a
    hollow version).
    """
    graph = StateGraph(ReportGenerationState)  # type: ignore[bad-specialization]

    graph.add_node("reset_failure_state", _reset_failure_state)
    graph.add_node("planner", PlannerNode(llm))
    graph.add_node(
        "gatherer",
        GathererNode(
            evidence,
            content_store=content_store,
            fusion=fusion,
            hyde_generator=hyde_generator,  # type: ignore[arg-type]
        ),
    )
    graph.add_node("curator", CuratorNode(llm, settings=settings))
    writer = WriterNode(llm, settings=settings) if settings is not None else WriterNode(llm)
    graph.add_node("writer", writer)
    graph.add_node("critic", CriticNode(llm))
    graph.add_node("finalize_guard", _finalize_guard)
    graph.add_node("finalizer", FinalizerNode(report_repo))

    graph.set_entry_point("reset_failure_state")
    graph.add_edge("reset_failure_state", "planner")
    graph.add_edge("planner", "gatherer")
    graph.add_edge("gatherer", "curator")

    if content_store is not None:
        graph.add_node("hydrator", HydratorNode(content_store))
        graph.add_node("hydration_guard", _hydration_guard)
        graph.add_node("compressor", CompressorNode())
        incremental_extract = checkpointer is not None
        graph.add_node("quote_selector", QuoteSelectorNode(llm, incremental=incremental_extract))
        # Settings injection for FactNormalizerNode (exec3.md Issue 2)
        if settings is not None:
            fact_normalizer = FactNormalizerNode(llm, settings, incremental=incremental_extract)
        else:
            # Fallback: use default config for backward compat (tests without settings)
            fact_normalizer = FactNormalizerNode(llm, Settings(), incremental=incremental_extract)
        graph.add_node("fact_normalizer", fact_normalizer)
        graph.add_node("section_planner", SectionPlannerNode(llm))
        graph.add_edge("curator", "hydrator")
        graph.add_edge("hydrator", "hydration_guard")
        graph.add_conditional_edges(
            "hydration_guard",
            _route_hydration_guard,
            {"hydrated": "compressor", "abort": END},
        )
        graph.add_edge("compressor", "quote_selector")
        if incremental_extract:
            graph.add_conditional_edges(
                "quote_selector",
                _route_quote_selector,
                {"more": "quote_selector", "done": "fact_normalizer"},
            )
            graph.add_conditional_edges(
                "fact_normalizer",
                _route_fact_normalizer,
                {"more": "fact_normalizer", "done": "section_planner"},
            )
        else:
            graph.add_edge("quote_selector", "fact_normalizer")
            graph.add_edge("fact_normalizer", "section_planner")
        graph.add_edge("section_planner", "writer")
    else:
        graph.add_edge("curator", "writer")

    graph.add_edge("writer", "critic")
    graph.add_conditional_edges(
        "critic",
        _route_critic,
        {"revise": "writer", "accept": "finalize_guard"},
    )
    graph.add_conditional_edges(
        "finalize_guard",
        _route_finalize_guard,
        {"finalize": "finalizer", "abort": END},
    )
    graph.add_edge("finalizer", END)

    compile_kwargs: dict = {}
    if checkpointer is not None:
        compile_kwargs["checkpointer"] = checkpointer
    return graph.compile(**compile_kwargs)
