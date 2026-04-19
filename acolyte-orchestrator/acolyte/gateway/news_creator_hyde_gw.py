"""News-creator-backed HyDE generator.

Wraps an existing LLMProviderPort (news-creator) with the HyDE prompt and
output sanitiser. The wrapper is thin by design: it exists so the Gatherer
can depend on a narrow HyDEGeneratorPort rather than the general LLM port.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.hyde import build_hyde_prompt, sanitize_hyde_output

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)


class NewsCreatorHyDEGenerator:
    """Default HyDEGeneratorPort implementation on top of news-creator."""

    def __init__(
        self,
        llm: LLMProviderPort,
        *,
        timeout_s: float = 8.0,
        max_chars: int = 600,
        num_predict: int = 400,
    ) -> None:
        self._llm = llm
        self._timeout_s = timeout_s
        self._max_chars = max_chars
        self._num_predict = num_predict

    async def generate_hypothetical_doc(self, topic: str, target_lang: str) -> str | None:
        if not topic or not topic.strip():
            return None
        if target_lang not in {"en", "ja"}:
            return None

        prompt = build_hyde_prompt(topic, target_lang)

        try:
            response = await asyncio.wait_for(
                self._llm.generate(
                    prompt,
                    num_predict=self._num_predict,
                    temperature=0.0,
                ),
                timeout=self._timeout_s,
            )
        except TimeoutError:
            logger.info("hyde: timeout", target_lang=target_lang)
            return None
        except Exception as exc:  # noqa: BLE001 - degrade gracefully, never break the graph
            logger.warning("hyde: generation failed", error=str(exc), target_lang=target_lang)
            return None

        cleaned = sanitize_hyde_output(response.text, target_lang, max_chars=self._max_chars)
        if cleaned is None:
            logger.info("hyde: output rejected by sanitiser", target_lang=target_lang)
        return cleaned
