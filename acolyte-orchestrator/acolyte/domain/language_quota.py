"""Post-curation soft language quota.

The gatherer and LLM curator rank evidence purely by relevance. On Japanese
topics BM25 favours Japanese sources, so English candidates with strong
substantive relevance can get bumped out of the top-K even when the pool has
both languages. ``rebalance_by_language`` swaps in under-represented
languages when the unfiltered pool contains them — deterministic, testable,
no prompt engineering required.
"""

from __future__ import annotations

import math
from collections.abc import Iterable

_UND = "und"


def _language_of(item: dict) -> str:
    value = item.get("language")
    if not value:
        return _UND
    return str(value)


def _score_of(item: dict) -> float:
    raw = item.get("score", 0.0)
    try:
        return float(raw)
    except (TypeError, ValueError):
        return 0.0


def _default_quota() -> dict[str, float]:
    return {"en": 0.2}


def rebalance_by_language(
    curated: list[dict],
    pool: Iterable[dict],
    quota: dict[str, float] | None,
) -> list[dict]:
    """Swap under-represented language items in after the LLM ranking step.

    - ``curated``: list already selected by the curator (preserves order).
    - ``pool``: the full candidate set the curator chose from. May contain the
      curated items themselves.
    - ``quota``: ``{language_code: min_share}``; 0.0 disables enforcement for
      that code. ``None`` applies the default ``{"en": 0.2}`` — a *new* dict
      per call so callers never share a mutable default.

    Items whose ``language`` is missing/empty are bucketed as ``und`` and
    considered the first candidates for displacement when a quota needs to be
    met.
    """
    if not curated:
        return []

    effective_quota = dict(quota) if quota is not None else _default_quota()
    if not effective_quota:
        return list(curated)

    result = list(curated)
    curated_ids = {item.get("id") for item in result if item.get("id")}
    slot_count = len(result)

    for language, share in effective_quota.items():
        if share <= 0:
            continue
        required = math.ceil(slot_count * share)
        if required <= 0:
            continue

        current = sum(1 for item in result if _language_of(item) == language)
        if current >= required:
            continue

        candidates = sorted(
            (item for item in pool if item.get("id") not in curated_ids and _language_of(item) == language),
            key=_score_of,
            reverse=True,
        )
        deficit = required - current
        for _ in range(deficit):
            if not candidates:
                break
            weakest_idx = _find_weakest_index_not_in_language(result, language)
            if weakest_idx is None:
                break
            replacement = candidates.pop(0)
            removed = result[weakest_idx]
            result[weakest_idx] = replacement
            if removed.get("id"):
                curated_ids.discard(removed["id"])
            if replacement.get("id"):
                curated_ids.add(replacement["id"])

    return result


def _find_weakest_index_not_in_language(items: list[dict], protected_language: str) -> int | None:
    """Return the index of the lowest-scored item *not* in ``protected_language``.

    Items with missing ``language`` (treated as ``und``) are preferred first to
    avoid displacing confirmed-language items.
    """
    und_candidates: list[tuple[float, int]] = []
    other_candidates: list[tuple[float, int]] = []
    for idx, item in enumerate(items):
        lang = _language_of(item)
        if lang == protected_language:
            continue
        pair = (_score_of(item), idx)
        if lang == _UND:
            und_candidates.append(pair)
        else:
            other_candidates.append(pair)

    if und_candidates:
        und_candidates.sort(key=lambda t: t[0])
        return und_candidates[0][1]
    if other_candidates:
        other_candidates.sort(key=lambda t: t[0])
        return other_candidates[0][1]
    return None
