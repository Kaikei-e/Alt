"""Expand Query usecase - business logic for RAG query expansion."""

import logging
import re as _re
import time
from datetime import datetime, timezone, timedelta
from typing import List, Tuple, Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import ConversationMessage, LLMGenerateResponse
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

# Prompt template for query expansion (single-turn)
# Uses XML-style structured contract to prevent small models from echoing meta-instructions.
EXPAND_QUERY_PROMPT_TEMPLATE = """<task>Generate search query variations for a knowledge retrieval system.</task>
<rules>
- Current Date: {current_date}
- Generate {japanese_count} Japanese and {english_count} English search queries for the input below
- Translate the input between Japanese and English as needed
- Interpret relative dates based on Current Date
- Cover different aspects: keywords, synonyms, related concepts, specific events
</rules>
<input>{query}</input>
Japanese({japanese_count}):
"""

# Prompt template for multi-turn expansion with coreference resolution
EXPAND_QUERY_WITH_HISTORY_TEMPLATE = """<task>Generate search query variations for a knowledge retrieval system. Resolve coreferences using the conversation history.</task>
<rules>
- Current Date: {current_date}
- Generate {japanese_count} Japanese and {english_count} English search queries for the input below
- Resolve pronouns and references ("that", "it", "tell me more") from conversation context
- Translate between Japanese and English as needed
- Cover different aspects: keywords, synonyms, related concepts
- IMPORTANT: Generate queries about the SAME TOPIC as the input. Do NOT copy topics from examples.
- Output ONLY search queries. No explanations, no labels, no numbering.
</rules>
<example>
<conversation>
user: [TOPIC_A] について調べています。
assistant: [TOPIC_A] の基本情報をまとめています。
</conversation>
<input>もっと詳しく知りたい</input>
[TOPIC_A] 詳細
[TOPIC_A] background
[TOPIC_A] key points
</example>
<example>
<conversation>
user: [TOPIC_B] はどう違いますか？
assistant: [TOPIC_B] には複数の選択肢があります。
</conversation>
<input>どちらがよい？</input>
[TOPIC_B] comparison
[TOPIC_B] pros and cons
[TOPIC_B] benchmark
</example>
---
<conversation>
{conversation_history}
</conversation>
<input>{query}</input>
Japanese({japanese_count}):
"""


