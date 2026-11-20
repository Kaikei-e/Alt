from http import HTTPStatus
from typing import Any
from unittest.mock import AsyncMock
from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.errors import PreemptedException, QueueFullError
from news_creator.domain.models import (
    BatchRecapSummaryError,
    BatchRecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
    RecapSummaryResponse,
)
from news_creator.handler.recap_summary_handler import create_recap_summary_router


def _build_request_payload() -> dict[str, Any]:
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


def test_recap_summary_handler_success() -> None:
    usecase = AsyncMock()
    response = RecapSummaryResponse(
        job_id=uuid4(),
        genre="world",
        summary=RecapSummary(
            title="世界情勢のまとめ",
            bullets=["世界経済は緩やかに回復している。"],
            language="ja",
        ),
        metadata=RecapSummaryMetadata(model="gemma4-e4b-q4km"),
    )
    usecase.generate_summary.return_value = response

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == HTTPStatus.OK
    assert resp.json()["summary"]["title"] == "世界情勢のまとめ"
    usecase.generate_summary.assert_awaited_once()


def test_recap_summary_handler_value_error() -> None:
    usecase = AsyncMock()
    secret = "invalid payload at /app/news_creator/usecase/recap_summary_usecase.py:80"  # noqa: S105 - test fixture string simulating leaked internal detail
    usecase.generate_summary.side_effect = ValueError(secret)

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == HTTPStatus.BAD_REQUEST
    assert resp.json()["detail"] == "Invalid request"
    assert secret not in resp.text


def test_recap_summary_handler_runtime_error() -> None:
    usecase = AsyncMock()
    secret = "llm failure at http://ollama.internal:11434"  # noqa: S105 - test fixture string simulating leaked internal detail
    usecase.generate_summary.side_effect = RuntimeError(secret)

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == HTTPStatus.BAD_GATEWAY
    assert resp.json()["detail"] == "Upstream service error"
    assert secret not in resp.text


def test_recap_summary_handler_queue_full_returns_429() -> None:
    """Queue saturation is expected backpressure (2026-02 queue-saturation design);
    it must surface as HTTP 429 with Retry-After, mirroring /api/v1/summarize and
    /api/chat, never fall through to the generic 500 branch.
    """
    usecase = AsyncMock()
    usecase.generate_summary.side_effect = QueueFullError("Queue depth 10 >= max 10")

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == HTTPStatus.TOO_MANY_REQUESTS
    assert "queue full" in resp.json()["error"]
    assert resp.headers.get("Retry-After") == "30"


def test_recap_summary_handler_preempted_returns_502() -> None:
    """Pins the contract: PreemptedException must surface as HTTP 502 (retryable
    upstream signal) — never 500.

    Earlier behaviour leaked PreemptedException into the generic Exception
    branch, returning 500 'Internal server error', which clients read as a
    permanent failure rather than 'BE was preempted by an RT request, please
    retry.' That mistranslation is what produced the May 3 2026 17:20-17:36
    error storm.
    """
    usecase = AsyncMock()
    secret = "request preempted during generation at slot 3"  # noqa: S105 - test fixture string simulating leaked internal detail
    usecase.generate_summary.side_effect = PreemptedException(secret)

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_request_payload()
    resp = client.post("/v1/summary/generate", json=payload)

    assert resp.status_code == HTTPStatus.BAD_GATEWAY
    assert resp.json()["detail"] == "Upstream service error"
    assert secret not in resp.text


# ============================================================================
# Batch Endpoint Tests
# ============================================================================


def _build_batch_request_payload() -> dict[str, Any]:
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
                        "representative_sentences": [
                            {"text": "Politics news sentence."}
                        ],
                        "top_terms": ["politics"],
                    }
                ],
                "options": {"max_bullets": 3},
            },
        ]
    }


def test_batch_recap_summary_handler_success() -> None:
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
                metadata=RecapSummaryMetadata(model="gemma4-e4b-q4km"),
            ),
            RecapSummaryResponse(
                job_id=job_id_2,
                genre="politics",
                summary=RecapSummary(
                    title="政治要約",
                    bullets=["政治の要点"],
                    language="ja",
                ),
                metadata=RecapSummaryMetadata(model="gemma4-e4b-q4km"),
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

    assert resp.status_code == HTTPStatus.OK
    data = resp.json()
    assert len(data["responses"]) == len(batch_response.responses)
    assert len(data["errors"]) == 0
    assert data["responses"][0]["genre"] == "tech"
    assert data["responses"][1]["genre"] == "politics"
    usecase.generate_batch_summary.assert_awaited_once()


def test_batch_recap_summary_handler_partial_failure() -> None:
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
                metadata=RecapSummaryMetadata(model="gemma4-e4b-q4km"),
            ),
        ],
        errors=[
            BatchRecapSummaryError(
                job_id=job_id_2,
                genre="politics",
                error="summary_generation_failed",
            ),
        ],
    )

    usecase.generate_batch_summary.return_value = batch_response

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    payload = _build_batch_request_payload()
    resp = client.post("/v1/summary/generate/batch", json=payload)

    assert resp.status_code == HTTPStatus.OK
    data = resp.json()
    assert len(data["responses"]) == 1
    assert len(data["errors"]) == 1
    assert data["errors"][0]["genre"] == "politics"


def test_batch_recap_summary_handler_invalid_request() -> None:
    """Test batch endpoint with invalid request (empty requests)."""
    usecase = AsyncMock()

    app = FastAPI()
    app.include_router(create_recap_summary_router(usecase))
    client = TestClient(app)

    # Empty requests should fail validation
    payload = {"requests": []}
    resp = client.post("/v1/summary/generate/batch", json=payload)

    assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY  # Validation error
