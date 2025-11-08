"""Recap summary usecase - generates structured summaries from clustering output."""

import json
import logging
import textwrap
from typing import Dict, Any, List, Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    RecapSummaryRequest,
    RecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
)
from news_creator.domain.prompts import RECAP_CLUSTER_SUMMARY_PROMPT
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)


class RecapSummaryUsecase:
    """Generate recap summaries from evidence clusters via LLM."""

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        self.config = config
        self.llm_provider = llm_provider

    async def generate_summary(self, request: RecapSummaryRequest) -> RecapSummaryResponse:
        """Produce structured summary JSON from clustering evidence."""
        if not request.clusters:
            raise ValueError("clusters must not be empty")

        max_bullets = self._resolve_max_bullets(request)
        temperature_override = (
            request.options.temperature if request.options and request.options.temperature is not None else None
        )

        prompt = self._build_prompt(request, max_bullets)

        llm_options: Optional[Dict[str, Any]] = None
        if temperature_override is not None:
            llm_options = {"temperature": float(temperature_override)}

        logger.info(
            "Generating recap summary",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "cluster_count": len(request.clusters),
                "max_bullets": max_bullets,
            },
        )

        llm_response = await self.llm_provider.generate(
            prompt,
            num_predict=self.config.summary_num_predict,
            options=llm_options,
        )

        summary_payload = self._parse_summary_json(llm_response.response)
        summary = RecapSummary(**summary_payload)

        metadata = RecapSummaryMetadata(
            model=llm_response.model,
            temperature=temperature_override if temperature_override is not None else self.config.llm_temperature,
            prompt_tokens=llm_response.prompt_eval_count,
            completion_tokens=llm_response.eval_count,
            processing_time_ms=self._nanoseconds_to_milliseconds(llm_response.total_duration),
        )

        logger.info(
            "Recap summary generated",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "bullet_count": len(summary.bullets),
            },
        )

        return RecapSummaryResponse(
            job_id=request.job_id,
            genre=request.genre,
            summary=summary,
            metadata=metadata,
        )

    def _build_prompt(self, request: RecapSummaryRequest, max_bullets: int) -> str:
        cluster_lines: List[str] = []
        for cluster in request.clusters:
            top_terms = ", ".join(cluster.top_terms or []) or "未提示"
            sentences = "\n".join(f"- {sentence}" for sentence in cluster.representative_sentences)
            cluster_block = textwrap.dedent(
                f"""
                ### Cluster {cluster.cluster_id}
                Top Terms: {top_terms}
                Representative Sentences:
                {sentences}
                """
            ).strip()
            cluster_lines.append(cluster_block)

        cluster_section = "\n\n".join(cluster_lines)

        return RECAP_CLUSTER_SUMMARY_PROMPT.format(
            job_id=request.job_id,
            genre=request.genre,
            max_bullets=max_bullets,
            cluster_section=cluster_section,
        )

    def _resolve_max_bullets(self, request: RecapSummaryRequest) -> int:
        if request.options and request.options.max_bullets is not None:
            return request.options.max_bullets
        return 5

    def _parse_summary_json(self, content: str) -> Dict[str, Any]:
        if not content:
            raise RuntimeError("LLM returned empty response for recap summary")

        candidate = self._extract_json_object(content)
        try:
            parsed = json.loads(candidate)
        except json.JSONDecodeError as exc:
            logger.error("Failed to parse recap summary JSON", extra={"content": content})
            raise RuntimeError("LLM returned invalid JSON for recap summary") from exc

        if not isinstance(parsed, dict):
            raise RuntimeError("LLM response must be a JSON object")

        return parsed

    def _extract_json_object(self, text: str) -> str:
        first_brace = text.find("{")
        last_brace = text.rfind("}")
        if first_brace == -1 or last_brace == -1 or first_brace >= last_brace:
            raise RuntimeError("Could not locate JSON object in LLM response")
        return text[first_brace : last_brace + 1]

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[int]:
        if value is None:
            return None
        try:
            return int(value / 1_000_000)
        except (TypeError, ValueError):
            return None

