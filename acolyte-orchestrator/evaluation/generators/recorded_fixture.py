"""Deterministic fixture-based generator for CI.

Each fixture file is a JSON document shaped like:

    {
      "body": "...",
      "source_map": {"S1": {"source_id": "uuid", "language": "ja"}, ...},
      "articles_by_id": {"uuid": {"language": "ja"}, ...},
      "evidence_by_short_id": {"S1": "quoted sentence", ...}
    }

The generator looks up ``<slug>.json`` where ``slug`` is derived from
``EvalCase.topic``. Missing fixtures raise ``FileNotFoundError`` rather than
silently returning empty data — CI should fail loudly on dataset drift.
"""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any

from evaluation.dataset import EvalCase

_SLUG_RE = re.compile(r"[^a-z0-9]+")


def slugify(topic: str) -> str:
    """Filesystem-safe slug derived from the topic. Non-ASCII is hashed so
    fixtures for Japanese topics still land on a short, predictable path."""
    lowered = topic.strip().lower()
    ascii_slug = _SLUG_RE.sub("-", lowered).strip("-")
    if not ascii_slug or not ascii_slug.isascii():
        # Non-cryptographic identity hash: a stable filesystem slug only.
        digest = hashlib.sha256(topic.encode("utf-8")).hexdigest()[:12]
        return f"topic-{digest}"
    return ascii_slug[:80]


class RecordedFixtureGenerator:
    """Callable that maps an ``EvalCase`` onto a recorded fixture."""

    def __init__(self, fixtures_dir: Path) -> None:
        self._dir = Path(fixtures_dir)

    def __call__(self, case: EvalCase) -> tuple[str, dict[str, dict], dict[str, dict], dict[str, str]]:
        fixture_path = self._dir / f"{slugify(case.topic)}.json"
        if not fixture_path.exists():
            raise FileNotFoundError(
                f"no fixture for topic {case.topic!r} at {fixture_path}",
            )
        data: dict[str, Any] = json.loads(fixture_path.read_text(encoding="utf-8"))
        return (
            str(data.get("body", "")),
            dict(data.get("source_map") or {}),
            dict(data.get("articles_by_id") or {}),
            dict(data.get("evidence_by_short_id") or {}),
        )
