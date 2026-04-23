"""Recap summary usecase - generates structured summaries from clustering output."""

import asyncio
import hashlib
import json
import logging
import re
import textwrap
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple, Union
from urllib.parse import urlparse

from jinja2 import Template
from pydantic import ValidationError

try:
    import json_repair
except ImportError:
    json_repair = None  # type: ignore

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    BatchRecapSummaryError,
    BatchRecapSummaryRequest,
    BatchRecapSummaryResponse,
    LLMGenerateResponse,
    RecapClusterInput,
    RecapSummaryRequest,
    RecapSummaryResponse,
    RecapSummary,
    RecapSummaryMetadata,
    IntermediateSummary,
    Reference,
    RepresentativeSentence,
)
from news_creator.port.cache_port import CachePort
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.utils.repetition_detector import detect_repetition

logger = logging.getLogger(__name__)

REFERENCE_MARKER_RE = re.compile(r"\[(\d+)\]")
PLACEHOLDER_BULLET_RE = re.compile(r"^\s*(?:\.\.\.|…)(?:\s*\[\d+\])?\s*$")
JAPANESE_CHAR_RE = re.compile(r"[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff]")
LATIN_CHAR_RE = re.compile(r"[A-Za-z]")

GEMMA_RECAP_SYSTEM_PROMPT = (
    "You are an expert Japanese news editor. "
    "Follow the JSON contract exactly and respond with only the requested JSON object."
)

GENRE_JA_MAP: Dict[str, str] = {
    "ai_data": "AI・データ",
    "climate_environment": "気候・環境",
    "consumer_tech": "家電・テクノロジー",
    "culture_arts": "文化・芸術",
    "cybersecurity": "サイバーセキュリティ",
    "diplomacy_security": "外交・安全保障",
    "economics_macro": "マクロ経済",
    "education": "教育",
    "energy_transition": "エネルギー",
    "film_tv": "映画・テレビ",
    "food_cuisine": "食・料理",
    "games_esports": "ゲーム・eスポーツ",
    "health_medicine": "医療・健康",
    "home_living": "住まい・暮らし",
    "industry_logistics": "産業・物流",
    "internet_platforms": "インターネット",
    "labor_workplace": "労働・キャリア",
    "law_crime": "法律・犯罪",
    "life_science": "生命科学",
    "markets_finance": "金融・市場",
    "mobility_automotive": "モビリティ・自動車",
    "music_audio": "音楽・オーディオ",
    "politics_government": "政治",
    "software_dev": "ソフトウェア開発",
    "space_astronomy": "宇宙・天文",
    "sports": "スポーツ",
    "startups_innovation": "スタートアップ",
    "travel_places": "旅行・観光",
}

