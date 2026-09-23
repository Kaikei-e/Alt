"""Tests for POST /v1/cluster-stories router."""

from __future__ import annotations

from fastapi import FastAPI
from starlette.testclient import TestClient

from recap_subworker.app import deps
from recap_subworker.app.routers import story_clustering
from recap_subworker.services.story_clusterer import StoryClustererService


def _build_app() -> FastAPI:
    """Minimal FastAPI app hosting only the story clustering router with DI overrides."""
    app = FastAPI()
    app.include_router(story_clustering.router, prefix="/v1")
    service = StoryClustererService()
    app.dependency_overrides[deps.get_story_clusterer_service_dep] = lambda: service
    return app


def test_cluster_stories_success():
    """Verify POST /v1/cluster-stories returns 200 with valid clusters and echoed params."""
    app = _build_app()
    client = TestClient(app)

    payload = {
        "items": [
            {
                "id": "11111111-1111-1111-1111-111111111111",
                "embedding": [1.0, 0.0, 0.0],
                "published_at": "2026-09-21T00:00:00Z",
            },
            {
                "id": "22222222-2222-2222-2222-222222222222",
                "embedding": [0.99, 0.01, 0.0],
                "published_at": "2026-09-21T01:00:00Z",
            },
            {
                "id": "33333333-3333-3333-3333-333333333333",
                "embedding": [0.0, 1.0, 0.0],
                "published_at": "2026-09-21T02:00:00Z",
            },
        ],
        "params": {
            "threshold": 0.78,
            "linkage": "average",
            "time_decay_per_day": 0.02,
            "min_cluster_size": 1,
        },
    }

    response = client.post("/v1/cluster-stories", json=payload)
    assert response.status_code == 200

    data = response.json()
    assert "clusters" in data
    assert "params" in data
    assert data["params"]["threshold"] == 0.78
    assert data["params"]["linkage"] == "average"
    assert data["params"]["time_decay_per_day"] == 0.02
    assert data["params"]["min_cluster_size"] == 1

    # First cluster should be the merged pair (size 2)
    assert len(data["clusters"]) == 2
    c0 = data["clusters"][0]
    assert c0["cluster_id"] == 0
    assert len(c0["member_ids"]) == 2
    assert c0["member_ids"] == [
        "11111111-1111-1111-1111-111111111111",
        "22222222-2222-2222-2222-222222222222",
    ]
    # Second cluster should be singleton (size 1)
    c1 = data["clusters"][1]
    assert c1["cluster_id"] == 1
    assert c1["member_ids"] == ["33333333-3333-3333-3333-333333333333"]


def test_cluster_stories_validation_empty_items_422():
    """Verify POST /v1/cluster-stories returns 422 when items list is empty (< 1 item)."""
    app = _build_app()
    client = TestClient(app)

    response = client.post("/v1/cluster-stories", json={"items": []})
    assert response.status_code == 422


def test_cluster_stories_validation_dimension_mismatch_422():
    """Verify POST /v1/cluster-stories returns 422 when embeddings have differing dimensions."""
    app = _build_app()
    client = TestClient(app)

    payload = {
        "items": [
            {
                "id": "11111111-1111-1111-1111-111111111111",
                "embedding": [1.0, 0.0, 0.0],
                "published_at": "2026-09-21T00:00:00Z",
            },
            {
                "id": "22222222-2222-2222-2222-222222222222",
                "embedding": [1.0, 0.0, 0.0, 0.5],  # 4-dim instead of 3-dim
                "published_at": "2026-09-21T00:00:00Z",
            },
        ],
    }

    response = client.post("/v1/cluster-stories", json=payload)
    assert response.status_code == 422


