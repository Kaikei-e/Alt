"""Tests for Sources footer rendering in Finalizer.

Citation hygiene: [Sn] markers stay inline, metadata goes to an appended
Sources block. Body text never contains expanded article titles.
"""

from acolyte.domain.source_map import SourceMap
from acolyte.usecase.graph.nodes.finalizer_node import render_sources_footer


class TestRenderSourcesFooter:
    def test_preserves_short_ids_inline(self):
        sm = SourceMap()
        sm.register("abc-123", "AI Chip Report 2026", publisher="TechDaily", url="https://t.example/a")

        body = "The chip market grew significantly [S1]."
        rendered = render_sources_footer(body, sm)

        assert "[S1]" in rendered
        assert "[AI Chip Report 2026]" not in rendered
        assert rendered.startswith("The chip market grew significantly [S1].")

    def test_appends_sources_footer_in_reference_order(self):
        sm = SourceMap()
        sm.register("abc-123", "First Article", publisher="PubA", url="https://a.example")
        sm.register("def-456", "Second Article", publisher="PubB", url="https://b.example")
        sm.register("ghi-789", "Third Article", publisher="PubC", url="https://c.example")

        body = "Point about two [S2]. Point about three [S3]. Point about one [S1]."
        rendered = render_sources_footer(body, sm)

        assert "\n\n---\nSources:" in rendered
        footer = rendered.split("\n\n---\nSources:\n", 1)[1]

        idx2 = footer.find("[S2]")
        idx3 = footer.find("[S3]")
        idx1 = footer.find("[S1]")
        assert -1 < idx2 < idx3 < idx1

    def test_only_emits_referenced_sources(self):
        sm = SourceMap()
        sm.register("abc-123", "Referenced", publisher="P", url="u")
        sm.register("def-456", "Unreferenced", publisher="P", url="u")

        body = "Only S1 is cited here [S1]."
        rendered = render_sources_footer(body, sm)

        footer = rendered.split("\n\n---\nSources:\n", 1)[1]
        assert "Referenced" in footer
        assert "Unreferenced" not in footer

    def test_footer_entry_includes_title_publisher_url(self):
        sm = SourceMap()
        sm.register(
            "abc-123",
            "AI Chip Report 2026",
            publisher="TechDaily",
            url="https://techdaily.example/ai-chip",
        )

        body = "Claim [S1]."
        rendered = render_sources_footer(body, sm)

        assert "- [S1] AI Chip Report 2026 — TechDaily (https://techdaily.example/ai-chip)" in rendered

    def test_footer_handles_missing_publisher_url(self):
        sm = SourceMap()
        sm.register("abc-123", "Bare Title")

        body = "Claim [S1]."
        rendered = render_sources_footer(body, sm)

        assert "- [S1] Bare Title" in rendered
        assert "—" not in rendered
        assert "()" not in rendered

    def test_no_short_ids_returns_body_unchanged(self):
        sm = SourceMap()
        body = "No citations here. Just text."
        assert render_sources_footer(body, sm) == body

    def test_multiple_same_id_emitted_once_in_footer(self):
        sm = SourceMap()
        sm.register("abc-123", "Article Title", publisher="P", url="u")

        body = "First reference [S1] and second [S1]."
        rendered = render_sources_footer(body, sm)

        footer = rendered.split("\n\n---\nSources:\n", 1)[1]
        assert footer.count("[S1]") == 1

    def test_unknown_short_id_skipped_in_footer(self):
        sm = SourceMap()
        sm.register("abc-123", "Known Article", publisher="P", url="u")

        body = "Known [S1]. Unknown [S99]."
        rendered = render_sources_footer(body, sm)

        assert "[S1]" in rendered
        assert "[S99]" in rendered
        footer = rendered.split("\n\n---\nSources:\n", 1)[1]
        assert "Known Article" in footer
        assert "[S99]" not in footer
