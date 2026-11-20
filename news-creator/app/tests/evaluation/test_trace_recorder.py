"""Tests for TraceRecorder — JSONL trace recording and loading."""

from pathlib import Path
from typing import Any

from news_creator.evaluation.trace_recorder import TraceRecord, TraceRecorder


def _make_trace(**overrides: object) -> TraceRecord:
    defaults: dict[str, Any] = {
        "job_id": "job-1",
        "genre": "ai",
        "window_days": 3,
        "template_name": "recap_summary_3days.jinja",
        "prompt_hash": "abc123",
        "schema_hash": "def456",
        "input_clusters_json": '[{"cluster_id":0}]',
        "rendered_prompt": "prompt text",
        "raw_llm_response": '{"bullets":["b"],"language":"ja"}',
        "parsed_summary_json": '{"title":"t","bullets":["b"],"language":"ja"}',
        "scores": {"source_grounding": 0.8, "readability": 0.7},
        "metadata": {"model": "gemma4-e4b-q4km", "endpoint": "/api/generate"},
        "is_degraded": False,
        "degradation_reason": None,
    }
    defaults.update(overrides)
    return TraceRecord(**defaults)


class TestTraceRecorder:
    def test_record_and_load_roundtrip(self, tmp_path: Path) -> None:
        """Write a trace, load it back, fields match."""
        expected_window_days = 3
        expected_grounding_score = 0.8
        recorder = TraceRecorder(output_dir=tmp_path)
        trace = _make_trace(
            window_days=expected_window_days,
            scores={"source_grounding": expected_grounding_score, "readability": 0.7},
        )
        recorder.record(trace)

        # Find the written file
        jsonl_files = list(tmp_path.glob("*.jsonl"))
        assert len(jsonl_files) == 1

        loaded = recorder.load_traces(jsonl_files[0])
        assert len(loaded) == 1
        assert loaded[0].job_id == "job-1"
        assert loaded[0].genre == "ai"
        assert loaded[0].window_days == expected_window_days
        assert loaded[0].scores["source_grounding"] == expected_grounding_score

    def test_multiple_records_appended(self, tmp_path: Path) -> None:
        """Multiple record() calls append to the same file."""
        recorder = TraceRecorder(output_dir=tmp_path)
        traces = [_make_trace(job_id="job-1"), _make_trace(job_id="job-2")]
        for trace in traces:
            recorder.record(trace)

        jsonl_files = list(tmp_path.glob("*.jsonl"))
        assert len(jsonl_files) == 1

        loaded = recorder.load_traces(jsonl_files[0])
        assert len(loaded) == len(traces)
        assert loaded[0].job_id == "job-1"
        assert loaded[1].job_id == "job-2"

    def test_degraded_trace_roundtrip(self, tmp_path: Path) -> None:
        """Degraded traces preserve is_degraded and degradation_reason."""
        recorder = TraceRecorder(output_dir=tmp_path)
        trace = _make_trace(
            is_degraded=True,
            degradation_reason="LLM generation failed",
        )
        recorder.record(trace)

        jsonl_files = list(tmp_path.glob("*.jsonl"))
        loaded = recorder.load_traces(jsonl_files[0])
        assert loaded[0].is_degraded is True
        assert loaded[0].degradation_reason == "LLM generation failed"
