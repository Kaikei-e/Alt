"""Tests for SourceMap — UUID → short ID mapping for citations."""

from acolyte.domain.source_map import SourceMap


class TestSourceMap:
    def test_register_returns_short_id(self):
        sm = SourceMap()
        sid = sm.register("abc-123", "Article Title")
        assert sid == "S1"

    def test_register_idempotent(self):
        sm = SourceMap()
        sid1 = sm.register("abc-123", "Title")
        sid2 = sm.register("abc-123", "Title")
        assert sid1 == sid2 == "S1"

    def test_register_sequential(self):
        sm = SourceMap()
        assert sm.register("a", "A") == "S1"
        assert sm.register("b", "B") == "S2"
        assert sm.register("c", "C") == "S3"

    def test_resolve(self):
        sm = SourceMap()
        sm.register("abc-123", "Article Title", publisher="Publisher", url="https://example.com")
        entry = sm.resolve("S1")
        assert entry is not None
        assert entry.source_id == "abc-123"
        assert entry.title == "Article Title"
        assert entry.publisher == "Publisher"
        assert entry.url == "https://example.com"

    def test_resolve_unknown_returns_none(self):
        sm = SourceMap()
        assert sm.resolve("S99") is None

    def test_short_id_for(self):
        sm = SourceMap()
        sm.register("abc-123", "Title")
        assert sm.short_id_for("abc-123") == "S1"
        assert sm.short_id_for("unknown") is None

    def test_all_entries(self):
        sm = SourceMap()
        sm.register("a", "A")
        sm.register("b", "B")
        entries = sm.all_entries()
        assert len(entries) == 2
        assert entries[0].short_id == "S1"
        assert entries[1].short_id == "S2"

    def test_round_trip_serialization(self):
        sm = SourceMap()
        sm.register("abc-123", "Title", publisher="Pub", url="https://example.com")
        sm.register("def-456", "Title 2")

        data = sm.to_dict()
        sm2 = SourceMap.from_dict(data)

        assert sm2.short_id_for("abc-123") == "S1"
        assert sm2.short_id_for("def-456") == "S2"
        entry = sm2.resolve("S1")
        assert entry is not None
        assert entry.title == "Title"
        assert entry.publisher == "Pub"
