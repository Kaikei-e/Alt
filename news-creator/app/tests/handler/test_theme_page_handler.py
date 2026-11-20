"""Tests for theme page handler."""

from __future__ import annotations

from collections.abc import Callable
from http import HTTPStatus
from typing import Any
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.errors import (
    CardGenerationInvariantError,
    PreemptedException,
    QueueFullError,
)
from news_creator.domain.models import (
    CardGenerationRejectedError,
    ThemeGenerateResponse,
    ThemeSentence,
    ThemeTimelineEntry,
)
from news_creator.handler.theme_page_handler import create_theme_page_router


def _build_theme_request_payload(**overrides: object) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "job_id": str(uuid4()),
        "theme_id": str(uuid4()),
        "language": "ja",
        "items": [
            {
                "n": 1,
                "title": "テスト記事1",
                "published_at": "2026-11-18",
                "text": "ソラリス社は分散ログ基盤を公開した。",
            },
            {
                "n": 2,
                "title": "テスト記事2",
                "published_at": None,
                "text": "2026-11-19に暗号化機能が追加された。",
            },
        ],
        "new_item_ns": [2],
        "previous": {
            "title": "前回のテーマ",
            "key_points": ["ソラリス社が開発を進めていた。[1]"],
            "open_questions": ["暗号化方式の詳細は未定だった。[1]"],
        },
        "failure_notes": [],
    }
    payload.update(overrides)
    return payload


def _build_app(usecase: AsyncMock) -> FastAPI:
    app = FastAPI()
    app.include_router(create_theme_page_router(usecase))
    return app


def test_theme_page_handler_success_200() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.return_value = ThemeGenerateResponse(
        title="ソラリス社の分散ログ基盤",
        key_points=[
            ThemeSentence(text="ソラリス社は分散ログ基盤を公開した。[1]", refs=[1]),
        ],
        timeline=[
            ThemeTimelineEntry(
                date="2026-11-18",
                text="ソラリス社が分散ログ基盤を公開した。[1]",
                refs=[1],
            ),
        ],
        what_changed=[
            ThemeSentence(text="暗号化機能が新たに追加された。[2]", refs=[2]),
        ],
        open_questions=[],
        model="gemma4-e4b-12k",
    )

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.OK
    data = resp.json()
    assert data == {
        "title": "ソラリス社の分散ログ基盤",
        "key_points": [
            {"text": "ソラリス社は分散ログ基盤を公開した。[1]", "refs": [1]}
        ],
        "timeline": [
            {
                "date": "2026-11-18",
                "text": "ソラリス社が分散ログ基盤を公開した。[1]",
                "refs": [1],
            }
        ],
        "what_changed": [{"text": "暗号化機能が新たに追加された。[2]", "refs": [2]}],
        "open_questions": [],
        "model": "gemma4-e4b-12k",
    }
    usecase.generate_theme_page.assert_awaited_once()


