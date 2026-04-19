"""Deterministic mock judge for CI.

Returns a configured constant regardless of input. The mock is the
safety net that lets ``run_eval`` execute without hitting a live LLM,
which keeps CI independent of news-creator availability.
"""

from __future__ import annotations


class MockRubricJudge:
    """Always returns ``mock_score``.

    Use in tests and CI. For per-case deterministic variety, pass a
    non-default ``mock_score`` per invocation site.
    """

    def __init__(self, mock_score: float = 0.5) -> None:
        if not 0.0 <= mock_score <= 1.0:
            raise ValueError(f"mock_score must be in [0.0, 1.0], got {mock_score}")
        self._score = mock_score

    def __call__(self, prompt: str) -> float:  # noqa: ARG002 - accepts prompt to satisfy judge signature
        return self._score
