"""Phase 6 E2E tests for /v1/runs via httpx.ASGITransport.

Replaces the synchronous ``TestClient`` pattern with an async transport
so integration tests exercise the real FastAPI lifespan + dependency
injection chain without the ``TestClient.__enter__`` monkey-patch used
in unit tests. The autouse conftest override still short-circuits the
heavy ``ServiceContainer`` properties (see [[000728]] Phase 1 unblock),
keeping the test suite fast while verifying handler + usecase glue.
"""

from __future__ import annotations

from uuid import uuid4

import httpx
import pytest

from recap_subworker.app import deps
from recap_subworker.app.main import create_app
from recap_subworker.db.dao import RunRecord
from recap_subworker.services.run_manager import RunSubmission
from recap_subworker.usecase.submit_run import GetRunUsecase, SubmitRunUsecase


class _InMemoryRunStore:
    def __init__(self) -> None:
        self.records: dict[int, RunRecord] = {}
        self._next_id = 1

    async def create_run(self, submission: RunSubmission) -> RunRecord:
        run_id = self._next_id
        self._next_id += 1
        record = RunRecord(
            run_id=run_id,
            job_id=submission.job_id,
            genre=submission.genre,
            status="running",
            cluster_count=0,
            request_payload={},
            response_payload=None,
            error_message=None,
        )
        self.records[run_id] = record
        return record

    async def get_run(self, run_id: int) -> RunRecord | None:
        return self.records.get(run_id)


def _cluster_payload() -> dict[str, object]:
    return {
        "params": {
            "max_sentences_total": 2000,
            "umap_n_components": 25,
            "hdbscan_min_cluster_size": 5,
            "mmr_lambda": 0.35,
        },
        "documents": [
            {
                "article_id": f"art-{i}",
                "paragraphs": ["x" * 40],
            }
            for i in range(5)
        ],
    }


@pytest.mark.asyncio
async def test_post_then_get_runs_round_trip_via_asgi_transport() -> None:
    app = create_app()
    store = _InMemoryRunStore()
    app.dependency_overrides[deps.get_submit_run_usecase_dep] = lambda: SubmitRunUsecase(
        submitter=store,
    )
    app.dependency_overrides[deps.get_get_run_usecase_dep] = lambda: GetRunUsecase(
        reader=store,
    )

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as client:
        job_id = str(uuid4())
        create = await client.post(
            "/v1/runs",
            json=_cluster_payload(),
            headers={"X-Alt-Job-Id": job_id, "X-Alt-Genre": "ai"},
        )
        assert create.status_code == 202
        run_id = create.json()["run_id"]

        read = await client.get(f"/v1/runs/{run_id}")
        assert read.status_code == 200
        body = read.json()
        assert body["run_id"] == run_id
        assert body["status"] == "running"


@pytest.mark.asyncio
async def test_missing_run_returns_404_via_asgi_transport() -> None:
    app = create_app()
    app.dependency_overrides[deps.get_get_run_usecase_dep] = lambda: GetRunUsecase(
        reader=_InMemoryRunStore(),
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as client:
        resp = await client.get("/v1/runs/999")
        assert resp.status_code == 404
