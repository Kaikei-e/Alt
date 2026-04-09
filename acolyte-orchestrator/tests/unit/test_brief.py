"""Unit tests for ReportBrief domain model."""

from __future__ import annotations

import pytest

from acolyte.domain.brief import ReportBrief


def test_from_scope_extracts_topic() -> None:
    scope = {"topic": "AI semiconductor supply chain", "time_range": "Q2 2026"}
    brief = ReportBrief.from_scope(scope, "weekly_briefing")
    assert brief.topic == "AI semiconductor supply chain"
    assert brief.report_type == "weekly_briefing"
    assert brief.time_range == "Q2 2026"


def test_from_scope_extracts_entities() -> None:
    scope = {"topic": "AI trends", "entities": "NVIDIA,TSMC,Intel"}
    brief = ReportBrief.from_scope(scope, "market_analysis")
    assert brief.entities == ["NVIDIA", "TSMC", "Intel"]


def test_from_scope_extracts_exclude() -> None:
    scope = {"topic": "AI trends", "exclude": "crypto,blockchain"}
    brief = ReportBrief.from_scope(scope, "weekly_briefing")
    assert brief.exclude_topics == ["crypto", "blockchain"]


def test_from_scope_empty_topic_raises() -> None:
    with pytest.raises(ValueError, match="topic"):
        ReportBrief.from_scope({}, "weekly_briefing")


def test_from_scope_blank_topic_raises() -> None:
    with pytest.raises(ValueError, match="topic"):
        ReportBrief.from_scope({"topic": "  "}, "weekly_briefing")


def test_to_dict_roundtrip() -> None:
    scope = {"topic": "AI semiconductor", "time_range": "Q2 2026", "entities": "NVIDIA,TSMC"}
    brief = ReportBrief.from_scope(scope, "weekly_briefing")
    d = brief.to_dict()
    assert d["topic"] == "AI semiconductor"
    assert d["report_type"] == "weekly_briefing"
    assert d["entities"] == ["NVIDIA", "TSMC"]


def test_constraints_captures_extra_fields() -> None:
    scope = {"topic": "AI trends", "region": "APAC", "language": "ja"}
    brief = ReportBrief.from_scope(scope, "weekly_briefing")
    assert brief.constraints == {"region": "APAC", "language": "ja"}
