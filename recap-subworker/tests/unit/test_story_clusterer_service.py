"""Unit tests for StoryClustererService with synthetic vectors."""

from __future__ import annotations

from datetime import datetime

import numpy as np
import pytest

from recap_subworker.services.story_clusterer import (
    ClusterStoryItem,
    StoryClustererService,
    StoryClusterParams,
)


def _make_item(
    item_id: str,
    vec: list[float],
    published_at: str = "2026-09-21T00:00:00Z",
    language: str | None = None,
) -> ClusterStoryItem:
    dt = datetime.fromisoformat(published_at.replace("Z", "+00:00"))
    return ClusterStoryItem(id=item_id, embedding=vec, published_at=dt, language=language)


def test_story_clusterer_single_item():
    """Verify that a single item forms a cluster of size 1."""
    service = StoryClustererService()
    items = [_make_item("item-1", [1.0, 0.0, 0.0])]
    params = StoryClusterParams(threshold=0.78)

    response = service.cluster_stories(items, params)

    assert len(response.clusters) == 1
    assert response.clusters[0].cluster_id == 0
    assert response.clusters[0].member_ids == ["item-1"]
    assert np.isclose(response.clusters[0].centroid, [1.0, 0.0, 0.0]).all()


def test_story_clusterer_time_decay_merge_vs_split():
    """Verify that two near-identical vectors merge when published 0 days apart,
    but do not merge when published 30 days apart with time_decay_per_day=0.05.
    """
    service = StoryClustererService()
    v1 = [1.0, 0.0, 0.0]
    # Very close vector (cos sim > 0.9999, cos dist < 0.0001)
    v2 = [0.9999, 0.0141, 0.0]
    v2 = (np.array(v2) / np.linalg.norm(v2)).tolist()

    # Case A: Published 0 days apart (same time)
    # Cos dist ≈ 0.0001 < distance_threshold (1 - 0.78 = 0.22) -> should MERGE
    items_0_days = [
        _make_item("item-1", v1, "2026-09-01T00:00:00Z"),
        _make_item("item-2", v2, "2026-09-01T00:00:00Z"),
    ]
    params = StoryClusterParams(threshold=0.78, time_decay_per_day=0.05)

    res_0_days = service.cluster_stories(items_0_days, params)
    assert len(res_0_days.clusters) == 1
    assert sorted(res_0_days.clusters[0].member_ids) == ["item-1", "item-2"]

    # Case B: Published 30 days apart
    # Distance = cos_dist + 0.05 * 30 = 0.0001 + 1.5 = 1.5 -> capped at 1.0
    # Since 1.0 > distance_threshold (0.22) -> should NOT merge
    items_30_days = [
        _make_item("item-1", v1, "2026-09-01T00:00:00Z"),
        _make_item("item-2", v2, "2026-10-01T00:00:00Z"),
    ]
    res_30_days = service.cluster_stories(items_30_days, params)
    assert len(res_30_days.clusters) == 2
    assert res_30_days.clusters[0].member_ids == ["item-1"]
    assert res_30_days.clusters[1].member_ids == ["item-2"]


def test_story_clusterer_determinism():
    """Verify that identical input generates strictly identical output across runs."""
    service = StoryClustererService()
    rng = np.random.default_rng(42)

    # 10 synthetic items
    items = []
    for i in range(10):
        vec = rng.normal(0, 1, 32).astype(np.float32)
        vec = (vec / np.linalg.norm(vec)).tolist()
        items.append(
            _make_item(
                f"uuid-{i:02d}",
                vec,
                f"2026-09-{(i % 5) + 1:02d}T12:00:00Z",
            ),
        )

    params = StoryClusterParams(
        threshold=0.6,
        linkage="average",
        time_decay_per_day=0.02,
    )

    run1 = service.cluster_stories(items, params)
    run2 = service.cluster_stories(items, params)

    assert run1.model_dump() == run2.model_dump()


