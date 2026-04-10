"""Writer node — generates section bodies from curated evidence or claim plans.

Issue 3: Paragraph-level micro-generation.
  Each claim produces exactly 1 paragraph via a single LLM call.
  Accepted paragraphs are immutable; only rejected ones are regenerated.
  best_sections tracks the best non-empty, non-blocking revision body.

When claim_plans are present (from SectionPlannerNode), uses paragraph-based
generation. Otherwise falls back to evidence-based generation for
backward compatibility.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from acolyte.port.llm_provider import LLMMode

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

# Paragraph-level prompts (Issue 3)
PARAGRAPH_WRITER_PROMPT = """あなたはプロのレポートライターです。以下のクレーム1件について、1段落で日本語で執筆してください。

<topic>{topic}</topic>
<section>{section_title}</section>
<claim>{claim}</claim>
<supporting_quotes>{supporting_quotes}</supporting_quotes>
<evidence_ids>{evidence_ids}</evidence_ids>
{delta_feedback_block}
ルール:
- 1段落のみ出力すること
- 参考記事番号を [1], [2] のように付記すること
- 新事実を追加しないこと
- numeric_facts がある場合は必ず本文に含めること"""

CONCLUSION_PARAGRAPH_PROMPT = """あなたはプロのレポートライターです。以下の統合クレーム1件について、1段落で日本語で結論を執筆してください。

<topic>{topic}</topic>
<section>{section_title}</section>
<claim>{claim}</claim>
<supporting_quotes>{supporting_quotes}</supporting_quotes>
{delta_feedback_block}
ルール:
- 1段落のみ出力すること
- 新事実を追加しないこと — 意味づけ・リスク・優先順位・推奨行動に限定
- Analysis の文をそのまま再掲しないこと
- 出典番号を [1], [2] のように付記すること"""

ES_PARAGRAPH_PROMPT = """あなたはプロのレポートライターです。以下の主要な発見1件について、1段落で日本語で要旨を執筆してください。

