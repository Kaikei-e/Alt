"""FactNormalizerNode — normalizes selected quotes into atomic facts.

Stage 2 of 2-stage extraction pipeline (Issue 2).
Processes each quote individually (per-quote LLM call) for tiny schema output.
On LLM failure, preserves quote text as claim with confidence=0.3 (is_fallback=True).

Settings-driven config (fact_num_predict, max_facts_total) replaces hardcoded values.
Cap uses section round-robin to avoid bias toward earlier sections.
"""

from __future__ import annotations

from collections import defaultdict
from typing import TYPE_CHECKING, Protocol

import structlog

from acolyte.domain.quote_selection import FactNormalizerOutput
from acolyte.port.llm_provider import LLMMode
from acolyte.usecase.graph.llm_parse import generate_validated
from acolyte.usecase.graph.state import ReportGenerationState

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)

_NORMALIZE_PROMPT = """Normalize one quote into one atomic fact.

Quote: "{text}"
Source: {source_title}

Return JSON with exactly these keys:
- claim
- confidence
- data_type  # statistic|date|quote|trend|comparison
"""


class _FactNormalizerConfig(Protocol):
    """Minimal config protocol — satisfied by Settings or test stubs."""

    fact_num_predict: int
    max_facts_total: int


def _fallback_fact(quote: dict) -> dict:
    """Create a fallback fact from a quote when LLM fails."""
    return {
        "claim": quote.get("text", "")[:200],
        "source_id": quote.get("source_id", ""),
        "quote": quote.get("text", "")[:120],
        "confidence": 0.3,
        "data_type": "quote",
        "is_fallback": True,
    }


def _cap_round_robin(quotes: list[dict], max_total: int) -> list[dict]:
    """Cap quotes to max_total using section round-robin for fairness.

    Groups quotes by section_key, then interleaves one from each section
    in round-robin order until the cap is reached.
    """
    if len(quotes) <= max_total:
        return quotes

    # Group by section_key, preserving insertion order within each group
    by_section: dict[str, list[dict]] = defaultdict(list)
    section_order: list[str] = []
    for q in quotes:
        key = q.get("section_key", "")
        if key not in by_section:
            section_order.append(key)
        by_section[key].append(q)

    # Round-robin: take one from each section in turn
    result: list[dict] = []
    iterators = {k: iter(v) for k, v in by_section.items()}

    while len(result) < max_total:
        exhausted = 0
        for key in section_order:
            if len(result) >= max_total:
                break
            val = next(iterators[key], None)
            if val is not None:
                result.append(val)
            else:
                exhausted += 1
        if exhausted == len(section_order):
            break

    logger.info(
        "Capped facts via round-robin",
        total=len(quotes),
        cap=max_total,
        selected=len(result),
    )
    return result


def should_continue_fact_normalization(state: ReportGenerationState) -> str:
    """Conditional edge for checkpoint-safe per-quote normalization."""
    quotes = state.get("fact_normalizer_work_quotes", [])
    cursor = state.get("fact_normalizer_cursor", 0)
    return "more" if cursor < len(quotes) else "done"


class FactNormalizerNode:
    def __init__(self, llm: LLMProviderPort, config: _FactNormalizerConfig, *, incremental: bool = False) -> None:
        self._llm = llm
        self._fact_num_predict = config.fact_num_predict
        self._max_facts = config.max_facts_total
        self._incremental = incremental

    async def __call__(self, state: ReportGenerationState) -> dict:
        if self._incremental:
            return await self._process_incremental(state)
        return await self._process_all(state)

    async def _process_all(self, state: ReportGenerationState) -> dict:
        selected_quotes = state.get("selected_quotes", [])

        # Cap via section round-robin (exec3.md Issue 2)
        selected_quotes = _cap_round_robin(selected_quotes, self._max_facts)

        all_facts: list[dict] = []

        for quote in selected_quotes:
            fact = await self._normalize_quote(quote)
            all_facts.append(fact)

        logger.info("FactNormalizer completed", fact_count=len(all_facts))
        return {"extracted_facts": all_facts}

    async def _process_incremental(self, state: ReportGenerationState) -> dict:
        """Checkpoint-safe path: normalize one quote per node invocation."""
        work_quotes = state.get("fact_normalizer_work_quotes")
        if work_quotes is None:
            work_quotes = _cap_round_robin(state.get("selected_quotes", []), self._max_facts)

        cursor = state.get("fact_normalizer_cursor", 0)
        extracted_facts = list(state.get("extracted_facts", []))

        if cursor >= len(work_quotes):
            logger.info("FactNormalizer completed", fact_count=len(extracted_facts))
            return {
                "extracted_facts": extracted_facts,
                "fact_normalizer_work_quotes": work_quotes,
                "fact_normalizer_cursor": cursor,
            }

        quote = work_quotes[cursor]
        fact = await self._normalize_quote(quote)
        extracted_facts.append(fact)
        next_cursor = cursor + 1

        if next_cursor >= len(work_quotes):
            logger.info("FactNormalizer completed", fact_count=len(extracted_facts))
        else:
            logger.info(
                "FactNormalizer progress",
                processed=next_cursor,
                total=len(work_quotes),
            )

        return {
            "extracted_facts": extracted_facts,
            "fact_normalizer_work_quotes": work_quotes,
            "fact_normalizer_cursor": next_cursor,
        }

    async def _normalize_quote(self, quote: dict) -> dict:
        """Normalize a single quote into a fact, with fallback.

        Uses sentinel identity check to detect fallback:
        generate_validated returns the fallback object as-is (not a copy),
        so `result is fallback_obj` reliably distinguishes LLM success from fallback.
        """
        try:
            prompt = _NORMALIZE_PROMPT.format(
                text=quote.get("text", ""),
                source_title=quote.get("source_title", ""),
            )

            fallback_obj = FactNormalizerOutput(claim=quote.get("text", "")[:200], confidence=0.3, data_type="quote")
            result = await generate_validated(
                self._llm,
                prompt,
                FactNormalizerOutput,
                temperature=0,
                num_predict=self._fact_num_predict,
                fallback=fallback_obj,
                mode=LLMMode.STRUCTURED,
            )

            # Sentinel identity check: generate_validated returns fallback_obj as-is
            is_fallback = result is fallback_obj

            return {
                "claim": result.claim,
                "source_id": quote.get("source_id", ""),
                "quote": quote.get("text", "")[:120],
                "confidence": result.confidence,
                "data_type": result.data_type,
                "is_fallback": is_fallback,
            }
        except Exception as exc:
            logger.warning(
                "FactNormalizer failed, using fallback",
                source_id=quote.get("source_id"),
                error=str(exc),
            )
            return _fallback_fact(quote)
