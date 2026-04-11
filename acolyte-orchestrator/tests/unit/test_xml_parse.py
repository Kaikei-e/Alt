"""Tests for XML DSL parser — pre-cleaner, tolerant parser, normalizers, orchestrator."""
# ruff: noqa: S314

from __future__ import annotations

import xml.etree.ElementTree as ET

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.xml_parse import (
    XmlParseError,
    extract_tag_block,
    generate_xml_validated,
    normalize_critic_output,
    normalize_plan_output,
    normalize_section_plan_output,
    parse_xmlish_block,
    strip_gemma_thought_block,
)

# ---------------------------------------------------------------------------
# Layer 1: Pre-cleaner
# ---------------------------------------------------------------------------


class TestStripGemmaThoughtBlock:
    def test_empty_think_block(self):
        text = "<think>\n</think>\n<plan><reasoning>ok</reasoning></plan>"
        assert "<think>" not in strip_gemma_thought_block(text)
        assert "<plan>" in strip_gemma_thought_block(text)

    def test_non_empty_think_block(self):
        text = "<think>\nLet me analyze this carefully.\n</think>\n<plan><reasoning>ok</reasoning></plan>"
        result = strip_gemma_thought_block(text)
        assert "Let me analyze" not in result
        assert "<plan>" in result

    def test_gemma4_channel_format(self):
        text = "<|channel>thought\nreasoning here<channel|><plan><reasoning>ok</reasoning></plan>"
        result = strip_gemma_thought_block(text)
        assert "reasoning here" not in result
        assert "<plan>" in result

    def test_no_thought_block(self):
        text = "<plan><reasoning>ok</reasoning></plan>"
        assert strip_gemma_thought_block(text) == text


class TestExtractTagBlock:
    def test_clean_xml(self):
        text = "<plan><reasoning>ok</reasoning></plan>"
        assert extract_tag_block(text, "plan") == text

    def test_prose_wrapping(self):
        text = "Here is the plan:\n<plan><reasoning>ok</reasoning></plan>\nDone."
        result = extract_tag_block(text, "plan")
        assert result == "<plan><reasoning>ok</reasoning></plan>"

    def test_missing_opening_tag(self):
        assert extract_tag_block("no xml here", "plan") is None

    def test_missing_closing_tag_truncation(self):
        text = "<plan><reasoning>ok</reasoning>"
        assert extract_tag_block(text, "plan") is None

    def test_multiline(self):
        text = "<plan>\n  <reasoning>ok</reasoning>\n</plan>"
        result = extract_tag_block(text, "plan")
        assert result is not None
        assert "<reasoning>ok</reasoning>" in result


# ---------------------------------------------------------------------------
# Layer 2: Tolerant parser
# ---------------------------------------------------------------------------


class TestParseXmlishBlock:
    def test_valid_xml(self):
        text = "<critic><verdict>accept</verdict></critic>"
        elem = parse_xmlish_block(text, "critic")
        assert elem.tag == "critic"
        assert elem.find("verdict").text == "accept"

    def test_with_thought_block(self):
        text = "<think>\nthinking...\n</think>\n<critic><verdict>revise</verdict></critic>"
        elem = parse_xmlish_block(text, "critic")
        assert elem.find("verdict").text == "revise"

    def test_bare_ampersand_repair(self):
        text = "<critic><reasoning>A &amp; B works, C & D too</reasoning><verdict>accept</verdict></critic>"
        elem = parse_xmlish_block(text, "critic")
        assert "C & D" in elem.find("reasoning").text or "C &amp; D" in ET.tostring(elem, encoding="unicode")

    def test_prose_around_xml(self):
        text = "Sure! Here's my analysis:\n<critic><verdict>accept</verdict></critic>\nHope this helps!"
        elem = parse_xmlish_block(text, "critic")
        assert elem.find("verdict").text == "accept"

    def test_no_xml_raises(self):
        with pytest.raises(XmlParseError):
            parse_xmlish_block("no xml here at all", "critic")

    def test_truncated_raises(self):
        with pytest.raises(XmlParseError):
            parse_xmlish_block("<critic><verdict>acc", "critic")

    def test_whitespace_in_content(self):
        text = "<plan>\n  <reasoning>\n    some reasoning\n  </reasoning>\n</plan>"
        elem = parse_xmlish_block(text, "plan")
        assert "some reasoning" in elem.find("reasoning").text


