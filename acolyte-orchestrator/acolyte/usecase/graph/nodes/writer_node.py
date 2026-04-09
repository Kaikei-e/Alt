"""Writer node — generates section bodies from curated evidence or claim plans.

When claim_plans are present (from SectionPlannerNode), uses claim-based
generation. Otherwise falls back to evidence-based generation for
backward compatibility.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

# Legacy evidence-based prompt (kept for RerunSectionUsecase import + backward compat)
WRITER_PROMPT = """あなたはプロのレポートライターです。「{title}」セクションを日本語で執筆してください。

トピック: {topic}

{evidence_block}

{revision_note}

以下のルールに従ってください:
- 必ず日本語で書くこと（技術用語・固有名詞は英語のまま可）
- 明確で構造化された、情報量の多いセクションを書くこと
- 具体的なデータや事例を含めること
- 参考記事がない場合は、その旨を明記し、証拠に基づかない主張を避けてください
- 追加情報を求めないこと — 手元の情報で最善のセクションを書くこと
- 各主張に参考記事番号を [1], [2] のように付記してください"""

# Claim-based prompt (used when SectionPlannerNode has produced claim_plans)
CLAIM_WRITER_PROMPT = """あなたはプロのレポートライターです。「{title}」セクションを日本語で執筆してください。

トピック: {topic}

以下の計画済みクレームに基づいて本文を構成してください。
計画にないクレームは追加しないでください。

{claims_block}

{revision_note}

ルール:
- 必ず日本語で書くこと（技術用語・固有名詞は英語のまま可）
- 各クレームの supporting_quotes を根拠として使うこと
- 各主張に参考記事番号を [1], [2] のように付記してください
- 計画にない新事実を追加しないこと
- numeric_facts がある場合は必ず本文に含めること"""


# Conclusion-specific prompt (used when section_role == "conclusion")
CONCLUSION_WRITER_PROMPT = """あなたはプロのレポートライターです。「{title}」セクション（結論・統合判断）を日本語で執筆してください。

トピック: {topic}

以下の統合クレームに基づいて本文を構成してください。

{claims_block}

{revision_note}

ルール:
- 必ず日本語で書くこと（技術用語・固有名詞は英語のまま可）
- 新事実を追加しないこと — Analysis で示された事実の意味づけ・解釈のみ
- Analysis の文をそのまま再掲しないこと
- 出力は「意味づけ」「リスク」「優先順位」「推奨行動」に限定すること
- 各統合クレームの出典番号を [1], [2] のように付記してください"""

# Executive Summary-specific prompt (used when section_role == "executive_summary")
ES_WRITER_PROMPT = """あなたはプロのレポートライターです。「{title}」セクション（要旨）を日本語で執筆してください。

トピック: {topic}

以下の主要な発見に基づいて要旨を構成してください。

{claims_block}

{revision_note}

