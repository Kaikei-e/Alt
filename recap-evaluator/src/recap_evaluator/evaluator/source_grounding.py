"""Source grounding — proportion of bullets that carry at least one source_sentence_id.

recap-worker populates bullet-level `source_sentence_ids` derived from the news-creator
[n] citation reconciled against recap_subworker_sentences. A bullet without any id
means the summarization flow could not ground that claim to a representative sentence.
The structured bullet list is stored in `recap_outputs.bullets_ja`; `body_json.summary.bullets`
mirrors it for downstream consumers.
"""

from typing import Any


def _extract_bullets(body_json: dict[str, Any]) -> list[Any]:
    candidates = [
        body_json.get("bullets"),
        (body_json.get("summary") or {}).get("bullets"),
    ]
    for candidate in candidates:
        if isinstance(candidate, list) and candidate:
            return candidate
    return []


class SourceGroundingEvaluator:
    def compute(self, body_json: dict[str, Any]) -> float:
        bullets = _extract_bullets(body_json)
        if not bullets:
            return 0.0

        grounded = 0
        for bullet in bullets:
            if not isinstance(bullet, dict):
                continue
            ids = bullet.get("source_sentence_ids") or []
            if len(ids) > 0:
                grounded += 1

        return grounded / len(bullets)

    def compute_batch(self, outputs: list[dict[str, Any]]) -> float:
        if not outputs:
            return 0.0

        scores: list[float] = []
        for output in outputs:
            bullets_ja = output.get("bullets_ja")
            if isinstance(bullets_ja, list) and bullets_ja:
                scores.append(self.compute({"bullets": bullets_ja}))
                continue
            body_json = output.get("body_json") or {}
            scores.append(self.compute(body_json))

        return sum(scores) / len(scores) if scores else 0.0
