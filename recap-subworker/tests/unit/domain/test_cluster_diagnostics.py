"""Unit tests for ClusterDiagnostics value object.

Phase 4 introduces a frozen, extra-forbidden Pydantic v2 value object
that the run manager uses instead of building a raw ``dict[str, Any]``
for persisted diagnostics. The VO's ``model_dump(mode="json",
exclude_none=True)`` output must match the legacy dict byte-for-byte
so Pact response shapes stay stable.
"""

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from recap_subworker.domain.value_objects import ClusterDiagnostics


def test_cluster_diagnostics_is_frozen() -> None:
    vo = ClusterDiagnostics(
        dedup_pairs=0,
        umap_used=True,
        partial=False,
        total_sentences=100,
    )
    with pytest.raises(ValidationError):
        vo.dedup_pairs = 5  # type: ignore[misc]


def test_cluster_diagnostics_rejects_unknown_fields() -> None:
    with pytest.raises(ValidationError):
        ClusterDiagnostics(
            dedup_pairs=0,
            umap_used=True,
            partial=False,
            total_sentences=100,
            unknown_field="bad",  # type: ignore[call-arg]
        )


def test_model_dump_matches_legacy_dict() -> None:
    legacy = {
        "dedup_pairs": 3,
        "umap_used": True,
        "partial": False,
        "total_sentences": 200,
        "embedding_ms": 42.5,
        "hdbscan_ms": 17.2,
        "noise_ratio": 0.15,
        "dbcv_score": 0.52,
        "silhouette_score": 0.48,
    }
    vo = ClusterDiagnostics(**legacy)
    dumped = vo.model_dump(mode="json", exclude_none=True)
    assert json.dumps(dumped, sort_keys=True) == json.dumps(legacy, sort_keys=True)


def test_model_dump_omits_none_fields() -> None:
    vo = ClusterDiagnostics(
        dedup_pairs=0,
        umap_used=False,
        partial=False,
        total_sentences=10,
    )
    dumped = vo.model_dump(mode="json", exclude_none=True)
    assert set(dumped.keys()) == {
        "dedup_pairs",
        "umap_used",
        "partial",
        "total_sentences",
    }
    assert "embedding_ms" not in dumped
    assert "hdbscan_ms" not in dumped
