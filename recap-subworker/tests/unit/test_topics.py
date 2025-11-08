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


def test_extract_topics_filters_alphanumeric_mixed_strings():
    """Test filtering of alphanumeric mixed strings like '11novemberfor', '000 avoidable'."""
    corpus = [
        "The product has 100 parameters and 11novemberfor testing",
        "We found 000 avoidable errors and 11 lowest scores in the system",
    ]

    result = topics.extract_topics(corpus, top_n=5, bm25_weighting=False)

    assert result
    first_cluster = result[0]
    # Alphanumeric mixed strings should be removed
    noise_terms = {
        "100 parameters",
        "11novemberfor",
        "000 avoidable",
        "11 lowest",
        "100",
        "11",
        "000",
    }
    for noise in noise_terms:
        assert noise not in first_cluster, f"Found alphanumeric mixed term: {noise}"

    # Informative terms should remain
    informative_terms = {"product", "parameters", "testing", "errors", "scores", "system"}
    found_informative = any(term in first_cluster for term in informative_terms)
    assert found_informative, "No informative terms found in results"


def test_extract_topics_filters_numeric_ngrams():
    """Test filtering of ngrams containing numbers like '11 lowest', '00 jumpe'."""
    corpus = [
        "The system shows 11 lowest values and 00 jumpe errors",
        "We need to fix 12 apps and 30m parameters",
    ]

    result = topics.extract_topics(corpus, top_n=5, bm25_weighting=False)

    assert result
    first_cluster = result[0]
    # Numeric ngrams should be removed
    numeric_ngrams = {"11 lowest", "00 jumpe", "12 apps", "30m parameters", "11", "00", "12", "30m"}
    for ngram in numeric_ngrams:
        assert ngram not in first_cluster, f"Found numeric ngram: {ngram}"

    # Informative terms should remain
    informative_terms = {"system", "values", "errors", "parameters"}
    found_informative = any(term in first_cluster for term in informative_terms)
    assert found_informative, "No informative terms found in results"


def test_is_informative_rejects_alphanumeric_mixed():
    """Test that _is_informative rejects alphanumeric mixed strings."""
    stopword_set = stopwords.get_stopwords()

    # Should reject alphanumeric mixed strings
    assert not topics._is_informative("11novemberfor", stopword_set)
    assert not topics._is_informative("100parameters", stopword_set)
    assert not topics._is_informative("abc123def", stopword_set)
    assert not topics._is_informative("11 lowest", stopword_set)
    assert not topics._is_informative("000 avoidable", stopword_set)

    # Should accept pure alphabetic strings
    assert topics._is_informative("technology", stopword_set)
    assert topics._is_informative("innovation", stopword_set)
    assert topics._is_informative("machine learning", stopword_set)

    # Should reject pure numeric strings
    assert not topics._is_informative("123", stopword_set)
    assert not topics._is_informative("11", stopword_set)

