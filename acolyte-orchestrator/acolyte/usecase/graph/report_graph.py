"""LangGraph report generation pipeline."""

from __future__ import annotations

from typing import TYPE_CHECKING

from langgraph.graph import END, StateGraph

from acolyte.usecase.graph.nodes.critic_node import CriticNode, should_revise
from acolyte.usecase.graph.nodes.curator_node import CuratorNode
from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.nodes.hydrator_node import HydratorNode
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
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
) -> StateGraph:
    """Build the report generation StateGraph.

    Pipeline: planner → gatherer → curator → hydrator → extractor → writer → critic → (revise|accept) → finalizer

    When content_store is provided, hydrator fetches full article bodies for curated evidence,
    and extractor extracts atomic facts from those bodies.
    """
    graph = StateGraph(ReportGenerationState)

    graph.add_node("planner", PlannerNode(llm))
    graph.add_node("gatherer", GathererNode(evidence, content_store=content_store))
    graph.add_node("curator", CuratorNode(llm))
    graph.add_node("writer", WriterNode(llm))
    graph.add_node("critic", CriticNode(llm))
    graph.add_node("finalizer", FinalizerNode(report_repo))

    graph.set_entry_point("planner")
    graph.add_edge("planner", "gatherer")
    graph.add_edge("gatherer", "curator")

    if content_store is not None:
        graph.add_node("hydrator", HydratorNode(content_store))
        graph.add_node("extractor", ExtractorNode(llm))
        graph.add_edge("curator", "hydrator")
        graph.add_edge("hydrator", "extractor")
        graph.add_edge("extractor", "writer")
    else:
        graph.add_edge("curator", "writer")

    graph.add_edge("writer", "critic")
    graph.add_conditional_edges(
        "critic",
        should_revise,
        {"revise": "writer", "accept": "finalizer"},
    )
    graph.add_edge("finalizer", END)

    return graph.compile()