# ---------------------------------------------------------------------------
# Layer 3: Normalizers
# ---------------------------------------------------------------------------


class TestNormalizePlanOutput:
    def test_basic_plan(self):
        xml = """<plan>
          <reasoning>good strategy</reasoning>
          <section>
            <key>analysis</key>
            <query>AI chip trends 2026</query>
            <query>NVIDIA vs AMD market share</query>
          </section>
          <section>
            <key>conclusion</key>
            <query>future outlook</query>
          </section>
        </plan>"""
        elem = ET.fromstring(xml)
        result = normalize_plan_output(elem)
        assert result["reasoning"] == "good strategy"
        assert result["queries"]["analysis"] == ["AI chip trends 2026", "NVIDIA vs AMD market share"]
        assert result["queries"]["conclusion"] == ["future outlook"]

    def test_empty_queries_skipped(self):
        xml = "<plan><reasoning>ok</reasoning><section><key>analysis</key><query></query></section></plan>"
        elem = ET.fromstring(xml)
        result = normalize_plan_output(elem)
        assert result["queries"]["analysis"] == []

    def test_missing_reasoning(self):
        xml = "<plan><section><key>analysis</key><query>q1</query></section></plan>"
        elem = ET.fromstring(xml)
        result = normalize_plan_output(elem)
        assert result["reasoning"] == ""


class TestNormalizeCriticOutput:
    def test_accept(self):
        xml = """<critic>
          <reasoning>all good</reasoning>
          <verdict>accept</verdict>
        </critic>"""
        elem = ET.fromstring(xml)
        result = normalize_critic_output(elem)
        assert result["verdict"] == "accept"
        assert result["revise_sections"] == []

    def test_revise_with_feedback(self):
        xml = """<critic>
          <reasoning>needs work</reasoning>
          <verdict>revise</verdict>
          <revise_section>analysis</revise_section>
          <revise_section>conclusion</revise_section>
          <feedback>
            <section>analysis</section>
            <message>add more citations</message>
          </feedback>
        </critic>"""
        elem = ET.fromstring(xml)
        result = normalize_critic_output(elem)
        assert result["verdict"] == "revise"
        assert result["revise_sections"] == ["analysis", "conclusion"]
        assert result["feedback"]["analysis"] == "add more citations"

    def test_unknown_verdict_defaults_to_revise(self):
        xml = "<critic><reasoning>hmm</reasoning><verdict>maybe</verdict></critic>"
        elem = ET.fromstring(xml)
        result = normalize_critic_output(elem)
        assert result["verdict"] == "revise"


class TestNormalizeSectionPlanOutput:
    def test_single_claim(self):
        xml = """<section_plan>
          <reasoning>plan</reasoning>
          <claim>
            <text>AI chips grew 30%</text>
            <claim_type>statistical</claim_type>
            <evidence_id>src_1</evidence_id>
            <evidence_id>src_2</evidence_id>
            <supporting_quote>revenue increased by 30%</supporting_quote>
            <numeric_fact>30%</numeric_fact>
            <must_cite>true</must_cite>
          </claim>
        </section_plan>"""
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert result["reasoning"] == "plan"
        assert len(result["claims"]) == 1
        claim = result["claims"][0]
        assert claim["claim"] == "AI chips grew 30%"
        assert claim["claim_type"] == "statistical"
        assert claim["evidence_ids"] == ["src_1", "src_2"]
        assert claim["supporting_quotes"] == ["revenue increased by 30%"]
        assert claim["numeric_facts"] == ["30%"]
        assert claim["must_cite"] is True

    def test_must_cite_false(self):
        xml = """<section_plan><reasoning>r</reasoning>
          <claim><text>opinion</text><claim_type>synthesis</claim_type><must_cite>false</must_cite></claim>
        </section_plan>"""
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert result["claims"][0]["must_cite"] is False

    def test_unknown_claim_type_defaults(self):
        xml = """<section_plan><reasoning>r</reasoning>
          <claim><text>claim</text><claim_type>unknown_type</claim_type></claim>
        </section_plan>"""
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert result["claims"][0]["claim_type"] == "factual"

    def test_missing_text_drops_claim(self):
        xml = """<section_plan><reasoning>r</reasoning>
          <claim><claim_type>factual</claim_type></claim>
          <claim><text>valid claim</text><claim_type>factual</claim_type></claim>
        </section_plan>"""
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert len(result["claims"]) == 1
        assert result["claims"][0]["claim"] == "valid claim"

    def test_empty_claims(self):
        xml = "<section_plan><reasoning>r</reasoning></section_plan>"
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert result["claims"] == []

    def test_multiple_claims(self):
        xml = """<section_plan><reasoning>r</reasoning>
          <claim><text>c1</text><claim_type>factual</claim_type><must_cite>true</must_cite></claim>
          <claim><text>c2</text><claim_type>comparative</claim_type><must_cite>false</must_cite></claim>
        </section_plan>"""
        elem = ET.fromstring(xml)
        result = normalize_section_plan_output(elem)
        assert len(result["claims"]) == 2


