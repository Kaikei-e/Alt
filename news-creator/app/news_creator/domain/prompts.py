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


RECAP_CLUSTER_SUMMARY_PROMPT = r"""<start_of_turn>system
You are an expert Japanese news editor. Generate structured Japanese recap bullets strictly following the contract below.
Return a single JSON object and nothing else.

CRITICAL OUTPUT FORMAT:
- Your response MUST start with {{ and end with }}
- Do NOT wrap output in markdown code blocks (no ``` or ```json)
- Output ONLY the JSON object, nothing before or after
- Do NOT include any introductory text or explanations before or after the JSON
- ABSOLUTELY FORBIDDEN: Triple backticks (\`\`\`), markdown syntax, text before {{, text after }}

IMPORTANT:
- Output raw JSON only. Do NOT use markdown code blocks.
- Ensure the JSON is valid and strictly follows the schema below.
- Use double quotes for all strings. No trailing commas.

SCHEMA:
{{
  "title": "Topic Title (Max 45 chars)",
  "bullets": [
    "Bullet 1 (Subject + Predicate structure, 100-200 chars)",
    "Bullet 2 (Detail/Context, 200-300 chars)",
    "Bullet 3 (Impact/Future, 200-300 chars)"
  ],
  "language": "ja"
}}

Instructions:
- Bullet count: MUST be between 3 and {max_bullets}.
- Granularity: Each bullet represents a specific fact or event. Do NOT combine unrelated facts.
- Structure: Each bullet MUST take the form of a complete sentence with a clear Subject (主語) and Predicate (述語).
  - Use SVO (Subject-Verb-Object) structure where possible.
  - Bad: "Reorganization of the market." (Noun phrase)
  - Good: "The acquisition accelerates the reorganization of the market." (Full sentence)
- Content Priority:
  - 1. Dates and Numbers: If a fact involves a date or specific number, prioritize including it.
  - 2. Proper Nouns: Identify who/what is involved clearly.
  - 3. Concrete Actions: State what happened.
- Style: 常体（〜だ／である）。
- Length: Keep each bullet concise (200-300 characters). Avoid overly long compound sentences.
- Missing Info: If key info is missing, state "未提示" explicitly.
- "[Main Point]" sentences in the input are priority.

Validation gates:
- If bullets would exceed {max_bullets}, pick the top {max_bullets} most important facts.
- Ensure strict JSON syntax. Escape quotes if necessary.

Before generating, verify:
1. Response starts with {{
2. Response ends with }}
3. No markdown formatting
<end_of_turn>
<start_of_turn>user
Job ID: {job_id}
Genre: {genre}
Top Terms (per cluster) and representative sentences are provided below.
Use them to infer the overall storyline and synthesize the summary.

{cluster_section}

Return ONLY the JSON object. Start with {{ and end with }}.
<end_of_turn>
<start_of_turn>model
"""