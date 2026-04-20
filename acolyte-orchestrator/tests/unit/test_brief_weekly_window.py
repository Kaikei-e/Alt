"""``weekly_briefing`` reports must default to a 7-day time window when the
caller does not supply one, so the Gatherer cannot silently surface
stale articles from months ago (see citations of report
``eb67863f-f6f0-48a4-bd76-a9f1d9a79054``)."""

from __future__ import annotations

import pytest

from acolyte.domain.brief import ReportBrief


def test_weekly_briefing_defaults_to_seven_day_window() -> None:
    brief = ReportBrief.from_scope({"topic": "イラン情勢"}, "weekly_briefing")
    assert brief.time_range == "P7D"


def test_weekly_briefing_respects_caller_time_range() -> None:
    brief = ReportBrief.from_scope({"topic": "イラン情勢", "time_range": "P14D"}, "weekly_briefing")
    assert brief.time_range == "P14D"


@pytest.mark.parametrize("report_type", ["market_analysis", "market_analysis_japan", "trend_report"])
def test_non_weekly_report_types_keep_none_when_unspecified(report_type: str) -> None:
    brief = ReportBrief.from_scope({"topic": "AI chips"}, report_type)
    assert brief.time_range is None


def test_weekly_briefing_treats_empty_string_as_missing() -> None:
    brief = ReportBrief.from_scope({"topic": "イラン情勢", "time_range": ""}, "weekly_briefing")
    assert brief.time_range == "P7D"
