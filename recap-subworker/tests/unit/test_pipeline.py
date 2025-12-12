"""Unit tests for the evidence pipeline."""

from __future__ import annotations

import numpy as np
import pytest

from recap_subworker.domain.models import ClusterDocument, EvidenceRequest
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings
from recap_subworker.services.pipeline import EvidencePipeline, normalize_text


class FakeEmbedder:
    def __init__(self) -> None:
        self.config = type("Cfg", (), {"backend": "sentence-transformers", "model_id": "fake"})()

    def encode(self, sentences):
        size = max(1, len(sentences))
        return np.eye(size, dtype=np.float32)

    def warmup(self, samples):
        return len(list(samples))

    def close(self):
        pass


class FakeClusterer:
    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        labels = np.zeros((embeddings.shape[0],), dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(labels, probs, False, HDBSCANSettings(min_cluster_size=min_cluster_size, min_samples=min_samples))

    def optimize_clustering(self, embeddings, *, min_cluster_size_range, min_samples_range, **kwargs):
        return self.cluster(embeddings, min_cluster_size=min_cluster_size_range[0], min_samples=min_samples_range[0])


class SplitClusterer:
    """Clusterer that forces each sentence into its own cluster."""

    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        labels = np.arange(embeddings.shape[0], dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(
            labels,
            probs,
            False,
            HDBSCANSettings(min_cluster_size=min_cluster_size, min_samples=min_samples),
        )

    def optimize_clustering(self, embeddings, *, min_cluster_size_range, min_samples_range, **kwargs):
        return self.cluster(embeddings, min_cluster_size=min_cluster_size_range[0], min_samples=min_samples_range[0])


def test_pipeline_basic_flow():
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = FakeClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="art1",
                paragraphs=["Paragraph one is sufficiently lengthy to satisfy validation."],
            ),
            ClusterDocument(
                article_id="art2",
                paragraphs=["Another qualifying document ensures topic extraction has enough data."],
            )
        ],
    )

    response = pipeline.run(request)

    assert response.job_id == "job"
    assert response.genre == "ai"
    assert response.clusters
    assert response.evidence_budget.sentences > 0


def test_pipeline_keeps_clusters_non_empty_even_when_articles_reused():
    settings = Settings(model_id="fake", max_sentences_per_cluster=1)
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = SplitClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="dup",
                paragraphs=[
                    "First sentence easily exceeds thirty characters. Second sentence also satisfies the minimum length requirement. Third sentence keeps the cluster occupied when reuse is required."
                ],
            )
        ],
    )

    response = pipeline.run(request)

    assert len(response.clusters) >= 2
    for cluster in response.clusters[:2]:
        assert cluster.representatives, "cluster should retain at least one representative even after reuse"
        assert cluster.supporting_ids == ["dup"], "fallback should not introduce additional article ids"


def test_normalize_text_url_replacement():
    """Test that URLs are replaced with placeholders."""
    text = "Visit https://example.com for more info."
    result = normalize_text(text)
    assert "<URL>" in result
    assert "https://example.com" not in result


def test_normalize_text_email_replacement():
    """Test that email addresses are replaced with placeholders."""
    text = "Contact us at user@example.com for support."
    result = normalize_text(text)
    assert "<EMAIL>" in result
    assert "user@example.com" not in result


def test_normalize_text_punctuation_reduction():
    """Test that excessive punctuation is reduced."""
    text = "This is important!!! Really important。。。"
    result = normalize_text(text)
    assert "!!!" not in result
    assert "。。。" not in result
    # Should have single punctuation
    assert "!" in result or "。" in result


def test_normalize_text_whitespace_normalization():
    """Test that excessive whitespace is normalized."""
    text = "This   has    multiple    spaces\n\nand newlines."
    result = normalize_text(text)
    # Should not have multiple consecutive spaces
    assert "  " not in result
    # Newlines should be collapsed to spaces
    assert "\n" not in result


def test_normalize_text_unicode_normalization():
    """Test that full-width characters are normalized."""
    text = "Ｈｅｌｌｏ　Ｗｏｒｌｄ"  # Full-width
    result = normalize_text(text)
    # Should be normalized to half-width (NFKC)
    assert "Ｈ" not in result  # Full-width H should be normalized


def test_normalize_text_preserves_content():
    """Test that normalization doesn't remove important content."""
    text = "This is a sentence with important information. It should remain readable."
    result = normalize_text(text)
    assert "important information" in result
    assert len(result) > 20  # Should not be too short


def test_normalize_text_empty_string():
    """Test that empty strings are handled."""
    assert normalize_text("") == ""
    assert normalize_text("   ") == ""


