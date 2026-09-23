"""Tests for recap card handler."""

from unittest.mock import AsyncMock
from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.models import (
    CardContent,
    CardGenerateResponse,
    CardGenerationMetadata,
    CardGenerationRejectedError,
    CardSentence,
)
from news_creator.gateway.hybrid_priority_semaphore import QueueFullError
from news_creator.handler.recap_card_handler import create_recap_card_router


def _build_card_request_payload():
    return {
        "job_id": str(uuid4()),
        "candidate_id": str(uuid4()),
        "prompt_version": "recap_card.v1",
        "items": [
            {
                "n": 1,
                "feed_id": str(uuid4()),
                "title": "テスト記事1",
                "host": "example.com",
                "url": "https://example.com/1",
                "pub_date": "2026-09-21T00:00:00Z",
                "lede": "テストリード1",
            },
            {
                "n": 2,
                "feed_id": str(uuid4()),
                "title": "テスト記事2",
                "host": "example.org",
                "url": "https://example.org/2",
                "pub_date": None,
                "lede": "テストリード2",
            },
        ],
        "revision_note": None,
    }


def test_recap_card_handler_success_200():
    usecase = AsyncMock()
    response = CardGenerateResponse(
        card=CardContent(
            headline_ja="ソラリス社、分散ログ基盤の次期版を公開",
            what_ja=[
                CardSentence(
                    text="ソラリス社はログ収集エンジンを公開した。[1]", refs=[1]
                ),
                CardSentence(text="メモリ使用量が40%削減された。[1][2]", refs=[1, 2]),
            ],
            why_ja=CardSentence(text="サーバ費用が年間25%削減される。[1]", refs=[1]),
            used_refs=[1, 2],
        ),
        generation=CardGenerationMetadata(
            model="gemma4-e4b-12k",
            prompt_version="recap_card.v1",
            cache_hit=False,
            prompt_tokens=150,
            completion_tokens=60,
            ms=300,
            raw_text="raw output",
        ),
    )
    usecase.generate_card.return_value = response

    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    payload = _build_card_request_payload()
    resp = client.post("/v1/cards/generate", json=payload)

    assert resp.status_code == 200
    data = resp.json()
    assert data["card"]["headline_ja"] == "ソラリス社、分散ログ基盤の次期版を公開"
    assert len(data["card"]["what_ja"]) == 2
    assert data["card"]["why_ja"]["text"] == "サーバ費用が年間25%削減される。[1]"
    assert data["card"]["used_refs"] == [1, 2]
    assert data["generation"]["cache_hit"] is False
    assert data["generation"]["prompt_version"] == "recap_card.v1"
    usecase.generate_card.assert_awaited_once()


def test_recap_card_handler_rejected_422():
    usecase = AsyncMock()
    usecase.generate_card.side_effect = CardGenerationRejectedError(
        reason="parse_failed",
        attempts=2,
        raw_text="不正なタグ出力",
    )

    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    payload = _build_card_request_payload()
    resp = client.post("/v1/cards/generate", json=payload)

    assert resp.status_code == 422
    assert resp.headers.get("X-Card-Rejection") == "1"
    data = resp.json()
    assert data["reason"] == "parse_failed"
    assert data["attempts"] == 2
    assert data["raw_text"] == "不正なタグ出力"


def test_recap_card_handler_unknown_ref_422():
    usecase = AsyncMock()
    usecase.generate_card.side_effect = CardGenerationRejectedError(
        reason="unknown_ref",
        attempts=2,
        raw_text="未知の参照 [99] を含む出力",
    )

    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    payload = _build_card_request_payload()
    resp = client.post("/v1/cards/generate", json=payload)

    assert resp.status_code == 422
    assert resp.headers.get("X-Card-Rejection") == "1"
    data = resp.json()
    assert data["reason"] == "unknown_ref"
    assert data["attempts"] == 2


def test_recap_card_handler_queue_full_429():
    usecase = AsyncMock()
    usecase.generate_card.side_effect = QueueFullError("Queue depth exceeded")

    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    payload = _build_card_request_payload()
    resp = client.post("/v1/cards/generate", json=payload)

    assert resp.status_code == 429
    assert resp.json() == {"error": "queue full"}
    assert resp.headers.get("Retry-After") == "30"


