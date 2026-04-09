"""Writer node — generates section bodies from curated evidence."""

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
- 参考記事がない場合は、トピックに関する一般知識で書くこと
- 追加情報を求めないこと — 手元の情報で最善のセクションを書くこと"""


def _format_evidence(curated: list[dict]) -> str:
    """Format evidence items into a readable block for the writer."""
    if not curated:
        return "参考記事なし。トピックに関する一般知識で執筆してください。"

    lines = ["参考記事:"]
    for i, item in enumerate(curated[:10], 1):
        title = item.get("title", "Untitled")
        excerpt = item.get("excerpt", "")
        source_type = item.get("type", "article")
        line = f"{i}. [{source_type}] {title}"
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
        scope = state.get("scope", {})
        critique = state.get("critique")
        existing_sections = state.get("sections", {})

        sections: dict[str, str] = dict(existing_sections)
        evidence_block = _format_evidence(curated)

        for section in outline:
            key = section.get("key", "")
            title = section.get("title", key)

            revision_note = ""
            if critique and key in critique.get("revise_sections", []):
                feedback = critique.get("feedback", {}).get(key, "")
                revision_note = f"Previous feedback: {feedback}\nPlease revise accordingly."

            prompt = WRITER_PROMPT.format(
                title=title,
                topic=scope.get("topic", ""),
                evidence_block=evidence_block,
                revision_note=revision_note,
            )
            response = await self._llm.generate(prompt, num_predict=2000)
            sections[key] = response.text

        logger.info("Writer completed", section_count=len(sections))
        return {"sections": sections, "revision_count": state.get("revision_count", 0) + 1}