def test_adjust_dedup_threshold_genre_override():
    """Test that genre-specific dedup thresholds override base threshold."""
    import json
    import os
    from recap_subworker.domain.models import CorpusMetadata

    # Temporarily clear environment variable to avoid interference
    old_val = os.environ.pop("RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS", None)
    try:
        # Create settings and directly set the field
        settings = Settings(model_id="fake")
        settings.genre_dedup_thresholds = '{"ai": 0.91, "politics": 0.94}'
        # Verify the property works
        assert settings.genre_dedup_thresholds_dict == {"ai": 0.91, "politics": 0.94}

        pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

        # Test genre override
        threshold_ai = pipeline._adjust_dedup_threshold(0.92, None, "ai")
        assert threshold_ai == 0.91

        threshold_politics = pipeline._adjust_dedup_threshold(0.92, None, "politics")
        assert threshold_politics == 0.94

        # Test genre without override (should use base)
        threshold_other = pipeline._adjust_dedup_threshold(0.92, None, "other")
        assert threshold_other == 0.92
    finally:
        if old_val is not None:
            os.environ["RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS"] = old_val


def test_adjust_dedup_threshold_classifier_adjustment():
    """Test that classifier-based adjustment works when genre override is not set."""
    import os
    from recap_subworker.domain.models import CorpusMetadata, CorpusClassifierStats

    # Temporarily clear environment variable
    old_val = os.environ.pop("RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS", None)
    try:
        settings = Settings(model_id="fake")
        settings.genre_dedup_thresholds = "{}"
        pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

        # Low confidence: should increase threshold
        low_conf_metadata = CorpusMetadata(
            article_count=10,
            sentence_count=100,
            primary_language="ja",
            character_count=5000,
            classifier=CorpusClassifierStats(
                avg_confidence=0.30,
                max_confidence=0.40,
                min_confidence=0.20,
                coverage_ratio=0.5
            )
        )
        threshold_low = pipeline._adjust_dedup_threshold(0.92, low_conf_metadata, "ai")
        assert threshold_low > 0.92
        assert threshold_low <= 0.97

        # High confidence: should decrease threshold
        high_conf_metadata = CorpusMetadata(
            article_count=10,
            sentence_count=100,
            primary_language="ja",
            character_count=5000,
            classifier=CorpusClassifierStats(
                avg_confidence=0.80,
                max_confidence=0.90,
                min_confidence=0.70,
                coverage_ratio=0.7
            )
        )
        threshold_high = pipeline._adjust_dedup_threshold(0.92, high_conf_metadata, "ai")
        assert threshold_high < 0.92
        assert threshold_high >= 0.82
    finally:
        if old_val is not None:
            os.environ["RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS"] = old_val


def test_adjust_dedup_threshold_genre_override_priority():
    """Test that genre override takes priority over classifier adjustment."""
    import json
    import os
    from recap_subworker.domain.models import CorpusMetadata, CorpusClassifierStats

    # Temporarily clear environment variable
    old_val = os.environ.pop("RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS", None)
    try:
        settings = Settings(model_id="fake")
        settings.genre_dedup_thresholds = '{"ai": 0.91}'
        pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

        # Even with low confidence (which would normally increase threshold),
        # genre override should be used
        low_conf_metadata = CorpusMetadata(
            article_count=10,
            sentence_count=100,
            primary_language="ja",
            character_count=5000,
            classifier=CorpusClassifierStats(
                avg_confidence=0.30,
                max_confidence=0.40,
                min_confidence=0.20,
                coverage_ratio=0.5
            )
        )
        threshold = pipeline._adjust_dedup_threshold(0.92, low_conf_metadata, "ai")
        assert threshold == 0.91  # Genre override, not classifier adjustment
    finally:
        if old_val is not None:
            os.environ["RECAP_SUBWORKER_GENRE_DEDUP_THRESHOLDS"] = old_val


def test_avg_pairwise_cosine_sim_high_similarity():
    """Test avg_sim calculation for highly similar (homogeneous) cluster."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    # Create nearly identical embeddings (high similarity)
    base_vec = np.array([1.0, 0.0, 0.0], dtype=np.float32)
    # Normalize
    base_vec = base_vec / np.linalg.norm(base_vec)
    # Add small variations
    embeddings = np.array([
        base_vec,
        base_vec + np.array([0.01, 0.0, 0.0], dtype=np.float32),
        base_vec + np.array([0.02, 0.0, 0.0], dtype=np.float32),
    ], dtype=np.float32)
    # Normalize each row
    norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
    embeddings = embeddings / norms

    avg_sim = pipeline._avg_pairwise_cosine_sim(embeddings)
    assert avg_sim is not None
    assert avg_sim > 0.9  # High similarity


def test_avg_pairwise_cosine_sim_low_similarity():
    """Test avg_sim calculation for diverse (orthogonal-like) cluster."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    # Create orthogonal embeddings (low similarity)
    embeddings = np.array([
        [1.0, 0.0, 0.0],
        [0.0, 1.0, 0.0],
        [0.0, 0.0, 1.0],
    ], dtype=np.float32)
    # Normalize each row
    norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
    embeddings = embeddings / norms

    avg_sim = pipeline._avg_pairwise_cosine_sim(embeddings)
    assert avg_sim is not None
    assert avg_sim < 0.1  # Low similarity (orthogonal vectors have cos=0)