@pytest.mark.parametrize(
    "mutator",
    [
        lambda p: p.update({"job_id": "not-a-uuid"}),
        lambda p: p.update({"theme_id": "not-a-uuid"}),
        lambda p: p.update({"language": "en"}),
        lambda p: p.update({"items": []}),
        lambda p: p.update(
            {
                "items": [
                    {
                        "n": i + 1,
                        "title": f"タイトル{i + 1}",
                        "published_at": "2026-11-18",
                        "text": f"本文{i + 1}",
                    }
                    for i in range(9)
                ],
                "new_item_ns": [1],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 0, "title": "タイトル", "published_at": None, "text": "本文"}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": -1, "title": "タイトル", "published_at": None, "text": "本文"}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 1, "title": "記事A", "published_at": None, "text": "本文A"},
                    {"n": 1, "title": "記事B", "published_at": None, "text": "本文B"},
                ],
                "new_item_ns": [1],
            }
        ),
        lambda p: p.update(
            {
                "items": [{"n": 1, "title": "", "published_at": None, "text": "本文"}],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 1, "title": "   ", "published_at": None, "text": "本文"}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 1, "title": "あ" * 121, "published_at": None, "text": "本文"}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 1, "title": "タイトル", "published_at": None, "text": ""}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {"n": 1, "title": "タイトル", "published_at": None, "text": "   "}
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {
                        "n": 1,
                        "title": "タイトル",
                        "published_at": None,
                        "text": "あ" * 401,
                    }
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {
                        "n": 1,
                        "title": "タイトル",
                        "published_at": "2026-02-30",
                        "text": "本文",
                    }
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update(
            {
                "items": [
                    {
                        "n": 1,
                        "title": "タイトル",
                        "published_at": "2026/11/18",
                        "text": "本文",
                    }
                ],
                "new_item_ns": [],
            }
        ),
        lambda p: p.update({"new_item_ns": [99]}),
        lambda p: p.update(
            {
                "previous": {
                    "title": "",
                    "key_points": [],
                    "open_questions": [],
                }
            }
        ),
        lambda p: p.update(
            {
                "previous": {
                    "title": "あ" * 121,
                    "key_points": [],
                    "open_questions": [],
                }
            }
        ),
        lambda p: p.update(
            {
                "previous": {
                    "title": "前回のテーマ",
                    "key_points": [f"要点{i}" for i in range(8)],
                    "open_questions": [],
                }
            }
        ),
        lambda p: p.update(
            {
                "previous": {
                    "title": "前回のテーマ",
                    "key_points": ["あ" * 201],
                    "open_questions": [],
                }
            }
        ),
        lambda p: p.update(
            {
                "previous": {
                    "title": "前回のテーマ",
                    "key_points": [],
                    "open_questions": [f"疑問{i}" for i in range(5)],
                }
            }
        ),
        lambda p: p.update(
            {
                "previous": {
                    "title": "前回のテーマ",
                    "key_points": [],
                    "open_questions": ["あ" * 201],
                }
            }
        ),
        lambda p: p.update({"failure_notes": [f"note{i}" for i in range(6)]}),
        lambda p: p.update({"failure_notes": ["あ" * 121]}),
        lambda p: p.update({"failure_notes": ["   "]}),
    ],
)
def test_theme_page_handler_validation_422(
    mutator: Callable[[dict[str, Any]], None],
) -> None:
    usecase = AsyncMock()
    client = TestClient(_build_app(usecase))
    payload = _build_theme_request_payload()
    mutator(payload)

    resp = client.post("/v1/themes/generate", json=payload)

    assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
    assert resp.headers.get("X-Card-Rejection") is None
    usecase.generate_theme_page.assert_not_called()


def test_theme_page_handler_rejected_422() -> None:
    rejected_attempts = 2
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = CardGenerationRejectedError(
        reason="empty_output",
        attempts=rejected_attempts,
        raw_text='{"title":"テスト","key_points":[]}',
    )

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
    assert resp.headers.get("X-Card-Rejection") == "1"
    data = resp.json()
    assert data["reason"] == "empty_output"
    assert data["attempts"] == rejected_attempts


def test_theme_page_handler_queue_full_429() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = QueueFullError("Queue depth exceeded")

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.TOO_MANY_REQUESTS
    assert resp.json() == {"error": "queue full"}
    assert resp.headers.get("Retry-After") == "30"


def test_theme_page_handler_value_error_400() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = ValueError("bad request")

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.BAD_REQUEST
    assert resp.json()["detail"] == "Invalid request"


def test_theme_page_handler_preempted_502() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = PreemptedException("preempted")

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.BAD_GATEWAY
    assert resp.json()["detail"] == "Upstream service error"


def test_theme_page_handler_runtime_error_502() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = RuntimeError("Ollama unavailable")

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.BAD_GATEWAY
    assert resp.json()["detail"] == "Upstream service error"


def test_theme_page_handler_invariant_error_500() -> None:
    usecase = AsyncMock()
    usecase.generate_theme_page.side_effect = CardGenerationInvariantError("invariant")

    client = TestClient(_build_app(usecase))
    resp = client.post("/v1/themes/generate", json=_build_theme_request_payload())

    assert resp.status_code == HTTPStatus.INTERNAL_SERVER_ERROR
    assert resp.json()["detail"] == "Internal server error"
