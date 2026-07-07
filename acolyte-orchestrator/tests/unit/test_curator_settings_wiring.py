"""Unit test for CuratorNode settings DI wiring inside build_report_graph.

report_graph.build_report_graph() previously constructed `CuratorNode(llm)`
without forwarding the `settings` parameter it already receives (and does
forward to WriterNode/FactNormalizerNode), so `_language_quota()` always
returned `{}` and language-quota rebalancing was silently disabled in
production. This test inspects the compiled graph's curator node instance
directly to assert settings actually reach CuratorNode.
"""

from __future__ import annotations

from typing import cast
from unittest.mock import MagicMock

from acolyte.config.settings import Settings
from acolyte.usecase.graph.nodes.curator_node import CuratorNode
from acolyte.usecase.graph.report_graph import build_report_graph


def _curator_instance(graph: object) -> CuratorNode:
    """Reach into the compiled LangGraph and return the CuratorNode instance
    bound to the "curator" node (LangGraph stores the original callable on
    `StateNodeSpec.runnable.afunc` for object-style node callables)."""
    return cast("CuratorNode", graph.builder.nodes["curator"].runnable.afunc)  # type: ignore[attr-defined]


def test_build_report_graph_wires_settings_into_curator() -> None:
    """When settings is provided, CuratorNode must receive it (not default to None)."""
    llm = MagicMock()
    evidence = MagicMock()
    repo = MagicMock()
    settings = Settings(language_quota_en=0.4)

    graph = build_report_graph(llm, evidence, repo, settings=settings)

    curator = _curator_instance(graph)
    assert curator._settings is settings


def test_curator_language_quota_nonempty_when_settings_wired() -> None:
    """With settings wired, _language_quota() must not silently collapse to {}."""
    llm = MagicMock()
    evidence = MagicMock()
    repo = MagicMock()
    settings = Settings(language_quota_en=0.4)

    graph = build_report_graph(llm, evidence, repo, settings=settings)

    curator = _curator_instance(graph)
    assert curator._language_quota("analysis", "weekly_briefing") == {"en": 0.4}


def test_build_report_graph_without_settings_still_works() -> None:
    """Backward compat: omitting settings must not raise (matches other nodes)."""
    llm = MagicMock()
    evidence = MagicMock()
    repo = MagicMock()

    graph = build_report_graph(llm, evidence, repo)

    curator = _curator_instance(graph)
    assert curator._settings is None