class ExpandQueryUsecase:
    """Usecase for generating expanded search queries for RAG retrieval."""

    # Use the same warmed RAG model as the main answer path to avoid backend model swaps.
    EXPANSION_MODEL = "gemma4-e4b-12k"

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        """Initialize expand query usecase."""
        self.config = config
        self.llm_provider = llm_provider

    async def expand_query(
        self,
        query: str,
        japanese_count: int = 1,
        english_count: int = 3,
        conversation_history: Optional[List[ConversationMessage]] = None,
        priority: str = "low",
    ) -> Tuple[List[str], str, Optional[float]]:
        """
        Generate expanded search queries from a user query.

        Args:
            query: Original user query
            japanese_count: Number of Japanese query variations to generate
            english_count: Number of English query variations to generate

        Returns:
            Tuple of (expanded_queries list, model name, processing_time_ms)

        Raises:
            ValueError: If query is empty
            RuntimeError: If LLM generation fails
        """
        if not query or not query.strip():
            raise ValueError("query cannot be empty")

        start_time = time.time()

        # Build prompt
        jst = timezone(timedelta(hours=9))
        current_date = datetime.now(jst).strftime("%Y-%m-%d")

        if conversation_history:
            # Multi-turn: include conversation context for coreference resolution
            history_lines = "\n".join(
                f"{msg.role}: {_sanitize_history_content(msg.content)}"
                for msg in conversation_history[-6:]  # Last 3 turns max
            )
            prompt = EXPAND_QUERY_WITH_HISTORY_TEMPLATE.format(
                current_date=current_date,
                conversation_history=history_lines,
                japanese_count=japanese_count,
                english_count=english_count,
                query=query.strip(),
            )
        else:
            prompt = EXPAND_QUERY_PROMPT_TEMPLATE.format(
                current_date=current_date,
                japanese_count=japanese_count,
                english_count=english_count,
                query=query.strip(),
            )

        total_queries = japanese_count + english_count
        # Estimate max tokens: ~50 tokens per query should be sufficient
        max_tokens = max(100, total_queries * 50)

        logger.info(
            "Generating expanded queries",
            extra={
                "query": query[:100],
                "japanese_count": japanese_count,
                "english_count": english_count,
                "max_tokens": max_tokens,
            },
        )

        try:
            # Use deterministic temperature to prevent garbage output in multi-turn
            llm_options = {
                "temperature": 0.0,
                "repeat_penalty": 1.1,
            }

            result = await self.llm_provider.generate(
                prompt,
                model=self.EXPANSION_MODEL,
                num_predict=max_tokens,
                options=llm_options,
                priority=priority,
            )

            # Narrow union type: non-streaming returns LLMGenerateResponse
            assert isinstance(result, LLMGenerateResponse), (
                "Expected non-streaming LLMGenerateResponse"
            )
            llm_response: LLMGenerateResponse = result

            # Parse response: split by newlines and filter empty lines
            raw_text = llm_response.response
            parsed_lines = _parse_expansion_lines(raw_text)

            # Validate: order-preserving dedup
            expanded_queries = _deduplicate_preserving_order(parsed_lines)

            # Validate: remove instruction echo / preamble leaks
            expanded_queries = _filter_instruction_leaks(expanded_queries)

            elapsed_ms = (time.time() - start_time) * 1000

            logger.info(
                "Query expansion completed",
                extra={
                    "query": query[:100],
                    "expanded_count": len(expanded_queries),
                    "raw_line_count": len(parsed_lines),
                    "model": llm_response.model,
                    "elapsed_ms": round(elapsed_ms, 2),
                },
            )

            return expanded_queries, llm_response.model, elapsed_ms

        except Exception as e:
            elapsed_ms = (time.time() - start_time) * 1000
            logger.error(
                "Query expansion failed",
                extra={
                    "query": query[:100],
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "elapsed_ms": round(elapsed_ms, 2),
                },
                exc_info=True,
            )
            raise RuntimeError(f"Query expansion failed: {e}") from e


# --- Output parsing and validation helpers ---

# Labels and section headers that small models sometimes emit
_LABEL_PREFIXES = (
    "japanese:",
    "english:",
    "日本語:",
    "英語:",
    "japanese(",
    "english(",
)

# Preamble patterns that indicate prose, not search queries
_PREAMBLE_PATTERNS = (
    "here are",
    "以下は",
    "the following",
    "generated queries",
    "search queries",
    "query variations",
    "i will generate",
    "let me generate",
)

# Known instruction fragments that indicate echo, not real queries.
# A line matching any of these exactly (normalized) is an instruction leak.
_INSTRUCTION_ECHO_EXACT = {
    "japanese queries and english queries must be translated to each other.",
    "japanese queries first, then english queries.",
    "output only the generated queries, one per line.",
    "do not add numbering, bullets, labels, or explanations.",
    "generate exactly",
    "output japanese queries first",
}

# Meta-words: if 3+ appear in a single line, it's likely an instruction leak
_META_WORDS = frozenset(
    {
        "queries",
        "generate",
        "variations",
        "translate",
        "numbering",
        "bullets",
        "labels",
        "explanations",
        "output",
        "exactly",
        "requirements",
    }
)


_URL_PATTERN = _re.compile(r"https?://\S+")
_SPECIAL_CHARS_PATTERN = _re.compile(
    r"[^\w\s\u3000-\u9FFF\u30A0-\u30FF\u3040-\u309F。、！？.,!?\-()（）]"
)


def _sanitize_history_content(content: str) -> str:
    """Sanitize conversation history content to prevent LLM confusion."""
    text = content[:150]
    text = _URL_PATTERN.sub("", text)
    text = _SPECIAL_CHARS_PATTERN.sub(" ", text)
    text = " ".join(text.split())
    return text


