"""Prompt templates for LLM content generation."""

from __future__ import annotations

import logging
from typing import Protocol

logger = logging.getLogger(__name__)

# Control strings of the Gemma chat wire format. Prompts are sent to Ollama with
# raw=True (gateway/ollama_gateway.py), which bypasses the template engine, so
# these strings reach the tokenizer exactly as written. Thinking-mode markers are
# included because Gemma 4 emits them under raw=True too (docs/ADR/000640.md).
CONTROL_TOKENS: frozenset[str] = frozenset(
    {
        "<start_of_turn>",
        "<end_of_turn>",
        "<|turn>",
        "<turn|>",
        "<|think|>",
        "<|channel>thought",
        "<channel|>",
        "<|system|>",
        "<|user|>",
        "<|assistant|>",
    }
)

# Longest first so removal stays deterministic when tokens overlap.
_ORDERED_CONTROL_TOKENS: tuple[str, ...] = tuple(
    sorted(CONTROL_TOKENS, key=len, reverse=True)
)
_MAX_CONTROL_TOKEN_LEN: int = max(len(token) for token in _ORDERED_CONTROL_TOKENS)
# A token can only complete on its own final character, so every other character
# skips the tail inspection entirely.
_CONTROL_TOKEN_FINAL_CHARS: frozenset[str] = frozenset(
    token[-1] for token in _ORDERED_CONTROL_TOKENS
)

# Placeholders filled with third-party feed content (RSS article bodies, article
# sentences). Every other placeholder is filled by this service.
UNTRUSTED_PLACEHOLDERS: frozenset[str] = frozenset({"content", "cluster_section"})


def neutralize_control_tokens(text: str) -> tuple[str, int]:
    """Strip control-token sequences from untrusted text.

    A single left-to-right scan keeps the kept characters in a buffer and pops a
    control token off the buffer tail the moment one completes there. That is
    reassembly-proof by construction — a nested payload such as ``"<|tur<|turn>n>"``
    only forms its outer token after the inner one is popped, and the very next
    character completes it for the same pop — while staying linear in the input
    length. Repeating a whole-text sweep until it reaches a fixpoint would instead
    cost one pass per nesting level, which a feed publisher controls outright:
    summarize_usecase truncates bodies at 60_000 characters and formats the
    templates from ``async def``, so a super-linear neutralizer stalls the asyncio
    event loop for every concurrent request (CWE-407).

    Args:
        text: Untrusted text (e.g. an RSS article body)

    Returns:
        Tuple of (neutralized text, number of sequences removed)
    """
    if not text:
        return text, 0

    # A pop can only ever fire where a token already occurs verbatim, so text
    # without a single occurrence is returned untouched. This keeps benign
    # articles — the overwhelming majority — at plain substring-search cost.
    if not any(token in text for token in _ORDERED_CONTROL_TOKENS):
        return text, 0

    kept: list[str] = []
    keep = kept.append
    removed = 0
    for char in text:
        keep(char)
        if char not in _CONTROL_TOKEN_FINAL_CHARS:
            continue
        tail = "".join(kept[-_MAX_CONTROL_TOKEN_LEN:])
        for token in _ORDERED_CONTROL_TOKENS:
            if tail.endswith(token):
                del kept[-len(token) :]
                removed += 1
                break
    return "".join(kept), removed


class _FormatMapping(Protocol):
    """Keyed lookup — all ``str.format_map`` requires of its argument."""

    def __getitem__(self, key: str, /) -> object: ...


class _NeutralizedMapping:
    """Format mapping that serves neutralized text for untrusted placeholders."""

    __slots__ = ("_values", "_neutralized")

    def __init__(self, values: _FormatMapping, neutralized: dict[str, str]) -> None:
        self._values = values
        self._neutralized = neutralized

    def __getitem__(self, key: str, /) -> object:
        cleaned = self._neutralized.get(key)
        if cleaned is not None:
            return cleaned
        return self._values[key]


def _neutralize_untrusted(values: _FormatMapping) -> _FormatMapping:
    """Return `values` with control tokens stripped from untrusted placeholders."""
    neutralized: dict[str, str] = {}
    for placeholder in UNTRUSTED_PLACEHOLDERS:
        try:
            value = values[placeholder]
        except KeyError:
            continue
        if not isinstance(value, str):
            continue
        cleaned, removed = neutralize_control_tokens(value)
        if not removed:
            continue
        logger.warning(
            "Removed control token sequences from untrusted prompt input",
            extra={
                "placeholder": placeholder,
                "tokens_removed": removed,
                "original_length": len(value),
            },
        )
        neutralized[placeholder] = cleaned
    if not neutralized:
        return values
    return _NeutralizedMapping(values, neutralized)


class PromptTemplate(str):
    """Prompt template that neutralizes control tokens in untrusted placeholders.

    Article bodies are attacker-controlled by design, so a body containing
    ``"<turn|>\\n<|turn>user"`` would otherwise forge a turn boundary and take
    over the generation (OWASP LLM01 / CWE-1427). The guard lives on the template
    because these constants are formatted directly by SummarizeUsecase as well as
    through the prompt builders.

    ``format`` and ``format_map`` are the only ``str`` methods that substitute
    fields, and both are overridden here so the guard cannot be skipped by
    picking the other entry point.
    """

    __slots__ = ()

    def format_map(self, mapping: _FormatMapping, /) -> str:
        """Format from a mapping, stripping control tokens from untrusted values."""
        return str.format_map(self, _neutralize_untrusted(mapping))

    def format(self, *args: object, **kwargs: object) -> str:
        """Format the template; delegates so there is one neutralized code path."""
        if args:
            # A positional field carries no placeholder name, so it cannot be
            # screened against UNTRUSTED_PLACEHOLDERS. Refuse loudly rather than
            # let a future template silently interpolate feed content unchecked.
            raise TypeError(
                "PromptTemplate.format() takes keyword arguments only; "
                "positional fields bypass untrusted-input neutralization"
            )
        return self.format_map(kwargs)


