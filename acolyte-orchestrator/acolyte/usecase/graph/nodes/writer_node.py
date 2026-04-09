"""Writer node — generates section bodies from curated evidence.

Reads per-section evidence from curated_by_section when available,
falling back to global curated list for backward compatibility.
Uses hydrated_evidence for full article bodies when present.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

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

        sections: dict[str, str] = dict(existing_sections)

        for section in outline:
            key = section.get("key", "")
            title = section.get("title", key)

            # Use per-section curated if available, else global curated
            if curated_by_section and key in curated_by_section:
                section_evidence = curated_by_section[key]
            else:
                section_evidence = curated

            evidence_block = _format_evidence(section_evidence, hydrated)

            revision_note = ""
            if critique and key in critique.get("revise_sections", []):
                feedback = critique.get("feedback", {}).get(key, "")
                revision_note = f"Previous feedback: {feedback}\nPlease revise accordingly."

            prompt = WRITER_PROMPT.format(
                title=title,
                topic=brief.get("topic", ""),
                evidence_block=evidence_block,
                revision_note=revision_note,
            )
            response = await self._llm.generate(prompt, num_predict=2000)
            sections[key] = response.text

        logger.info("Writer completed", section_count=len(sections))
        return {"sections": sections, "revision_count": state.get("revision_count", 0) + 1}
