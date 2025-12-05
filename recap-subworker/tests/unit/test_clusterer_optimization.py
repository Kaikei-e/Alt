import numpy as np
import pytest
from recap_subworker.services.clusterer import Clusterer
from recap_subworker.infra.config import Settings

@pytest.fixture
def settings():
    return Settings()

def test_optimize_clustering_finds_best_params(settings):
    clusterer = Clusterer(settings)

    # Create 3 distinct clusters of 10 points each
    c1 = np.random.normal(loc=[0, 0], scale=0.1, size=(10, 2))
    c2 = np.random.normal(loc=[5, 5], scale=0.1, size=(10, 2))
    c3 = np.random.normal(loc=[10, 0], scale=0.1, size=(10, 2))
    embeddings = np.vstack([c1, c2, c3])

    # Search range that includes the optimal size (around 10)
    # Note: HDBSCAN min_cluster_size is a lower bound.
    # If we set min_cluster_size=5, it should find the clusters.
    # If we set min_cluster_size=20, it might merge them or find nothing.

    result = clusterer.optimize_clustering(
        embeddings,
        min_cluster_size_range=[5, 15],
        min_samples_range=[1, 3]
    )

    assert result.labels.size == 30
    assert result.dbcv_score >= -1.0 and result.dbcv_score <= 1.0
    # We expect it to find 3 clusters (labels 0, 1, 2) plus maybe noise (-1)
    unique_labels = set(result.labels)
    unique_labels.discard(-1)
    assert len(unique_labels) >= 2 # Should find at least 2 clusters

    # Check that it selected valid params
    assert result.params.min_cluster_size in [5, 15]
    assert result.params.min_samples in [1, 3]

def test_optimize_clustering_handles_empty(settings):
    clusterer = Clusterer(settings)
    embeddings = np.empty((0, 2))
    result = clusterer.optimize_clustering(embeddings)
    assert result.labels.size == 0
    assert result.dbcv_score == 0.0
