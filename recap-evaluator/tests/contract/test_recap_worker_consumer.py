"""Consumer contract tests for recap-evaluator → recap-worker.

Verifies that recap-evaluator's expectations of the recap-worker
evaluation API are documented as Pact contracts.

Run with:
    cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov
"""

import json
from pathlib import Path

import httpx
from pact import Pact

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent / "pacts"


def _new_pact() -> Pact:
    return Pact("recap-evaluator", "recap-worker")


def test_trigger_genre_evaluation():
    """Verify contract for POST /v1/evaluation/genres."""
    pact = _new_pact()
    (
        pact.upon_receiving("a trigger genre evaluation request")
        .given("the recap-worker is ready for evaluation")
        .with_request("POST", "/v1/evaluation/genres")
        .will_respond_with(200)
        .with_body(json.dumps({"run_id": 1, "status": "running"}), "application/json")
    )

    with pact.serve() as srv:
        resp = httpx.post(f"{srv.url}/v1/evaluation/genres")
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "running"
        assert "run_id" in data

    pact.write_file(str(PACT_DIR))


def test_fetch_latest_genre_evaluation():
    """Verify contract for GET /v1/evaluation/genres/latest."""
    pact = _new_pact()
    (
        pact.upon_receiving("a request for the latest genre evaluation")
        .given("a completed genre evaluation exists")
        .with_request("GET", "/v1/evaluation/genres/latest")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {"run_id": 42, "status": "succeeded", "accuracy": 0.85, "macro_f1": 0.82}
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.get(f"{srv.url}/v1/evaluation/genres/latest")
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "succeeded"
        assert "accuracy" in data

    pact.write_file(str(PACT_DIR))


def test_fetch_genre_evaluation_by_id():
    """Verify contract for GET /v1/evaluation/genres/{run_id}."""
    pact = _new_pact()
    (
        pact.upon_receiving("a request for genre evaluation by run_id 42")
        .given("genre evaluation with run_id 42 exists")
        .with_request("GET", "/v1/evaluation/genres/42")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {"run_id": 42, "status": "succeeded", "accuracy": 0.85, "macro_f1": 0.82}
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.get(f"{srv.url}/v1/evaluation/genres/42")
        assert resp.status_code == 200
        data = resp.json()
        assert data["run_id"] == 42
        assert data["status"] == "succeeded"

    pact.write_file(str(PACT_DIR))
