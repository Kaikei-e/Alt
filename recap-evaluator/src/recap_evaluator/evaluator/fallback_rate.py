"""Fallback rate — proportion of recap_outputs that are degraded or low-evidence extractive."""

from typing import Any

_LOW_EVIDENCE_MODEL = "low-evidence-extractive"


class FallbackRateEvaluator:
    """Pure-Python stats evaluator — no external deps."""

    def compute(self, outputs: list[dict[str, Any]]) -> float:
        if not outputs:
            return 0.0

        fallback_count = 0
        for output in outputs:
            metadata = output.get("body_json", {}).get("metadata", {}) or {}
            if metadata.get("is_degraded") is True:
                fallback_count += 1
                continue
            if metadata.get("model") == _LOW_EVIDENCE_MODEL:
                fallback_count += 1

        return fallback_count / len(outputs)
