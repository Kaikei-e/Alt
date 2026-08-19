"""Clamp TruncatedSVD n_components to the TF-IDF matrix rank.

train.py used to pass DEFAULT_HYPERPARAMS['svd_components'] (200) straight
into TruncatedSVD. sklearn 1.8.0 fail-closes when n_components > n_features
(and the mathematical rank is min(n_samples, n_features)). A tiny or
heavily-filtered corpus — including the 30-class smoke CSV — produces a
vocabulary smaller than 200, so training died before artefacts were written.
That is a production bug, not a lock-file surprise: the same crash happens
on sklearn 1.8.0 with any matrix whose feature count is below the requested
rank.
"""

from __future__ import annotations

import numpy as np
import pytest
from scipy.sparse import csr_matrix
from sklearn.decomposition import TruncatedSVD

from recap_subworker.infra.classifier.train import clamp_svd_components


def test_clamp_keeps_requested_when_rank_is_large_enough() -> None:
    assert clamp_svd_components(200, n_samples=800, n_features=1000) == 200


def test_clamp_to_n_features_when_vocabulary_is_smaller() -> None:
    assert clamp_svd_components(200, n_samples=600, n_features=120) == 120


def test_clamp_to_n_samples_when_samples_are_the_rank_limit() -> None:
    assert clamp_svd_components(200, n_samples=80, n_features=1000) == 80


def test_clamp_allows_n_components_equal_to_n_features() -> None:
    """sklearn 1.8 randomized SVD accepts n_components == n_features."""
    assert clamp_svd_components(50, n_samples=100, n_features=50) == 50


def test_clamp_rejects_fewer_than_two_features() -> None:
    with pytest.raises(ValueError, match="at least 2"):
        clamp_svd_components(200, n_samples=100, n_features=1)


def test_clamp_rejects_non_positive_request() -> None:
    with pytest.raises(ValueError, match=">= 1"):
        clamp_svd_components(0, n_samples=100, n_features=50)


def test_clamped_rank_fits_truncated_svd() -> None:
    """The value we would pass to TruncatedSVD must be accepted by sklearn 1.8."""
    rng = np.random.default_rng(0)
    n_samples, n_features = 30, 12
    requested = 200
    n_components = clamp_svd_components(requested, n_samples=n_samples, n_features=n_features)
    X = csr_matrix(rng.random((n_samples, n_features)))
    svd = TruncatedSVD(n_components=n_components, random_state=0)
    transformed = svd.fit_transform(X)
    assert transformed.shape == (n_samples, n_features)
