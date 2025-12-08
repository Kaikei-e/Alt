import numpy as np
import pytest
from unittest.mock import MagicMock, patch
from recap_subworker.services.clusterer import Clusterer, ClusterResult
from recap_subworker.infra.config import Settings
from recap_subworker.domain.models import HDBSCANSettings

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
    return settings

@pytest.fixture
def clusterer(mock_settings):
    return Clusterer(mock_settings)

def test_optimize_clustering_defaults(clusterer):
    # Mock cluster method to avoid actual heavy computation and just return a dummy result with score
    with patch.object(clusterer, 'cluster') as mock_cluster:
        # Side effect: return different scores for different calls
        # We want to verify it iterates.
        # Let's say we have 3 iterations.

        # Mock result factory
        def create_result(score, mcs):
            return ClusterResult(
                labels=np.array([]),
                probabilities=np.array([]),
                used_umap=True,
                params=HDBSCANSettings(min_cluster_size=mcs, min_samples=1),
                dbcv_score=score,
                silhouette_score=0.5
            )

        # Arrange mock to return results.
        # It's hard to predict exact order so we just make it return a result with the score
        # based on arguments or just fixed list.
        # optimize loop: umap_n (3) * umap_comp (1) * mcs (6) * ms (4) ... lots of calls.
        # Let's just check if it returns the one with best score.

        mock_cluster.return_value = create_result(0.5, 5)

        embeddings = np.random.rand(20, 384) # 20 points

        # Mocking specific call to return higher score
        def side_effect(*args, **kwargs):
            mcs = kwargs.get('min_cluster_size')
            if mcs == 10:
                return create_result(0.9, 10)
            return create_result(0.1, mcs)

        mock_cluster.side_effect = side_effect

        result = clusterer.optimize_clustering(embeddings)

        assert result.dbcv_score == 0.9
        assert result.params.min_cluster_size == 10
        assert mock_cluster.call_count > 1

def test_optimize_clustering_small_data(clusterer):
    embeddings = np.random.rand(5, 384)
    # With 5 points, mcs range [3, 4, 6, 8, 10, 12] should only try 3, 4.

    with patch.object(clusterer, 'cluster') as mock_cluster:
        mock_cluster.return_value = ClusterResult(
                labels=np.array([]),
                probabilities=np.array([]),
                used_umap=False,
                params=HDBSCANSettings(min_cluster_size=3, min_samples=1),
                dbcv_score=0.5
            )

        clusterer.optimize_clustering(embeddings)

        # Verify calls arguments
        for call in mock_cluster.call_args_list:
            args, kwargs = call
            assert kwargs['min_cluster_size'] < 5

def test_subcluster_other_uses_leaf(clusterer):
    embeddings = np.random.rand(20, 384)
    with patch.object(clusterer, 'optimize_clustering') as mock_optimize:
        clusterer.subcluster_other(embeddings)

        args, kwargs = mock_optimize.call_args
        assert kwargs['hdbscan_cluster_selection_method'] == 'leaf'
        assert kwargs['hdbscan_allow_single_cluster'] is True
        assert kwargs['min_cluster_size_range'] == [3, 4, 5]
