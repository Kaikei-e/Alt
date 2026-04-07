"""Trace recorder for recap summary evaluation runs."""

import json
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional


@dataclass
class TraceRecord:
    """Single trace entry for a recap summary generation run."""

    job_id: str
    genre: str
    window_days: Optional[int]
    template_name: str
    prompt_hash: str
    schema_hash: str
    input_clusters_json: str
    rendered_prompt: str
    raw_llm_response: str
    parsed_summary_json: Optional[str]
    scores: Dict[str, float]
    metadata: Dict[str, Any]
    is_degraded: bool = False
    degradation_reason: Optional[str] = None


class TraceRecorder:
    """Records evaluation traces to JSONL files.

    Each recorder instance writes to a single JSONL file in output_dir.
    The filename is based on the current timestamp.
    """

    def __init__(self, output_dir: Path):
        self._output_dir = output_dir
        self._output_dir.mkdir(parents=True, exist_ok=True)
        ts = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")
        self._file_path = self._output_dir / f"trace_{ts}.jsonl"

    def record(self, trace: TraceRecord) -> None:
        """Append a trace record to the JSONL file."""
        with open(self._file_path, "a", encoding="utf-8") as f:
            f.write(json.dumps(asdict(trace), ensure_ascii=False) + "\n")

    def load_traces(self, path: Path) -> List[TraceRecord]:
        """Load trace records from a JSONL file."""
        records: List[TraceRecord] = []
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                data = json.loads(line)
                records.append(TraceRecord(**data))
        return records
