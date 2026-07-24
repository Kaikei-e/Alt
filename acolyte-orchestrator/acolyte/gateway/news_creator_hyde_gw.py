"""News-creator-backed HyDE generator.

Wraps an existing LLMProviderPort (news-creator) with the HyDE prompt and
output sanitiser. The wrapper is thin by design: it exists so the Gatherer
can depend on a narrow HyDEGeneratorPort rather than the general LLM port.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

import structlog

from acolyte.domain.hyde import build_hyde_messages, sanitize_hyde_output

if TYPE_CHECKING:
    from acolyte.config.settings import Settings
    from acolyte.port.hyde_generator import HyDEGeneratorPort
    from acolyte.port.llm_provider import LLMProviderPort

logger = structlog.get_logger(__name__)


def build_hyde_generator(llm: LLMProviderPort, settings: Settings) -> HyDEGeneratorPort | None:
    """Compose the HyDE generator from settings — the single source of truth
    for this wiring decision, shared by main.py and scripts/resume_run.py so
    the two entry points can't silently drift on whether HyDE is enabled.

    Returns None when ``settings.hyde_enabled`` is False, matching
    report_graph.build_report_graph's "no HyDE" branch (query expansion
    falls back to BM25+RRF alone).
    """
    if not settings.hyde_enabled:
        return None
    return NewsCreatorHyDEGenerator(
        llm,
        timeout_s=settings.hyde_timeout_s,
        max_chars=settings.hyde_max_chars,
        num_predict=settings.hyde_num_predict,
    )


class NewsCreatorHyDEGenerator:
    """Default HyDEGeneratorPort implementation on top of news-creator."""

    def __init__(  # noqa: PLR0913 — generation knobs, each independently optional with a sensible default
        self,
        llm: LLMProviderPort,
        *,
        timeout_s: float = 8.0,
        max_chars: int = 600,
        num_predict: int = 400,
        temperature: float = 1.0,
        top_p: float = 0.95,
        top_k: int = 64,
    ) -> None:
        self._llm = llm
        self._timeout_s = timeout_s
        self._max_chars = max_chars
        self._num_predict = num_predict
        self._temperature = temperature
        self._top_p = top_p
        self._top_k = top_k

    async def generate_hypothetical_doc(self, topic: str, target_lang: str) -> str | None:
        if not topic or not topic.strip():
            return None
        if target_lang not in {"en", "ja"}:
            return None

        system_prompt, user_prompt = build_hyde_messages(topic, target_lang)

        try:
            response = await asyncio.wait_for(
                self._llm.generate(
                    user_prompt,
                    system_prompt=system_prompt,
                    num_predict=self._num_predict,
                    temperature=self._temperature,
                    top_p=self._top_p,
                    top_k=self._top_k,
                    # Gemma 4 variants advertise a `thinking` capability on
                    # Ollama. When the prompt contains CJK characters the
                    # model silently enters thinking mode, consumes the
                    # num_predict budget on internal reasoning, and returns
                    # an empty ``response`` field. Pinning ``think=False``
                    # forces direct generation and recovers full output.
                    think=False,
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
