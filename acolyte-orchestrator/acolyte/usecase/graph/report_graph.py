"""LangGraph report generation pipeline."""

from __future__ import annotations

from typing import TYPE_CHECKING

from langgraph.graph import END, StateGraph

from acolyte.usecase.graph.nodes.critic_node import CriticNode, should_revise
from acolyte.usecase.graph.nodes.curator_node import CuratorNode
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.nodes.planner_node import PlannerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.port.evidence_provider import EvidenceProviderPort
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort


def build_report_graph(
    llm: LLMProviderPort,
    evidence: EvidenceProviderPort,
    report_repo: ReportRepositoryPort,
) -> StateGraph:
    """Build the report generation StateGraph.

    Pipeline: planner → gatherer → curator → writer → critic → (revise|accept) → finalizer
    """
    graph = StateGraph(ReportGenerationState)

    graph.add_node("planner", PlannerNode(llm))
    graph.add_node("gatherer", GathererNode(evidence))
    graph.add_node("curator", CuratorNode(llm))
    graph.add_node("writer", WriterNode(llm))
    graph.add_node("critic", CriticNode(llm))
    graph.add_node("finalizer", FinalizerNode(report_repo))

    graph.set_entry_point("planner")
    graph.add_edge("planner", "gatherer")
    graph.add_edge("gatherer", "curator")
    graph.add_edge("curator", "writer")
    graph.add_edge("writer", "critic")
    graph.add_conditional_edges(
        "critic",
        should_revise,
        {"revise": "writer", "accept": "finalizer"},
    )
    graph.add_edge("finalizer", END)

    return graph.compile()
