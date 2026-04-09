"""E2E tests: CreateReport and ListReports via Connect-RPC."""

from __future__ import annotations

import json

from starlette.testclient import TestClient


def test_create_report_returns_report_id(client: TestClient) -> None:
    """CreateReport returns a reportId."""
    resp = client.post(
        "/alt.acolyte.v1.AcolyteService/CreateReport",
        content=json.dumps({"title": "Weekly AI Briefing", "reportType": "weekly_briefing"}),
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200, f"Expected 200, got {resp.status_code}: {resp.text}"
    data = resp.json()
    assert "reportId" in data
    assert len(data["reportId"]) > 0


def test_list_reports_returns_created_report(client: TestClient) -> None:
    """ListReports returns reports that were created."""
    # Create a report first
    create_resp = client.post(
        "/alt.acolyte.v1.AcolyteService/CreateReport",
        content=json.dumps({"title": "Test Report", "reportType": "market_analysis"}),
        headers={"Content-Type": "application/json"},
    )
    assert create_resp.status_code == 200

    # List should include it
    list_resp = client.post(
        "/alt.acolyte.v1.AcolyteService/ListReports",
        content=json.dumps({"limit": 10}),
        headers={"Content-Type": "application/json"},
    )
    assert list_resp.status_code == 200, f"Expected 200, got {list_resp.status_code}: {list_resp.text}"
    data = list_resp.json()
    assert "reports" in data
    assert len(data["reports"]) >= 1
    titles = [r["title"] for r in data["reports"]]
    assert "Test Report" in titles


def test_get_report_returns_details(client: TestClient) -> None:
    """GetReport returns report with sections."""
    # Create
    create_resp = client.post(
        "/alt.acolyte.v1.AcolyteService/CreateReport",
        content=json.dumps({"title": "Detail Test", "reportType": "tech_review"}),
        headers={"Content-Type": "application/json"},
    )
    report_id = create_resp.json()["reportId"]

    # Get
    get_resp = client.post(
        "/alt.acolyte.v1.AcolyteService/GetReport",
        content=json.dumps({"reportId": report_id}),
        headers={"Content-Type": "application/json"},
    )
    assert get_resp.status_code == 200
    data = get_resp.json()
    assert data["report"]["title"] == "Detail Test"
    # proto3: int default 0 is omitted from JSON, so currentVersion may be absent
    assert data["report"].get("currentVersion", 0) == 0
