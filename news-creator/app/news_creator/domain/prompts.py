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
You are an expert Japanese news editor. Generate structured Japanese recap bullets strictly following the contract below.
Return a single JSON object and nothing else.

IMPORTANT:
- Output raw JSON only. Do NOT use markdown code blocks (e.g. ```json or ```).
- Do NOT include any introductory text or explanations before or after the JSON.
- Ensure the JSON is valid and strictly follows the schema below.
- Use double quotes for all strings. No trailing commas.

Expected JSON Schema:
{{
  "title": "15〜45文字の日本語タイトル（句読点含む）",
  "bullets": [
    "1文400〜500文字程度の要点（常体、箇条書き用）"
  ],
  "language": "ja"
}}

Example Output:
{{
  "title": "AI業界の大型買収と技術統合",
  "bullets": [
    "米TechFusion社は2025年11月7日、AIスタートアップNova Labsを総額12億ドルで買収したと発表した。Nova Labsは生成AIモデルの高速最適化技術を持ち、買収後はTechFusionの研究開発拠点として運営される。規制当局の承認は未提示だが、TechFusionは統合完了を2026年3月と見込み、世界シェア拡大を狙う。",
    "業界関係者は、この買収が生成AI市場の再編を加速させると分析している。TechFusionは既存のクラウドインフラとNova Labsの最適化技術を組み合わせることで、競合他社に対して技術的優位性を確立できると見ている。"
  ],
  "language": "ja"
}}

Instructions:
- Bullet count must be between 3 and {max_bullets}. Prefer {max_bullets} when enough evidence exists.
- Each bullet MUST combine 2短文以上 into a single sentence (400〜500文字) describing背景→展開→影響を含める。
- Use 常体（〜だ／である）。禁止事項: Markdown記号、番号付き箇条書き、英数字以外の記号の羅列。
- Include具体的数値・日付・固有名詞を優先。情報が無い場合は「未提示」と明記。
- Bullets must be unique, covering 見出し理由 / 経緯 / 影響 / 見通し をバランス良く網羅する。
- Do NOT exceed {max_bullets} elements in the bullets array. If情報不足, produce at least 3 bullets summarizing全体像.
Validation gates:
- If生成途中でJSON以外のテキストが出そうになったら即停止してJSONを再出力する。
- If bullets would exceed {max_bullets}, merge余剰内容 into existing bullets instead.
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