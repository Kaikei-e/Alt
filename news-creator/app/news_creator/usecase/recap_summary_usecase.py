"""Recap summary usecase - generates structured summaries from clustering output."""

import json
import logging
import re
import textwrap
import os
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple
from urllib.parse import urlparse

from jinja2 import Template

try:
    import json_repair
except ImportError:
    json_repair = None  # type: ignore

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    RecapSummaryRequest,
    RecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
)
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.utils.repetition_detector import detect_repetition

logger = logging.getLogger(__name__)


class RecapSummaryUsecase:
    """Generate recap summaries from evidence clusters via LLM."""

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        self.config = config
        self.llm_provider = llm_provider

        # Load prompt template
        template_path = Path(__file__).parent.parent.parent / "prompts" / "recap_summary.jinja"
        with open(template_path, "r", encoding="utf-8") as f:
            self.template = Template(f.read())

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

        max_retries = max(2, self.config.max_repetition_retries)
        last_error = None
        last_response = None
        json_validation_error_count = 0

        for attempt in range(max_retries + 1):
            # リトライ時は温度を下げる、Repetition Penaltyを上げる
            current_temp = temperature_override
            current_repeat_penalty = self.config.llm_repeat_penalty

            if attempt > 0:
                base_temp = temperature_override if temperature_override is not None else self.config.llm_temperature
                current_temp = max(0.05, base_temp - (0.05 * attempt))
                current_repeat_penalty = min(1.2, current_repeat_penalty + (0.05 * attempt))
                logger.warning(
                    "Retrying recap summary generation with adjusted parameters",
                    extra={
                        "job_id": str(request.job_id),
                        "attempt": attempt + 1,
                        "temperature": current_temp,
                        "repeat_penalty": current_repeat_penalty,
                    },
                )

            # Prepare schema for Structured Outputs
            json_schema = RecapSummary.model_json_schema()

            llm_options_retry = None
            if current_temp is not None:
                llm_options_retry = {
                     **(llm_options or {}),
                    "temperature": float(current_temp),
                    "repeat_penalty": float(current_repeat_penalty),
                }
            elif llm_options is not None:
                 llm_options_retry = {**llm_options, "repeat_penalty": float(current_repeat_penalty)}
            else:
                 llm_options_retry = {"repeat_penalty": float(current_repeat_penalty)}

            # Use JSON schema for structured output (Ollama structured output mode)
            llm_response = await self.llm_provider.generate(
                prompt,
                num_predict=self.config.summary_num_predict,
                format=json_schema,
                options=llm_options_retry,
            )

            # Check for repetition in raw response (before JSON parsing)
            has_repetition, rep_score, rep_patterns = detect_repetition(
                llm_response.response,
                threshold=self.config.repetition_threshold
            )

            if has_repetition and attempt < max_retries:
                logger.warning(
                    "Repetition detected in recap summary, will retry",
                    extra={
                        "job_id": str(request.job_id),
                        "genre": request.genre,
                        "attempt": attempt + 1,
                        "repetition_score": rep_score,
                        "patterns": rep_patterns,
                        "response_preview": llm_response.response[:200],
                    }
                )
                last_error = RuntimeError(f"Repetition detected (score: {rep_score:.2f})")
                last_response = llm_response
                continue  # Retry

            try:
                summary_payload, parse_errors = self._parse_summary_json(llm_response.response, max_bullets)
                json_validation_error_count += parse_errors

                # Also check for repetition in parsed summary text
                summary_text = summary_payload.get("title", "") + " " + " ".join(summary_payload.get("bullets", []))
                has_repetition_in_summary, rep_score_summary, rep_patterns_summary = detect_repetition(
                    summary_text,
                    threshold=self.config.repetition_threshold
                )

                if has_repetition_in_summary and attempt < max_retries:
                    logger.warning(
                        "Repetition detected in parsed recap summary, will retry",
                        extra={
                            "job_id": str(request.job_id),
                            "genre": request.genre,
                            "attempt": attempt + 1,
                            "repetition_score": rep_score_summary,
                            "patterns": rep_patterns_summary,
                        }
                    )
                    last_error = RuntimeError(f"Repetition in parsed summary (score: {rep_score_summary:.2f})")
                    continue  # Retry
                # 成功した場合は結果を返す
                summary = RecapSummary(**summary_payload)

                metadata = RecapSummaryMetadata(
                    model=llm_response.model,
                    temperature=current_temp if current_temp is not None else self.config.llm_temperature,
                    prompt_tokens=llm_response.prompt_eval_count,
                    completion_tokens=llm_response.eval_count,
                    processing_time_ms=self._nanoseconds_to_milliseconds(llm_response.total_duration),
                    json_validation_errors=json_validation_error_count,
                    summary_length_bullets=len(summary.bullets),
                )

                if attempt > 0:
                    logger.info(
                        "Recap summary generated successfully after retry",
                        extra={
                            "job_id": str(request.job_id),
                            "genre": request.genre,
                            "bullet_count": len(summary.bullets),
                            "attempts": attempt + 1,
                        },
                    )
                else:
                    logger.info(
                        "Recap summary generated",
                        extra={
                            "job_id": str(request.job_id),
                            "genre": request.genre,
                            "bullet_count": len(summary.bullets),
                            "attempt": attempt + 1,
                        },
                    )

                return RecapSummaryResponse(
                    job_id=request.job_id,
                    genre=request.genre,
                    summary=summary,
                    metadata=metadata,
                )
            except RuntimeError as e:
                last_error = e
                last_response = llm_response
                if attempt < max_retries:
                    continue
                # 最後の試行でも失敗した場合はエラーを投げる
                logger.error(
                    "Failed to generate recap summary after all retries",
                    extra={
                        "job_id": str(request.job_id),
                        "genre": request.genre,
                        "attempts": attempt + 1,
                        "error": str(e),
                    },
                )
                raise

        # Fail-safe: If all retries fail, return the extracted genre highlights as the summary
        if request.genre_highlights:
            logger.warning(
                "Falling back to genre highlights due to LLM failure",
                extra={"job_id": str(request.job_id), "genre": request.genre}
            )
            return getattr(self, "_create_fallback_response")(request)

        # ここに到達することはないはずだが、念のため
        if last_error:
            raise last_error
        raise RuntimeError("Failed to generate recap summary")

    def _create_fallback_response(self, request: RecapSummaryRequest) -> RecapSummaryResponse:
        """Create a response from genre highlights when LLM generation fails."""
        highlights = request.genre_highlights or []
        bullets = [h.text for h in highlights[:15]]
        if not bullets:
            bullets = ["要約の生成に失敗しました。"]

        summary = RecapSummary(
            title=f"{request.genre}の主要トピック (自動抽出)",
            bullets=bullets,
            language="ja"
        )

        metadata = RecapSummaryMetadata(
            model="extraction-fallback",
            temperature=0.0,
            prompt_tokens=0,
            completion_tokens=0,
            processing_time_ms=0,
            json_validation_errors=1,  # Fallback implies failure
            summary_length_bullets=len(summary.bullets),
        )

        return RecapSummaryResponse(
            job_id=request.job_id,
            genre=request.genre,
            summary=summary,
            metadata=metadata
        )

    def _build_prompt(self, request: RecapSummaryRequest, max_bullets: int) -> str:
        # Truncate cluster section to fit within 80K context window
        # Context window is now 80K tokens (81920), configured in entrypoint.sh and config.py
        # 71K tokens ≈ 284K-568K chars, but we need to reserve space for prompt template
        # Reserve ~1K tokens for prompt template and safety margin, leaving ~70K tokens for content
        # Using ~280K chars (≈70K tokens) for cluster_section to leave room for prompt template
        MAX_CLUSTER_SECTION_LENGTH = 280_000  # characters (conservative estimate for ~70K tokens in 71K context)

        max_clusters = max(3, min(len(request.clusters), max_bullets + 2))
        cluster_lines: List[str] = []
        cluster_section_length = 0

        for cluster in request.clusters[:max_clusters]:
            top_terms = ", ".join(cluster.top_terms or []) or "未提示"
            sentence_lines: List[str] = []
            for sentence in cluster.representative_sentences:
                prefix = "- [Main Point] " if sentence.is_centroid else "- "
                parts: List[str] = [f"{prefix}{sentence.text}"]
                if sentence.published_at:
                    parts.append(f"  (公開日: {sentence.published_at})")
                if sentence.source_url:
                    # URLを短縮表示（urllib.parseで安全にドメインを抽出）
                    try:
                        # スキームがない場合は追加してからパース
                        url_to_parse = sentence.source_url
                        if "://" not in url_to_parse:
                            url_to_parse = f"https://{url_to_parse}"
                        parsed = urlparse(url_to_parse)
                        source_domain = parsed.netloc or parsed.path.split("/")[0] or sentence.source_url
                    except Exception:
                        # パースに失敗した場合は元のURLを使用
                        source_domain = sentence.source_url
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

            # Check if adding this cluster would exceed the limit
            estimated_length = cluster_section_length + len(cluster_block) + 2  # +2 for "\n\n"
            if estimated_length > MAX_CLUSTER_SECTION_LENGTH:
                logger.warning(
                    "Cluster section truncated to fit context window",
                    extra={
                        "job_id": str(request.job_id),
                        "genre": request.genre,
                        "clusters_included": len(cluster_lines),
                        "total_clusters": len(request.clusters),
                        "cluster_section_length": cluster_section_length,
                        "max_length": MAX_CLUSTER_SECTION_LENGTH,
                    }
                )
                break

            cluster_lines.append(cluster_block)
            cluster_section_length = estimated_length

        cluster_section = "\n\n".join(cluster_lines)

        # Final truncation if still too long (safety check)
        if len(cluster_section) > MAX_CLUSTER_SECTION_LENGTH:
            original_length = len(cluster_section)
            cluster_section = cluster_section[:MAX_CLUSTER_SECTION_LENGTH]
            logger.warning(
                "Cluster section truncated at final check",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "original_length": original_length,
                    "truncated_length": len(cluster_section),
                    "max_length": MAX_CLUSTER_SECTION_LENGTH,
                }
            )

        # Decide whether to use cluster_section or genre_highlights
        # If genre_highlights is present and we prefer it (e.g. for efficiency), we can suppress cluster_section.
        # However, the prompt template logic handles it:
        # {% if cluster_section %} ... {% else %} ... {% endif %}
        # So passing BOTH might be ambiguous if the template prioritizes one.
        # My template says {% if cluster_section %}, so if I pass it, it uses it.
        # To use highlights, I should NOT pass cluster_section, or update logic.

        # Policy: If genre_highlights are provided, USE THEM and ignore clusters for the prompt context
        # (unless user explicitly wants full clustering, but here we want optimization).
        # Actually, @refine_plan4.md says: "Genre Merge Logic: ... collected into mini-summary... input to LLM".
        # So yes, if highlights exist, we use them.

        render_kwargs = {
            "job_id": str(request.job_id),
            "genre": request.genre,
            "max_bullets": max_bullets,
        }

        if request.genre_highlights:
             render_kwargs["highlights"] = request.genre_highlights
             render_kwargs["cluster_section"] = None # Force use of highlights path
        else:
             render_kwargs["cluster_section"] = cluster_section
             render_kwargs["highlights"] = None

        return self.template.render(**render_kwargs)

    def _resolve_max_bullets(self, request: RecapSummaryRequest) -> int:
        if request.options and request.options.max_bullets is not None:
            return request.options.max_bullets
        return 15

    def _parse_summary_json(self, content: str, max_bullets: int) -> Tuple[Dict[str, Any], int]:
        if not content:
            raise RuntimeError("LLM returned empty response for recap summary")

        try:
            # Structured Outputs should trigger clean JSON, but just in case, straightforward load.
            parsed = json.loads(content)
        except json.JSONDecodeError as exc:
            logger.warning(
                "Structured Output parsing failed, attempting repair",
                extra={"error": str(exc), "content_preview": content[:200]},
            )
            # Minimal fallback if even structured output fails (rare)
            if json_repair:
                try:
                    repaired_json = json_repair.repair_json(content)
                    parsed = json.loads(repaired_json)
                except Exception:
                     # If repair fails, we can't do much without heuristics,
                     # but let's assume Structured Outputs generally work.
                     # We could re-introduce heuristics if strictly necessary,
                     # but usually this means the model completely refused or broke.
                     raise RuntimeError(f"Failed to parse structured output: {exc}")
            else:
                 raise RuntimeError(f"Failed to parse structured output: {exc}")

        if not isinstance(parsed, dict):
            raise RuntimeError("LLM response must be a JSON object")

        return self._sanitize_summary_payload(parsed, max_bullets), 0



    def _sanitize_summary_payload(
        self,
        payload: Dict[str, Any],
        max_bullets: int,
    ) -> Dict[str, Any]:
        summary_section = payload.get("summary")
        if isinstance(summary_section, dict):
            payload = summary_section

        title = payload.get("title")

        # タイトルがコードフェンス等の不正値の場合は修正
        invalid_titles = ["```json", "```", "json", "{", ""]
        if not isinstance(title, str) or title.strip().lower() in invalid_titles:
            # bulletsの先頭から抽出を試みる
            bullets = payload.get("bullets", [])
            if bullets and isinstance(bullets[0], str):
                # 最初のbulletから15-45文字を抽出してタイトル化
                first_bullet = bullets[0]
                title = self._extract_title_from_bullet(first_bullet)
            else:
                title = "主要トピックのまとめ"

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

        max_allowed = min(max(1, max_bullets), 15)
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

    def _extract_title_from_bullet(self, bullet: str) -> str:
        """bulletテキストから適切なタイトルを抽出する"""
        # 最初の句点または45文字までを取得
        for i, char in enumerate(bullet):
            if char in "。、" and 15 <= i <= 45:
                return bullet[:i+1]
            if i >= 45:
                return bullet[:45] + "…"
        return bullet[:45] if len(bullet) > 45 else bullet

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[int]:
        if value is None:
            return None
        try:
            return int(value / 1_000_000)
        except (TypeError, ValueError):
            return None

