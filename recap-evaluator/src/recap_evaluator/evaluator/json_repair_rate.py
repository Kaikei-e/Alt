"""JSON repair rate — proportion of recap_outputs whose upstream LLM JSON needed repair.

news-creator writes `metadata.json_repair_attempted = True` into body_json when the
raw LLM response required JSON5-style repair before schema validation.
"""

from typing import Any


class JsonRepairRateEvaluator:
    def compute(self, outputs: list[dict[str, Any]]) -> float:
        if not outputs:
            return 0.0

        repaired = 0
        for output in outputs:
            metadata = output.get("body_json", {}).get("metadata", {}) or {}
            if metadata.get("json_repair_attempted") is True:
                repaired += 1

        return repaired / len(outputs)
