"""Tests for citation resolution in Finalizer."""

from acolyte.domain.source_map import SourceMap
from acolyte.usecase.graph.nodes.finalizer_node import resolve_citations


class TestResolveCitations:
    def test_resolves_short_ids(self):
        sm = SourceMap()
        sm.register("abc-123", "AI Chip Report 2026")
        sm.register("def-456", "GPU Market Analysis")

        body = "The chip market grew significantly [S1]. GPU prices are rising [S2]."
        resolved = resolve_citations(body, sm)
        assert "[AI Chip Report 2026]" in resolved
        assert "[GPU Market Analysis]" in resolved
        assert "[S1]" not in resolved
        assert "[S2]" not in resolved

    def test_unknown_short_id_preserved(self):
        sm = SourceMap()
        sm.register("abc-123", "Known Article")

        body = "Known source [S1]. Unknown source [S99]."
        resolved = resolve_citations(body, sm)
        assert "[Known Article]" in resolved
        assert "[S99]" in resolved  # preserved as-is

    def test_no_short_ids_unchanged(self):
        sm = SourceMap()
        body = "No citations here. Just text."
        assert resolve_citations(body, sm) == body

    def test_multiple_same_id(self):
        sm = SourceMap()
        sm.register("abc-123", "Article Title")

        body = "First reference [S1] and second [S1]."
        resolved = resolve_citations(body, sm)
        assert resolved.count("[Article Title]") == 2