def test_cluster_stories_validation_threshold_outside_range_422():
    """Verify POST /v1/cluster-stories returns 422 when threshold is <= 0 or > 1."""
    app = _build_app()
    client = TestClient(app)

    base_item = {
        "id": "11111111-1111-1111-1111-111111111111",
        "embedding": [1.0, 0.0],
        "published_at": "2026-09-21T00:00:00Z",
    }

    # threshold = 0.0 (outside (0, 1])
    res0 = client.post(
        "/v1/cluster-stories",
        json={"items": [base_item], "params": {"threshold": 0.0}},
    )
    assert res0.status_code == 422

    # threshold = -0.5
    res_neg = client.post(
        "/v1/cluster-stories",
        json={"items": [base_item], "params": {"threshold": -0.5}},
    )
    assert res_neg.status_code == 422

    # threshold = 1.05
    res_high = client.post(
        "/v1/cluster-stories",
        json={"items": [base_item], "params": {"threshold": 1.05}},
    )
    assert res_high.status_code == 422


def test_cluster_stories_validation_threshold_boundary_1_success():
    """Verify POST /v1/cluster-stories accepts threshold = 1.0 (boundary of (0, 1])."""
    app = _build_app()
    client = TestClient(app)

    item = {
        "id": "11111111-1111-1111-1111-111111111111",
        "embedding": [1.0, 0.0],
        "published_at": "2026-09-21T00:00:00Z",
    }
    response = client.post(
        "/v1/cluster-stories",
        json={"items": [item], "params": {"threshold": 1.0}},
    )
    assert response.status_code == 200
    assert response.json()["params"]["threshold"] == 1.0


def test_cluster_stories_validation_unknown_linkage_422():
    """Verify POST /v1/cluster-stories returns 422 on unknown or unsupported linkage."""
    app = _build_app()
    client = TestClient(app)

    item = {
        "id": "11111111-1111-1111-1111-111111111111",
        "embedding": [1.0, 0.0],
        "published_at": "2026-09-21T00:00:00Z",
    }

    # "ward" is unsupported for precomputed distance matrix
    res_ward = client.post(
        "/v1/cluster-stories",
        json={"items": [item], "params": {"linkage": "ward"}},
    )
    assert res_ward.status_code == 422

    # unknown linkage
    res_unknown = client.post(
        "/v1/cluster-stories",
        json={"items": [item], "params": {"linkage": "centroid"}},
    )
    assert res_unknown.status_code == 422


def test_cluster_stories_default_params_echoed():
    """Verify POST /v1/cluster-stories fills in and echoes default params when omitted."""
    app = _build_app()
    client = TestClient(app)

    item = {
        "id": "11111111-1111-1111-1111-111111111111",
        "embedding": [1.0, 0.0],
        "published_at": "2026-09-21T00:00:00Z",
    }
    response = client.post("/v1/cluster-stories", json={"items": [item]})
    assert response.status_code == 200
    params = response.json()["params"]
    assert params["threshold"] == 0.78
    assert params["linkage"] == "average"
    assert params["time_decay_per_day"] == 0.02
    assert params["min_cluster_size"] == 1


def test_cluster_stories_per_language_min_size():
    """Verify POST /v1/cluster-stories supports language field and min_cluster_size_by_language."""
    app = _build_app()
    client = TestClient(app)

    payload = {
        "items": [
            {
                "id": "11111111-1111-1111-1111-111111111111",
                "embedding": [1.0, 0.0],
                "published_at": "2026-09-21T00:00:00Z",
                "language": "ja",
            },
            {
                "id": "22222222-2222-2222-2222-222222222222",
                "embedding": [0.99, 0.01],
                "published_at": "2026-09-21T01:00:00Z",
                "language": "ja",
            },
        ],
        "params": {
            "min_cluster_size_by_language": {"ja": 2, "en": 3},
        },
    }
    response = client.post("/v1/cluster-stories", json=payload)
    assert response.status_code == 200
    data = response.json()
    assert len(data["clusters"]) == 1
    assert data["params"]["min_cluster_size_by_language"] == {"ja": 2, "en": 3}
