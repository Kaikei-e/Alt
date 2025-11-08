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


RECAP_CLUSTER_SUMMARY_PROMPT = """<start_of_turn>system
You are an expert Japanese news editor. Generate concise Japanese bullet summaries for grouped evidence clusters.
Always respond in JSON following this structure exactly:
{{
  "title": "15〜40文字程度の日本語タイトル（句読点含む）",
  "bullets": [
    "1文40〜90文字程度の要点（常体、箇条書き用）"
  ],
  "language": "ja"
}}
Constraints:
- Bullet count <= {max_bullets}. Prefer {max_bullets} unless information is clearly insufficient.
- Use 常体（〜だ／である）; no Markdown or numbering.
- Mention concrete facts (数値、日付、固有名詞) if available. Avoid speculation.
- If critical data missing, state 「未提示」 within the bullet.
- Keep each bullet unique; cover背景・影響・見通し where possible.
<end_of_turn>
<start_of_turn>user
Job ID: {job_id}
Genre: {genre}
Top Terms (per cluster) and representative sentences are provided below.
Use them to infer the overall storyline and synthesize the summary.

{cluster_section}

Return ONLY the JSON object, without explanations.
<end_of_turn>
<start_of_turn>model
"""