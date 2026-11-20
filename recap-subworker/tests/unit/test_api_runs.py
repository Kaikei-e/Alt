"""FastAPI endpoint tests for run management."""

from __future__ import annotations

from http import HTTPStatus
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from recap_subworker.app import deps
from recap_subworker.app.main import create_app
from recap_subworker.db.dao import RunRecord
from recap_subworker.services.run_manager import RunSubmission
from recap_subworker.usecase.submit_run import GetRunUsecase, SubmitRunUsecase

pytestmark = pytest.mark.usefixtures("stub_startup_sweep")


class FakeRunManager:
    def __init__(self, record: RunRecord | None = None) -> None:
        self.record = record
        self.created = False

    async def create_run(self, submission: RunSubmission) -> RunRecord:
        self.created = True
        assert self.record is not None
        return self.record

    async def get_run(self, run_id: int) -> RunRecord | None:
        return self.record


def _make_payload() -> dict[str, object]:
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


def test_post_runs_returns_accepted(monkeypatch: pytest.MonkeyPatch) -> None:
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

    app.dependency_overrides[deps.get_submit_run_usecase_dep] = lambda: SubmitRunUsecase(
        submitter=manager,
    )
    app.dependency_overrides[deps.get_get_run_usecase_dep] = lambda: GetRunUsecase(
        reader=manager,
    )

    with TestClient(app) as client:
        response = client.post(
            "/v1/runs",
            json=_make_payload(),
            headers={"X-Alt-Job-Id": str(job_id), "X-Alt-Genre": "ai"},
        )

    assert response.status_code == HTTPStatus.ACCEPTED
    assert response.json()["run_id"] == 1


def test_post_runs_requires_job_id_header() -> None:
    app = create_app()
    with TestClient(app) as client:
        response = client.post("/v1/runs", json=_make_payload(), headers={"X-Alt-Genre": "ai"})

    assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY


def test_get_runs_not_found(monkeypatch: pytest.MonkeyPatch) -> None:
    app = create_app()

    class EmptyManager(FakeRunManager):
        async def get_run(self, run_id: int) -> None:  # type: ignore[override]
            return None

    app.dependency_overrides[deps.get_get_run_usecase_dep] = lambda: GetRunUsecase(
        reader=EmptyManager(),
    )
    with TestClient(app) as client:
        response = client.get("/v1/runs/999")

    assert response.status_code == HTTPStatus.NOT_FOUND
