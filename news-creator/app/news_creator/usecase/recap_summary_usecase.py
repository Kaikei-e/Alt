"""Recap summary usecase - generates structured summaries from clustering output."""

import asyncio
import hashlib
import json
import logging
import textwrap
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple, Union
from urllib.parse import urlparse

from jinja2 import Template

try:
    import json_repair
except ImportError:
    json_repair = None  # type: ignore

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    BatchRecapSummaryError,
    BatchRecapSummaryRequest,
    BatchRecapSummaryResponse,
    RecapClusterInput,
    RecapSummaryRequest,
    RecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
    IntermediateSummary,
    RepresentativeSentence,
)
from news_creator.port.cache_port import CachePort
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.utils.repetition_detector import detect_repetition

logger = logging.getLogger(__name__)


class RecapSummaryUsecase:
    """Generate recap summaries from evidence clusters via LLM."""

    def __init__(
        self,
        config: NewsCreatorConfig,
        llm_provider: LLMProviderPort,
        cache: Optional[CachePort] = None,
    ):
        self.config = config
        self.llm_provider = llm_provider
        self.cache = cache

        # Load prompt template
        template_path = Path(__file__).parent.parent.parent / "prompts" / "recap_summary.jinja"
        with open(template_path, "r", encoding="utf-8") as f:
            self.template = Template(f.read())

    async def generate_summary(self, request: RecapSummaryRequest) -> RecapSummaryResponse:
        """Produce structured summary JSON from clustering evidence."""
        if not request.clusters:
            raise ValueError("clusters must not be empty")

        # Try to get from cache first
        cache_key = self._generate_cache_key(request)
        cached_response = await self._get_cached_response(cache_key)
        if cached_response is not None:
            logger.info(
                "Returning cached recap summary",
                extra={"job_id": str(request.job_id), "genre": request.genre},
            )
            return cached_response

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
            response = await self._generate_hierarchical_summary(request, max_bullets, temperature_override)
            await self._cache_response(cache_key, response)
            return response

        # Single-shot summarization for smaller inputs
        response = await self._generate_single_shot_summary(request, max_bullets, temperature_override)
        await self._cache_response(cache_key, response)
        return response

    async def generate_batch_summary(
        self, batch_request: BatchRecapSummaryRequest
    ) -> BatchRecapSummaryResponse:
        """Process multiple recap summary requests in parallel.

        This method reduces the "chatty microservices" anti-pattern by allowing
        multiple genres to be processed in a single HTTP request.

        Args:
            batch_request: Contains a list of individual recap summary requests

        Returns:
            BatchRecapSummaryResponse with successful responses and any errors
        """
        logger.info(
            "Processing batch recap summary request",
            extra={"request_count": len(batch_request.requests)},
        )

        # Throttle parallel execution to avoid saturating the LLM queue
        batch_semaphore = asyncio.Semaphore(
            min(3, self.config.ollama_request_concurrency)
        )

        async def throttled_request(req: RecapSummaryRequest):
            async with batch_semaphore:
                return await self._generate_summary_with_error_handling(req)

        tasks = [throttled_request(req) for req in batch_request.requests]

        # Execute with concurrency limited by semaphore
        results = await asyncio.gather(*tasks, return_exceptions=False)

        # Separate successful responses from errors
        responses: List[RecapSummaryResponse] = []
        errors: List[BatchRecapSummaryError] = []

        for req, result in zip(batch_request.requests, results):
            if isinstance(result, BatchRecapSummaryError):
                errors.append(result)
            else:
                responses.append(result)

        logger.info(
            "Batch recap summary completed",
            extra={
                "total_requests": len(batch_request.requests),
                "successful": len(responses),
                "failed": len(errors),
            },
        )

        return BatchRecapSummaryResponse(responses=responses, errors=errors)

    async def _generate_summary_with_error_handling(
        self, request: RecapSummaryRequest
    ) -> Union[RecapSummaryResponse, BatchRecapSummaryError]:
        """Generate a summary with error handling for batch processing.

        Args:
            request: Individual recap summary request

        Returns:
            Either the successful response or an error object
        """
        try:
            return await self.generate_summary(request)
        except Exception as e:
            logger.warning(
                "Failed to generate summary in batch",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "error": str(e),
                },
            )
            return BatchRecapSummaryError(
                job_id=request.job_id,
                genre=request.genre,
                error=str(e),
            )

    async def _generate_chunk_summary(
        self,
        request: RecapSummaryRequest,
        chunk_idx: int,
        chunk_clusters: List,
        json_schema: Dict[str, Any],
        llm_options: Dict[str, Any],
        total_chunks: int,
    ) -> Optional[IntermediateSummary]:
        """Generate an intermediate summary for a single chunk.

        Args:
            request: Original recap summary request
            chunk_idx: Index of the current chunk (0-based)
            chunk_clusters: Clusters for this chunk
            json_schema: JSON schema for intermediate summary
            llm_options: LLM generation options
            total_chunks: Total number of chunks

        Returns:
            IntermediateSummary if successful, None otherwise
        """
        logger.debug(
            "Map phase: Processing chunk",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "chunk_index": chunk_idx + 1,
                "total_chunks": total_chunks,
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
        chunk_prompt = self._build_prompt(chunk_request, max_bullets=4, intermediate=True)

        try:
            llm_response = await self.llm_provider.generate(
                chunk_prompt,
                num_predict=self.config.summary_num_predict // 2,
                format=json_schema,
                options=llm_options,
            )

            # Parse intermediate summary
            parsed = json.loads(llm_response.response)
            return IntermediateSummary(**parsed)
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
            return None

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
        """Generate summary using hierarchical (map-reduce) approach with recursive reduce."""
        # Map phase: Split clusters into chunks and summarize each in parallel
        chunks = self._split_clusters_into_chunks(request.clusters)

        json_schema_intermediate = IntermediateSummary.model_json_schema()
        llm_options = {}
        if temperature_override is not None:
            llm_options["temperature"] = float(temperature_override)
        else:
            llm_options["temperature"] = float(self.config.llm_temperature)

        logger.info(
            "Map phase: Processing chunks in parallel",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "total_chunks": len(chunks),
            },
        )

        # Throttle map phase concurrency to prevent queue saturation
        map_semaphore = asyncio.Semaphore(
            min(3, self.config.ollama_request_concurrency)
        )

        async def throttled_chunk_summary(chunk_idx, chunk_clusters):
            async with map_semaphore:
                return await self._generate_chunk_summary(
                    request=request,
                    chunk_idx=chunk_idx,
                    chunk_clusters=chunk_clusters,
                    json_schema=json_schema_intermediate,
                    llm_options=llm_options,
                    total_chunks=len(chunks),
                )

        map_tasks = [
            throttled_chunk_summary(chunk_idx, chunk_clusters)
            for chunk_idx, chunk_clusters in enumerate(chunks)
        ]

        # Execute map tasks with concurrency limited by semaphore
        map_results = await asyncio.gather(*map_tasks, return_exceptions=False)

        # Collect successful intermediate summaries
        intermediate_summaries: List[IntermediateSummary] = [
            result for result in map_results if result is not None
        ]

        if not intermediate_summaries:
            # Fallback if all map phases failed
            return self._create_fallback_from_clusters(request)

        # Reduce phase: Combine intermediate summaries into final summary (recursive if needed)
        return await self._recursive_reduce_phase(
            intermediate_summaries, request, max_bullets, temperature_override, llm_options
        )

    async def _recursive_reduce_phase(
        self,
        summaries: List[IntermediateSummary],
        request: RecapSummaryRequest,
        max_bullets: int,
        temperature_override: Optional[float],
        llm_options: Dict[str, Any],
        depth: int = 0,
    ) -> RecapSummaryResponse:
        """Recursively reduce summaries until they fit in 12K context.

        Args:
            summaries: List of intermediate summaries to reduce
            request: Original recap summary request
            max_bullets: Maximum number of bullets in final summary
            temperature_override: Optional temperature override
            llm_options: LLM generation options
            depth: Current recursion depth

        Returns:
            Final RecapSummaryResponse
        """
        max_reduce_chars = getattr(self.config, 'recursive_reduce_max_chars', 10_000)
        max_recursion_depth = getattr(self.config, 'recursive_reduce_max_depth', 3)

        # Combine all bullets from intermediate summaries
        combined_bullets = []
        for s in summaries:
            combined_bullets.extend(s.bullets)
        combined_text = "\n".join(combined_bullets)

        logger.info(
            "Reduce phase: Evaluating intermediate summaries",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "intermediate_count": len(summaries),
                "combined_length": len(combined_text),
                "max_reduce_chars": max_reduce_chars,
                "depth": depth,
            },
        )

        # If combined text fits within limit or max depth reached, do final reduce
        if len(combined_text) <= max_reduce_chars or depth >= max_recursion_depth:
            return await self._final_reduce(
                summaries, request, max_bullets, temperature_override
            )

        # Recursive reduce: Split summaries into groups and reduce each
        logger.info(
            "Recursive reduce: Intermediate summaries too large, splitting",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "combined_length": len(combined_text),
                "depth": depth,
            },
        )

        # Split summaries into 2-3 groups
        num_groups = min(3, max(2, len(summaries) // 2))
        chunk_size = len(summaries) // num_groups + (1 if len(summaries) % num_groups else 0)
        summary_groups = [
            summaries[i:i + chunk_size]
            for i in range(0, len(summaries), chunk_size)
        ]

        # Reduce each group in parallel
        json_schema_intermediate = IntermediateSummary.model_json_schema()
        reduce_tasks = [
            self._reduce_group(group, request, llm_options, json_schema_intermediate)
            for group in summary_groups
        ]

        reduced_results = await asyncio.gather(*reduce_tasks, return_exceptions=False)

        # Collect successful reduced summaries
        reduced_summaries: List[IntermediateSummary] = [
            result for result in reduced_results
            if result is not None and isinstance(result, IntermediateSummary)
        ]

        if not reduced_summaries:
            # Fallback: use original summaries and do final reduce anyway
            logger.warning(
                "Recursive reduce failed, falling back to final reduce",
                extra={"job_id": str(request.job_id), "genre": request.genre},
            )
            return await self._final_reduce(
                summaries, request, max_bullets, temperature_override
            )

        # Recurse with reduced summaries
        return await self._recursive_reduce_phase(
            reduced_summaries, request, max_bullets, temperature_override,
            llm_options, depth + 1
        )

    async def _reduce_group(
        self,
        group: List[IntermediateSummary],
        request: RecapSummaryRequest,
        llm_options: Dict[str, Any],
        json_schema: Dict[str, Any],
    ) -> Optional[IntermediateSummary]:
        """Reduce a group of intermediate summaries into one.

        Args:
            group: List of intermediate summaries in this group
            request: Original recap summary request
            llm_options: LLM generation options
            json_schema: JSON schema for intermediate summary

        Returns:
            Reduced IntermediateSummary or None if failed
        """
        # Combine bullets from group
        combined_bullets = []
        for s in group:
            combined_bullets.extend(s.bullets)

        # Create a minimal prompt for reducing this group
        bullets_text = "\n".join(f"- {bullet}" for bullet in combined_bullets)
        reduce_prompt = f"""以下の要点リストを3-4項目に要約してください。重要な情報を保持し、冗長な内容を統合してください。

# 入力要点
{bullets_text}

# 出力形式
JSONで bullets フィールドに要約した要点リストを返してください。"""

        try:
            llm_response = await self.llm_provider.generate(
                reduce_prompt,
                num_predict=self.config.summary_num_predict // 2,
                format=json_schema,
                options=llm_options,
            )

            parsed = json.loads(llm_response.response)
            return IntermediateSummary(**parsed)
        except Exception as e:
            logger.warning(
                "Failed to reduce group in recursive reduce",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "error": str(e),
                },
            )
            return None

    async def _final_reduce(
        self,
        summaries: List[IntermediateSummary],
        request: RecapSummaryRequest,
        max_bullets: int,
        temperature_override: Optional[float],
    ) -> RecapSummaryResponse:
        """Perform final reduce to create the summary response.

        Args:
            summaries: Intermediate summaries to combine
            request: Original recap summary request
            max_bullets: Maximum number of bullets
            temperature_override: Optional temperature override

        Returns:
            Final RecapSummaryResponse
        """
        logger.info(
            "Final reduce phase: Combining intermediate summaries",
            extra={
                "job_id": str(request.job_id),
                "genre": request.genre,
                "intermediate_count": len(summaries),
            },
        )

        # Convert intermediate summaries to highlights format
        reduce_highlights = []
        for inter_summary in summaries:
            for bullet in inter_summary.bullets:
                reduce_highlights.append(
                    RepresentativeSentence(
                        text=bullet,
                        published_at=None,
                        source_url=None,
                        article_id=None,
                        is_centroid=False,
                    )
                )

        # Create a dummy cluster to satisfy Pydantic validation
        # The genre_highlights will be used for the actual summarization
        dummy_cluster = RecapClusterInput(
            cluster_id=0,
            representative_sentences=[
                RepresentativeSentence(
                    text="(Hierarchical reduce - see genre_highlights)",
                    is_centroid=True,
                )
            ],
            top_terms=[],
        )

        # Create reduce request with highlights
        reduce_request = RecapSummaryRequest(
            job_id=request.job_id,
            genre=request.genre,
            clusters=[dummy_cluster],  # Dummy cluster to satisfy validation
            genre_highlights=reduce_highlights,
            options=request.options,
        )

        # Use single-shot path for final reduce
        return await self._generate_single_shot_summary(
            reduce_request, max_bullets, temperature_override
        )

    def _split_clusters_into_chunks(self, clusters: List) -> List[List]:
        """Split clusters into chunks that fit within max_chunk_chars with overlap.

        Uses overlap to preserve context between chunks, preventing information
        loss at chunk boundaries. The overlap ratio is configured via
        hierarchical_chunk_overlap_ratio (default 15%).
        """
        if not clusters:
            return []

        max_chars = self.config.hierarchical_chunk_max_chars
        overlap_ratio = getattr(self.config, 'hierarchical_chunk_overlap_ratio', 0.15)

        # Calculate cluster lengths
        cluster_lengths = []
        for cluster in clusters:
            length = sum(len(s.text) for s in cluster.representative_sentences) + 200  # Overhead
            cluster_lengths.append(length)

        chunks: List[List] = []
        current_chunk: List = []
        current_length = 0
        chunk_start_idx = 0

        for idx, cluster in enumerate(clusters):
            cluster_length = cluster_lengths[idx]

            if current_length + cluster_length > max_chars and current_chunk:
                chunks.append(current_chunk)

                # Calculate overlap: include trailing clusters from previous chunk
                overlap_chars = int(max_chars * overlap_ratio)
                overlap_clusters: List = []
                overlap_length = 0

                # Add clusters from end of previous chunk for overlap
                for j in range(len(current_chunk) - 1, -1, -1):
                    overlap_idx = chunk_start_idx + j
                    if overlap_length + cluster_lengths[overlap_idx] <= overlap_chars:
                        overlap_clusters.insert(0, current_chunk[j])
                        overlap_length += cluster_lengths[overlap_idx]
                    else:
                        break

                # Start new chunk with overlap clusters
                current_chunk = overlap_clusters + [cluster]
                current_length = overlap_length + cluster_length
                chunk_start_idx = idx - len(overlap_clusters)
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
        # Context window is 12K (default) or 60K tokens, configured in entrypoint.sh and config.py
        # Model routing automatically selects 12K or 60K based on input size
        # Reserve ~1K tokens for prompt template and safety margin
        # Using conservative 12K chars (~3K tokens) for faster LLM inference
        # Larger inputs will trigger hierarchical (map-reduce) summarization
        MAX_CLUSTER_SECTION_LENGTH = 12_000  # characters (~3K tokens, reduced from 50K for faster processing)

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
            "bullets": [bullet[:1000] for bullet in bullets],
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

    # ============================================================================
    # Cache Methods
    # ============================================================================

    def _generate_cache_key(self, request: RecapSummaryRequest) -> str:
        """Generate a unique cache key for a recap summary request.

        The key is based on the request content (clusters, genre, options),
        not the job_id, so identical requests return the same cached result.
        """
        # Create a deterministic hash of the request content
        key_data = {
            "genre": request.genre,
            "clusters": [
                {
                    "cluster_id": c.cluster_id,
                    "sentences": [s.text for s in c.representative_sentences],
                    "top_terms": c.top_terms,
                }
                for c in request.clusters
            ],
            "max_bullets": request.options.max_bullets if request.options else None,
        }
        content_hash = hashlib.sha256(
            json.dumps(key_data, sort_keys=True, ensure_ascii=False).encode()
        ).hexdigest()[:16]

        return f"recap:summary:{request.genre}:{content_hash}"

    async def _get_cached_response(
        self, cache_key: str
    ) -> Optional[RecapSummaryResponse]:
        """Try to retrieve a cached response.

        Args:
            cache_key: The cache key to look up

        Returns:
            The cached response if found and valid, None otherwise
        """
        if self.cache is None:
            return None

        try:
            cached_json = await self.cache.get(cache_key)
            if cached_json is None:
                return None

            cached_data = json.loads(cached_json)
            return RecapSummaryResponse(**cached_data)
        except Exception as e:
            logger.warning(
                "Failed to retrieve cached response",
                extra={"cache_key": cache_key, "error": str(e)},
            )
            return None

    async def _cache_response(
        self, cache_key: str, response: RecapSummaryResponse
    ) -> None:
        """Cache a response.

        Args:
            cache_key: The cache key to store under
            response: The response to cache
        """
        if self.cache is None:
            return

        try:
            response_json = response.model_dump_json()
            await self.cache.set(cache_key, response_json)
            logger.debug(
                "Cached recap summary response",
                extra={"cache_key": cache_key},
            )
        except Exception as e:
            logger.warning(
                "Failed to cache response",
                extra={"cache_key": cache_key, "error": str(e)},
            )