def test_avg_pairwise_cosine_sim_single_vector():
    """Test avg_sim returns None for single vector."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    embeddings = np.array([[1.0, 0.0, 0.0]], dtype=np.float32)
    avg_sim = pipeline._avg_pairwise_cosine_sim(embeddings)
    assert avg_sim is None


def test_lambda_from_avg_sim_high_similarity():
    """Test lambda calculation for high avg_sim (homogeneous cluster)."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    base_lambda = 0.7
    # High avg_sim (e.g., 0.95) -> lambda should be close to 0.5
    lambda_param = pipeline._lambda_from_avg_sim(0.95, base_lambda)
    assert abs(lambda_param - 0.515) < 0.01  # 0.5 + 0.3 * (1 - 0.95) = 0.515


def test_lambda_from_avg_sim_low_similarity():
    """Test lambda calculation for low avg_sim (diverse cluster)."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    base_lambda = 0.7
    # Low avg_sim (e.g., 0.1) -> lambda should be close to 0.8
    lambda_param = pipeline._lambda_from_avg_sim(0.1, base_lambda)
    assert abs(lambda_param - 0.77) < 0.01  # 0.5 + 0.3 * (1 - 0.1) = 0.77


def test_lambda_from_avg_sim_none():
    """Test lambda calculation falls back to base_lambda when avg_sim is None."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    base_lambda = 0.7
    lambda_param = pipeline._lambda_from_avg_sim(None, base_lambda)
    assert lambda_param == base_lambda


def test_lambda_from_avg_sim_clipping():
    """Test lambda parameter is clipped to [0.0, 1.0] range."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    base_lambda = 0.7
    # Extreme values should be clipped
    lambda_param_high = pipeline._lambda_from_avg_sim(-1.5, base_lambda)
    lambda_param_low = pipeline._lambda_from_avg_sim(2.0, base_lambda)

    assert 0.0 <= lambda_param_high <= 1.0
    assert 0.0 <= lambda_param_low <= 1.0


def test_is_valid_representative_text_short_text():
    """Test that text shorter than 20 characters is rejected."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    assert not pipeline._is_valid_representative_text("short")
    assert not pipeline._is_valid_representative_text("x" * 19)
    assert pipeline._is_valid_representative_text("x" * 20)
    assert pipeline._is_valid_representative_text("This is a valid sentence with enough length.")


def test_is_valid_representative_text_stack_trace():
    """Test that stack trace-like text is rejected."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    assert not pipeline._is_valid_representative_text("stackTrace, }){ ...")
    assert not pipeline._is_valid_representative_text("Traceback (most recent call last):")
    assert not pipeline._is_valid_representative_text("Exception occurred at line 42")
    assert not pipeline._is_valid_representative_text('File "/path/to/file.py", line 10')
    assert not pipeline._is_valid_representative_text("Error: something went wrong")


def test_is_valid_representative_text_code_fragments():
    """Test that code fragment-like text is rejected."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)

    # High symbol density
    assert not pipeline._is_valid_representative_text("function() { return {}; }")
    # Code-like patterns
    assert not pipeline._is_valid_representative_text("stackTrace, }){ ...")
    assert not pipeline._is_valid_representative_text("});")
    assert not pipeline._is_valid_representative_text("() { }")


def test_pipeline_filters_short_sentences():
    """Test that pipeline filters out short sentences from representatives."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = FakeClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="art1",
                paragraphs=[
                    # Paragraph contains a short sentence that should be filtered out
                    "This is a valid sentence with sufficient length to pass validation. short. Another valid sentence that exceeds the minimum length requirement.",
                ],
            ),
            ClusterDocument(
                article_id="art2",
                paragraphs=["Yet another valid sentence that meets the length criteria."],
            ),
        ],
    )

    response = pipeline.run(request)

    assert response.job_id == "job"
    assert response.genre == "ai"
    # All representatives should have valid text (>= 20 chars)
    for cluster in response.clusters:
        for rep in cluster.representatives:
            assert len(rep.text) >= 20, f"Representative text too short: {rep.text}"


def test_pipeline_filters_code_fragments():
    """Test that pipeline filters out code fragment-like sentences from representatives."""
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = FakeClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="art1",
                paragraphs=[
                    # Paragraph contains a code fragment-like sentence that should be filtered out
                    "This is a valid sentence with sufficient length to pass validation. stackTrace, }){ ... Another valid sentence that exceeds the minimum length requirement.",
                ],
            ),
            ClusterDocument(
                article_id="art2",
                paragraphs=["Yet another valid sentence that meets the length criteria."],
            ),
        ],
    )

    response = pipeline.run(request)

    assert response.job_id == "job"
    assert response.genre == "ai"
    # All representatives should have valid text (no code fragments)
    for cluster in response.clusters:
        for rep in cluster.representatives:
            assert len(rep.text) >= 20, f"Representative text too short: {rep.text}"
            assert "stackTrace" not in rep.text.lower()
            assert "}){" not in rep.text