_MIN_FALLBACK_SENTENCE_LEN = 20


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

        # Load prompt templates (7days default + 3days change-focused)
        prompts_dir = Path(__file__).parent.parent.parent / "prompts"
        with open(prompts_dir / "recap_summary.jinja", "r", encoding="utf-8") as f:
            self.template_7days = Template(f.read())
        template_3days_path = prompts_dir / "recap_summary_3days.jinja"
        if template_3days_path.exists():
            with open(template_3days_path, "r", encoding="utf-8") as f:
                self.template_3days = Template(f.read())
        else:
            self.template_3days = self.template_7days  # fallback
        # Backward compat alias
        self.template = self.template_7days

    @staticmethod
    def _is_3days_request(request: RecapSummaryRequest) -> bool:
        window_days = getattr(request, "window_days", None)
        return isinstance(window_days, int) and window_days <= 3

    def _max_cluster_section_length(self, request: RecapSummaryRequest) -> int:
        return 8_000 if self._is_3days_request(request) else 12_000

    _NUMERIC_PACKING_PATTERN = re.compile(
        r"(?:\d+(?:[.,]\d+)?\s*(?:%|％|円|ドル|億|千|百|兆|USD|JPY))"
        r"|(?:\$\s*\d+(?:[.,]\d+)?)"
        r"|(?:\b\d{4}[-/年]\d{1,2}[-/月]\d{1,2}(?:日)?\b)"
        r"|(?:\b\d{1,4}年\d{1,2}月\b)"
        r"|(?:\d+(?:\.\d+)?\s*%)",
    )

    @staticmethod
    def _source_domain(url: Optional[str]) -> Optional[str]:
        if not url:
            return None
        try:
            parsed = urlparse(url if "://" in url else f"https://{url}")
            return parsed.netloc or None
        except Exception:
            return None

    def _score_sentence_for_packing(
        self,
        sentence: RepresentativeSentence,
        cluster_sentences: List[RepresentativeSentence],
    ) -> float:
        score = 1.0
        if getattr(sentence, "is_centroid", False):
            score += 2.0

        text = getattr(sentence, "text", "") or ""
        if self._NUMERIC_PACKING_PATTERN.search(text):
            score += 1.0

        own_domain = self._source_domain(getattr(sentence, "source_url", None))
        other_domains = {
            self._source_domain(getattr(s, "source_url", None))
            for s in cluster_sentences
            if s is not sentence
        }
        other_domains.discard(None)
        other_domains.discard(own_domain)
        if own_domain and other_domains:
            score += 0.5

        return score

    def _resolve_generation_temperature(self, request: RecapSummaryRequest) -> float:
        if request.options and request.options.temperature is not None:
            return float(request.options.temperature)
        if self._is_3days_request(request):
            return float(self._config_float("recap_summary_temperature", 0.0))
        return float(self.config.llm_temperature)

    def _wrap_gemma_prompt(self, prompt_body: str) -> str:
        body = prompt_body.strip()
        return (
            "<|turn>system\n"
            f"{GEMMA_RECAP_SYSTEM_PROMPT}\n"
            "<turn|>\n"
            "<|turn>user\n"
            f"{body}\n"
            "<turn|>\n"
            "<|turn>model\n"
        )

    async def generate_summary(
        self, request: RecapSummaryRequest
    ) -> RecapSummaryResponse:
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
        temperature_override = self._resolve_generation_temperature(request)

        if self._should_bypass_llm(request):
            logger.info(
                "Bypassing LLM for low-evidence recap request",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "window_days": request.window_days,
                },
            )
            response = self._create_low_evidence_response(request)
            await self._cache_response(cache_key, response)
            return response

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
            response = await self._generate_hierarchical_summary(
                request, max_bullets, temperature_override
            )
            await self._cache_response(cache_key, response)
            return response

        # Single-shot summarization for smaller inputs
        response = await self._generate_single_shot_summary(
            request, max_bullets, temperature_override
        )
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

    def _should_bypass_llm(self, request: RecapSummaryRequest) -> bool:
        if request.window_days is None or request.window_days > 3:
            return False

        min_sources = max(1, self._config_int("recap_min_source_articles_for_llm", 1))
        min_sentences = max(
            1, self._config_int("recap_min_representative_sentences_for_llm", 2)
        )

        distinct_sources, representative_sentence_count = self._count_request_evidence(
            request
        )
        if representative_sentence_count < min_sentences:
            return True
        if distinct_sources and distinct_sources < min_sources:
            return True
        return False

    def _count_request_evidence(self, request: RecapSummaryRequest) -> Tuple[int, int]:
        source_keys = set()
        representative_sentence_count = 0

        for cluster in request.clusters:
            for sentence in cluster.representative_sentences:
                representative_sentence_count += 1
                key = sentence.article_id or sentence.source_url
                if key:
                    source_keys.add(key)

        return len(source_keys), representative_sentence_count

    def _config_int(self, name: str, default: int) -> int:
        value = getattr(self.config, name, default)
        if isinstance(value, bool):
            return int(value)
        if isinstance(value, int):
            return value
        if isinstance(value, float):
            return int(value)
        if isinstance(value, str):
            try:
                return int(value)
            except ValueError:
                return default
        return default

    def _config_float(self, name: str, default: float) -> float:
        value = getattr(self.config, name, default)
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            return float(value)
        if isinstance(value, str):
            try:
                return float(value)
            except ValueError:
                return default
        return default

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

        Uses hold_slot() + generate_raw() (BE path) instead of generate() (local-only)
        so that DistributingGateway can dispatch to remote Ollama instances.

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
            options=request.options,
            window_days=request.window_days,
        )
        chunk_prompt = self._build_prompt(
            chunk_request, max_bullets=4, intermediate=True
        )

        try:
            async with self.llm_provider.hold_slot(is_high_priority=False) as (
                _wait_time,
                cancel_event,
                task_id,
            ):
                llm_response = await self.llm_provider.generate_raw(
                    chunk_prompt,
                    cancel_event=cancel_event,
                    task_id=task_id,
                    num_predict=self.config.recap_summary_num_predict // 2,
                    format=json_schema,
                    options=llm_options,
                )

            return self._parse_intermediate_summary_json(llm_response.response, request)
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
            intermediate_summaries,
            request,
            max_bullets,
            temperature_override,
            llm_options,
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
        max_reduce_chars = getattr(self.config, "recursive_reduce_max_chars", 10_000)
        max_recursion_depth = getattr(self.config, "recursive_reduce_max_depth", 3)

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
        chunk_size = len(summaries) // num_groups + (
            1 if len(summaries) % num_groups else 0
        )
        summary_groups = [
            summaries[i : i + chunk_size] for i in range(0, len(summaries), chunk_size)
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
            result
            for result in reduced_results
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
            reduced_summaries,
            request,
            max_bullets,
            temperature_override,
            llm_options,
            depth + 1,
        )

    async def _reduce_group(
        self,
        group: List[IntermediateSummary],
        request: RecapSummaryRequest,
        llm_options: Dict[str, Any],
        json_schema: Dict[str, Any],
    ) -> Optional[IntermediateSummary]:
        """Reduce a group of intermediate summaries into one.

        Uses hold_slot() + generate_raw() (BE path) instead of generate() (local-only)
        so that DistributingGateway can dispatch to remote Ollama instances.

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

        reduce_prompt = self._build_reduce_group_prompt(request, combined_bullets)

        try:
            async with self.llm_provider.hold_slot(is_high_priority=False) as (
                _wait_time,
                cancel_event,
                task_id,
            ):
                llm_response = await self.llm_provider.generate_raw(
                    reduce_prompt,
                    cancel_event=cancel_event,
                    task_id=task_id,
                    num_predict=self.config.recap_summary_num_predict // 2,
                    format=json_schema,
                    options=llm_options,
                )

            return self._parse_intermediate_summary_json(llm_response.response, request)
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

    def _build_reduce_group_prompt(
        self,
        request: RecapSummaryRequest,
        combined_bullets: List[str],
    ) -> str:
        bullet_target = "2〜3" if self._is_3days_request(request) else "3〜4"
        bullets_text = "\n".join(f"- {bullet}" for bullet in combined_bullets)
        prompt_body = textwrap.dedent(
            f"""
            以下の要点リストを {bullet_target} 項目に統合・要約してください。

            ### 必須契約
            - 出力は JSON オブジェクト 1 つのみ。
            - Markdown、説明文、前置きは禁止。
            - `language` は必ず `"ja"`。
            - `bullets` は placeholder を禁止し、すべて完結した日本語にする。
            - 出典マーカー `[n]` がある場合は必ず保持する。
            - 企業名・サービス名・固有名詞・数値は落とさない。
            - 関連する変化は統合してもよいが、無関係な話題は混ぜない。

            ### Schema Mirror
            {{
              "bullets": [
                "要約1 [1]",
                "要約2 [2]"
              ],
              "language": "ja"
            }}

            ### 不正な例
            {{
              "bullets": ["..."],
              "language": "en"
            }}

            ### 入力要点
            {bullets_text}

            ### 出力
            JSON オブジェクトのみを返してください。
            """
        ).strip()
        return self._wrap_gemma_prompt(prompt_body)

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
            window_days=request.window_days,
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
        overlap_ratio = getattr(self.config, "hierarchical_chunk_overlap_ratio", 0.15)

        # Calculate cluster lengths
        cluster_lengths = []
        for cluster in clusters:
            length = (
                sum(len(s.text) for s in cluster.representative_sentences) + 200
            )  # Overhead
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

        return (
            chunks if chunks else [[c] for c in clusters]
        )  # At least one chunk per cluster if needed

    def _build_chat_messages(
        self, request: RecapSummaryRequest, max_bullets: int, intermediate: bool = False
    ) -> List[Dict[str, str]]:
        """Build chat messages for /api/chat (no manual turn tokens)."""
        prompt_body, _ = self._render_prompt_body(
            request, max_bullets, intermediate=intermediate
        )
        return [
            {"role": "system", "content": GEMMA_RECAP_SYSTEM_PROMPT},
            {"role": "user", "content": prompt_body},
        ]

    async def _call_llm_for_recap(
        self,
        messages: List[Dict[str, str]],
        json_schema: Dict[str, Any],
        llm_options: Dict[str, Any],
    ) -> LLMGenerateResponse:
        """Call LLM via /api/chat (think enabled) with generate() fallback.

        Prefers chat_generate() for correct format constraint enforcement
        with Gemma 4 thinking mode (Ollama #15260 workaround).
        Falls back to generate() for backward compatibility.
        """
        try:
            chat_payload: Dict[str, Any] = {
                "model": self.config.model_name,
                "messages": messages,
                "format": json_schema,
                "options": llm_options,
            }
            raw_response = await self.llm_provider.chat_generate(chat_payload)
            # Validate that response is a proper dict with message.content
            if not isinstance(raw_response, dict):
                raise TypeError("chat_generate returned non-dict response")
            content = raw_response.get("message", {}).get("content")
            if not isinstance(content, str):
                raise TypeError("chat_generate response missing message.content string")
            return LLMGenerateResponse(
                response=content,
                model=raw_response.get("model", self.config.model_name),
                prompt_eval_count=raw_response.get("prompt_eval_count", 0),
                eval_count=raw_response.get("eval_count", 0),
                total_duration=raw_response.get("total_duration", 0),
            )
        except (NotImplementedError, TypeError):
            # Fallback: use generate() with raw prompt (legacy path)
            opts = {**llm_options}
            num_predict = opts.pop("num_predict", self.config.recap_summary_num_predict)
            prompt = self._wrap_gemma_prompt(messages[1]["content"])
            result = await self.llm_provider.generate(
                prompt,
                num_predict=num_predict,
                format=json_schema,
                options=opts,
            )
            assert isinstance(result, LLMGenerateResponse)
            return result

    async def _generate_single_shot_summary(
        self,
        request: RecapSummaryRequest,
        max_bullets: int,
        temperature_override: Optional[float],
    ) -> RecapSummaryResponse:
        """Generate summary using single-shot approach via /api/chat with thinking."""
        messages = self._build_chat_messages(request, max_bullets)
        active_messages = messages
        is_3days = self._is_3days_request(request)

        base_temperature = (
            float(temperature_override)
            if temperature_override is not None
            else self._resolve_generation_temperature(request)
        )
        llm_options: Optional[Dict[str, Any]] = {"temperature": base_temperature}

        max_retries = max(2, self.config.max_repetition_retries)
        remaining_repair_attempts = max(
            0, self._config_int("recap_summary_repair_attempts", 1)
        )
        last_error = None
        json_validation_error_count = 0

        for attempt in range(max_retries + 1):
            current_temp = base_temperature
            # Use lower repeat_penalty for recap to avoid EOS bias (ADR-632 insight)
            current_repeat_penalty = 1.0

            if attempt > 0:
                if is_3days:
                    current_temp = base_temperature
                else:
                    current_temp = max(0.05, base_temperature - (0.05 * attempt))
                current_repeat_penalty = min(
                    1.2, current_repeat_penalty + (0.05 * attempt)
                )

            json_schema = RecapSummary.model_json_schema()

            llm_options_retry = {
                **(llm_options or {}),
                "temperature": float(current_temp),
                "repeat_penalty": float(current_repeat_penalty),
                "num_predict": self.config.recap_summary_num_predict,
            }

            llm_response = await self._call_llm_for_recap(
                active_messages, json_schema, llm_options_retry
            )

            has_repetition, rep_score, rep_patterns = detect_repetition(
                llm_response.response, threshold=self.config.repetition_threshold
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
                    },
                )
                last_error = RuntimeError(
                    f"Repetition detected (score: {rep_score:.2f})"
                )
                continue

            try:
                summary_payload, parse_errors = self._parse_summary_json(
                    llm_response.response,
                    max_bullets,
                    strict_contract=is_3days,
                )
                json_validation_error_count += parse_errors

                summary_text = (
                    summary_payload.get("title", "")
                    + " "
                    + " ".join(summary_payload.get("bullets", []))
                )
                has_repetition_in_summary, rep_score_summary, rep_patterns_summary = (
                    detect_repetition(
                        summary_text, threshold=self.config.repetition_threshold
                    )
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
                        },
                    )
                    last_error = RuntimeError(
                        f"Repetition in parsed summary (score: {rep_score_summary:.2f})"
                    )
                    continue

                quality_issues = self._validate_summary_quality(
                    request, summary_payload
                )
                if quality_issues:
                    logger.warning(
                        "Semantic contract violation detected in recap summary",
                        extra={
                            "job_id": str(request.job_id),
                            "genre": request.genre,
                            "attempt": attempt + 1,
                            "issues": quality_issues,
                        },
                    )
                    last_error = RuntimeError("; ".join(quality_issues))
                    json_validation_error_count += len(quality_issues)
                    if remaining_repair_attempts > 0:
                        remaining_repair_attempts -= 1
                        repair_body = self._build_repair_prompt_body(
                            request,
                            max_bullets,
                            llm_response.response,
                            quality_issues,
                        )
                        active_messages = [
                            {"role": "system", "content": GEMMA_RECAP_SYSTEM_PROMPT},
                            {"role": "user", "content": repair_body},
                        ]
                        continue
                    break

                # Bullet length quality gate: reject too-short bullets
                bullets = summary_payload.get("bullets", [])
                min_avg = self._config_int("recap_min_avg_bullet_length", 150)
                if bullets and min_avg > 0:
                    avg_bullet_len = sum(len(b) for b in bullets) / len(bullets)
                    if avg_bullet_len < min_avg and attempt < max_retries:
                        logger.warning(
                            "Recap bullets too short, will retry",
                            extra={
                                "job_id": str(request.job_id),
                                "genre": request.genre,
                                "attempt": attempt + 1,
                                "avg_bullet_length": avg_bullet_len,
                                "min_required": min_avg,
                            },
                        )
                        last_error = RuntimeError(
                            f"Bullets too short (avg: {avg_bullet_len:.0f} < {min_avg})"
                        )
                        continue

                summary = RecapSummary(**summary_payload)

                metadata = RecapSummaryMetadata(
                    model=llm_response.model,
                    temperature=current_temp
                    if current_temp is not None
                    else self.config.llm_temperature,
                    prompt_tokens=llm_response.prompt_eval_count,
                    completion_tokens=llm_response.eval_count,
                    processing_time_ms=self._nanoseconds_to_milliseconds(
                        llm_response.total_duration
                    ),
                    json_validation_errors=json_validation_error_count,
                    summary_length_bullets=len(summary.bullets),
                )

                return RecapSummaryResponse(
                    job_id=request.job_id,
                    genre=request.genre,
                    summary=summary,
                    metadata=metadata,
                )
            except (RuntimeError, ValidationError) as e:
                last_error = e if isinstance(e, RuntimeError) else RuntimeError(str(e))
                if isinstance(e, ValidationError):
                    validation_issues = [err["msg"] for err in e.errors()]
                    json_validation_error_count += len(validation_issues)
                    if remaining_repair_attempts > 0:
                        remaining_repair_attempts -= 1
                        repair_body = self._build_repair_prompt_body(
                            request,
                            max_bullets,
                            llm_response.response,
                            validation_issues,
                        )
                        active_messages = [
                            {"role": "system", "content": GEMMA_RECAP_SYSTEM_PROMPT},
                            {"role": "user", "content": repair_body},
                        ]
                        continue
                if attempt < max_retries:
                    continue
                # Last attempt failed — fall through to fallback path
                break

        # Fail-safe: If all retries fail, return fallback
        if request.genre_highlights:
            logger.warning(
                "Falling back to genre highlights due to LLM failure",
                extra={"job_id": str(request.job_id), "genre": request.genre},
            )
            return self._create_fallback_response(request)

        # Fallback: Use representative sentences from clusters
        if request.clusters:
            logger.warning(
                "Falling back to cluster representatives due to LLM failure",
                extra={"job_id": str(request.job_id), "genre": request.genre},
            )
            return self._create_fallback_from_clusters(request)

        if last_error:
            raise last_error
        raise RuntimeError("Failed to generate recap summary")

    def _create_fallback_response(
        self, request: RecapSummaryRequest
    ) -> RecapSummaryResponse:
        """Create a response from genre highlights when LLM generation fails."""
        highlights = request.genre_highlights or []
        candidates = [
            {
                "text": highlight.text,
                "source_url": highlight.source_url,
                "article_id": highlight.article_id,
                "topic_label": request.genre.replace("_", " "),
            }
            for highlight in highlights
        ]
        return self._create_degraded_response(
            request,
            candidates,
            model_name="extractive-fallback",
            degradation_reason="llm_failed_after_repair",
            json_validation_errors=1,
        )

    def _create_low_evidence_response(
        self, request: RecapSummaryRequest
    ) -> RecapSummaryResponse:
        """Create a degraded response when 3days evidence is too thin for reliable generation."""
        response = self._create_fallback_from_clusters(request)
        response.metadata.model = "low-evidence-extractive"
        response.metadata.json_validation_errors = 0
        response.metadata.degradation_reason = "low_evidence_extractive"
        return response

    def _create_fallback_from_clusters(
        self, request: RecapSummaryRequest
    ) -> RecapSummaryResponse:
        """Create a degraded response preserving references and preferring centroids."""
        candidates = self._collect_fallback_candidates_from_clusters(request)
        return self._create_degraded_response(
            request,
            candidates,
            model_name="cluster-fallback",
            degradation_reason="llm_failed_after_repair",
            json_validation_errors=1,
        )

    def _collect_fallback_candidates_from_clusters(
        self,
        request: RecapSummaryRequest,
    ) -> List[Dict[str, Any]]:
        max_fallback_bullets = 4 if self._is_3days_request(request) else 7
        sorted_clusters = sorted(
            request.clusters,
            key=lambda c: len(c.representative_sentences),
            reverse=True,
        )

        candidates: List[Dict[str, Any]] = []
        seen_texts: set[str] = set()

        # Collect all valid sentences, preferring Japanese
        ja_candidates: List[Dict[str, Any]] = []
        other_candidates: List[Dict[str, Any]] = []

        for cluster in sorted_clusters:
            ordered_sentences = sorted(
                cluster.representative_sentences,
                key=lambda sentence: (not sentence.is_centroid, len(sentence.text)),
            )
            topic_label = self._genre_to_japanese(request.genre)
            for sentence in ordered_sentences:
                normalized_text = " ".join(sentence.text.split())
                if not normalized_text or normalized_text in seen_texts:
                    continue
                if len(normalized_text) < _MIN_FALLBACK_SENTENCE_LEN:
                    continue
                # Filter code-heavy sentences (>30% backticks)
                if normalized_text.count("`") > len(normalized_text) * 0.3:
                    continue
                seen_texts.add(normalized_text)
                entry = {
                    "text": normalized_text,
                    "source_url": sentence.source_url,
                    "article_id": sentence.article_id,
                    "topic_label": topic_label,
                }
                if self._looks_japanese(normalized_text):
                    ja_candidates.append(entry)
                else:
                    other_candidates.append(entry)

        # Prefer Japanese sentences, then fill with others
        candidates = (ja_candidates + other_candidates)[:max_fallback_bullets]
        return candidates

    def _create_degraded_response(
        self,
        request: RecapSummaryRequest,
        candidates: List[Dict[str, Any]],
        *,
        model_name: str,
        degradation_reason: str,
        json_validation_errors: int,
    ) -> RecapSummaryResponse:
        references: List[Reference] = []
        bullets: List[str] = []
        ref_id = 1

        for candidate in candidates:
            source_url = candidate.get("source_url")
            article_id = candidate.get("article_id")
            topic_label = str(
                candidate.get("topic_label") or request.genre.replace("_", " ")
            ).strip()
            reference_id: Optional[int] = None
            if source_url:
                reference_id = ref_id
                references.append(
                    Reference(
                        id=ref_id,
                        url=source_url,
                        domain=self._extract_domain(source_url),
                        article_id=article_id,
                    )
                )
                ref_id += 1
            bullets.append(
                self._format_degraded_bullet(
                    request,
                    text=str(candidate.get("text") or "").strip(),
                    topic_label=topic_label or request.genre.replace("_", " "),
                    reference_id=reference_id,
                )
            )

        if not bullets:
            bullets = [
                "関連する更新が確認されたが、要約の自動生成には失敗した。出典の再処理が必要である。"
            ]

        title_suffix = "直近更新" if self._is_3days_request(request) else "主要トピック"
        genre_ja = self._genre_to_japanese(request.genre)
        summary = RecapSummary(
            title=f"{genre_ja}の{title_suffix}",
            bullets=bullets,
            language="ja",
            references=references if references else None,
        )

        metadata = RecapSummaryMetadata(
            model=model_name,
            temperature=0.0,
            prompt_tokens=0,
            completion_tokens=0,
            processing_time_ms=0,
            json_validation_errors=json_validation_errors,
            summary_length_bullets=len(summary.bullets),
            is_degraded=True,
            degradation_reason=degradation_reason,
        )

        return RecapSummaryResponse(
            job_id=request.job_id,
            genre=request.genre,
            summary=summary,
            metadata=metadata,
        )

    def _format_degraded_bullet(
        self,
        request: RecapSummaryRequest,
        *,
        text: str,
        topic_label: str,
        reference_id: Optional[int],
    ) -> str:
        reference_suffix = f" [{reference_id}]" if reference_id is not None else ""
        if self._looks_japanese(text):
            return f"{text}{reference_suffix}"
        genre_ja = self._genre_to_japanese(request.genre)
        return f"【{genre_ja}】{text}{reference_suffix}"

    @staticmethod
    def _looks_japanese(text: str) -> bool:
        if not text:
            return False
        ja_count = len(JAPANESE_CHAR_RE.findall(text))
        return ja_count / max(len(text), 1) > 0.15

    @staticmethod
    def _genre_to_japanese(genre: str) -> str:
        base_genre = re.sub(r"_\d+$", "", genre)
        return GENRE_JA_MAP.get(base_genre, genre.replace("_", " "))

    @staticmethod
    def _truncate_fallback_snippet(text: str, max_chars: int = 120) -> str:
        cleaned = " ".join(text.split())
        if len(cleaned) <= max_chars:
            return cleaned
        return cleaned[: max_chars - 1].rstrip() + "…"

    @staticmethod
    def _extract_domain(url: str) -> str:
        try:
            parsed = urlparse(url if "://" in url else f"https://{url}")
            return parsed.netloc or parsed.path.split("/")[0] or url
        except Exception:
            return url

    def _render_prompt_body(
        self,
        request: RecapSummaryRequest,
        max_bullets: int,
        intermediate: bool = False,
    ) -> Tuple[str, str]:
        MAX_CLUSTER_SECTION_LENGTH = self._max_cluster_section_length(request)

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
                        source_domain = (
                            parsed.netloc
                            or parsed.path.split("/")[0]
                            or sentence.source_url
                        )
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
            estimated_length = (
                cluster_section_length + len(cluster_block) + 2
            )  # +2 for "\n\n"
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
                    },
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
                },
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

        render_kwargs: Dict[str, Any] = {
            "job_id": str(request.job_id),
            "genre": request.genre,
            "max_bullets": max_bullets,
            "intermediate": intermediate,
        }

        if request.genre_highlights:
            render_kwargs["highlights"] = request.genre_highlights
            render_kwargs["cluster_section"] = None  # Force use of highlights path
        else:
            render_kwargs["cluster_section"] = cluster_section
            render_kwargs["highlights"] = None

        # Select template: 3days change-focused vs 7days deep-dive
        is_3days = self._is_3days_request(request)
        template = self.template_3days if is_3days else self.template_7days
        prompt_body = template.render(**render_kwargs)
        return prompt_body, cluster_section

    def _build_prompt(
        self, request: RecapSummaryRequest, max_bullets: int, intermediate: bool = False
    ) -> str:
        prompt_body, cluster_section = self._render_prompt_body(
            request,
            max_bullets,
            intermediate=intermediate,
        )
        prompt = self._wrap_gemma_prompt(prompt_body)
        prompt_length = len(prompt)
        estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars

        # Validate prompt size - detect abnormal amplification
        ABNORMAL_PROMPT_THRESHOLD = 100_000  # characters (>25K tokens)
        if prompt_length > ABNORMAL_PROMPT_THRESHOLD:
            # Check for repetition in the prompt
            has_repetition, repetition_score, repetition_patterns = detect_repetition(
                prompt, threshold=0.3
            )

            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED in recap_summary_usecase._build_prompt",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "cluster_section_length": len(cluster_section)
                    if cluster_section
                    else 0,
                    "highlights_length": len(str(request.genre_highlights))
                    if request.genre_highlights
                    else 0,
                    "prompt_preview_start": prompt[:500],
                    "prompt_preview_end": prompt[-500:] if prompt_length > 1000 else "",
                    "has_repetition": has_repetition,
                    "repetition_score": repetition_score,
                    "repetition_patterns": repetition_patterns,
                },
            )
            # Check if prompt contains repeated content
            if cluster_section and len(cluster_section) * 10 < prompt_length:
                logger.error(
                    "Prompt size is much larger than cluster_section - possible repetition or amplification",
                    extra={
                        "job_id": str(request.job_id),
                        "cluster_section_length": len(cluster_section),
                        "prompt_length": prompt_length,
                        "ratio": prompt_length / len(cluster_section)
                        if cluster_section
                        else 0,
                        "has_repetition": has_repetition,
                        "repetition_score": repetition_score,
                    },
                )
        else:
            logger.info(
                "Recap summary prompt built",
                extra={
                    "job_id": str(request.job_id),
                    "genre": request.genre,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "cluster_section_length": len(cluster_section)
                    if cluster_section
                    else 0,
                    "highlights_length": len(str(request.genre_highlights))
                    if request.genre_highlights
                    else 0,
                },
            )

        return prompt

    def _build_repair_prompt_body(
        self,
        request: RecapSummaryRequest,
        max_bullets: int,
        invalid_response: str,
        issues: List[str],
    ) -> str:
        """Build repair prompt body (without turn tokens, for /api/chat)."""
        issue_lines = "\n".join(f"- {issue}" for issue in issues)
        truncated_response = invalid_response[:3000]
        base_prompt_body, _ = self._render_prompt_body(request, max_bullets)
        return (
            f"{base_prompt_body}\n\n"
            "### 修正タスク\n"
            "前回の出力は契約違反でした。入力にない事実を追加せず、"
            "下記の問題だけを修正して JSON オブジェクト 1 つだけを返してください。\n"
            f"{issue_lines}\n\n"
            "### 前回の不正な出力\n"
            f"{truncated_response}\n"
        )

    def _validate_summary_quality(
        self,
        request: RecapSummaryRequest,
        payload: Dict[str, Any],
    ) -> List[str]:
        is_3days = request.window_days is not None and request.window_days <= 3
        if not is_3days:
            return []

        issues: List[str] = []
        title = str(payload.get("title", "")).strip()
        bullets = payload.get("bullets") or []
        language = payload.get("language")

        # Count total representative sentences for thin-evidence detection
        _, total_sentences = self._count_request_evidence(request)
        is_thin_evidence = total_sentences <= 3

        if not title:
            issues.append("title must be a non-empty Japanese string")

        min_bullets = 1 if is_thin_evidence else 2
        if len(bullets) < min_bullets:
            issues.append(
                f"bullets must contain at least {min_bullets} non-empty items"
            )

        if len(bullets) == 1 and bullets[0].strip() == title:
            issues.append("bullets must not collapse to a title-only fallback")

        if any(self._is_placeholder_bullet(bullet) for bullet in bullets):
            issues.append("bullets must not contain placeholder text such as '... [1]'")

        if language != "ja":
            issues.append("language must be 'ja'")

        japanese_ratio = self._compute_japanese_ratio(" ".join([title, *bullets]))
        min_ja_ratio = self._config_float("recap_ja_ratio_threshold", 0.6)
        if japanese_ratio < min_ja_ratio:
            issues.append(f"Japanese text ratio must be >= {min_ja_ratio:.2f}")
        title_ja_ratio = self._compute_japanese_ratio(title)
        if title and title_ja_ratio < min_ja_ratio:
            issues.append(f"title Japanese text ratio must be >= {min_ja_ratio:.2f}")

        references = payload.get("references") or []
        reference_ids = {
            ref.get("id")
            for ref in references
            if isinstance(ref, dict) and isinstance(ref.get("id"), int)
        }
        cited_ids = self._extract_cited_reference_ids(bullets)
        if any(not REFERENCE_MARKER_RE.search(str(bullet)) for bullet in bullets):
            issues.append("every 3days bullet must cite at least one [n] reference")
        if cited_ids and not cited_ids.issubset(reference_ids):
            issues.append("every cited [n] marker in bullets must exist in references")

        if any(len(str(bullet).strip()) < 40 for bullet in bullets):
            issues.append(
                "3days recap bullets must be substantive, not ultra-short stubs"
            )

        return issues

    def _validate_intermediate_summary_quality(
        self,
        request: RecapSummaryRequest,
        payload: Dict[str, Any],
    ) -> List[str]:
        if not self._is_3days_request(request):
            return []

        issues: List[str] = []
        bullets = payload.get("bullets") or []
        language = payload.get("language")

        if language != "ja":
            issues.append("intermediate summary language must be 'ja'")
        if len(bullets) < 2:
            issues.append("intermediate summary must contain at least 2 bullets")
        if any(self._is_placeholder_bullet(str(bullet)) for bullet in bullets):
            issues.append(
                "intermediate summary bullets must not contain placeholder text"
            )

        japanese_ratio = self._compute_japanese_ratio(
            " ".join(str(bullet) for bullet in bullets)
        )
        min_ja_ratio = self._config_float("recap_ja_ratio_threshold", 0.6)
        if japanese_ratio < min_ja_ratio:
            issues.append(
                f"intermediate summary Japanese text ratio must be >= {min_ja_ratio:.2f}"
            )

        return issues

    def _is_placeholder_bullet(self, bullet: str) -> bool:
        return bool(PLACEHOLDER_BULLET_RE.fullmatch(bullet.strip()))

    def _compute_japanese_ratio(self, text: str) -> float:
        japanese_chars = len(JAPANESE_CHAR_RE.findall(text))
        latin_chars = len(LATIN_CHAR_RE.findall(text))
        relevant_chars = japanese_chars + latin_chars
        if relevant_chars == 0:
            return 1.0
        return japanese_chars / relevant_chars

    def _extract_cited_reference_ids(self, bullets: List[str]) -> set[int]:
        cited_ids: set[int] = set()
        for bullet in bullets:
            for match in REFERENCE_MARKER_RE.findall(bullet):
                try:
                    cited_ids.add(int(match))
                except ValueError:
                    continue
        return cited_ids

    def _resolve_max_bullets(self, request: RecapSummaryRequest) -> int:
        if request.options and request.options.max_bullets is not None:
            return request.options.max_bullets
        if request.window_days is not None and request.window_days <= 3:
            return 7
        return 15

    def _parse_summary_json(
        self,
        content: str,
        max_bullets: int,
        *,
        strict_contract: bool = False,
    ) -> Tuple[Dict[str, Any], int]:
        if not content:
            raise RuntimeError("LLM returned empty response for recap summary")

        # Strip Gemma 4 thinking blocks before JSON parse
        content = re.sub(
            r"<\|channel>thought.*?<channel\|>", "", content, flags=re.DOTALL
        )

        parse_errors = 0
        try:
            # Structured Outputs should trigger clean JSON, but just in case, straightforward load.
            parsed = json.loads(content)
        except json.JSONDecodeError as exc:
            parse_errors += 1
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

        return self._sanitize_summary_payload(
            parsed,
            max_bullets,
            strict_contract=strict_contract,
        ), parse_errors

    def _parse_intermediate_summary_json(
        self,
        content: str,
        request: RecapSummaryRequest,
    ) -> IntermediateSummary:
        if not content:
            raise RuntimeError(
                "LLM returned empty response for intermediate recap summary"
            )

        # Strip Gemma 4 thinking blocks before JSON parse
        content = re.sub(
            r"<\|channel>thought.*?<channel\|>", "", content, flags=re.DOTALL
        )

        try:
            parsed = json.loads(content)
        except json.JSONDecodeError as exc:
            if json_repair:
                repaired_json = json_repair.repair_json(content)
                parsed = json.loads(repaired_json)
            else:
                raise RuntimeError(
                    f"Failed to parse intermediate structured output: {exc}"
                ) from exc

        if not isinstance(parsed, dict):
            raise RuntimeError("Intermediate summary response must be a JSON object")

        payload = self._sanitize_intermediate_payload(parsed)
        if not payload.get("language") and not self._is_3days_request(request):
            payload["language"] = "ja"
        issues = self._validate_intermediate_summary_quality(request, payload)
        if issues:
            raise RuntimeError("; ".join(issues))

        return IntermediateSummary(**payload)

    def _sanitize_summary_payload(
        self,
        payload: Dict[str, Any],
        max_bullets: int,
        *,
        strict_contract: bool = False,
    ) -> Dict[str, Any]:
        summary_section = payload.get("summary")
        if isinstance(summary_section, dict):
            payload = summary_section

        title = payload.get("title")

        # タイトルがコードフェンス等の不正値の場合は修正
        invalid_titles = ["```json", "```", "json", "{", ""]
        if strict_contract:
            if isinstance(title, str):
                title = title.strip()[:200]
            else:
                title = ""
        elif not isinstance(title, str) or title.strip().lower() in invalid_titles:
            # bulletsの先頭から抽出を試みる
            bullets = payload.get("bullets", [])
            if bullets and isinstance(bullets[0], str):
                # 最初のbulletから15-45文字を抽出してタイトル化
                first_bullet = bullets[0]
                title = self._extract_title_from_bullet(first_bullet)
            else:
                title = "主要トピックのまとめ"

        if not isinstance(title, str) or not title.strip():
            title = "" if strict_contract else "主要トピックのまとめ"
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
        if strict_contract:
            language = language.strip() if isinstance(language, str) else ""
        elif not isinstance(language, str) or not language.strip():
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
        if not bullets and not strict_contract:
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
                                parsed = urlparse(
                                    url if "://" in url else f"https://{url}"
                                )
                                domain = (
                                    parsed.netloc or parsed.path.split("/")[0] or url
                                )
                            except Exception:
                                domain = url
                        else:
                            continue
                    validated_refs.append(
                        {
                            "id": ref_id,
                            "url": url,
                            "domain": domain,
                            "article_id": ref.get("article_id"),
                        }
                    )
                references = validated_refs if validated_refs else None

        # Auto-inject [n] reference markers into bullets that lack them
        final_bullets = []
        for idx, bullet in enumerate(bullets):
            trimmed = bullet[:1000]
            if references and not REFERENCE_MARKER_RE.search(trimmed):
                # Assign the reference ID round-robin from available references
                ref_id = references[idx % len(references)]["id"]
                trimmed = f"{trimmed} [{ref_id}]"
            final_bullets.append(trimmed)

        sanitized = {
            "title": title,
            "bullets": final_bullets,
            "language": language,
        }
        if references:
            sanitized["references"] = references

        return sanitized

    def _sanitize_intermediate_payload(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        summary_section = payload.get("summary")
        if isinstance(summary_section, dict):
            payload = summary_section

        bullets_field = payload.get("bullets")
        if isinstance(bullets_field, list):
            bullets = [
                str(bullet).strip()[:1000]
                for bullet in bullets_field
                if isinstance(bullet, (str, int, float)) and str(bullet).strip()
            ]
        else:
            bullets = []

        language = payload.get("language")
        if not isinstance(language, str):
            language = ""

        return {
            "bullets": bullets,
            "language": language.strip(),
        }

    def _extract_title_from_bullet(self, bullet: str) -> str:
        """bulletテキストから適切なタイトルを抽出する"""
        # 最初の句点または45文字までを取得
        for i, char in enumerate(bullet):
            if char in "。、" and 15 <= i <= 45:
                return bullet[: i + 1]
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
            "window_days": request.window_days,
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
