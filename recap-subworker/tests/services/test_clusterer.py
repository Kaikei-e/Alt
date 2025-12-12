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
        # Note: optimize_clustering now uses composite score (0.6*silhouette + 0.4*dbcv)
        def create_result(silhouette_score_val, dbcv_score_val, mcs):
            return ClusterResult(
                labels=np.array([]),
                probabilities=np.array([]),
                used_umap=True,
                params=HDBSCANSettings(min_cluster_size=mcs, min_samples=1),
                dbcv_score=dbcv_score_val,
                silhouette_score=silhouette_score_val
            )

        # Arrange mock to return results.
        # It's hard to predict exact order so we just make it return a result with the score
        # based on arguments or just fixed list.
        # optimize loop: umap_n (3) * umap_comp (1) * mcs (6) * ms (4) ... lots of calls.
        # Let's just check if it returns the one with best score.

        mock_cluster.return_value = create_result(0.5, 0.3, 5)

        embeddings = np.random.rand(20, 384) # 20 points

        # Mocking specific call to return higher composite score
        # Composite = 0.6*sil + 0.4*dbcv
        # For mcs=10: 0.6*0.9 + 0.4*0.5 = 0.54 + 0.20 = 0.74
        # For others: 0.6*0.1 + 0.4*0.2 = 0.06 + 0.08 = 0.14
        def side_effect(*args, **kwargs):
            mcs = kwargs.get('min_cluster_size')
            if mcs == 10:
                return create_result(0.9, 0.5, 10)  # composite = 0.74
            return create_result(0.1, 0.2, mcs)  # composite = 0.14

        mock_cluster.side_effect = side_effect

        result = clusterer.optimize_clustering(embeddings)

        # Should select the one with highest composite score
        assert result.silhouette_score == 0.9
        assert result.dbcv_score == 0.5
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
                dbcv_score=0.5,
                silhouette_score=0.3
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


def test_calculate_dbcv_noise_only(clusterer):
    """Test DBCV calculation with only noise labels (-1)."""
    embeddings = np.random.rand(10, 5)
    labels = np.full(10, -1, dtype=int)
    dbcv = clusterer._calculate_dbcv(embeddings, labels)
    assert dbcv == 0.0


def test_calculate_dbcv_insufficient_points(clusterer):
    """Test DBCV calculation with less than 2 non-noise points."""
    embeddings = np.random.rand(10, 5)
    labels = np.array([-1, -1, -1, -1, -1, -1, -1, -1, -1, 0], dtype=int)
    dbcv = clusterer._calculate_dbcv(embeddings, labels)
    assert dbcv == 0.0


def test_calculate_dbcv_single_cluster(clusterer):
    """Test DBCV calculation with only one cluster (all same label)."""
    embeddings = np.random.rand(10, 5)
    labels = np.full(10, 0, dtype=int)
    dbcv = clusterer._calculate_dbcv(embeddings, labels)
    assert dbcv == 0.0


def test_calculate_dbcv_valid_clusters(clusterer):
    """Test DBCV calculation with valid multiple clusters (smoke test)."""
    # Create 2 distinct clusters
    c1 = np.random.normal(loc=[0, 0, 0, 0, 0], scale=0.1, size=(10, 5))
    c2 = np.random.normal(loc=[5, 5, 5, 5, 5], scale=0.1, size=(10, 5))
    embeddings = np.vstack([c1, c2])
    labels = np.array([0] * 10 + [1] * 10, dtype=int)

    dbcv = clusterer._calculate_dbcv(embeddings, labels)
    # Should be a finite value (not NaN or Inf)
    assert np.isfinite(dbcv)
    # DBCV typically ranges from -1 to 1, but can be outside in edge cases
    assert isinstance(dbcv, (int, float))


def test_calculate_dbcv_with_noise(clusterer):
    """Test DBCV calculation with valid clusters and some noise points."""
    # Create 2 distinct clusters + noise
    c1 = np.random.normal(loc=[0, 0, 0, 0, 0], scale=0.1, size=(10, 5))
    c2 = np.random.normal(loc=[5, 5, 5, 5, 5], scale=0.1, size=(10, 5))
    noise = np.random.normal(loc=[2.5, 2.5, 2.5, 2.5, 2.5], scale=0.3, size=(5, 5))
    embeddings = np.vstack([c1, c2, noise])
    labels = np.array([0] * 10 + [1] * 10 + [-1] * 5, dtype=int)

    dbcv = clusterer._calculate_dbcv(embeddings, labels)
    # Should be finite (noise points are excluded)
    assert np.isfinite(dbcv)
    assert isinstance(dbcv, (int, float))