def test_story_clusterer_sorting_order():
    """Verify clusters are sorted by size descending, then by smallest member_id
    ascending, and member_ids within each cluster are sorted.
    """
    service = StoryClustererService()

    # Form 3 clusters:
    # Cluster A: 3 items (starts with uuid-3)
    # Cluster B: 2 items (starts with uuid-1)
    # Cluster C: 2 items (starts with uuid-2)
    # Expected order: Cluster A (size 3), Cluster B (size 2, min uuid-1),
    # Cluster C (size 2, min uuid-2), Cluster D (size 1)

    v_a = [1.0, 0.0, 0.0]
    v_b = [0.0, 1.0, 0.0]
    v_c = [0.0, 0.0, 1.0]
    v_d = [0.577, 0.577, 0.577]

    items = [
        # Cluster A items (3 items)
        _make_item("uuid-9", v_a),
        _make_item("uuid-3", v_a),
        _make_item("uuid-7", v_a),
        # Cluster B items (2 items, min uuid-1)
        _make_item("uuid-5", v_b),
        _make_item("uuid-1", v_b),
        # Cluster C items (2 items, min uuid-2)
        _make_item("uuid-6", v_c),
        _make_item("uuid-2", v_c),
        # Cluster D singleton (1 item)
        _make_item("uuid-0", v_d),
    ]

    params = StoryClusterParams(
        threshold=0.9,
        linkage="average",
        time_decay_per_day=0.0,
    )
    response = service.cluster_stories(items, params)

    assert len(response.clusters) == 4

    # Cluster 0: size 3, member_ids sorted
    assert response.clusters[0].cluster_id == 0
    assert response.clusters[0].member_ids == ["uuid-3", "uuid-7", "uuid-9"]

    # Cluster 1: size 2, smallest member uuid-1
    assert response.clusters[1].cluster_id == 1
    assert response.clusters[1].member_ids == ["uuid-1", "uuid-5"]

    # Cluster 2: size 2, smallest member uuid-2
    assert response.clusters[2].cluster_id == 2
    assert response.clusters[2].member_ids == ["uuid-2", "uuid-6"]

    # Cluster 3: size 1, smallest member uuid-0
    assert response.clusters[3].cluster_id == 3
    assert response.clusters[3].member_ids == ["uuid-0"]


def test_story_clusterer_centroid_re_normalization():
    """Verify that centroid is the L2-re-normalized mean of member embeddings."""
    service = StoryClustererService()
    v1 = [1.0, 0.0, 0.0]
    v2 = [0.6, 0.8, 0.0]  # cos = 0.6, cos_dist = 0.4
    # With threshold=0.5 -> distance_threshold = 0.5 -> 0.4 < 0.5, they merge!
    items = [
        _make_item("item-1", v1),
        _make_item("item-2", v2),
    ]
    params = StoryClusterParams(threshold=0.5, time_decay_per_day=0.0)
    response = service.cluster_stories(items, params)

    assert len(response.clusters) == 1
    centroid = np.array(response.clusters[0].centroid)
    assert np.linalg.norm(centroid) == pytest.approx(1.0, rel=1e-5)

    # Mean = [0.8, 0.4, 0.0]
    # Norm = sqrt(0.8^2 + 0.4^2) = sqrt(0.8) ≈ 0.894427
    expected = np.array([0.8, 0.4, 0.0]) / np.linalg.norm([0.8, 0.4, 0.0])
    assert np.allclose(centroid, expected, atol=1e-5)


def test_story_clusterer_singletons_preserved():
    """Verify that singletons are preserved as clusters of size 1 (no noise bucket)."""
    service = StoryClustererService()
    # Three orthogonal vectors, threshold = 0.8 (distance_threshold = 0.2)
    # Cos distances are 1.0 > 0.2 -> none merge
    items = [
        _make_item("item-1", [1.0, 0.0, 0.0]),
        _make_item("item-2", [0.0, 1.0, 0.0]),
        _make_item("item-3", [0.0, 0.0, 1.0]),
    ]
    params = StoryClusterParams(
        threshold=0.8,
        time_decay_per_day=0.0,
        min_cluster_size=1,
    )
    response = service.cluster_stories(items, params)

    assert len(response.clusters) == 3
    for cluster in response.clusters:
        assert len(cluster.member_ids) == 1


