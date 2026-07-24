"""Tests for the recap_jobs tab header (Recap's 7-day window was removed 2026-04)."""

from streamlit.testing.v1 import AppTest


def _render_with_stubbed_fetch() -> AppTest:
    def render() -> None:
        from tabs import recap_jobs

        recap_jobs.fetch_table_or_warn = lambda *a, **k: None
        recap_jobs.render_recap_jobs(4 * 3600)

    at = AppTest.from_function(render)
    at.run()
    return at


class TestRecapJobsHeader:
    def test_header_does_not_reference_removed_7_day_window(self) -> None:
        at = _render_with_stubbed_fetch()
        header_values = [h.value for h in at.header]
        assert not any("7-Day" in value or "7-day" in value for value in header_values)
