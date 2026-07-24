"""Tests for the build_hyde_generator DI factory.

main.py and scripts/resume_run.py must both derive their HyDEGeneratorPort
from settings.hyde_enabled the same way — previously resume_run.py never
called this construction at all (report_graph.build_report_graph defaults
hyde_generator to None), so a resumed run silently dropped HyDE query
expansion even when HYDE_ENABLED=true in production. Factoring the
construction into one function shared by both entry points prevents that
drift structurally.
"""

from __future__ import annotations

from unittest.mock import MagicMock

from acolyte.config.settings import Settings
from acolyte.gateway.news_creator_hyde_gw import NewsCreatorHyDEGenerator, build_hyde_generator


def test_build_hyde_generator_returns_none_when_disabled() -> None:
    settings = Settings(hyde_enabled=False)
    llm = MagicMock()

    assert build_hyde_generator(llm, settings) is None


def test_build_hyde_generator_returns_configured_instance_when_enabled() -> None:
    settings = Settings(
        hyde_enabled=True,
        hyde_timeout_s=3.0,
        hyde_max_chars=123,
        hyde_num_predict=99,
    )
    llm = MagicMock()

    gen = build_hyde_generator(llm, settings)

    assert isinstance(gen, NewsCreatorHyDEGenerator)
    assert gen._llm is llm
    assert gen._timeout_s == 3.0
    assert gen._max_chars == 123
    assert gen._num_predict == 99
