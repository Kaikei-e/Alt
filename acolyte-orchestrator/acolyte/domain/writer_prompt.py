"""Shared evidence-based writer prompt and formatting.

Public API used by both the LangGraph WriterNode (section-level fallback
path) and RerunSectionUsecase, so it lives outside either module's
private namespace.
"""

from __future__ import annotations

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
- 出典は必ず [S1], [S2] の形式のみで本文中に記す。記事タイトル・URL・サイト名・タグを本文中に書いてはならない"""


def format_evidence(
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
