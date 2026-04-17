from unittest.mock import AsyncMock

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.handler.morning_letter_handler import create_morning_letter_router


def _build_request_payload():
    return {
        "target_date": "2026-04-17",
        "overnight_groups": [],
    }


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
