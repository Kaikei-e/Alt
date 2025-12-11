import numpy as np
import pytest
from unittest.mock import MagicMock
from recap_subworker.services.clusterer import Clusterer
from recap_subworker.infra.config import Settings

@pytest.fixture
def mock_settings():
    settings = MagicMock(spec=Settings)
    settings.hdbscan_min_cluster_size = 5
    settings.hdbscan_min_samples = 2
    settings.enable_umap_force = False
    settings.enable_umap_auto = True
    settings.umap_threshold_sentences = 10
    settings.umap_n_neighbors = 15
    settings.umap_n_components = 5
    settings.umap_min_dist = 0.0
    settings.hdbscan_cluster_selection_method = "eom"
    # Recursive settings
    settings.clustering_recursive_enabled = True
    settings.clustering_max_tokens_per_cluster = 100 # Low threshold to force split
    settings.clustering_min_split_size = 5
    return settings

@pytest.fixture
def clusterer(mock_settings):
    return Clusterer(mock_settings)

def test_recursive_cluster_splits_large_cluster(clusterer):
    # Setup: 20 points
    # Cluster 0: 10 points, each 20 tokens => 200 total > 100 limit
    # Cluster 1: 10 points, each 5 tokens => 50 total < 100 limit

    # 2 distinct blobs for embeddings so K-Means can separate them easily if needed
    # Blob 0: around [0,0]
    # Blob 1: around [10,10]

    # Within Blob 0, let's make it separable too, so bisecting k-means works well
    # Blob 0a: [0,0], Blob 0b: [2,2]

    embeddings = np.zeros((20, 2))
    # Cluster 0 members (indices 0-9)
    embeddings[0:5] = np.random.normal(0, 0.1, (5, 2))
    embeddings[5:10] = np.random.normal(2, 0.1, (5, 2))

    # Cluster 1 members (indices 10-19)
    embeddings[10:20] = np.random.normal(10, 0.1, (10, 2))

    labels = np.array([0]*10 + [1]*10)
    probabilities = np.ones(20)

    # Token counts
    token_counts = np.array([20]*10 + [5]*10) # Cluster 0 has 200, Cluster 1 has 50

    new_labels, new_probs = clusterer.recursive_cluster(
        embeddings, labels, probabilities, token_counts
    )

    # Assertions
    # Cluster 1 should be untouched (label 1, count 10)
    assert np.sum(new_labels == 1) == 10

    # Cluster 0 should be split.
    # It originally had 10 items.
    # Now those 10 items should be split into at least 2 labels.
    # The labels corresponding to indices 0-9
    c0_new_labels = new_labels[0:10]
    unique_c0_labels = np.unique(c0_new_labels)

    assert len(unique_c0_labels) >= 2, "Cluster 0 should have been split"
    assert 0 in unique_c0_labels, "Original label 0 should persist for one half"
    # The new label should be > 1
    assert any(l > 1 for l in unique_c0_labels)

    # Probabilities for split items should be 1.0
    assert np.all(new_probs[0:10] == 1.0)

def test_recursive_cluster_respects_disable(clusterer):
    clusterer.settings.clustering_recursive_enabled = False

    embeddings = np.random.rand(20, 2)
    labels = np.zeros(20, dtype=int)
    probabilities = np.ones(20)
    token_counts = np.full(20, 100) # Huge count

    new_labels, _ = clusterer.recursive_cluster(
        embeddings, labels, probabilities, token_counts
    )

    assert np.array_equal(new_labels, labels)

def test_recursive_cluster_min_size(clusterer):
    # Cluster size 4 < min_split_size 5
    # Even if tokens are huge, it shouldn't split
    clusterer.settings.clustering_min_split_size = 5

    embeddings = np.random.rand(4, 2)
    labels = np.zeros(4, dtype=int)
    probabilities = np.ones(4)
    token_counts = np.full(4, 1000) # Huge count

    new_labels, _ = clusterer.recursive_cluster(
        embeddings, labels, probabilities, token_counts
    )

    assert np.array_equal(new_labels, labels)
