"""Unit tests for ClaimPlan domain models."""

from __future__ import annotations

import json

from acolyte.domain.claim import ClaimPlannerOutput, PlannedClaim, SectionCitation, SectionClaimPlan


class TestPlannedClaim:
    def test_minimal_claim(self) -> None:
        claim = PlannedClaim(
            claim="AI market grew 20%",
            claim_type="statistical",
            evidence_ids=["art-1"],
            supporting_quotes=["The AI market expanded by 20%"],
        )
        assert claim.claim == "AI market grew 20%"
        assert claim.must_cite is True

    def test_claim_id_defaults_to_empty(self) -> None:
        claim = PlannedClaim(
            claim="test",
            claim_type="factual",
            evidence_ids=[],
            supporting_quotes=[],
        )
        assert claim.claim_id == ""

    def test_claim_id_can_be_set(self) -> None:
        claim = PlannedClaim(
            claim_id="analysis-1",
            claim="test",
            claim_type="factual",
            evidence_ids=["art-1"],
            supporting_quotes=["quote"],
        )
        assert claim.claim_id == "analysis-1"

    def test_claim_defaults(self) -> None:
        claim = PlannedClaim(
            claim="test",
            claim_type="factual",
            evidence_ids=[],
            supporting_quotes=[],
        )
        assert claim.numeric_facts == []
        assert claim.novelty_against == []
        assert claim.must_cite is True

    def test_claim_json_roundtrip(self) -> None:
        claim = PlannedClaim(
            claim="NVIDIA dominates GPU market",
            claim_type="factual",
            evidence_ids=["art-1", "art-2"],
            supporting_quotes=["NVIDIA controls 80%"],
            numeric_facts=["80%"],
            novelty_against=["analysis"],
            must_cite=True,
        )
        dumped = claim.model_dump()
        restored = PlannedClaim.model_validate(dumped)
        assert restored == claim

    def test_claim_json_schema_has_required_fields(self) -> None:
        schema = PlannedClaim.model_json_schema()
        required = schema.get("required", [])
        assert "claim" in required
        assert "claim_type" in required
        assert "evidence_ids" in required
        assert "supporting_quotes" in required


class TestSectionClaimPlan:
    def test_section_plan_with_claims(self) -> None:
        plan = SectionClaimPlan(
            section_key="analysis",
            claims=[
                PlannedClaim(
                    claim="test claim",
                    claim_type="factual",
                    evidence_ids=["art-1"],
                    supporting_quotes=["quote"],
                ),
            ],
        )
        assert plan.section_key == "analysis"
        assert len(plan.claims) == 1

    def test_section_plan_empty_claims(self) -> None:
        plan = SectionClaimPlan(section_key="conclusion", claims=[])
        assert plan.claims == []


class TestClaimPlannerOutput:
    def test_planner_output_with_reasoning(self) -> None:
        output = ClaimPlannerOutput(
            reasoning="This section needs statistical claims",
            claims=[
                PlannedClaim(
                    claim="AI market grew 20%",
                    claim_type="statistical",
                    evidence_ids=["art-1"],
                    supporting_quotes=["expanded by 20%"],
                ),
            ],
        )
        assert output.reasoning
        assert len(output.claims) == 1

    def test_planner_output_json_schema_for_ollama(self) -> None:
        """Schema should be usable as Ollama format parameter."""
        schema = ClaimPlannerOutput.model_json_schema()
        assert schema["type"] == "object"
        assert "reasoning" in schema["properties"]
        assert "claims" in schema["properties"]

    def test_planner_output_from_llm_json(self) -> None:
        """Simulate parsing LLM JSON output."""
        llm_json = json.dumps({
            "reasoning": "Need statistical and factual claims",
            "claims": [
                {
                    "claim": "AI market grew 20%",
                    "claim_type": "statistical",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["expanded by 20%"],
                    "numeric_facts": ["20%"],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        })
        parsed = json.loads(llm_json)
        output = ClaimPlannerOutput.model_validate(parsed)
        assert len(output.claims) == 1
        assert output.claims[0].numeric_facts == ["20%"]


class TestSectionCitation:
    def test_minimal_citation(self) -> None:
        citation = SectionCitation(
            claim_id="analysis-1",
            source_id="art-1",
        )
        assert citation.claim_id == "analysis-1"
        assert citation.source_id == "art-1"
        assert citation.source_type == "article"
        assert citation.quote == ""
        assert citation.offset_start == -1
        assert citation.offset_end == -1

    def test_citation_with_all_fields(self) -> None:
        citation = SectionCitation(
            claim_id="analysis-2",
            source_id="art-5",
            source_type="recap",
            quote="The market expanded by 20%",
            offset_start=42,
            offset_end=68,
        )
        assert citation.source_type == "recap"
        assert citation.quote == "The market expanded by 20%"
        assert citation.offset_start == 42
        assert citation.offset_end == 68

    def test_citation_json_roundtrip(self) -> None:
        citation = SectionCitation(
            claim_id="analysis-1",
            source_id="art-1",
            source_type="article",
            quote="AI grew 20%",
            offset_start=10,
            offset_end=21,
        )
        dumped = citation.model_dump()
        restored = SectionCitation.model_validate(dumped)
        assert restored == citation

    def test_citation_list_serialization(self) -> None:
        citations = [
            SectionCitation(claim_id="analysis-1", source_id="art-1", quote="quote1"),
            SectionCitation(claim_id="analysis-1", source_id="art-2", quote="quote2"),
        ]
        dumped = [c.model_dump() for c in citations]
        json_str = json.dumps(dumped)
        parsed = json.loads(json_str)
        restored = [SectionCitation.model_validate(d) for d in parsed]
        assert len(restored) == 2
        assert restored[0].source_id == "art-1"
        assert restored[1].source_id == "art-2"
