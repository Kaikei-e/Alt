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
    IntermediateSummary,
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

        # Check if hierarchical summarization is needed
        cluster_section_length = self._estimate_cluster_section_length(request)
        use_hierarchical = (
            cluster_section_length > self.config.hierarchical_threshold_chars
            or len(request.clusters) > self.config.hierarchical_threshold_clusters
        )

        if use_hierarchical:
            logger.info(
                "Using hierarchical summarization (map-reduce)",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "cluster_section_length": cluster_section_length,
                    "cluster_count": len(request.clusters),
                },
            )
            return await self._generate_hierarchical_summary(request, max_bullets, temperature_override)

        # Single-shot summarization for smaller inputs
        return await self._generate_single_shot_summary(request, max_bullets, temperature_override)

    def _estimate_cluster_section_length(self, request: RecapSummaryRequest) -> int:
        """Estimate the character length of cluster section for prompt."""
        if request.genre_highlights:
            # If highlights are provided, they will be used instead
            return 0

        estimated = 0
        for cluster in request.clusters:
            # Rough estimate: cluster header + sentences
            estimated += 100  # Header overhead
            for sentence in cluster.representative_sentences:
                estimated += len(sentence.text) + 50  # Text + metadata overhead
        return estimated

    async def _generate_hierarchical_summary(
        self,
        request: RecapSummaryRequest,
        max_bullets: int,
        temperature_override: Optional[float],
    ) -> RecapSummaryResponse:
        """Generate summary using hierarchical (map-reduce) approach."""
        # Map phase: Split clusters into chunks and summarize each
        chunks = self._split_clusters_into_chunks(request.clusters)
        intermediate_summaries: List[IntermediateSummary] = []

        json_schema_intermediate = IntermediateSummary.model_json_schema()
        llm_options = {}
        if temperature_override is not None:
            llm_options["temperature"] = float(temperature_override)
        else:
            llm_options["temperature"] = float(self.config.llm_temperature)

        for chunk_idx, chunk_clusters in enumerate(chunks):
            logger.info(
                "Map phase: Summarizing chunk",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "chunk_index": chunk_idx + 1,
                    "total_chunks": len(chunks),
                    "clusters_in_chunk": len(chunk_clusters),
                },
            )

            # Create a temporary request for this chunk
            chunk_request = RecapSummaryRequest(
                job_id=request.job_id,
                genre=request.genre,
                clusters=chunk_clusters,
                genre_highlights=None,
                options=None,
            )
            chunk_prompt = self._build_prompt(chunk_request, max_bullets=4, intermediate=True)  # Short bullets for intermediate

            try:
                llm_response = await self.llm_provider.generate(
                    chunk_prompt,
                    num_predict=self.config.summary_num_predict // 2,  # Shorter for intermediate
                    format=json_schema_intermediate,
                    options=llm_options,
                )

                # Parse intermediate summary
                parsed = json.loads(llm_response.response)
                intermediate = IntermediateSummary(**parsed)
                intermediate_summaries.append(intermediate)
            except Exception as e:
                logger.warning(
                    "Failed to generate intermediate summary for chunk, skipping",
                    extra={
                        "job_id": str(request.job_id),
                        "genre": request.genre,
                        "chunk_index": chunk_idx + 1,
                        "error": str(e),
                    },
                )
                # Continue with other chunks

        if not intermediate_summaries:
            # Fallback if all map phases failed
            return self._create_fallback_from_clusters(request)

        # Reduce phase: Combine intermediate summaries into final summary
        logger.info(
            "Reduce phase: Combining intermediate summaries",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "intermediate_count": len(intermediate_summaries),
            },
        )

        # Convert intermediate summaries to highlights format
        reduce_highlights = []
        for inter_summary in intermediate_summaries:
            for bullet in inter_summary.bullets:
                from news_creator.domain.models import RepresentativeSentence
                reduce_highlights.append(
                    RepresentativeSentence(
                        text=bullet,
                        published_at=None,
                        source_url=None,
                        article_id=None,
                        is_centroid=False,
                    )
                )

        # Create reduce request with highlights
        reduce_request = RecapSummaryRequest(
            job_id=request.job_id,
            genre=request.genre,
            clusters=[],  # Empty clusters, use highlights only
            genre_highlights=reduce_highlights,
            options=request.options,
        )

        # Use single-shot path for reduce phase (it's smaller)
        return await self._generate_single_shot_summary(reduce_request, max_bullets, temperature_override)

    def _split_clusters_into_chunks(self, clusters: List) -> List[List]:
        """Split clusters into chunks that fit within max_chunk_chars."""
        if not clusters:
            return []

        chunks: List[List] = []
        current_chunk: List = []
        current_length = 0

        for cluster in clusters:
            cluster_length = sum(len(s.text) for s in cluster.representative_sentences) + 200  # Overhead

            if current_length + cluster_length > self.config.hierarchical_chunk_max_chars and current_chunk:
                chunks.append(current_chunk)
                current_chunk = [cluster]
                current_length = cluster_length
            else:
                current_chunk.append(cluster)
                current_length += cluster_length

        if current_chunk:
            chunks.append(current_chunk)

        return chunks if chunks else [[c] for c in clusters]  # At least one chunk per cluster if needed

    async def _generate_single_shot_summary(
        self,
        request: RecapSummaryRequest,
        max_bullets: int,
        temperature_override: Optional[float],
    ) -> RecapSummaryResponse:
        """Generate summary using single-shot approach (original logic)."""
        prompt = self._build_prompt(request, max_bullets)

        llm_options: Optional[Dict[str, Any]] = None
        if temperature_override is not None:
            llm_options = {"temperature": float(temperature_override)}

        max_retries = max(2, self.config.max_repetition_retries)
        last_error = None
        last_response = None
        json_validation_error_count = 0

        for attempt in range(max_retries + 1):
            current_temp = temperature_override
            current_repeat_penalty = self.config.llm_repeat_penalty

            if attempt > 0:
                base_temp = temperature_override if temperature_override is not None else self.config.llm_temperature
                current_temp = max(0.05, base_temp - (0.05 * attempt))
                current_repeat_penalty = min(1.2, current_repeat_penalty + (0.05 * attempt))

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

            llm_response = await self.llm_provider.generate(
                prompt,
                num_predict=self.config.summary_num_predict,
                format=json_schema,
                options=llm_options_retry,
            )

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
                    }
                )
                last_error = RuntimeError(f"Repetition detected (score: {rep_score:.2f})")
                last_response = llm_response
                continue

            try:
                summary_payload, parse_errors = self._parse_summary_json(llm_response.response, max_bullets)
                json_validation_error_count += parse_errors

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
                    continue

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
                raise

        # Fail-safe: If all retries fail, return fallback
        if request.genre_highlights:
            logger.warning(
                "Falling back to genre highlights due to LLM failure",
                extra={"job_id": str(request.job_id), "genre": request.genre}
            )
            return self._create_fallback_response(request)

        # Fallback: Use representative sentences from clusters
        if request.clusters:
            logger.warning(
                "Falling back to cluster representatives due to LLM failure",
                extra={"job_id": str(request.job_id), "genre": request.genre}
            )
            return self._create_fallback_from_clusters(request)

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

    def _create_fallback_from_clusters(self, request: RecapSummaryRequest) -> RecapSummaryResponse:
        """Create a response from cluster representatives when LLM generation fails."""
        bullets: List[str] = []
        max_fallback_bullets = 15

        for cluster in request.clusters[:max_fallback_bullets]:
            for sentence in cluster.representative_sentences[:1]:  # One sentence per cluster
                if len(bullets) >= max_fallback_bullets:
                    break
                bullets.append(sentence.text)
            if len(bullets) >= max_fallback_bullets:
                break

        if not bullets:
            bullets = ["要約の生成に失敗しました。"]

        summary = RecapSummary(
            title=f"{request.genre}の主要トピック (自動抽出)",
            bullets=bullets[:max_fallback_bullets],
            language="ja"
        )

        metadata = RecapSummaryMetadata(
            model="cluster-fallback",
            temperature=0.0,
            prompt_tokens=0,
            completion_tokens=0,
            processing_time_ms=0,
            json_validation_errors=1,
            summary_length_bullets=len(summary.bullets),
        )

        return RecapSummaryResponse(
            job_id=request.job_id,
            genre=request.genre,
            summary=summary,
            metadata=metadata
        )

    def _build_prompt(self, request: RecapSummaryRequest, max_bullets: int, intermediate: bool = False) -> str:
        # Truncate cluster section to fit within context window
        # Context window is 16K (default) or 60K tokens (61440), configured in entrypoint.sh and config.py
        # Model routing automatically selects 16K or 60K based on input size
        # Reserve ~1K tokens for prompt template and safety margin
        # Using conservative 50K chars (~12K tokens) for cluster_section to fit in 16K context
        MAX_CLUSTER_SECTION_LENGTH = 50_000  # characters (conservative estimate for ~12K tokens in 16K context)

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
            "intermediate": intermediate,
        }

        if request.genre_highlights:
             render_kwargs["highlights"] = request.genre_highlights
             render_kwargs["cluster_section"] = None # Force use of highlights path
        else:
             render_kwargs["cluster_section"] = cluster_section
             render_kwargs["highlights"] = None

        prompt = self.template.render(**render_kwargs)
        prompt_length = len(prompt)
        estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars

        # Validate prompt size - detect abnormal amplification
        ABNORMAL_PROMPT_THRESHOLD = 100_000  # characters (>25K tokens)
        if prompt_length > ABNORMAL_PROMPT_THRESHOLD:
            # Check for repetition in the prompt
            has_repetition, repetition_score, repetition_patterns = detect_repetition(prompt, threshold=0.3)

            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED in recap_summary_usecase._build_prompt",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "cluster_section_length": len(cluster_section) if cluster_section else 0,
                    "highlights_length": len(str(request.genre_highlights)) if request.genre_highlights else 0,
                    "prompt_preview_start": prompt[:500],
                    "prompt_preview_end": prompt[-500:] if prompt_length > 1000 else "",
                    "has_repetition": has_repetition,
                    "repetition_score": repetition_score,
                    "repetition_patterns": repetition_patterns,
                }
            )
            # Check if prompt contains repeated content
            if cluster_section and len(cluster_section) * 10 < prompt_length:
                logger.error(
                    "Prompt size is much larger than cluster_section - possible repetition or amplification",
                    extra={
                        "job_id": str(request.job_id),
                        "cluster_section_length": len(cluster_section),
                        "prompt_length": prompt_length,
                        "ratio": prompt_length / len(cluster_section) if cluster_section else 0,
                        "has_repetition": has_repetition,
                        "repetition_score": repetition_score,
                    }
                )
        else:
            logger.info(
                "Recap summary prompt built",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "cluster_section_length": len(cluster_section) if cluster_section else 0,
                    "highlights_length": len(str(request.genre_highlights)) if request.genre_highlights else 0,
                }
            )

        return prompt

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

        # Validate and sanitize references
        references = payload.get("references")
        if references is not None:
            if not isinstance(references, list):
                references = None
            else:
                # Validate reference structure and IDs
                validated_refs = []
                seen_ids = set()
                for ref in references:
                    if not isinstance(ref, dict):
                        continue
                    ref_id = ref.get("id")
                    if not isinstance(ref_id, int) or ref_id < 1 or ref_id in seen_ids:
                        continue
                    seen_ids.add(ref_id)
                    url = ref.get("url", "")
                    domain = ref.get("domain", "")
                    if not url or not domain:
                        # Try to extract domain from URL if missing
                        if url and not domain:
                            try:
                                parsed = urlparse(url if "://" in url else f"https://{url}")
                                domain = parsed.netloc or parsed.path.split("/")[0] or url
                            except Exception:
                                domain = url
                        else:
                            continue
                    validated_refs.append({
                        "id": ref_id,
                        "url": url,
                        "domain": domain,
                        "article_id": ref.get("article_id"),
                    })
                references = validated_refs if validated_refs else None

        sanitized = {
            "title": title,
            "bullets": [bullet[:500] for bullet in bullets],
            "language": language,
        }
        if references:
            sanitized["references"] = references

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

