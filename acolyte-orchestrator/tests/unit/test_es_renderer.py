"""Tests for ExecutiveSummaryRenderer — deterministic ES generation from accepted claims."""

from __future__ import annotations

from acolyte.domain.executive_summary import ExecutiveSummaryRenderer


class TestExecutiveSummaryRenderer:
    def test_produces_nonempty_body(self):
        """Claims → non-empty Japanese summary string."""
        claims = [
            {
                "claim_id": "es-1",
                "claim": "AI半導体市場は2026年に30%成長した",
                "claim_type": "synthesis",
                "evidence_ids": ["src_1"],
                "supporting_quotes": ["市場は30%成長"],
                "numeric_facts": ["30%"],
                "must_cite": True,
            },
            {
                "claim_id": "es-2",
                "claim": "電力効率が次世代チップの最大課題である",
                "claim_type": "synthesis",
                "evidence_ids": ["src_2"],
                "supporting_quotes": [],
                "numeric_facts": [],
                "must_cite": True,
            },
        ]
        renderer = ExecutiveSummaryRenderer()
        body = renderer.render(claims, topic="AI半導体の2026年動向")
        assert body
        assert len(body) > 0

    def test_includes_numeric_facts(self):
        """Body must contain numeric data from claims."""
        claims = [
            {
                "claim_id": "es-1",
                "claim": "市場規模が50億ドルに達した",
                "numeric_facts": ["50億ドル"],
                "evidence_ids": ["src_1"],
                "supporting_quotes": [],
            },
        ]
        renderer = ExecutiveSummaryRenderer()
        body = renderer.render(claims, topic="市場動向")
        assert "50億ドル" in body

    def test_respects_max_claims(self):
        """Should not produce more sentences than claims provided."""
        claims = [
            {
                "claim_id": f"es-{i}",
                "claim": f"クレーム{i}の内容",
                "numeric_facts": [],
                "evidence_ids": [],
                "supporting_quotes": [],
            }
            for i in range(5)
        ]
        renderer = ExecutiveSummaryRenderer()
        body = renderer.render(claims, topic="テスト")
        # Should have at most 5 sentence-ending periods
        sentence_count = body.count("。")
        assert sentence_count <= 5

    def test_empty_claims_returns_empty(self):
        """No claims → empty string."""
        renderer = ExecutiveSummaryRenderer()
        body = renderer.render([], topic="テスト")
        assert body == ""

    def test_citations_from_evidence_ids(self):
        """Renderer must produce citations from claims' evidence_ids."""
        claims = [
            {
                "claim_id": "es-1",
                "claim": "重要な発見",
                "evidence_ids": ["src_1", "src_2"],
                "supporting_quotes": ["quote1"],
                "numeric_facts": [],
            },
        ]
        renderer = ExecutiveSummaryRenderer()
        citations = renderer.build_citations(claims)
        assert len(citations) == 2
        assert citations[0]["source_id"] == "src_1"
        assert citations[1]["source_id"] == "src_2"
