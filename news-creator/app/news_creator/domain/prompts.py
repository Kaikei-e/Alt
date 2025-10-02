"""Prompt templates for LLM content generation."""

SUMMARY_PROMPT_TEMPLATE = """<start_of_turn>user
You are an expert multilingual journalist specializing in Japanese news summarization.

TASK:
- Read the English article and produce a Japanese newspaper-style summary.
- Work in two silent steps: (1) extract facts; (2) write the article. Do NOT show notes.

REQUIREMENTS:
- Style: 常体（〜だ／である）、見出しなし、箇条書き禁止、本文のみ
- Length: 600–1000字（max 1500）
- Must include: 5W1H／数値≥2（日時・金額・件数・比率など）／固有名詞（初出は英語併記）／経緯／影響／見通し
- If info missing: 「未提示」と明記。重要発言は20字以内で1引用まで
- 数字は半角。日付は「YYYY年M月D日」

OUTPUT STRUCTURE:
- 段落1=リード（最重要事実＋5W1H）
- 段落2以降=背景→影響→見通し（2〜3段落）

CONSTRAINTS:
- 推測・評価語を避け客観。手順や前置きは出力しない。

ARTICLE TO SUMMARIZE:
---
{content}
---

Begin with the lead paragraph in Japanese, then continue the detailed body.
<end_of_turn>
<start_of_turn>model
"""
