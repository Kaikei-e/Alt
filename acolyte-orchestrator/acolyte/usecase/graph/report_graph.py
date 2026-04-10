"""LangGraph report generation pipeline."""

from __future__ import annotations

from typing import TYPE_CHECKING

from langgraph.graph import END, StateGraph

from acolyte.usecase.graph.nodes.compressor_node import CompressorNode
from acolyte.usecase.graph.nodes.critic_node import CriticNode, should_revise
from acolyte.usecase.graph.nodes.curator_node import CuratorNode
from acolyte.usecase.graph.nodes.fact_normalizer_node import FactNormalizerNode
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.nodes.hydrator_node import HydratorNode
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.quote_selector_node import QuoteSelectorNode
from acolyte.usecase.graph.nodes.section_planner_node import SectionPlannerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.config.settings import Settings
    from acolyte.domain.fusion import FusionStrategy
    from acolyte.port.content_store import ContentStorePort
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort


def build_report_graph(
    llm: LLMProviderPort,
    evidence: EvidenceProviderPort,
    report_repo: ReportRepositoryPort,
    *,
    content_store: ContentStorePort | None = None,
    fusion: FusionStrategy | None = None,
    checkpointer: object | None = None,
    settings: Settings | None = None,
) -> StateGraph:
    """Build the report generation StateGraph.

    Pipeline:
      Without content_store: planner → gatherer → curator → writer → critic → finalizer
      With content_store:    planner → gatherer → curator → hydrator → compressor → extractor → section_planner → writer → critic → finalizer

    Revision loop: critic → writer (section_planner is NOT re-run on revision;
    claim_plans persist in state and writer re-uses them with revision feedback).
    """
    graph = StateGraph(ReportGenerationState)

    graph.add_node("planner", PlannerNode(llm))
    graph.add_node("gatherer", GathererNode(evidence, content_store=content_store, fusion=fusion))
    graph.add_node("curator", CuratorNode(llm))
    graph.add_node("writer", WriterNode(llm))
    graph.add_node("critic", CriticNode(llm))
    graph.add_node("finalizer", FinalizerNode(report_repo))

    graph.set_entry_point("planner")
    graph.add_edge("planner", "gatherer")
    graph.add_edge("gatherer", "curator")

    if content_store is not None:
        graph.add_node("hydrator", HydratorNode(content_store))
        graph.add_node("compressor", CompressorNode())
        graph.add_node("quote_selector", QuoteSelectorNode(llm))
        # Settings injection for FactNormalizerNode (exec3.md Issue 2)
        if settings is not None:
            fact_normalizer = FactNormalizerNode(llm, settings)
        else:
            # Fallback: use default config for backward compat (tests without settings)
            from acolyte.config.settings import Settings as _Settings

            fact_normalizer = FactNormalizerNode(llm, _Settings())
        graph.add_node("fact_normalizer", fact_normalizer)
        graph.add_node("section_planner", SectionPlannerNode(llm))
        graph.add_edge("curator", "hydrator")
        graph.add_edge("hydrator", "compressor")
        graph.add_edge("compressor", "quote_selector")
        graph.add_edge("quote_selector", "fact_normalizer")
        graph.add_edge("fact_normalizer", "section_planner")
        graph.add_edge("section_planner", "writer")
    else:
        graph.add_edge("curator", "writer")

    graph.add_edge("writer", "critic")
    graph.add_conditional_edges(
        "critic",
        should_revise,
        {"revise": "writer", "accept": "finalizer"},
    )
    graph.add_edge("finalizer", END)

    compile_kwargs: dict = {}
    if checkpointer is not None:
        compile_kwargs["checkpointer"] = checkpointer
    return graph.compile(**compile_kwargs)
