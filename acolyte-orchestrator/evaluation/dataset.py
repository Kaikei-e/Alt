"""JSONL dataset loader for evaluation runs.

Each record shape:

    {
      "topic": "2026 Q1 AI chip market",
      "query_lang": "ja",
      "gold_source_ids": ["uuid-a", "uuid-b"],
      "expected_lang_mix": {"ja": 0.6, "en": 0.4}
    }

``expected_lang_mix`` is informational only — metrics compare the observed
mix against it; we do not enforce equality.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path


@dataclass(frozen=True)
class EvalCase:
    topic: str
    query_lang: str
    gold_source_ids: frozenset[str]
    expected_lang_mix: dict[str, float] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, raw: dict) -> EvalCase:
        return cls(
            topic=str(raw.get("topic", "")),
            query_lang=str(raw.get("query_lang", "und")),
            gold_source_ids=frozenset(raw.get("gold_source_ids", []) or []),
            expected_lang_mix=dict(raw.get("expected_lang_mix", {}) or {}),
        )


def load_cases(path: str | Path) -> list[EvalCase]:
    """Load JSONL records into EvalCase objects."""
    path = Path(path)
    cases: list[EvalCase] = []
    with path.open("r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            cases.append(EvalCase.from_dict(json.loads(line)))
    return cases
