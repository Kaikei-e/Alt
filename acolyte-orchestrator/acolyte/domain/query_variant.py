"""Query variant generation for multi-query retrieval.

Design (Issue 7 + resolve): Deterministic variant generation from a single
QueryFacet. No LLM involvement. Generates 1-3 query strings per facet:
- primary: render_query_string(facet) — standard facet query
- broad: topic + entities — wider catch
- narrow: must_have_terms only — precision match (only when >= 2 terms)
"""

from __future__ import annotations

from acolyte.domain.query_facet import render_query_string


def generate_query_variants(
    facet: dict,
    topic: str,
    brief: dict,
) -> list[tuple[str, str]]:
    """Generate (query_string, source_label) pairs from a single facet.

    Returns 1-3 variants for multi-query retrieval + RRF fusion.
    """
    variants: list[tuple[str, str]] = []

    # 1. Primary: standard facet rendering
    primary_query = render_query_string(facet)
    if primary_query:
        variants.append((primary_query, "primary"))

    # 2. Broad: topic + brief entities (wider catch)
    entities = brief.get("entities", [])
    if entities:
        entity_str = " ".join(entities[:3])
        broad_query = f"{topic} {entity_str}".strip()
        if broad_query and broad_query != primary_query:
            variants.append((broad_query, "broad"))

    # 3. Narrow: must_have_terms only (precision match)
    must_have = facet.get("must_have_terms", [])
    if len(must_have) >= 2:
        narrow_query = " ".join(must_have)
        if narrow_query != primary_query:
            variants.append((narrow_query, "narrow"))

    # Ensure at least primary exists
    if not variants:
        raw = facet.get("raw_query", topic)
        variants.append((raw or topic, "primary"))

    return variants[:3]
