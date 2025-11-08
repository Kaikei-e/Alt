"""Tests for topic extraction helpers."""

from __future__ import annotations

import pytest

from recap_subworker.domain import stopwords
from recap_subworker.domain import topics


@pytest.fixture(autouse=True)
def _reset_stopword_cache(monkeypatch):
    monkeypatch.delenv("RECAP_SUBWORKER_STOPWORDS_EXTRA", raising=False)
    monkeypatch.delenv("RECAP_SUBWORKER_STOPWORDS_FILE", raising=False)
    stopwords.get_stopwords.cache_clear()
    yield
    stopwords.get_stopwords.cache_clear()


def test_extract_topics_filters_common_noise():
    corpus = [
        "And for her demonstration we outline the policy launch",
        "New product launch highlights AI roadmap and research.",
    ]

    result = topics.extract_topics(corpus, top_n=3, bm25_weighting=False)

    assert result
    first_cluster = result[0]
    # Noise terms should be removed.
    assert "and" not in first_cluster
    assert "demonstration" not in first_cluster
    # Informative terms remain.
    assert any(term in {"policy", "launch"} for term in first_cluster)


def test_get_stopwords_supports_env_extensions(monkeypatch, tmp_path):
    extra_file = tmp_path / "stopwords.txt"
    extra_file.write_text("widget\n# comment line\n", encoding="utf-8")

    monkeypatch.setenv("RECAP_SUBWORKER_STOPWORDS_EXTRA", "gizmo")
    monkeypatch.setenv("RECAP_SUBWORKER_STOPWORDS_FILE", str(extra_file))

    stopwords.get_stopwords.cache_clear()
    terms = stopwords.get_stopwords()
    assert "gizmo" in terms
    assert "widget" in terms

