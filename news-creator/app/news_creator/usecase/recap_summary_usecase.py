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

        summary_payload = self._parse_summary_json(llm_response.response, max_bullets)
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
        max_clusters = max(3, min(len(request.clusters), max_bullets + 2))
        cluster_lines: List[str] = []
        for cluster in request.clusters[:max_clusters]:
            top_terms = ", ".join(cluster.top_terms or []) or "未提示"
            sentence_lines: List[str] = []
            for sentence in cluster.representative_sentences:
                parts: List[str] = [f"- {sentence.text}"]
                if sentence.published_at:
                    parts.append(f"  (公開日: {sentence.published_at})")
                if sentence.source_url:
                    # URLを短縮表示
                    source_domain = sentence.source_url.split("/")[2] if "/" in sentence.source_url else sentence.source_url
                    parts.append(f"  (出典: {source_domain})")
                sentence_lines.append(" ".join(parts))
            sentences = "\n".join(sentence_lines)
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
        return 10

    def _parse_summary_json(self, content: str, max_bullets: int) -> Dict[str, Any]:
        if not content:
            raise RuntimeError("LLM returned empty response for recap summary")

        candidate: Optional[str] = None
        try:
            candidate = self._extract_json_object(content)
        except RuntimeError as exc:
            logger.warning(
                "Structured JSON not found in recap summary response; falling back to heuristic parsing",
                extra={"content_preview": content[:200]},
            )
            fallback = self._fallback_summary_from_text(content, max_bullets)
            return self._sanitize_summary_payload(fallback, max_bullets)

        if candidate is None:
            fallback = self._fallback_summary_from_text(content, max_bullets)
            return self._sanitize_summary_payload(fallback, max_bullets)

        try:
            parsed = json.loads(candidate)
        except json.JSONDecodeError as exc:
            logger.error("Failed to parse recap summary JSON", extra={"content": content})
            fallback = self._fallback_summary_from_text(content, max_bullets)
            return self._sanitize_summary_payload(fallback, max_bullets)

        if not isinstance(parsed, dict):
            raise RuntimeError("LLM response must be a JSON object")

        return self._sanitize_summary_payload(parsed, max_bullets)

    def _extract_json_object(self, text: str) -> str:
        first_brace = text.find("{")
        last_brace = text.rfind("}")
        if first_brace == -1 or last_brace == -1 or first_brace >= last_brace:
            raise RuntimeError("Could not locate JSON object in LLM response")
        return text[first_brace : last_brace + 1]

    def _fallback_summary_from_text(self, text: str, max_bullets: int) -> Dict[str, Any]:
        lines = [line.strip() for line in text.splitlines()]
        non_empty = [line for line in lines if line]

        if not non_empty:
            raise RuntimeError("LLM returned empty response for recap summary")

        title = non_empty[0][:200]
        bullet_candidates: List[str] = []

        for line in non_empty[1:]:
            cleaned = line.lstrip("-*•●・ 　")
            if cleaned:
                bullet_candidates.append(cleaned)

        if not bullet_candidates:
            # Use remaining lines if the first line was the only content
            bullet_candidates = [
                line.lstrip("-*•●・ 　") for line in non_empty[1:max_bullets + 1] if line
            ]
        if not bullet_candidates:
            bullet_candidates = [title]

        merged_bullets: List[str] = []
        chunk_size = 2
        for idx in range(0, len(bullet_candidates), chunk_size):
            chunk = bullet_candidates[idx : idx + chunk_size]
            merged_text = " ".join(chunk).strip()
            if merged_text:
                merged_bullets.append(merged_text)

        if not merged_bullets:
            merged_bullets = bullet_candidates

        return {
            "title": title,
            "bullets": merged_bullets,
            "language": "ja",
        }

    def _sanitize_summary_payload(
        self,
        payload: Dict[str, Any],
        max_bullets: int,
    ) -> Dict[str, Any]:
        summary_section = payload.get("summary")
        if isinstance(summary_section, dict):
            payload = summary_section

        title = payload.get("title")
        if not isinstance(title, str) or not title.strip():
            title = "主要トピックのまとめ"
        title = title.strip()[:200]

        bullets_field = payload.get("bullets")
        if isinstance(bullets_field, list):
            bullets = [
                str(bullet).strip()
                for bullet in bullets_field
                if isinstance(bullet, (str, int, float)) and str(bullet).strip()
            ]
        else:
            bullets = []

        language = payload.get("language")
        if not isinstance(language, str) or not language.strip():
            language = "ja"

        max_allowed = min(max(1, max_bullets), 10)
        if len(bullets) > max_allowed:
            logger.debug(
                "Trimming recap summary bullets to schema limit",
                extra={
                    "original_count": len(bullets),
                    "trimmed_to": max_allowed,
                },
            )
            bullets = bullets[:max_allowed]

        # Ensure at least one bullet is present
        if not bullets:
            bullets = [title]

        sanitized = {
            "title": title,
            "bullets": [bullet[:500] for bullet in bullets],
            "language": language,
        }

        return sanitized

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[int]:
        if value is None:
            return None
        try:
            return int(value / 1_000_000)
        except (TypeError, ValueError):
            return None

