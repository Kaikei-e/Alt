"""Unit test for scripts/resume_run.py's DI parity with main.py.

resume_run.py previously called build_report_graph without a hyde_generator
and constructed MemoryContentStore() without settings.content_store_max_size,
so a resumed run silently used a different pipeline configuration than the
production start_report_run path (HyDE cross-lingual query expansion
dropped; content-store cap reverted to the class default). This test
exercises the extracted `_build_pipeline_deps` helper directly — it is pure
w.r.t. settings/llm and needs no DB/checkpointer, unlike `_resume` itself.
"""

from __future__ import annotations

from unittest.mock import MagicMock

from acolyte.config.settings import Settings
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.news_creator_hyde_gw import NewsCreatorHyDEGenerator
from scripts.resume_run import _build_pipeline_deps


def test_build_pipeline_deps_wires_hyde_generator_when_enabled() -> None:
    settings = Settings(hyde_enabled=True, hyde_timeout_s=5.0)
    llm = MagicMock()

    hyde_generator, _content_store = _build_pipeline_deps(settings, llm)

    assert isinstance(hyde_generator, NewsCreatorHyDEGenerator)
    assert hyde_generator._llm is llm
    assert hyde_generator._timeout_s == 5.0


def test_build_pipeline_deps_omits_hyde_generator_when_disabled() -> None:
    settings = Settings(hyde_enabled=False)
    llm = MagicMock()

    hyde_generator, _content_store = _build_pipeline_deps(settings, llm)

    assert hyde_generator is None


def test_build_pipeline_deps_honors_content_store_max_size() -> None:
    settings = Settings(content_store_max_size=42)
    llm = MagicMock()

    _hyde_generator, content_store = _build_pipeline_deps(settings, llm)

    assert isinstance(content_store, MemoryContentStore)
    assert content_store._max_size == 42