def test_recap_card_handler_request_validation_failure_422_without_rejection_header():
    """FastAPI request-validation 422 must NOT carry X-Card-Rejection header."""
    usecase = AsyncMock()
    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    # Missing required fields
    resp = client.post("/v1/cards/generate", json={"invalid": "payload"})

    assert resp.status_code == 422
    assert resp.headers.get("X-Card-Rejection") is None
    data = resp.json()
    assert "detail" in data
    usecase.generate_card.assert_not_called()


def test_recap_card_handler_language_rejection_422_includes_ratio_and_counts():
    """Verify 422 language rejection includes measured_ratio and character_counts next to raw_text."""
    usecase = AsyncMock()
    usecase.generate_card.side_effect = CardGenerationRejectedError(
        reason="language",
        attempts=2,
        raw_text="English headline here",
        measured_ratio=0.42,
        character_counts={"japanese": 42, "substantive": 100, "total": 120},
    )

    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)

    payload = _build_card_request_payload()
    resp = client.post("/v1/cards/generate", json=payload)

    assert resp.status_code == 422
    assert resp.headers.get("X-Card-Rejection") == "1"
    data = resp.json()
    assert data["reason"] == "language"
    assert data["attempts"] == 2
    assert data["raw_text"] == "English headline here"
    assert data["measured_ratio"] == 0.42
    assert data["character_counts"] == {
        "japanese": 42,
        "substantive": 100,
        "total": 120,
    }


def test_recap_card_handler_structured_logging_on_rejection(caplog):
    """Verify structured log has raw_text_len, measured_ratio for language, and never raw_text."""
    import logging

    usecase = AsyncMock()
    app = FastAPI()
    app.include_router(create_recap_card_router(usecase))
    client = TestClient(app)
    payload = _build_card_request_payload()

    # 1. Parse failed rejection: logs raw_text_len, never raw_text
    usecase.generate_card.side_effect = CardGenerationRejectedError(
        reason="parse_failed",
        attempts=2,
        raw_text="SECRET_RAW_TEXT_PARSE_FAILED",
    )
    with caplog.at_level(logging.WARNING):
        caplog.clear()
        resp = client.post("/v1/cards/generate", json=payload)
        assert resp.status_code == 422

        rejection_records = [
            r for r in caplog.records if r.getMessage() == "Card generation rejected"
        ]
        assert len(rejection_records) == 1
        record = rejection_records[0]
        assert getattr(record, "raw_text_len", None) == len(
            "SECRET_RAW_TEXT_PARSE_FAILED"
        )
        assert getattr(record, "reason", None) == "parse_failed"
        assert getattr(record, "attempts", None) == 2
        # Never log raw_text itself in extra
        assert not hasattr(record, "raw_text")
        assert "SECRET_RAW_TEXT_PARSE_FAILED" not in record.getMessage()

    # 2. Language rejection: logs raw_text_len and measured_ratio
    usecase.generate_card.side_effect = CardGenerationRejectedError(
        reason="language",
        attempts=2,
        raw_text="SECRET_RAW_TEXT_LANGUAGE",
        measured_ratio=0.35,
        character_counts={"japanese": 7, "substantive": 20, "total": 25},
    )
    with caplog.at_level(logging.WARNING):
        caplog.clear()
        resp = client.post("/v1/cards/generate", json=payload)
        assert resp.status_code == 422

        rejection_records = [
            r for r in caplog.records if r.getMessage() == "Card generation rejected"
        ]
        assert len(rejection_records) == 1
        record = rejection_records[0]
        assert getattr(record, "raw_text_len", None) == len("SECRET_RAW_TEXT_LANGUAGE")
        assert getattr(record, "reason", None) == "language"
        assert getattr(record, "measured_ratio", None) == 0.35
        # Never log raw_text itself in extra
        assert not hasattr(record, "raw_text")
        assert "SECRET_RAW_TEXT_LANGUAGE" not in record.getMessage()