def _is_repeating_pattern(line: str) -> bool:
    """Detect repetitive character patterns like ':):):):)...' or 'hahaha'."""
    stripped = line.strip()
    if len(stripped) < 6:
        return False
    for pat_len in range(1, 5):
        if len(stripped) < pat_len * 3:
            continue
        pat = stripped[:pat_len]
        repetitions = 0
        for i in range(0, len(stripped), pat_len):
            if stripped[i : i + pat_len] == pat:
                repetitions += 1
            else:
                break
        if repetitions >= 3 and repetitions * pat_len * 3 >= len(stripped) * 2:
            return True
    return False


def _parse_expansion_lines(raw_text: str) -> List[str]:
    """Parse raw LLM output into candidate query lines."""
    lines = []
    for line in raw_text.split("\n"):
        trimmed = line.strip()
        if not trimmed:
            continue
        # Skip section labels (e.g., "Japanese:", "English(3):")
        if trimmed.lower().startswith(_LABEL_PREFIXES):
            continue
        # Remove leading numbers/bullets
        if len(trimmed) > 2 and trimmed[0].isdigit() and trimmed[1] in ".):":
            trimmed = trimmed[2:].strip()
        if trimmed.startswith(("-", "*", "•")):
            trimmed = trimmed[1:].strip()
        if trimmed:
            lines.append(trimmed)
    return lines


def _deduplicate_preserving_order(queries: List[str]) -> List[str]:
    """Remove duplicate queries while preserving first-occurrence order."""
    seen: dict[str, None] = {}
    result = []
    for q in queries:
        key = q.strip().lower()
        if key not in seen:
            seen[key] = None
            result.append(q)
    return result


def _is_instruction_leak(line: str) -> bool:
    """Detect if a line is an echoed instruction rather than a real search query."""
    normalized = line.strip().lower().rstrip(".")
    # Exact match against known instruction echoes
    for pattern in _INSTRUCTION_ECHO_EXACT:
        if normalized == pattern.rstrip("."):
            return True
        # High overlap: if the line contains a known instruction pattern
        if len(pattern) > 20 and pattern.rstrip(".") in normalized:
            return True

    # Meta-word density: if 3+ meta words appear, likely instruction
    words = set(normalized.split())
    meta_count = len(words & _META_WORDS)
    if meta_count >= 3:
        return True

    return False


def _is_preamble(line: str) -> bool:
    """Detect preamble/prose lines that are not search queries."""
    lower = line.strip().lower()
    return any(p in lower for p in _PREAMBLE_PATTERNS)


def _is_xml_tag_leak(line: str) -> bool:
    """Detect leaked XML tags from the prompt structure."""
    stripped = line.strip()
    # XML tags like </example>, <input>..., </task>, <rules>, etc.
    if stripped.startswith("<") and (">" in stripped):
        return True
    if stripped.startswith("</") or stripped.endswith("/>"):
        return True
    return False


def _filter_instruction_leaks(queries: List[str]) -> List[str]:
    """Remove instruction echoes and preamble from query list."""
    result = []
    for q in queries:
        if _is_instruction_leak(q):
            logger.info(
                "Rejected instruction echo",
                extra={"rejected_line": q[:100], "reason": "instruction_leak"},
            )
            continue
        if _is_preamble(q):
            logger.info(
                "Rejected preamble line",
                extra={"rejected_line": q[:100], "reason": "preamble"},
            )
            continue
        if _is_xml_tag_leak(q):
            logger.info(
                "Rejected XML tag leak",
                extra={"rejected_line": q[:100], "reason": "xml_tag_leak"},
            )
            continue
        if _is_repeating_pattern(q):
            logger.info(
                "Rejected repeating garbage pattern",
                extra={"rejected_line": q[:100], "reason": "repeating_pattern"},
            )
            continue
        result.append(q)
    return result
