"""FastAPI endpoint tests for run management."""

from __future__ import annotations

from uuid import uuid4

from fastapi.testclient import TestClient

from recap_subworker.app import deps
from recap_subworker.app.main import create_app
from recap_subworker.db.dao import RunRecord


class FakeRunManager:
    def __init__(self, record: RunRecord | None = None):
        self.record = record
        self.created = False

    async def create_run(self, submission):
        self.created = True
        return self.record

    async def get_run(self, run_id: int):
        return self.record


def _make_payload():
    documents = [
        {
            "article_id": f"art-{i}",
            "paragraphs": ["x" * 35],
        }
        for i in range(10)
    ]
    return {
        "params": {
            "max_sentences_total": 2000,
            "umap_n_components": 25,
            "hdbscan_min_cluster_size": 5,
            "mmr_lambda": 0.35,
        },
        "documents": documents,
    }


def test_post_runs_returns_accepted(monkeypatch):
    job_id = uuid4()
    record = RunRecord(
        run_id=1,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )
    manager = FakeRunManager(record)
    app = create_app()
    app.dependency_overrides[deps.get_run_manager_dep] = lambda: manager
    client = TestClient(app)

    response = client.post(
        "/v1/runs",
        json=_make_payload(),
        headers={"X-Alt-Job-Id": str(job_id), "X-Alt-Genre": "ai"},
    )

    assert response.status_code == 202
    assert response.json()["run_id"] == 1


def test_post_runs_requires_job_id_header():
    app = create_app()
    client = TestClient(app)

    response = client.post("/v1/runs", json=_make_payload(), headers={"X-Alt-Genre": "ai"})

    assert response.status_code == 422


def test_get_runs_not_found(monkeypatch):
    app = create_app()

    class EmptyManager(FakeRunManager):
        async def get_run(self, run_id: int):  # type: ignore[override]
            return None

    app.dependency_overrides[deps.get_run_manager_dep] = lambda: EmptyManager()
    client = TestClient(app)

    response = client.get("/v1/runs/999")

    assert response.status_code == 404
