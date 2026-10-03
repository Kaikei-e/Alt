"""C-05 security tests: bearer auth on /api/v1/evaluations/* endpoints.

Old anonymous success tests should fail RED (they bypass auth).
Health and metrics endpoints remain unauthed.
"""
import secrets
import tempfile
import os
from pathlib import Path
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient
import contextlib

from recap_evaluator.main import create_app
from recap_evaluator.domain.models import EvaluationRun, EvaluationType, AlertLevel
from datetime import datetime, timezone
from uuid import uuid4

VALID_TOKEN = secrets.token_urlsafe(32)
WRONG_TOKEN = secrets.token_urlsafe(32)

@pytest.fixture
def auth_file():
    with tempfile.NamedTemporaryFile(mode='w+', delete=False) as f:
        f.write(VALID_TOKEN)
        path = f.name

    yield path
    os.remove(path)

@pytest.fixture
def test_app(auth_file):
    os.environ["EVALUATOR_API_TOKEN_FILE"] = auth_file
    app = create_app()

    @contextlib.asynccontextmanager
    async def dummy_lifespan(app):
        app.state.run_evaluation = AsyncMock()
        app.state.run_evaluation.execute.return_value = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[uuid4()],
            created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
            window_days=7,
            overall_alert_level=AlertLevel.OK,
        )
        app.state.get_metrics = AsyncMock()
        app.state.get_metrics.get_evaluation_history.return_value = []
        app.state.get_metrics.get_latest.return_value = {
            "genre_macro_f1": 0.8,
            "genre_alert_level": "ok",
            "cluster_avg_silhouette": 0.5,
            "cluster_alert_level": "ok",
            "pipeline_success_rate": 0.99,
            "pipeline_alert_level": "ok",
            "last_evaluation_at": datetime(2025, 1, 1, tzinfo=timezone.utc),
        }
        app.state.genre_evaluator = AsyncMock()
        app.state.cluster_evaluator = AsyncMock()
        app.state.summary_evaluator = AsyncMock()

        mock_db = AsyncMock()
        mock_db.health_check.return_value = True
        app.state.db = mock_db

        # Load token at startup like main.py will
        from recap_evaluator.infra.bearer_auth import load_bearer_token_from_file
        try:
            app.state.api_token = load_bearer_token_from_file()
        except Exception:
            pass # might fail if bearer_auth is not implemented yet

        yield

    app.router.lifespan_context = dummy_lifespan
    return app

@pytest.fixture
def client(test_app):
    with TestClient(test_app) as c:
        yield c

def test_evaluations_run_rejects_no_auth(client):
    resp = client.post("/api/v1/evaluations/run", json={"window_days": 7})
    assert resp.status_code == 401

def test_evaluations_run_rejects_wrong_token(client):
    resp = client.post("/api/v1/evaluations/run", json={"window_days": 7}, headers={"Authorization": f"Bearer {WRONG_TOKEN}"})
    assert resp.status_code == 401

def test_evaluations_run_rejects_bearer_cafe(client):
    """Fast mounted dependency HTTP fixture test: Bearer cafe must return 401."""
    resp = client.post("/api/v1/evaluations/run", json={"window_days": 7}, headers={"Authorization": "Bearer cafe"})
    assert resp.status_code == 401
    assert resp.headers.get("www-authenticate") == "Bearer"

def test_require_bearer_token_rejects_unicode_credentials():
    """Dependency unit test: Unicode characters in credentials must raise 401."""
    from recap_evaluator.infra.bearer_auth import require_bearer_token
    from fastapi.security import HTTPAuthorizationCredentials
    from fastapi import HTTPException
    from unittest.mock import MagicMock

    mock_req = MagicMock()
    mock_req.app.state.api_token = VALID_TOKEN
    creds = HTTPAuthorizationCredentials(scheme="Bearer", credentials="cafe\u2615bad")
    with pytest.raises(HTTPException) as exc:
        require_bearer_token(mock_req, creds)
    assert exc.value.status_code == 401
    assert exc.value.headers.get("WWW-Authenticate") == "Bearer"


def test_evaluations_run_accepts_valid_token(client):
    resp = client.post("/api/v1/evaluations/run", json={"window_days": 7}, headers={"Authorization": f"Bearer {VALID_TOKEN}"})
    assert resp.status_code == 200

def test_health_accessible_without_auth(client):
    resp = client.get("/health")
    assert resp.status_code == 200

def test_metrics_latest_accessible_without_auth(client):
    resp = client.get("/api/v1/metrics/latest")
    assert resp.status_code == 200

def test_evaluations_list_rejects_no_auth(client):
    resp = client.get("/api/v1/evaluations")
    assert resp.status_code == 401

def test_startup_fails_on_missing_token_file():
    from recap_evaluator.infra.bearer_auth import load_bearer_token_from_file
    with pytest.raises(RuntimeError):
        os.environ["EVALUATOR_API_TOKEN_FILE"] = "/tmp/does-not-exist-12345"
        load_bearer_token_from_file()

def test_startup_fails_on_empty_token():
    from recap_evaluator.infra.bearer_auth import load_bearer_token_from_file
    with tempfile.NamedTemporaryFile(mode='w+', delete=False) as f:
        path = f.name

    os.environ["EVALUATOR_API_TOKEN_FILE"] = path
    try:
        with pytest.raises(RuntimeError):
            load_bearer_token_from_file()
    finally:
        os.remove(path)

def test_token_rfc6750_validation():
    from recap_evaluator.infra.bearer_auth import load_bearer_token_from_file
    with tempfile.NamedTemporaryFile(mode='w+', delete=False) as f:
        f.write("invalid char @")
        path = f.name

    os.environ["EVALUATOR_API_TOKEN_FILE"] = path
    try:
        with pytest.raises(RuntimeError):
            load_bearer_token_from_file()
    finally:
        os.remove(path)

def test_constant_time_comparison():
    # Will just check if hmac is used in the module
    import recap_evaluator.infra.bearer_auth as ba
    import inspect
    source = inspect.getsource(ba)
    assert "hmac.compare_digest" in source
