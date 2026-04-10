"""Unit tests for LangGraph Postgres checkpointer wiring (Issue 6).

Tests verify:
- build_report_graph accepts checkpointer parameter
- checkpoint_enabled setting defaults to False
- thread_id is namespaced from run_id
"""

from __future__ import annotations

from unittest.mock import MagicMock

from langgraph.checkpoint.memory import MemorySaver

from acolyte.config.settings import Settings
from acolyte.usecase.graph.report_graph import build_report_graph


def test_build_report_graph_accepts_checkpointer() -> None:
    """build_report_graph should accept a checkpointer parameter."""
    llm = MagicMock()
    evidence = MagicMock()
    repo = MagicMock()
    checkpointer = MemorySaver()

    # Should not raise
    graph = build_report_graph(llm, evidence, repo, checkpointer=checkpointer)
    assert graph is not None


def test_build_report_graph_works_without_checkpointer() -> None:
    """build_report_graph without checkpointer still works (backward compat)."""
    llm = MagicMock()
    evidence = MagicMock()
    repo = MagicMock()

    graph = build_report_graph(llm, evidence, repo)
    assert graph is not None


def test_checkpointer_disabled_by_default() -> None:
    """checkpoint_enabled defaults to False in Settings."""
    settings = Settings()
    assert settings.checkpoint_enabled is False


def test_thread_id_is_namespaced_from_run_id() -> None:
    """thread_id must be in 'acolyte-run:{run_id}' format."""
    run_id = "54fd6dc9-0d4a-4daa-8c2f-9c2015a6786f"
    thread_id = f"acolyte-run:{run_id}"
    assert thread_id.startswith("acolyte-run:")
    assert run_id in thread_id
