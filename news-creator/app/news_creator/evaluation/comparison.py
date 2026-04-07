"""Comparison report for before/after evaluation runs."""

from dataclasses import dataclass
from typing import Dict, List, Optional

from news_creator.evaluation.recap_quality import LOWER_IS_BETTER_AXES
from news_creator.evaluation.trace_recorder import TraceRecord


@dataclass
class AxisDelta:
    """Per-axis score delta between two runs."""

    axis: str
    before_mean: float
    after_mean: float
    delta: float
    improved: bool


@dataclass
class ComparisonReport:
    """Comparison between two sets of evaluation traces."""

    axis_deltas: List[AxisDelta]
    fallback_rate_before: float
    fallback_rate_after: float
    fallback_rate_delta: float
    case_count_before: int
    case_count_after: int


def _mean_scores(traces: List[TraceRecord], axis: str) -> float:
    """Compute mean score for an axis across traces."""
    values = [t.scores[axis] for t in traces if axis in t.scores]
    return sum(values) / len(values) if values else 0.0


def _fallback_rate(traces: List[TraceRecord]) -> float:
    """Compute fraction of traces that are degraded."""
    if not traces:
        return 0.0
    return sum(1 for t in traces if t.is_degraded) / len(traces)


def compare_runs(
    before: List[TraceRecord],
    after: List[TraceRecord],
    axes: Optional[List[str]] = None,
) -> ComparisonReport:
    """Compare two evaluation runs and produce a delta report.

    Args:
        before: Trace records from baseline run
        after: Trace records from candidate run
        axes: Score axes to compare (defaults to all axes found in before traces)

    Returns:
        ComparisonReport with per-axis deltas and fallback rate change
    """
    # Determine axes to compare
    if axes is None:
        # Collect all axes from both runs
        all_axes: set = set()
        for t in before:
            all_axes.update(t.scores.keys())
        axes = sorted(all_axes)

    axis_deltas: List[AxisDelta] = []
    for axis in axes:
        before_mean = _mean_scores(before, axis)
        after_mean = _mean_scores(after, axis)
        delta = after_mean - before_mean

        # For "lower is better" axes (like redundancy), a decrease is an improvement
        if axis in LOWER_IS_BETTER_AXES:
            improved = delta < 0
        else:
            improved = delta > 0

        axis_deltas.append(AxisDelta(
            axis=axis,
            before_mean=before_mean,
            after_mean=after_mean,
            delta=delta,
            improved=improved,
        ))

    fb_before = _fallback_rate(before)
    fb_after = _fallback_rate(after)

    return ComparisonReport(
        axis_deltas=axis_deltas,
        fallback_rate_before=fb_before,
        fallback_rate_after=fb_after,
        fallback_rate_delta=fb_after - fb_before,
        case_count_before=len(before),
        case_count_after=len(after),
    )
