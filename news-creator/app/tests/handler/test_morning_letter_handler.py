from unittest.mock import AsyncMock

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.models import (
    MorningLetterContent,
    MorningLetterResponse,
    MorningLetterSection,
    RecapSummaryMetadata,
)
from news_creator.handler.morning_letter_handler import create_morning_letter_router


def _build_request_payload():
    return {
        "target_date": "2026-04-17",
        "overnight_groups": [],
    }


def test_morning_letter_handler_success_200():
    """Happy path returns 200 OK with MorningLetterResponse."""
    usecase = AsyncMock()
    usecase.generate_letter.return_value = MorningLetterResponse(
        target_date="2026-04-17",
        edition_timezone="Asia/Tokyo",
        content=MorningLetterContent(
            schema_version=1,
            lead="本日の主要トピックです。",
            sections=[
                MorningLetterSection(
                    key="top3",
                    title="Top 3 Stories",
                    bullets=["トピック1の要約です。"],
                )
            ],
            generated_at="2026-04-17T06:00:00Z",
            source_recap_window_days=3,
        ),
        metadata=RecapSummaryMetadata(
            model="gemma4-e4b-12k",
            is_degraded=False,
        ),
    )

    app = FastAPI()
    app.include_router(create_morning_letter_router(usecase))
    client = TestClient(app)

    resp = client.post("/v1/morning-letter/generate", json=_build_request_payload())

    assert resp.status_code == 200
    body = resp.json()
    assert body["target_date"] == "2026-04-17"
    assert body["edition_timezone"] == "Asia/Tokyo"
    assert body["content"]["lead"] == "本日の主要トピックです。"
    assert body["metadata"]["is_degraded"] is False
    usecase.generate_letter.assert_awaited_once()


def test_value_error_returns_generic_message_not_exception_text():
    """Regression: ValueError from usecase must not leak to the client body.

    CodeQL py/stack-trace-exposure flagged the old ``str(e)`` path.
    """
    usecase = AsyncMock()
    secret = "internal validation failed at /app/news_creator/usecase/step3.py:214"
    usecase.generate_letter.side_effect = ValueError(secret)

    app = FastAPI()
    app.include_router(create_morning_letter_router(usecase))
    client = TestClient(app)

    resp = client.post("/v1/morning-letter/generate", json=_build_request_payload())

    assert resp.status_code == 400
    body = resp.json()
    assert body.get("error") == "Invalid request"
    assert secret not in resp.text


def test_unexpected_exception_returns_500_without_leaking_details():
    """Unexpected exception returns 500 with generic error message."""
    usecase = AsyncMock()
    secret = "database connection pool exhausted at internal host"
    usecase.generate_letter.side_effect = RuntimeError(secret)

    app = FastAPI()
    app.include_router(create_morning_letter_router(usecase))
    client = TestClient(app)

    resp = client.post("/v1/morning-letter/generate", json=_build_request_payload())

    assert resp.status_code == 500
    body = resp.json()
    assert body.get("error") == "Internal server error"
    assert secret not in resp.text


def test_missing_target_date_returns_422():
    """Missing required target_date fails Pydantic validation with 422."""
    usecase = AsyncMock()

    app = FastAPI()
    app.include_router(create_morning_letter_router(usecase))
    client = TestClient(app)

    resp = client.post("/v1/morning-letter/generate", json={"overnight_groups": []})

    assert resp.status_code == 422
    usecase.generate_letter.assert_not_called()