# ---------------------------------------------------------------------------
# Orchestrator: generate_xml_validated
# ---------------------------------------------------------------------------


class FakeLLM:
    def __init__(self, responses: list[str] | None = None, default: str = "") -> None:
        self._responses = list(responses) if responses else []
        self._default = default
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        text = self._responses.pop(0) if self._responses else self._default
        return LLMResponse(text=text, model="fake", completion_tokens=50)


class TestGenerateXmlValidated:
    @pytest.mark.asyncio
    async def test_success(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        llm = FakeLLM(
            default="<plan><reasoning>ok</reasoning><section><key>analysis</key><query>q1</query></section></plan>"
        )
        result = await generate_xml_validated(
            llm,
            "test prompt",
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
        )
        assert isinstance(result, QueryExpansionOutput)
        assert result.queries == {"analysis": ["q1"]}

    @pytest.mark.asyncio
    async def test_fallback_on_parse_failure(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        fallback = QueryExpansionOutput(reasoning="fallback", queries={})
        llm = FakeLLM(default="broken output no xml")
        result = await generate_xml_validated(
            llm,
            "test",
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
            fallback=fallback,
        )
        assert result is fallback

    @pytest.mark.asyncio
    async def test_raises_without_fallback(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        llm = FakeLLM(default="broken")
        with pytest.raises(ValueError):
            await generate_xml_validated(
                llm,
                "test",
                QueryExpansionOutput,
                root_tag="plan",
                normalizer=normalize_plan_output,
            )

    @pytest.mark.asyncio
    async def test_no_format_in_llm_kwargs(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        llm = FakeLLM(default="<plan><reasoning>ok</reasoning></plan>")
        fallback = QueryExpansionOutput(reasoning="fb", queries={})
        await generate_xml_validated(
            llm,
            "test",
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
            fallback=fallback,
        )
        assert "format" not in llm.calls[0]

    @pytest.mark.asyncio
    async def test_retry_on_first_failure(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        llm = FakeLLM(
            responses=[
                "broken xml",
                "<plan><reasoning>ok</reasoning><section><key>a</key><query>q</query></section></plan>",
            ]
        )
        result = await generate_xml_validated(
            llm,
            "test",
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
            retries=1,
        )
        assert len(llm.calls) == 2
        assert result.queries == {"a": ["q"]}

    @pytest.mark.asyncio
    async def test_thought_block_stripped(self):
        from acolyte.domain.section_contract import QueryExpansionOutput

        xml_with_think = "<think>\nthinking...\n</think>\n<plan><reasoning>ok</reasoning></plan>"
        llm = FakeLLM(default=xml_with_think)
        fallback = QueryExpansionOutput(reasoning="fb", queries={})
        result = await generate_xml_validated(
            llm,
            "test",
            QueryExpansionOutput,
            root_tag="plan",
            normalizer=normalize_plan_output,
            fallback=fallback,
        )
        assert result.reasoning == "ok"