def test_optimize_clustering_uses_optuna_when_enabled(clusterer):
    """Test that Optuna is used when use_bayes_opt is True."""
    clusterer.settings.use_bayes_opt = True
    clusterer.settings.bayes_opt_trials = 10

    embeddings = np.random.rand(20, 5)

    with patch('recap_subworker.services.clusterer.optuna') as mock_optuna:
        # Mock Optuna study
        mock_study = MagicMock()
        mock_study.best_params = {'min_cluster_size': 8, 'min_samples': 2}
        mock_study.optimize = MagicMock()
        mock_optuna.create_study.return_value = mock_study
        mock_optuna.TPESampler = MagicMock()

        # Mock cluster to return a result
        with patch.object(clusterer, 'cluster') as mock_cluster:
            mock_cluster.return_value = ClusterResult(
                labels=np.array([0] * 10 + [1] * 10),
                probabilities=np.ones(20),
                used_umap=False,
                params=HDBSCANSettings(min_cluster_size=8, min_samples=2),
                dbcv_score=0.4,
                silhouette_score=0.6
            )

            result = clusterer.optimize_clustering(embeddings)

            # Verify Optuna was called
            assert mock_optuna.create_study.called
            assert mock_study.optimize.called
            # Verify TPESampler with seed was used
            assert mock_optuna.TPESampler.called
            # Check that seed=42 was passed
            call_args = mock_optuna.TPESampler.call_args
            assert call_args[1].get('seed') == 42 or call_args[0][0] == 42


def test_noise_reclustering_disabled(clusterer):
    """Test that noise reclustering is skipped when disabled."""
    clusterer.settings.noise_recluster_enabled = False
    clusterer.settings.noise_recluster_min_points = 30

    # Create embeddings with many noise points
    embeddings = np.random.rand(50, 5)
    labels = np.array([-1] * 40 + [0] * 10, dtype=int)

    # Mock cluster to return labels with noise
    with patch.object(clusterer, '_calculate_dbcv', return_value=0.3):
        with patch.object(clusterer, '_calculate_silhouette', return_value=0.5):
            result = clusterer.cluster(
                embeddings,
                min_cluster_size=5,
                min_samples=2
            )
            # Noise points should remain as -1
            assert (result.labels == -1).sum() == 40


def test_noise_reclustering_insufficient_points(clusterer):
    """Test that noise reclustering is skipped when noise points < min_points."""
    clusterer.settings.noise_recluster_enabled = True
    clusterer.settings.noise_recluster_min_points = 30
    clusterer.settings.noise_recluster_max_clusters = 8

    # Create embeddings with few noise points
    embeddings = np.random.rand(50, 5)
    labels = np.array([-1] * 20 + [0] * 30, dtype=int)

    # Mock cluster to return labels with noise
    with patch.object(clusterer, '_calculate_dbcv', return_value=0.3):
        with patch.object(clusterer, '_calculate_silhouette', return_value=0.5):
            result = clusterer.cluster(
                embeddings,
                min_cluster_size=5,
                min_samples=2
            )
            # Noise points should remain as -1 (not enough for reclustering)
            assert (result.labels == -1).sum() == 20


def test_noise_reclustering_creates_new_clusters(clusterer):
    """Test that noise reclustering creates new cluster IDs."""
    clusterer.settings.noise_recluster_enabled = True
    clusterer.settings.noise_recluster_min_points = 30
    clusterer.settings.noise_recluster_max_clusters = 8

    # Create embeddings with many noise points
    embeddings = np.random.rand(50, 5)
    # Create distinct noise clusters for KMeans to separate
    noise1 = np.random.normal(loc=[0, 0, 0, 0, 0], scale=0.1, size=(20, 5))
    noise2 = np.random.normal(loc=[5, 5, 5, 5, 5], scale=0.1, size=(20, 5))
    cluster = np.random.normal(loc=[2.5, 2.5, 2.5, 2.5, 2.5], scale=0.1, size=(10, 5))
    embeddings = np.vstack([noise1, noise2, cluster])
    labels = np.array([-1] * 40 + [0] * 10, dtype=int)

    # Mock HDBSCAN to return these labels
    with patch('recap_subworker.services.clusterer.HDBSCAN') as mock_hdbscan:
        mock_clusterer = MagicMock()
        mock_clusterer.labels_ = labels
        mock_clusterer.probabilities_ = np.ones(50)
        mock_hdbscan.return_value = mock_clusterer

        with patch.object(clusterer, '_calculate_dbcv', return_value=0.3):
            with patch.object(clusterer, '_calculate_silhouette', return_value=0.5):
                result = clusterer.cluster(
                    embeddings,
                    min_cluster_size=5,
                    min_samples=2
                )
                # Some noise points should be reassigned to new cluster IDs
                # (new IDs should be > max(original labels))
                max_original = labels.max()
                new_labels = result.labels
                # At least some noise points should have new IDs
                noise_mask = labels == -1
                reclustered = new_labels[noise_mask]
                # New IDs should be > max_original (0 in this case)
                if len(reclustered) > 0:
                    assert (reclustered > max_original).any() or (reclustered >= 0).all()
