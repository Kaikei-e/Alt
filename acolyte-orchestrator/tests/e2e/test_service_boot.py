"""E2E tests: service boot and health check."""

from __future__ import annotations

import json

from starlette.testclient import TestClient


def test_health_endpoint_returns_ok(client: TestClient) -> None:
    """GET /health returns 200 with service status."""
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["service"] == "acolyte-orchestrator"


def test_connect_health_check_returns_ok(client: TestClient) -> None:
    """Connect-RPC HealthCheck returns ok status."""
    resp = client.post(
        "/alt.acolyte.v1.AcolyteService/HealthCheck",
        content=json.dumps({}),
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"


def test_unimplemented_rpc_returns_error(client: TestClient) -> None:
    """Unimplemented RPCs return Connect error."""
    resp = client.post(
        "/alt.acolyte.v1.AcolyteService/DiffReportVersions",
        content=json.dumps({"reportId": "fake", "fromVersion": 1, "toVersion": 2}),
        headers={"Content-Type": "application/json"},
    )
    # Connect-RPC returns 501 for unimplemented
    assert resp.status_code == 501