<topic>{topic}</topic>
<section>{section_title}</section>
<claim>{claim}</claim>
<supporting_quotes>{supporting_quotes}</supporting_quotes>
{delta_feedback_block}
ルール:
- 1段落のみ、簡潔に出力すること
- 新事実を追加しないこと — 既存セクションの発見の要約のみ
- 数値データがある場合は必ず含めること
- 出典番号を [1], [2] のように付記すること"""

# Legacy section-level prompts (kept for backward compat path)
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


def _assemble_paragraph_citations(claim: dict, paragraph_body: str) -> list[dict]:
    """Build citations for a single paragraph from its claim."""
    evidence_ids = claim.get("evidence_ids", [])
    quotes = claim.get("supporting_quotes", [])
    claim_id = claim.get("claim_id", "")
    citations: list[dict] = []

    for eid in evidence_ids:
        citation: dict = {
            "claim_id": claim_id,
            "source_id": eid,
            "source_type": "article",
            "quote": "",
            "offset_start": -1,
            "offset_end": -1,
        }
        for q in quotes:
            search_fragment = q[:50] if len(q) > 50 else q
            idx = paragraph_body.find(search_fragment)
            if idx >= 0:
                citation["quote"] = q
                citation["offset_start"] = idx
                citation["offset_end"] = idx + len(q)
                break
        if not citation["quote"] and quotes:
            citation["quote"] = quotes[0]
        citations.append(citation)
    return citations


def _select_paragraph_prompt(section_role: str) -> str:
    """Select the paragraph prompt template based on section_role."""
    if section_role == "conclusion":
        return CONCLUSION_PARAGRAPH_PROMPT
    if section_role == "executive_summary":
        return ES_PARAGRAPH_PROMPT
    return PARAGRAPH_WRITER_PROMPT


def _format_supporting_quotes(quotes: list[str]) -> str:
    """Format supporting quotes for prompt injection."""
    if not quotes:
        return "なし"
    return "\n".join(f'- "{q}"' for q in quotes)


def _update_best_sections(
    current_best: dict[str, str],
    current_metrics: dict[str, dict],
    section_key: str,
    body: str,
    blocking_count: int,
) -> tuple[dict[str, str], dict[str, dict]]:
    """Update best_sections if this revision is better than the previous best.

    Selection: blocking_count minimum → non-empty → char_len.
    """
    new_best = dict(current_best)
    new_metrics = dict(current_metrics)

    char_len = len(body)
    existing = current_metrics.get(section_key)

    if not existing:
        # No previous best — accept if non-empty
        if char_len > 0:
            new_best[section_key] = body
            new_metrics[section_key] = {"blocking_count": blocking_count, "char_len": char_len}
    else:
        # Compare: fewer blocking → non-empty → longer
        prev_blocking = existing["blocking_count"]
        prev_len = existing["char_len"]
        is_better = False

        if blocking_count < prev_blocking:
            is_better = True
        elif blocking_count == prev_blocking and char_len > 0:
            if prev_len == 0:
                is_better = True
            elif char_len > prev_len:
                is_better = True

        if is_better:
            new_best[section_key] = body
            new_metrics[section_key] = {"blocking_count": blocking_count, "char_len": char_len}

    return new_best, new_metrics


class WriterNode:
    def __init__(self, llm: LLMProviderPort) -> None:
        self._llm = llm

    async def _generate_paragraph(
        self,
        claim: dict,
        section_title: str,
        section_role: str,
        topic: str,
        *,
        delta_feedback: str = "",
        num_predict: int = 1000,
    ) -> dict:
        """Generate a single paragraph from one claim via LLM.

        Returns a GeneratedParagraph-compatible dict.
        """
        prompt_template = _select_paragraph_prompt(section_role)
        quotes_str = _format_supporting_quotes(claim.get("supporting_quotes", []))
        eids = claim.get("evidence_ids", [])
        numeric_facts = claim.get("numeric_facts", [])

        delta_block = ""
        if delta_feedback:
            delta_block = f"<delta_feedback>{delta_feedback}</delta_feedback>\n"

        prompt = prompt_template.format(
            topic=topic,
            section_title=section_title,
            claim=claim.get("claim", ""),
            supporting_quotes=quotes_str,
            evidence_ids=", ".join(eids),
            delta_feedback_block=delta_block,
        )

        # Add numeric facts reminder if present
        if numeric_facts:
            prompt += f"\n\n数値データ: {', '.join(numeric_facts)}"

        response = await self._llm.generate(prompt, num_predict=num_predict, mode=LLMMode.LONGFORM)
        body = response.text.strip()

        citations = _assemble_paragraph_citations(claim, body) if body else []
        status = "accepted" if body else "rejected"

        return {
            "claim_id": claim.get("claim_id", ""),
            "claim_text": claim.get("claim", ""),
            "body": body,
            "status": status,
            "citations": citations,
            "revision_feedback": delta_feedback,
        }

    async def _generate_section_paragraphs(
        self,
        claims: list[dict],
        section_title: str,
        section_role: str,
        topic: str,
        *,
        existing_paragraphs: list[dict] | None = None,
        claim_feedbacks: list[dict] | None = None,
        num_predict: int = 1000,
    ) -> list[dict]:
        """Generate paragraphs for all claims in a section.

        On revision, only regenerates rejected/targeted paragraphs.
        """
        # Build lookup for existing paragraphs and feedbacks
        existing_by_id: dict[str, dict] = {}
        if existing_paragraphs:
            for p in existing_paragraphs:
                existing_by_id[p["claim_id"]] = p

        feedback_by_id: dict[str, dict] = {}
        if claim_feedbacks:
            for fb in claim_feedbacks:
                feedback_by_id[fb["claim_id"]] = fb

        paragraphs: list[dict] = []
        for claim in claims:
            claim_id = claim.get("claim_id", "")
            existing = existing_by_id.get(claim_id)

            if existing and existing.get("status") == "accepted" and claim_id not in feedback_by_id:
                # Accepted paragraph — keep immutable
                paragraphs.append(existing)
                continue

            # Generate or regenerate
            delta = ""
            fb = feedback_by_id.get(claim_id)
            if fb:
                delta = fb.get("reason", "")

            para = await self._generate_paragraph(
                claim,
                section_title,
                section_role,
                topic,
                delta_feedback=delta,
                num_predict=num_predict,
            )
            paragraphs.append(para)

        return paragraphs

    async def __call__(self, state: ReportGenerationState) -> dict:
        outline = state.get("outline", [])
        curated = state.get("curated", [])
        curated_by_section = state.get("curated_by_section")
        hydrated = state.get("hydrated_evidence")
        brief = state.get("brief") or state.get("scope") or {}
        critique = state.get("critique")
        existing_sections = state.get("sections", {})
        claim_plans = state.get("claim_plans")
        existing_paragraphs = state.get("section_paragraphs", {})
        current_best = dict(state.get("best_sections", {}))
        current_metrics = dict(state.get("best_section_metrics", {}))

        sections: dict[str, str] = dict(existing_sections)
        section_citations: dict[str, list[dict]] = {}
        section_paragraphs: dict[str, list[dict]] = dict(existing_paragraphs)
        use_claims = claim_plans is not None
        topic = brief.get("topic", "")

        # Extract claim_feedbacks from critique
        all_claim_feedbacks: dict[str, list[dict]] = {}
        if critique:
            all_claim_feedbacks = critique.get("claim_feedbacks", {})

        # Process ES last so it uses accepted claims from all other sections
        non_es = [s for s in outline if s.get("section_role") != "executive_summary"]
        es_sections = [s for s in outline if s.get("section_role") == "executive_summary"]

        for section in non_es + es_sections:
            key = section.get("key", "")
            title = section.get("title", key)
            section_role = section.get("section_role", "general")

            if use_claims:
                # Paragraph-based generation path
                claims = claim_plans.get(key, [])
                if not claims:
                    logger.warning("No claims for section, producing empty body", section_key=key)
                    sections[key] = ""
                    section_citations[key] = []
                    section_paragraphs[key] = []
                    continue

                # Get existing paragraphs and feedbacks for this section
                sect_existing = existing_paragraphs.get(key)
                sect_feedbacks = all_claim_feedbacks.get(key)

                paragraphs = await self._generate_section_paragraphs(
                    claims,
                    title,
                    section_role,
                    topic,
                    existing_paragraphs=sect_existing,
                    claim_feedbacks=sect_feedbacks,
                    num_predict=1000,
                )

                section_paragraphs[key] = paragraphs

                # Assemble section body from accepted/generated paragraphs (in order)
                accepted_bodies = [p["body"] for p in paragraphs if p["body"]]
                section_body = "\n\n".join(accepted_bodies)
                sections[key] = section_body

                # Assemble all citations from paragraphs
                all_cites: list[dict] = []
                for p in paragraphs:
                    all_cites.extend(p.get("citations", []))
                section_citations[key] = all_cites

                # Count blocking paragraphs (rejected/empty)
                blocking_count = sum(1 for p in paragraphs if p["status"] == "rejected")
                current_best, current_metrics = _update_best_sections(
                    current_best, current_metrics, key, section_body, blocking_count,
                )
            else:
                # Legacy evidence-based path (backward compat)
                revision_note = ""
                if critique and key in critique.get("revise_sections", []):
                    feedback = critique.get("feedback", {}).get(key, "")
                    revision_note = f"Previous feedback: {feedback}\nPlease revise accordingly."

                if curated_by_section and key in curated_by_section:
                    section_evidence = curated_by_section[key]
                else:
                    section_evidence = curated

                evidence_block = _format_evidence(section_evidence, hydrated)
                prompt = WRITER_PROMPT.format(
                    title=title,
                    topic=topic,
                    evidence_block=evidence_block,
                    revision_note=revision_note,
                )

                response = await self._llm.generate(prompt, num_predict=2000, mode=LLMMode.LONGFORM)
                sections[key] = response.text

        logger.info("Writer completed", section_count=len(sections), claim_based=use_claims)
        result: dict = {
            "sections": sections,
            "revision_count": state.get("revision_count", 0) + 1,
        }
        if use_claims:
            result["section_citations"] = section_citations
            result["section_paragraphs"] = section_paragraphs
            result["best_sections"] = current_best
            result["best_section_metrics"] = current_metrics
        return result
