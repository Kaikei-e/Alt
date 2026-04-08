"""Tests for compare_runs — before/after evaluation comparison."""

import pytest

from news_creator.evaluation.trace_recorder import TraceRecord
from news_creator.evaluation.comparison import compare_runs, ComparisonReport


def _make_trace(
    job_id: str = "job-1",
    genre: str = "ai",
    is_degraded: bool = False,
    scores: dict | None = None,
) -> TraceRecord:
    """Create a minimal TraceRecord for comparison testing."""
    return TraceRecord(
        job_id=job_id,
        genre=genre,
        window_days=3,
        template_name="recap_summary_3days.jinja",
        prompt_hash="abc123",
        schema_hash="def456",
        input_clusters_json="[]",
        rendered_prompt="test prompt",
        raw_llm_response='{"bullets":["test"],"language":"ja"}',
        parsed_summary_json='{"title":"t","bullets":["b"],"language":"ja"}',
        scores=scores
        or {
            "source_grounding": 0.8,
            "redundancy": 0.1,
            "readability": 0.7,
            "structure": 0.6,
            "entity_density": 0.5,
        },
        metadata={"model": "gemma4-e4b-q4km"},
        is_degraded=is_degraded,
    )


class TestCompareRuns:
    """compare_runs produces correct deltas between two trace sets."""

    def test_identical_runs_have_zero_delta(self):
        """Same scores before and after → all deltas are 0."""
        traces = [_make_trace()]
        report = compare_runs(before=traces, after=traces)

        assert isinstance(report, ComparisonReport)
        for axis_delta in report.axis_deltas:
            assert axis_delta.delta == pytest.approx(0.0)
        assert report.fallback_rate_delta == pytest.approx(0.0)

    def test_improvement_detected(self):
        """After run has higher source_grounding → positive delta, improved=True."""
        before = [
            _make_trace(
                scores={
                    "source_grounding": 0.6,
                    "redundancy": 0.2,
                    "readability": 0.7,
                    "structure": 0.5,
                    "entity_density": 0.4,
                }
            )
        ]
        after = [
            _make_trace(
                scores={
                    "source_grounding": 0.9,
                    "redundancy": 0.1,
                    "readability": 0.8,
                    "structure": 0.7,
                    "entity_density": 0.6,
                }
            )
        ]

        report = compare_runs(before=before, after=after)

        grounding = next(d for d in report.axis_deltas if d.axis == "source_grounding")
        assert grounding.delta == pytest.approx(0.3)
        assert grounding.improved is True

    def test_regression_detected(self):
        """After run has lower readability → negative delta, improved=False."""
        before = [
            _make_trace(
                scores={
                    "source_grounding": 0.8,
                    "redundancy": 0.1,
                    "readability": 0.9,
                    "structure": 0.6,
                    "entity_density": 0.5,
                }
            )
        ]
        after = [
            _make_trace(
                scores={
                    "source_grounding": 0.8,
                    "redundancy": 0.1,
                    "readability": 0.5,
                    "structure": 0.6,
                    "entity_density": 0.5,
                }
            )
        ]

        report = compare_runs(before=before, after=after)

        readability = next(d for d in report.axis_deltas if d.axis == "readability")
        assert readability.delta == pytest.approx(-0.4)
        assert readability.improved is False

    def test_fallback_rate_delta(self):
        """Fallback rate computed from is_degraded counts."""
        before = [
            _make_trace(job_id="1", is_degraded=False),
            _make_trace(job_id="2", is_degraded=True),
        ]  # 50% fallback
        after = [
            _make_trace(job_id="1", is_degraded=False),
            _make_trace(job_id="2", is_degraded=False),
        ]  # 0% fallback

        report = compare_runs(before=before, after=after)

        assert report.fallback_rate_before == pytest.approx(0.5)
        assert report.fallback_rate_after == pytest.approx(0.0)
        assert report.fallback_rate_delta == pytest.approx(-0.5)

    def test_case_counts(self):
        """Report includes trace counts for both runs."""
        before = [_make_trace(job_id=str(i)) for i in range(5)]
        after = [_make_trace(job_id=str(i)) for i in range(3)]

        report = compare_runs(before=before, after=after)

        assert report.case_count_before == 5
        assert report.case_count_after == 3

    def test_subset_axes(self):
        """When axes is specified, only those axes appear in deltas."""
        traces = [_make_trace()]
        report = compare_runs(
            before=traces,
            after=traces,
            axes=["source_grounding", "readability"],
        )

        axis_names = {d.axis for d in report.axis_deltas}
        assert axis_names == {"source_grounding", "readability"}

    def test_redundancy_improvement_is_decrease(self):
        """For redundancy, lower is better → decrease is 'improved'."""
        before = [
            _make_trace(
                scores={
                    "source_grounding": 0.8,
                    "redundancy": 0.5,
                    "readability": 0.7,
                    "structure": 0.6,
                    "entity_density": 0.5,
                }
            )
        ]
        after = [
            _make_trace(
                scores={
                    "source_grounding": 0.8,
                    "redundancy": 0.1,
                    "readability": 0.7,
                    "structure": 0.6,
                    "entity_density": 0.5,
                }
            )
        ]

        report = compare_runs(before=before, after=after)

        redundancy = next(d for d in report.axis_deltas if d.axis == "redundancy")
        assert redundancy.delta == pytest.approx(-0.4)
        assert redundancy.improved is True  # lower redundancy = improvement