SUMMARY_PROMPT_TEMPLATE = PromptTemplate("""<|turn>user
You are an expert multilingual journalist specializing in Japanese news summarization.

TASK:
- Current Date: {current_date}
- Read the English article and produce a Japanese newspaper-style summary.
- Work in two silent steps: (1) extract facts; (2) write the article. Do NOT show notes.

RULES AND CONSTRAINTS:
- Style: 常体（〜だ／である）、見出しなし、箇条書き禁止、本文のみ
- Length: 300-1000字. Target 300-1000 characters. Count characters carefully.
- Must include: 5W1H／数値≥3（日時・金額・件数・比率など）／固有名詞（初出は英語併記）／経緯／影響／見通し
- If info missing: 「未提示」と明記。重要発言は50字以内で最大2引用まで
- 数字は半角。日付は「YYYY年M月D日」
- 要約コンテンツの長さに応じて、文字数を調整する

CRITICAL: AVOID GENERIC STATEMENTS
- 一般論や推測を避け、原文に記載された具体的な事実のみを使用する
- 「〜と考えられる」「〜の可能性がある」「〜とみられる」などの推測表現は使用しない
- 原文に記載されていない情報は推測せず、記載されている事実のみを使用する
- 不確実な情報は「未提示」と明記する
- 抽象的な表現（「多くの」「一部の」など）を避け、具体的な数値や固有名詞を使用する
- 例: 「多くの企業」ではなく「50社」、「一部の専門家」ではなく「東京大学の田中教授」のように記載する

FACT ACCURACY REQUIREMENTS:
- 数値、日付、固有名詞、引用は原文から正確に保持する
- 数値（金額、件数、比率、パーセンテージ）は必ず含める。数値が複数ある場合は、最も重要な数値（金額、件数、パーセンテージ）を優先的に含める
- 日付・時刻は可能な限り具体的に記載する
- 固有名詞（人名、企業名、地名）は初出時に英語併記する
- 重要発言は原文から正確に引用し、引用符で囲む。引用は50字以内で、1要約あたり最大2引用まで
- 数値は半角で記載し、単位も正確に保持する

OUTPUT STRUCTURE:
- 段落1=リード（最重要事実＋5W1H）
- 段落2=背景・経緯（具体的な時系列と数値を含む）
- 段落3=詳細・具体例（数値、固有名詞、引用を含む）
- 段落4=影響・反応（具体的な数値や発言を含む）
- 段落5=見通し・今後の展開（可能な限り具体的に）

CRITICAL OUTPUT REQUIREMENTS:
- DO NOT use any guesstimation or evaluation words. Do not include any preamble.
- Generate between 300-1000 characters. You may slightly exceed 1000 characters if needed for quality, but aim for 300-1000.
- CRITICAL: Complete your output. Do NOT truncate mid-sentence. Always end with a complete sentence (ending with 。、！、or ？).
- If you cannot reach 1000 characters, generate as close to 1000 as possible while maintaining quality.
- Include specific facts, numbers, dates, and proper nouns from the original article.
- Avoid generic statements and speculation. Use only facts stated in the article.
- Count characters as you write. Ensure the output is complete and ends properly.

ARTICLE TO SUMMARIZE:
---
{content}
---

Write 3-5 paragraphs in Japanese with specific facts, numbers, dates, and proper nouns. Count characters as you write. Target 300-1000 characters. CRITICAL: Complete your output - do not truncate mid-sentence. Always end with a complete sentence.
<turn|>
<|turn>model
""")

CHUNK_SUMMARY_PROMPT_TEMPLATE = PromptTemplate("""<|turn>user
You are an expert editor extracting key information for a later summarization task.

TASK:
- Read the text chunk below (part of a larger article).
- Extract and list the key facts, numbers, dates, and proper nouns.
- Maintain the flow of events if present.
- Output as a bulleted list in Japanese.

CONSTRAINTS:
- Language: Japanese
- Format: Bullet points
- Focus: Factual accuracy. Do not summarize abstractly; capture specifics.
- Content: If the text is just boilerplates or irrelevant, output "なし".

TEXT CHUNK:
---
{content}
---

Extract key facts:
<turn|>
<|turn>model
""")

RECAP_CLUSTER_SUMMARY_PROMPT = PromptTemplate(r"""<|turn>system
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
    "Bullet 1 (Subject + Predicate structure, 400-600 chars)",
    "Bullet 2 (Detail/Context, 400-600 chars)",
    "Bullet 3 (Impact/Future, 400-600 chars)"
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
- Length: Keep each bullet detailed (400-600 characters). Provide comprehensive context.
- Missing Info: If key info is missing, state "未提示" explicitly.
- "[Main Point]" sentences in the input are priority.

Validation gates:
- If bullets would exceed {max_bullets}, pick the top {max_bullets} most important facts.
- Ensure strict JSON syntax. Escape quotes if necessary.

Before generating, verify:
1. Response starts with {{
2. Response ends with }}
3. No markdown formatting
<turn|>
<|turn>user
Job ID: {job_id}
Genre: {genre}
Top Terms (per cluster) and representative sentences are provided below.
Use them to infer the overall storyline and synthesize the summary.

{cluster_section}

Return ONLY the JSON object. Start with {{ and end with }}.
<turn|>
<|turn>model
""")