def test_story_clusterer_min_cluster_size_filter():
    """Verify that min_cluster_size > 1 filters out singletons."""
    service = StoryClustererService()
    items = [
        _make_item("item-1", [1.0, 0.0, 0.0]),
        _make_item("item-2", [0.99, 0.01, 0.0]),  # merges with item-1
        _make_item("item-3", [0.0, 1.0, 0.0]),  # singleton
    ]
    params = StoryClusterParams(
        threshold=0.78,
        time_decay_per_day=0.0,
        min_cluster_size=2,
    )
    response = service.cluster_stories(items, params)

    assert len(response.clusters) == 1
    assert response.clusters[0].member_ids == ["item-1", "item-2"]


def test_story_clusterer_min_cluster_size_by_language_filtering():
    """Verify majority-language min cluster size filtering logic."""
    service = StoryClustererService()
    v_ja = [1.0, 0.0, 0.0]
    v_en = [0.0, 1.0, 0.0]
    v_en_merge = [0.0, 0.99, 0.01]

    # Cluster 1: 2 'ja' items
    # Cluster 2: 2 'en' items
    items = [
        _make_item("ja-1", v_ja, language="ja"),
        _make_item("ja-2", [0.99, 0.0, 0.01], language="ja"),
        _make_item("en-1", v_en, language="en"),
        _make_item("en-2", v_en_merge, language="en"),
    ]

    # Case A: ja requires min 2, en requires min 3
    # ja cluster has 2 items -> kept
    # en cluster has 2 items -> dropped (2 < 3)
    params_a = StoryClusterParams(
        threshold=0.78,
        time_decay_per_day=0.0,
        min_cluster_size=1,
        min_cluster_size_by_language={"ja": 2, "en": 3},
    )
    res_a = service.cluster_stories(items, params_a)
    assert len(res_a.clusters) == 1
    assert res_a.clusters[0].member_ids == ["ja-1", "ja-2"]

    # Case B: Mixed cluster (2 'en', 1 'ja') -> majority language is 'en'
    # en requires min 3 -> kept because len(members) == 3 >= 3
    mixed_items = [
        _make_item("m-en-1", [1.0, 0.0, 0.0], language="en"),
        _make_item("m-en-2", [0.99, 0.01, 0.0], language="en"),
        _make_item("m-ja-1", [0.99, 0.0, 0.01], language="ja"),
    ]
    params_b = StoryClusterParams(
        threshold=0.78,
        time_decay_per_day=0.0,
        min_cluster_size=1,
        min_cluster_size_by_language={"ja": 2, "en": 3},
    )
    res_b = service.cluster_stories(mixed_items, params_b)
    assert len(res_b.clusters) == 1
    assert res_b.clusters[0].member_ids == ["m-en-1", "m-en-2", "m-ja-1"]

    # Case C: Backward compatibility when language is omitted
    unlabeled_items = [
        _make_item("u-1", [1.0, 0.0, 0.0]),
        _make_item("u-2", [0.99, 0.01, 0.0]),
    ]
    # Falls back to min_cluster_size=3 -> dropped
    params_c = StoryClusterParams(
        threshold=0.78,
        time_decay_per_day=0.0,
        min_cluster_size=3,
        min_cluster_size_by_language={"ja": 2},
    )
    res_c = service.cluster_stories(unlabeled_items, params_c)
    assert len(res_c.clusters) == 0


