from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock
from uuid import uuid4

from news_creator.domain.models import (
    BatchRecapSummaryError,
    BatchRecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
    RecapSummaryResponse,
)
from news_creator.handler.recap_summary_handler import create_recap_summary_router


def _build_request_payload():
    return {
        "job_id": str(uuid4()),
        "genre": "world",
        "clusters": [
            {
                "cluster_id": 0,
                "representative_sentences": [{"text": "Sample sentence A."}],
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


# ============================================================================
# Batch Endpoint Tests
# ============================================================================


def _build_batch_request_payload():
    """Build a batch request payload for testing."""
    return {
        "requests": [
            {
                "job_id": str(uuid4()),
                "genre": "tech",
                "clusters": [
                    {
                        "cluster_id": 0,
                        "representative_sentences": [{"text": "Tech news sentence."}],
                        "top_terms": ["tech"],
                    }
                ],
                "options": {"max_bullets": 3},
            },
            {
                "job_id": str(uuid4()),
                "genre": "politics",
                "clusters": [
                    {
                        "cluster_id": 0,
                        "representative_sentences": [{"text": "Politics news sentence."}],
                        "top_terms": ["politics"],
                    }
                ],
                "options": {"max_bullets": 3},
            },
        ]
    }


def test_batch_recap_summary_handler_success():
    """Test batch endpoint with successful responses."""
    usecase = AsyncMock()

    job_id_1 = uuid4()
    job_id_2 = uuid4()

    batch_response = BatchRecapSummaryResponse(
        responses=[
            RecapSummaryResponse(
                job_id=job_id_1,
                genre="tech",
                summary=RecapSummary(
                    title="テック要約",
                    bullets=["テックの要点"],
                    language="ja",
                ),
                metadata=RecapSummaryMetadata(model="gemma3:4b"),
            ),
            RecapSummaryResponse(
                job_id=job_id_2,
                genre="politics",
                summary=RecapSummary(
                    title="政治要約",
                    bullets=["政治の要点"],
                    language="ja",
                ),
                metadata=RecapSummaryMetadata(model="gemma3:4b"),
            ),
        ],
        errors=[],
    )
    usecase.generate_batch_summary.return_value = batch_response

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_batch_request_payload()
    resp = client.post("/v1/summary/generate/batch", json=payload)

    assert resp.status_code == 200
    data = resp.json()
    assert len(data["responses"]) == 2
    assert len(data["errors"]) == 0
    assert data["responses"][0]["genre"] == "tech"
    assert data["responses"][1]["genre"] == "politics"
    usecase.generate_batch_summary.assert_awaited_once()


def test_batch_recap_summary_handler_partial_failure():
    """Test batch endpoint with partial failures."""
    usecase = AsyncMock()

    job_id_1 = uuid4()
    job_id_2 = uuid4()

    # Define the batch response with one success and one error
    batch_response = BatchRecapSummaryResponse(
        responses=[
            RecapSummaryResponse(
                job_id=job_id_1,
                genre="tech",
                summary=RecapSummary(
                    title="テック要約",
                    bullets=["テックの要点"],
                    language="ja",
                ),
                metadata=RecapSummaryMetadata(model="gemma3:4b"),
            ),
        ],
        errors=[
            BatchRecapSummaryError(
                job_id=job_id_2,
                genre="politics",
                error="LLM service unavailable",
            ),
        ],
    )

    usecase.generate_batch_summary.return_value = batch_response

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_batch_request_payload()
    resp = client.post("/v1/summary/generate/batch", json=payload)

    assert resp.status_code == 200
    data = resp.json()
    assert len(data["responses"]) == 1
    assert len(data["errors"]) == 1
    assert data["errors"][0]["genre"] == "politics"


def test_batch_recap_summary_handler_invalid_request():
    """Test batch endpoint with invalid request (empty requests)."""
    usecase = AsyncMock()

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    # Empty requests should fail validation
    payload = {"requests": []}
    resp = client.post("/v1/summary/generate/batch", json=payload)

    assert resp.status_code == 422  # Validation error

