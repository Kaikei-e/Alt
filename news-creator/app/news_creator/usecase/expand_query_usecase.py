"""Expand Query usecase - business logic for RAG query expansion."""

import logging
import time
from datetime import datetime, timezone, timedelta
from typing import List, Tuple, Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

# Prompt template for query expansion
EXPAND_QUERY_PROMPT_TEMPLATE = """You are an expert search query generator for a knowledge retrieval system.
Current Date: {current_date}

Generate search query variations based on the user's input.
Requirements:
- Generate exactly {japanese_count} Japanese query variation(s)
- Generate exactly {english_count} English query variation(s)
- If the input is Japanese, translate it to English for English variations
- If the input is English, translate it to Japanese for Japanese variations
- If the user specifies a time (e.g., "December" or "this month"), interpret it based on the Current Date
- Focus on different aspects: main keywords, synonyms, related concepts, and specific events

Output ONLY the generated queries, one per line.
Do not add numbering, bullets, labels, or explanations.
Output Japanese queries first, then English queries.

User Input: {query}"""


class ExpandQueryUsecase:
    """Usecase for generating expanded search queries for RAG retrieval."""

    # Model to use for query expansion (gemma3-4b-8k is lightweight and fast)
    EXPANSION_MODEL = "gemma3-4b-8k"

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        """Initialize expand query usecase."""
        self.config = config
        self.llm_provider = llm_provider

    async def expand_query(
        self,
        query: str,
        japanese_count: int = 1,
        english_count: int = 3,
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
            }
        )

        try:
            # Use low temperature for consistent, focused query generation
            llm_options = {
                "temperature": 0.3,
                "repeat_penalty": 1.1,
            }

            llm_response = await self.llm_provider.generate(
                prompt,
                model=self.EXPANSION_MODEL,
                num_predict=max_tokens,
                options=llm_options,
            )

            # Parse response: split by newlines and filter empty lines
            raw_text = llm_response.response
            expanded_queries = []

            for line in raw_text.split("\n"):
                trimmed = line.strip()
                # Skip empty lines and lines that look like labels/headers
                if not trimmed:
                    continue
                if trimmed.lower().startswith(("japanese:", "english:", "日本語:", "英語:")):
                    continue
                # Remove leading numbers/bullets if present
                if len(trimmed) > 2 and trimmed[0].isdigit() and trimmed[1] in ".):":
                    trimmed = trimmed[2:].strip()
                if trimmed.startswith(("-", "*", "•")):
                    trimmed = trimmed[1:].strip()
                if trimmed:
                    expanded_queries.append(trimmed)

            elapsed_ms = (time.time() - start_time) * 1000

            logger.info(
                "Query expansion completed",
                extra={
                    "query": query[:100],
                    "expanded_count": len(expanded_queries),
                    "model": llm_response.model,
                    "elapsed_ms": round(elapsed_ms, 2),
                }
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
