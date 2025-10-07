"""Prompt templates for LLM content generation."""

SUMMARY_PROMPT_TEMPLATE = """<start_of_turn>user
You are an expert multilingual journalist specializing in Japanese news summarization.

TASK:
- Read the English article and produce a Japanese newspaper-style summary.
- Work in two silent steps: (1) extract facts; (2) write the article. Do NOT show notes.

RULES AND CONSTRAINTS:
- Style: 常体（〜だ／である）、見出しなし、箇条書き禁止、本文のみ
- Length: EXACTLY 400-500字. Do NOT exceed 500 characters. Count characters carefully.
- Must include: 5W1H／数値≥2（日時・金額・件数・比率など）／固有名詞（初出は英語併記）／経緯／影響／見通し
- If info missing: 「未提示」と明記。重要発言は20字以内で1引用まで
- 数字は半角。日付は「YYYY年M月D日」

OUTPUT STRUCTURE:
- 段落1=リード（最重要事実＋5W1H）
- 段落2=背景・経緯（簡潔に）
- 段落3=影響・見通し（簡潔に）

CONSTRAINTS:
- DO NOT use any guesstimation or evaluation words. Do not include any preamble.
- CRITICAL: Stop generating when you reach 500 characters. Do not continue beyond this limit.
- Be extremely concise. Avoid unnecessary elaboration or repetition.
- Count characters as you write. Stop immediately at 500 characters.

ARTICLE TO SUMMARIZE:
---
{content}
---

Write exactly 3 short paragraphs in Japanese. Count characters as you write. Stop at 500 characters maximum.
<end_of_turn>
<start_of_turn>model
"""