"""Metrics for evaluating Acolyte report quality.

Three axes:

- ``citation_precision``: fraction of emitted ``[Sn]`` markers whose resolved
  ``source_id`` appears in the gold set for the topic.
- ``lang_mix_ratio``: per-language share of cited sources.
- ``faithfulness``: delegate to a user-provided LLM judge; returns ``None`` when
  there are no citations to score.
"""

from __future__ import annotations

import re
from collections import Counter
from collections.abc import Callable

_SHORT_ID_RE = re.compile(r"\[S(\d+)\]")


def extract_short_ids(body: str) -> list[str]:
    """Return ``[Sn]`` tokens in first-occurrence order, without duplicates."""
    ordered: list[str] = []
    seen: set[str] = set()
    for match in _SHORT_ID_RE.finditer(body):
        short_id = f"S{match.group(1)}"
        if short_id not in seen:
            seen.add(short_id)
            ordered.append(short_id)
    return ordered


def _resolved_source_ids(body: str, source_map: dict[str, dict]) -> list[str]:
    ids: list[str] = []
    for short_id in extract_short_ids(body):
        entry = source_map.get(short_id)
        if entry is None:
            continue
        source_id = entry.get("source_id") or ""
        if source_id:
            ids.append(source_id)
    return ids


def citation_precision(body: str, source_map: dict[str, dict], gold: set[str]) -> float | None:
    """Fraction of cited source_ids present in ``gold``.

    Returns ``None`` when no valid citations are emitted (no denominator).
    """
    resolved = _resolved_source_ids(body, source_map)
    if not resolved:
        return None
    hits = sum(1 for source_id in resolved if source_id in gold)
    return hits / len(resolved)


def lang_mix_ratio(
    body: str,
    source_map: dict[str, dict],
    articles_by_id: dict[str, dict],
) -> dict[str, float]:
    """Per-language share of citations. Missing ``language`` is reported as ``und``."""
    resolved = _resolved_source_ids(body, source_map)
    if not resolved:
        return {}
    counts: Counter[str] = Counter()
    for source_id in resolved:
        article = articles_by_id.get(source_id) or {}
        lang = article.get("language") or "und"
        counts[lang] += 1
    total = sum(counts.values())
    return {lang: count / total for lang, count in counts.items()}


def faithfulness(
    body: str,
    evidence_by_short_id: dict[str, str],
    judge: Callable[[str], float],
) -> float | None:
    """Delegate to a caller-supplied judge.

    The judge receives a prompt containing the body + evidence excerpts and
    returns a 0..1 score. Returns ``None`` when no evidence is supplied (there
    is nothing to be faithful to).
    """
    if not evidence_by_short_id:
        return None
    prompt_parts: list[str] = ["<body>", body, "</body>", "<evidence>"]
    for short_id, excerpt in evidence_by_short_id.items():
        prompt_parts.append(f"[{short_id}] {excerpt}")
    prompt_parts.append("</evidence>")
    return judge("\n".join(prompt_parts))
