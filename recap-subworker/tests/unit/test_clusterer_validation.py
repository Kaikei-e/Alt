import numpy as np
import pytest
from recap_subworker.services.clusterer import Clusterer
from recap_subworker.infra.config import Settings

@pytest.fixture
def settings():
    return Settings()

def test_clusterer_handles_nan_embeddings(settings):
    clusterer = Clusterer(settings)
    embeddings = np.array([[1.0, np.nan], [0.5, 0.5]])
    result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
    assert result.labels.size == 0
    assert result.probabilities.size == 0

def test_clusterer_handles_inf_embeddings(settings):
    clusterer = Clusterer(settings)
    embeddings = np.array([[1.0, np.inf], [0.5, 0.5]])
    result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
    assert result.labels.size == 0

def test_clusterer_handles_zero_embeddings(settings):
    clusterer = Clusterer(settings)
    embeddings = np.array([[0.0, 0.0], [0.5, 0.5]])
    result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
    assert result.labels.size == 0

def test_clusterer_handles_valid_embeddings(settings):
    clusterer = Clusterer(settings)
    # 5 points, 2 clusters
    embeddings = np.array([
        [1.0, 0.0], [0.9, 0.1],
        [0.0, 1.0], [0.1, 0.9],
        [0.5, 0.5]
    ])
    result = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
    # Should return valid labels (size match input)
    assert result.labels.size == 5