def test_story_clusterer_single_item_with_language_filter():
    """Verify single-item (n=1) edge case with language filtering."""
    service = StoryClustererService()
    item_ja = _make_item("ja-single", [1.0, 0.0, 0.0], language="ja")

    # Min size 2 for ja -> drops single item
    params_drop = StoryClusterParams(
        min_cluster_size_by_language={"ja": 2},
    )
    res_drop = service.cluster_stories([item_ja], params_drop)
    assert len(res_drop.clusters) == 0

    # Min size 1 for ja -> keeps single item
    params_keep = StoryClusterParams(
        min_cluster_size_by_language={"ja": 1},
    )
    res_keep = service.cluster_stories([item_ja], params_keep)
    assert len(res_keep.clusters) == 1
    assert res_keep.clusters[0].member_ids == ["ja-single"]


def test_story_clusterer_language_keys_case_insensitive():
    """Verify that min_cluster_size_by_language keys are matched case-insensitively."""
    service = StoryClustererService()
    items = [
        _make_item("ja-1", [1.0, 0.0, 0.0], language="ja"),
        _make_item("ja-2", [0.99, 0.01, 0.0], language="JA"),
    ]
    # Keys in params are uppercase "JA"
    params = StoryClusterParams(
        threshold=0.78,
        min_cluster_size=1,
        min_cluster_size_by_language={"JA": 3},
    )
    # Cluster has 2 members, but JA requires 3 -> dropped
    res = service.cluster_stories(items, params)
    assert len(res.clusters) == 0


def test_story_clusterer_majority_language_tie_break_alphabetical():
    """Verify that ties in language counts break canonically by alphabetical order."""
    service = StoryClustererService()
    # 1 'ja', 1 'en' -> tie in count (1 vs 1). 'en' < 'ja' alphabetically -> majority is 'en'
    # Try both insertion orders: [ja, en] and [en, ja]
    items_ja_first = [
        _make_item("item-ja", [1.0, 0.0, 0.0], language="ja"),
        _make_item("item-en", [0.99, 0.01, 0.0], language="en"),
    ]
    items_en_first = [
        _make_item("item-en", [0.99, 0.01, 0.0], language="en"),
        _make_item("item-ja", [1.0, 0.0, 0.0], language="ja"),
    ]

    # If 'en' is chosen: min size 3 -> cluster of 2 dropped
    # If 'ja' was chosen: min size 2 -> cluster of 2 kept
    params = StoryClusterParams(
        threshold=0.78,
        min_cluster_size=1,
        min_cluster_size_by_language={"en": 3, "ja": 2},
    )

    # In both orders, 'en' wins the tie-break -> size requirement is 3 -> dropped
    res_ja_first = service.cluster_stories(items_ja_first, params)
    assert len(res_ja_first.clusters) == 0

    res_en_first = service.cluster_stories(items_en_first, params)
    assert len(res_en_first.clusters) == 0


def test_story_clusterer_distance_exceeds_one_no_upper_clip():
    """Verify that distances > 1.0 are not clipped at 1.0, preventing far-apart items from collapsing."""
    service = StoryClustererService()
    v1 = [1.0, 0.0, 0.0]
    v2 = [1.0, 0.0, 0.0]

    # 40 days apart with time_decay_per_day = 0.05 -> time penalty = 2.0.
    # Total distance = 0.0 (cos) + 2.0 = 2.0.
    items = [
        _make_item("item-1", v1, "2026-08-01T00:00:00Z"),
        _make_item("item-2", v2, "2026-09-10T00:00:00Z"),
    ]

    # With threshold = 0.1, distance_threshold = 1 - 0.1 = 0.9.
    # Total distance = 2.0 > 0.9 -> correctly SPLIT into 2 clusters.
    params = StoryClusterParams(
        threshold=0.1,
        time_decay_per_day=0.05,
    )
    res = service.cluster_stories(items, params)
    assert len(res.clusters) == 2
    assert res.clusters[0].member_ids == ["item-1"]
    assert res.clusters[1].member_ids == ["item-2"]