ルール:
- 必ず日本語で書くこと（技術用語・固有名詞は英語のまま可）
- 新事実を追加しないこと — 各セクションの主要な発見の要約のみ
- 簡潔に3-5文で最重要ポイントをまとめること
- 数値データがある場合は必ず1つ以上含めること
- 各発見の出典番号を [1], [2] のように付記してください"""


def _format_evidence(
    curated: list[dict],
    hydrated: dict[str, str] | None = None,
) -> str:
    """Format evidence items into a readable block for the writer."""
    if not curated:
        return "参考記事なし。トピックに関する一般知識で執筆してください。"

    hydrated = hydrated or {}
    lines = ["参考記事:"]
    for i, item in enumerate(curated[:10], 1):
        title = item.get("title", "Untitled")
        source_type = item.get("type", "article")
        item_id = item.get("id", "")
        line = f"{i}. [{source_type}] {title}"

        # Use hydrated body if available, otherwise fall back to excerpt
        body = hydrated.get(item_id, "")
        if body:
            line += f"\n   {body[:300]}"
        else:
            excerpt = item.get("excerpt", "")
            if excerpt:
                line += f"\n   {excerpt[:150]}"
        lines.append(line)
    return "\n".join(lines)


def _build_contract_instructions(section: dict) -> str:
    """Build contract-driven instructions from outline section dict."""
    lines: list[str] = []

    novelty_against = section.get("novelty_against", [])
    if novelty_against:
        keys_str = ", ".join(novelty_against)
        lines.append(f"- このセクションは以下のセクションの内容と重複してはいけません: {keys_str}")

    must_include = section.get("must_include_data_types", [])
    if must_include:
        types_str = ", ".join(must_include)
        lines.append(f"- 以下のデータ種別を必ず含めてください: {types_str}")

    if section.get("synthesis_only"):
        lines.append("- 既存クレームの統合のみ。新事実追加禁止")

    return "\n".join(lines)


def _format_claims(claims: list[dict], *, header: str = "計画済みクレーム:") -> str:
    """Format planned claims into a readable block for the writer."""
    lines = [header]
    for i, claim in enumerate(claims, 1):
        lines.append(f"{i}. [{claim.get('claim_type', 'factual')}] {claim['claim']}")
        for q in claim.get("supporting_quotes", []):
            lines.append(f'   根拠: "{q}"')
        for n in claim.get("numeric_facts", []):
            lines.append(f"   数値: {n}")
        eids = claim.get("evidence_ids", [])
        if eids:
            lines.append(f"   出典: {', '.join(eids)}")
    return "\n".join(lines)


def _assemble_citations(
    claims: list[dict],
    section_body: str,
) -> list[dict]:
    """Build structured citations from claim plan evidence mappings."""
    citations: list[dict] = []
    for claim in claims:
        claim_id = claim.get("claim_id", "")
        evidence_ids = claim.get("evidence_ids", [])
        quotes = claim.get("supporting_quotes", [])

        if claim.get("must_cite", True) and not evidence_ids:
            logger.warning("Claim has must_cite=True but no evidence_ids", claim_id=claim_id)

        for eid in evidence_ids:
            citation: dict = {
                "claim_id": claim_id,
                "source_id": eid,
                "source_type": "article",
                "quote": "",
                "offset_start": -1,
                "offset_end": -1,
            }
            # Best-effort: find a supporting quote in the section body
            for q in quotes:
                search_fragment = q[:50] if len(q) > 50 else q
                idx = section_body.find(search_fragment)
                if idx >= 0:
                    citation["quote"] = q
                    citation["offset_start"] = idx
                    citation["offset_end"] = idx + len(q)
                    break
            if not citation["quote"] and quotes:
                citation["quote"] = quotes[0]
            citations.append(citation)
    return citations


class WriterNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def __call__(self, state: ReportGenerationState) -> dict:
        outline = state.get("outline", [])
        curated = state.get("curated", [])
        curated_by_section = state.get("curated_by_section")
        hydrated = state.get("hydrated_evidence")
        brief = state.get("brief") or state.get("scope") or {}
        critique = state.get("critique")
        existing_sections = state.get("sections", {})
        claim_plans = state.get("claim_plans")

        sections: dict[str, str] = dict(existing_sections)
        section_citations: dict[str, list[dict]] = {}
        use_claims = claim_plans is not None

        # Process ES last so it uses accepted claims from all other sections
        non_es = [s for s in outline if s.get("section_role") != "executive_summary"]
        es_sections = [s for s in outline if s.get("section_role") == "executive_summary"]

        for section in non_es + es_sections:
            key = section.get("key", "")
            title = section.get("title", key)

            revision_note = ""
            if critique and key in critique.get("revise_sections", []):
                feedback = critique.get("feedback", {}).get(key, "")
                revision_note = f"Previous feedback: {feedback}\nPlease revise accordingly."

            if use_claims:
                # Claim-based generation path
                claims = claim_plans.get(key, [])
                if not claims:
                    logger.warning("No claims for section, producing empty body", section_key=key)
                    sections[key] = ""
                    section_citations[key] = []
                    continue

                # Build contract-driven instructions from outline
                contract_instructions = _build_contract_instructions(section)

                section_role = section.get("section_role", "general")
                if section_role == "executive_summary":
                    claims_block = _format_claims(claims, header="主要な発見:")
                    prompt = ES_WRITER_PROMPT.format(
                        title=title,
                        topic=brief.get("topic", ""),
                        claims_block=claims_block,
                        revision_note=revision_note,
                    )
                elif section_role == "conclusion":
                    claims_block = _format_claims(claims, header="統合クレーム:")
                    prompt = CONCLUSION_WRITER_PROMPT.format(
                        title=title,
                        topic=brief.get("topic", ""),
                        claims_block=claims_block,
                        revision_note=revision_note,
                    )
                else:
                    claims_block = _format_claims(claims)
                    prompt = CLAIM_WRITER_PROMPT.format(
                        title=title,
                        topic=brief.get("topic", ""),
                        claims_block=claims_block,
                        revision_note=revision_note,
                    )

                # Append contract instructions to prompt
                if contract_instructions:
                    prompt += f"\n\n追加制約:\n{contract_instructions}"

                response = await self._llm.generate(prompt, num_predict=2000)
                assembled = _assemble_citations(claims, response.text)

                # Reject section if ALL must_cite claims lack citations
                must_cite_claims = [c for c in claims if c.get("must_cite", True)]
                cited_ids = {ct["claim_id"] for ct in assembled}
                uncited = [c for c in must_cite_claims if c.get("claim_id", "") not in cited_ids]
                if uncited and len(uncited) == len(must_cite_claims):
                    logger.warning(
                        "All must_cite claims lack citations, rejecting section",
                        section_key=key,
                        uncited_count=len(uncited),
                    )
                    sections[key] = ""
                    section_citations[key] = []
                else:
                    if uncited:
                        logger.warning(
                            "Some must_cite claims lack citations",
                            section_key=key,
                            uncited_claim_ids=[c.get("claim_id") for c in uncited],
                        )
                    sections[key] = response.text
                    section_citations[key] = assembled
            else:
                # Legacy evidence-based path (backward compat)
                if curated_by_section and key in curated_by_section:
                    section_evidence = curated_by_section[key]
                else:
                    section_evidence = curated

                evidence_block = _format_evidence(section_evidence, hydrated)
                prompt = WRITER_PROMPT.format(
                    title=title,
                    topic=brief.get("topic", ""),
                    evidence_block=evidence_block,
                    revision_note=revision_note,
                )

                response = await self._llm.generate(prompt, num_predict=2000)
                sections[key] = response.text

        logger.info("Writer completed", section_count=len(sections), claim_based=use_claims)
        result: dict = {"sections": sections, "revision_count": state.get("revision_count", 0) + 1}
        if use_claims:
            result["section_citations"] = section_citations
        return result
