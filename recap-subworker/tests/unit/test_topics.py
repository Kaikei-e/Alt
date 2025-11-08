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


def test_extract_topics_filters_real_world_noise():
    """Test filtering of noise terms found in actual Recap data."""
    # Based on actual data from response.json
    # AI genre had: ["ai", "and", "for", "her", "with"]
    # Business genre had: ["after", "as", "bedding", "been", "by"]
    corpus = [
        "AI technology and innovation for her research with new developments",
        "Business news after the announcement as reported by officials been confirmed",
    ]

    result = topics.extract_topics(corpus, top_n=5, bm25_weighting=False)

    assert result
    first_cluster = result[0]
    # Noise terms from actual data should be removed
    noise_terms = {"and", "for", "her", "with", "after", "as", "been", "by"}
    for noise in noise_terms:
        assert noise not in first_cluster, f"Found noise term: {noise}"

    # Informative terms should remain
    informative_terms = {"ai", "technology", "innovation", "research", "business", "news"}
    found_informative = any(term in first_cluster for term in informative_terms)
    assert found_informative, "No informative terms found in results"


def test_extract_topics_filters_news_domain_stopwords():
    """Test filtering of news domain-specific stopwords."""
    corpus = [
        "According to sources, officials said the announcement was made",
        "The report shows that according to officials, the situation is developing",
    ]

    result = topics.extract_topics(corpus, top_n=5, bm25_weighting=False)

    assert result
    first_cluster = result[0]
    # News domain stopwords should be removed
    news_stopwords = {"according", "sources", "officials", "said", "shows", "report"}
    for stopword in news_stopwords:
        assert stopword not in first_cluster, f"Found news domain stopword: {stopword}"


def test_get_stopwords_includes_nltk_and_news_domain():
    """Test that stopwords include NLTK and news domain words."""
    stopwords.get_stopwords.cache_clear()
    terms = stopwords.get_stopwords()

    # Should include scikit-learn defaults
    assert "the" in terms
    assert "is" in terms
    assert "and" in terms

    # Should include news domain stopwords
    assert "according" in terms
    assert "reported" in terms
    assert "demonstrate" in terms
    assert "showing" in terms

    # Should include common noise terms from actual data
    assert "her" in terms
    assert "with" in terms
    assert "after" in terms
    assert "been" in terms

