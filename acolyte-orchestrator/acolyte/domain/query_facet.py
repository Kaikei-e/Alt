"""QueryFacet — structured search facet decomposed from search query strings.

Design (Issue 6 + resolve + ADR-666): LLM generates search_queries as plain
strings. This module deterministically decomposes them into structured facets
using ReportBrief metadata (entities, time_range, exclude_topics) and
section_role. No LLM call involved.
"""

from __future__ import annotations

from pydantic import BaseModel

WEAK_FACET_THRESHOLD: int = 2

_COMPARE_KEYWORDS = frozenset(
    {"vs", "versus", "compared", "comparison", "differ", "差", "比較", "対"}
)
_TREND_KEYWORDS = frozenset(
    {"trend", "forecast", "growth", "decline", "outlook", "動向", "推移", "予測"}
)
_STOP_WORDS = frozenset(
    {
        "the",
        "a",
        "an",
        "of",
        "in",
        "for",
        "and",
        "or",
        "to",
        "is",
        "are",
        "on",
        "at",
        "by",
        "with",
        "from",
        "as",
        "it",
        "that",
        "this",
        "was",
        "be",
    }
)

_MAX_RENDERED_LENGTH = 120


class QueryFacet(BaseModel):
    """Structured search facet decomposed from a search query string."""

    intent: str  # "investigate" | "compare" | "trend" | "background"
    raw_query: str  # Original search_query string from LLM
    entities: list[str] = []  # From brief.entities + query extraction
    must_have_terms: list[str] = []  # Non-stopword significant tokens
    optional_terms: list[str] = []  # Topic tokens not in must_have
    time_range: str | None = None  # From brief.time_range
    source_bias: str = "article"  # "article" | "recap" | "any"


def _infer_intent(section_role: str, query: str) -> str:
    """Infer search intent from section_role and query keywords."""
    lower = query.lower()
    tokens = set(lower.split())

    if tokens & _COMPARE_KEYWORDS:
        return "compare"
    if tokens & _TREND_KEYWORDS:
        return "trend"

    if section_role == "analysis":
        return "investigate"
    return "background"


def _extract_significant_tokens(text: str) -> list[str]:
    """Extract non-stopword tokens with length > 2."""
    return [w for w in text.split() if len(w) > 2 and w.lower() not in _STOP_WORDS]


def decompose_queries(
    search_queries: list[str],
    brief_dict: dict,
    section_dict: dict,
) -> list[QueryFacet]:
    """Deterministically decompose search query strings into structured facets.

    Args:
        search_queries: Raw query strings from LLM planner output.
        brief_dict: ReportBrief.to_dict() — contains topic, entities, time_range, etc.
        section_dict: Section outline dict — contains section_role, synthesis_only, etc.

    Returns:
        List of QueryFacet objects. Empty if section is synthesis_only.
    """
    if section_dict.get("synthesis_only", False):
        return []

    section_role = section_dict.get("section_role", "general")
    brief_entities = brief_dict.get("entities", [])
    time_range = brief_dict.get("time_range")
    topic = brief_dict.get("topic", "")

    topic_tokens = _extract_significant_tokens(topic)

    facets: list[QueryFacet] = []
    for query in search_queries:
        intent = _infer_intent(section_role, query)

        # Entities: brief entities that appear in the query
        query_lower = query.lower()
        matched_entities = [e for e in brief_entities if e.lower() in query_lower]

        # Must-have terms: significant tokens from query (top 3)
        must_have = _extract_significant_tokens(query)[:3]

        # Optional terms: topic tokens not already in must_have
        must_have_lower = {t.lower() for t in must_have}
        optional = [t for t in topic_tokens if t.lower() not in must_have_lower][:3]

        facets.append(
            QueryFacet(
                intent=intent,
                raw_query=query,
                entities=matched_entities,
                must_have_terms=must_have,
                optional_terms=optional,
                time_range=time_range,
                source_bias="article",
            )
        )

    return facets


def render_query_string(facet: QueryFacet | dict) -> str:
    """Render a QueryFacet into a flat search string for the search-indexer API.

    Combines must_have_terms + entities, deduplicates, and caps at 120 chars.
    Falls back to raw_query if no terms available.
    Accepts both QueryFacet objects and dicts (from serialized state).
    """
    if isinstance(facet, dict):
        must_have = facet.get("must_have_terms", [])
        entities = facet.get("entities", [])
        time_range = facet.get("time_range")
        raw_query = facet.get("raw_query", "")
    else:
        must_have = facet.must_have_terms
        entities = facet.entities
        time_range = facet.time_range
        raw_query = facet.raw_query

    parts: list[str] = []

    # Must-have terms first
    seen: set[str] = set()
    for term in must_have:
        low = term.lower()
        if low not in seen:
            parts.append(term)
            seen.add(low)

    # Entities not already covered
    for entity in entities:
        low = entity.lower()
        if low not in seen:
            parts.append(entity)
            seen.add(low)

    # Time range if present
    if time_range:
        parts.append(time_range)

    if not parts:
        result = raw_query
    else:
        result = " ".join(parts)

    if len(result) > _MAX_RENDERED_LENGTH:
        # Truncate at word boundary
        truncated = result[:_MAX_RENDERED_LENGTH]
        last_space = truncated.rfind(" ")
        if last_space > 0:
            truncated = truncated[:last_space]
        result = truncated

    return result
