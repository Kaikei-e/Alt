from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock
from uuid import uuid4

from news_creator.domain.models import RecapSummary, RecapSummaryMetadata, RecapSummaryResponse
from news_creator.handler.recap_summary_handler import create_recap_summary_router


def _build_request_payload():
    return {
        "job_id": str(uuid4()),
        "genre": "world",
        "clusters": [
            {
                "cluster_id": 0,
                "representative_sentences": ["Sample sentence A."],
                "top_terms": ["sample"],
            }
        ],
        "options": {"max_bullets": 3},
    }


def test_recap_summary_handler_success():
    usecase = AsyncMock()
    response = RecapSummaryResponse(
        job_id=uuid4(),
        genre="world",
        summary=RecapSummary(
            title="世界情勢のまとめ",
            bullets=["世界経済は緩やかに回復している。"],
            language="ja",
        ),
        metadata=RecapSummaryMetadata(model="gemma3:4b"),
    )
    usecase.generate_summary.return_value = response

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == 200
    assert resp.json()["summary"]["title"] == "世界情勢のまとめ"
    usecase.generate_summary.assert_awaited_once()


def test_recap_summary_handler_value_error():
    usecase = AsyncMock()
    usecase.generate_summary.side_effect = ValueError("invalid payload")

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == 400
    assert resp.json()["detail"] == "invalid payload"


def test_recap_summary_handler_runtime_error():
    usecase = AsyncMock()
    usecase.generate_summary.side_effect = RuntimeError("llm failure")

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == 502
    assert resp.json()["detail"] == "llm failure"

