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
NO_CONTENT_FAILURE_CODE = "no_content"


async def _route_quote_selector(state: ReportGenerationState) -> str:
    return should_continue_quote_selection(state)


async def _route_fact_normalizer(state: ReportGenerationState) -> str:
    return should_continue_fact_normalization(state)


async def _route_critic(state: ReportGenerationState) -> str:
    return should_revise(state)


def _compressed_char_count(compressed_evidence: dict[str, list[dict]]) -> int:
    """Total character count across every CompressedSpan the compressor kept."""
    return sum(len(span.get("text", "")) for spans in compressed_evidence.values() for span in spans)


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
      Without content_store: planner → gatherer → curator → writer → critic → finalize_guard → finalizer
      With content_store:    planner → gatherer → curator → hydrator → compressor → quote_selector → fact_normalizer → section_planner → writer → critic → finalize_guard → finalizer

    Note: ExtractorNode (usecase/graph/nodes/extractor_node.py) implements an
    older single-pass extraction strategy and is not wired into this graph —
    quote_selector + fact_normalizer replaced it with a two-phase approach.

    Revision loop: critic → writer (section_planner is NOT re-run on revision;
    claim_plans persist in state and writer re-uses them with revision feedback).

    finalize_guard: runs on every critic "accept" route (including the
    MAX_REVISIONS forced accept) and aborts straight to END — without
    persisting a version — when the gatherer reported an error, curated
    evidence is empty, or (content_store pipeline only) curated evidence
    existed but hydrator/compressor/quote_selector/fact_normalizer produced
    zero groundable content (CLAUDE.md Rule 8: no silent fallback to a
    hollow version).
    """
    graph = StateGraph(ReportGenerationState)  # type: ignore[bad-specialization]

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

    graph.set_entry_point("planner")
    graph.add_edge("planner", "gatherer")
    graph.add_edge("gatherer", "curator")

    if content_store is not None:
        graph.add_node("hydrator", HydratorNode(content_store))
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
        graph.add_edge("hydrator", "compressor")
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
