"""Evidence pipeline orchestrating preprocessing, clustering, and selection."""

from __future__ import annotations

import math
import re
import time
import unicodedata
from dataclasses import dataclass
from typing import Optional, Sequence

import numpy as np
import structlog

from ..domain.models import (
    CorpusClassifierStats,
    CorpusMetadata,
    EvidenceBudget,
    EvidenceCluster,
    EvidenceRequest,
    EvidenceResponse,
    WarmupResponse,
    ClusterLabel,
    ClusterStats,
    RepresentativeSentence,
    RepresentativeSource,
    build_response_template,
)
from ..domain import selectors, topics
from ..domain.schema import validate_request
from ..infra.config import Settings
from ..infra.telemetry import (
    DEDUP_REMOVED,
    EMBED_SECONDS,
    HDBSCAN_SECONDS,
    MMR_SELECTED,
    REQUEST_EMBED_SENTENCES,
    REQUEST_PROCESS_SECONDS,
)
from .clusterer import Clusterer
from .embedder import Embedder


_SENTENCE_SPLIT_RE = re.compile(r"(?<=[.!?。！？])\s+")
_LOGGER = structlog.get_logger(__name__)
_MIN_DOCUMENTS_PER_GENRE = 3

# URL and email regex patterns
_URL_PATTERN = re.compile(
    r"https?://[^\s]+|www\.[^\s]+|[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"
)
# Punctuation repetition pattern (3+ consecutive identical punctuation marks)
_PUNCT_REPEAT_PATTERN = re.compile(r"([。！？!?])\1{2,}")


def normalize_text(text: str, enable_sudachi: bool = False) -> str:
    """
    Normalize text for embedding generation to reduce noise and improve distance stability.

    Applies:
    - Whitespace normalization (removes excessive newlines/spaces)
    - Unicode normalization (NFKC: full-width to half-width, etc.)
    - URL/email placeholder replacement
    - Punctuation repetition reduction

    Args:
        text: Input text to normalize
        enable_sudachi: If True, apply Sudachi morphological normalization (experimental)

    Returns:
        Normalized text string
    """
    if not text:
        return text

    # Step 1: Unicode normalization (NFKC: full-width to half-width, etc.)
    normalized = unicodedata.normalize("NFKC", text)

    # Step 2: Replace URLs and emails with placeholders
    normalized = _URL_PATTERN.sub(
        lambda m: "<URL>" if "://" in m.group(0) or m.group(0).startswith("www.") else "<EMAIL>",
        normalized
    )

    # Step 3: Reduce excessive punctuation repetition (e.g., "。。。" -> "。")
    normalized = _PUNCT_REPEAT_PATTERN.sub(r"\1", normalized)

    # Step 4: Whitespace normalization (collapse multiple spaces/newlines to single space)
    # Preserve paragraph boundaries by keeping single newlines, but collapse multiple
    normalized = re.sub(r"\s+", " ", normalized)

    # Step 5: Optional Sudachi preprocessing (for dedup/internal representation only)
    if enable_sudachi:
        try:
            from .classifier import SudachiTokenizer
            tokenizer = SudachiTokenizer(mode="C")
            # Tokenize and rejoin with spaces (surface form normalization)
            tokens = tokenizer.tokenize(normalized)
            normalized = " ".join(tokens)
        except ImportError:
            _LOGGER.warning("Sudachi not available, skipping morphological normalization")
        except Exception as e:
            _LOGGER.warning("Sudachi normalization failed", error=str(e))

    return normalized.strip()


@dataclass
class SentenceRecord:
    text: str
    article_id: str
    url: str | None
    paragraph_idx: int
    sentence_idx: int
    lang: str | None
    tokens_estimate: int


class EvidencePipeline:
    """Top-level orchestrator for the recap evidence workflow."""

    def __init__(self, *, settings: Settings, embedder: Embedder, process_pool) -> None:
        self.settings = settings
        self.embedder = embedder
        self.clusterer = Clusterer(settings)
        self.process_pool = process_pool

    def run(self, request: EvidenceRequest) -> EvidenceResponse:
        start_time = time.perf_counter()
        logger = _LOGGER.bind(job_id=request.job_id, genre=request.genre)
        if request.telemetry:
            if request.telemetry.request_id:
                logger = logger.bind(request_id=request.telemetry.request_id)
            if request.telemetry.prompt_version:
                logger = logger.bind(prompt_version=request.telemetry.prompt_version)
        logger.info("pipeline.start", documents=len(request.documents))
        payload_dict = request.model_dump(mode="json")
        validate_request(payload_dict)

        response = build_response_template(request)
        sentences = self._extract_sentences(request)
        if not sentences:
            logger.info("pipeline.no-sentences")
            return response
        logger.info("pipeline.sentences.collected", sentences=len(sentences))

        if len(sentences) > request.constraints.max_total_sentences:
            sentences = sentences[: request.constraints.max_total_sentences]
            response.diagnostics.partial = True

        response.diagnostics.total_sentences = len(sentences)

        sentence_texts = [record.text for record in sentences]
        REQUEST_EMBED_SENTENCES.observe(len(sentence_texts))

        embed_start = time.perf_counter()
        embeddings = self.embedder.encode(sentence_texts)
        embed_duration = time.perf_counter() - embed_start
        EMBED_SECONDS.observe(embed_duration)
        response.diagnostics.embedding_ms = embed_duration * 1000
        logger.info(
            "pipeline.embedding.complete",
            sentences=len(sentence_texts),
            embedding_ms=response.diagnostics.embedding_ms,
        )

        classifier_stats = request.metadata.classifier if request.metadata else None

        dedup_threshold = self._adjust_dedup_threshold(
            request.constraints.dedup_threshold, request.metadata, request.genre
        )
        keep_indices, removed = selectors.prune_duplicates(embeddings, threshold=dedup_threshold)
        DEDUP_REMOVED.inc(removed)
        sentences = [sentences[idx] for idx in keep_indices]
        embeddings = embeddings[keep_indices]
        logger.info(
            "pipeline.dedup.complete",
            kept=len(sentences),
            removed=removed,
            threshold=dedup_threshold,
        )

        if not sentences:
            logger.warning("pipeline.exhausted-after-dedup")
            return response

        classifier_stats = request.metadata.classifier if request.metadata else None
        cluster_params = self._compute_cluster_params(
            len(sentences), request.constraints, classifier_stats
        )
        logger.info(
            "pipeline.cluster.start",
            sentence_count=len(sentences),
            min_cluster_size=cluster_params[0],
            min_samples=cluster_params[1],
        )
        cluster_start = time.perf_counter()
        base_mcs, base_ms = cluster_params

        # Define search range around the heuristic base
        mcs_range = range(
            max(2, base_mcs - self.settings.clustering_search_range_mcs_window_lower),
            base_mcs + self.settings.clustering_search_range_mcs_window_upper,
        )
        ms_range = range(1, self.settings.clustering_search_range_ms_max)

        if request.genre == "other":
            token_counts = np.array([s.tokens_estimate for s in sentences], dtype=int)
            cluster_result = self.clusterer.subcluster_other(embeddings, token_counts=token_counts)
        else:
            # Plan-based parameter selection
            count = len(sentences)
            if count < 10:
                # Small: No dim reduction (implicit if n_neighbors=None?), min_cluster_size 3-5
                # The prompt said "No dim reduction". optimize_clustering with umap_n_neighbors_range=None uses default [None] -> No UMAP.
                mcs_range = [3, 4, 5]
                ms_range = [1, 2, 3] # Heuristic
                umap_range = None
            elif count < 50:
                # Medium: n_neighbors 15-30, min_cluster_size 5-10
                mcs_range = [5, 7, 10]
                ms_range = [None, 3, 5]
                umap_range = [15, 30]
            else:
                # Large: n_neighbors 30-50, min_cluster_size 10-20
                mcs_range = [10, 15, 20]
                ms_range = [None, 5]
                umap_range = [30, 50]

            cluster_result = self.clusterer.optimize_clustering(
                embeddings,
                min_cluster_size_range=mcs_range,
                min_samples_range=ms_range,
                umap_n_neighbors_range=umap_range if self.settings.enable_umap_auto else None,
                token_counts=np.array([s.tokens_estimate for s in sentences], dtype=int),
            )

        hdbscan_duration = time.perf_counter() - cluster_start
        HDBSCAN_SECONDS.observe(hdbscan_duration)
        response.diagnostics.hdbscan_ms = hdbscan_duration * 1000

        # Merge excessive clusters if > 10
        if cluster_result.labels.size > 0:
            cluster_result.labels = self._merge_excessive_clusters(
                cluster_result.labels, embeddings, max_clusters=10
            )

        if cluster_result.labels.size > 0:
            noise = int((cluster_result.labels < 0).sum())
            response.diagnostics.noise_ratio = noise / float(cluster_result.labels.size)

        unique_labels, cluster_indices = self._group_clusters(cluster_result.labels)
        logger.info(
            "pipeline.cluster.complete",
            cluster_count=len(unique_labels),
            hdbscan_ms=response.diagnostics.hdbscan_ms,
            noise_ratio=response.diagnostics.noise_ratio,
        )
        corpora = ["\n".join(sentences[idx].text for idx in indices) for indices in cluster_indices]
        logger.info("pipeline.topics.start", corpora=len(corpora))
        top_terms = self._compute_topics(corpora)
        logger.info("pipeline.topics.complete", corpora=len(corpora))
        clusters, budget_tokens, rep_indices = self._build_clusters(
            sentences,
            embeddings,
            unique_labels,
            cluster_indices,
            top_terms,
            request.constraints.max_sentences_per_cluster,
            request.constraints.mmr_lambda,
        )

        # Hierarchical Summarization: Select genre-level highlights from cluster representatives
        genre_highlights = self._select_genre_highlights(
            sentences, embeddings, rep_indices, request.constraints.mmr_lambda
        )

        response.clusters = clusters
        response.evidence_budget = EvidenceBudget(
            sentences=sum(len(cluster.representatives) for cluster in clusters),
            tokens_estimated=budget_tokens,
        )
        response.genre_highlights = genre_highlights
        response.diagnostics.dedup_pairs = removed
        response.diagnostics.umap_used = cluster_result.used_umap
        response.diagnostics.hdbscan = cluster_result.params
        response.diagnostics.dbcv_score = cluster_result.dbcv_score
        response.diagnostics.silhouette_score = cluster_result.silhouette_score
        logger.info(
            "pipeline.clusters.built",
            clusters=len(response.clusters),
            representatives=response.evidence_budget.sentences,
            budget_tokens=budget_tokens,
        )

        REQUEST_PROCESS_SECONDS.observe(time.perf_counter() - start_time)
        logger.info(
            "pipeline.success",
            clusters=len(response.clusters),
            representatives=response.evidence_budget.sentences,
            dedup_removed=removed,
        )
        return response

    def _merge_excessive_clusters(
        self, labels: np.ndarray, embeddings: np.ndarray, max_clusters: int
    ) -> np.ndarray:
        """
        Merge excessive clusters using Ward hierarchical clustering.
        Uses scipy.cluster.hierarchy to merge clusters until count <= max_clusters.
        """
        from scipy.cluster.hierarchy import linkage, fcluster

        unique_labels = sorted(set(labels))
        unique_labels = [lbl for lbl in unique_labels if lbl != -1]  # Ignore noise

        # Early return if already within limit or too few clusters
        if len(unique_labels) <= max_clusters or len(unique_labels) < 2:
            return labels

        # Calculate cluster centroids (Ward method uses Euclidean distance)
        centroids = []
        label_to_index = {}
        for idx, lbl in enumerate(unique_labels):
            mask = labels == lbl
            centroids.append(embeddings[mask].mean(axis=0))
            label_to_index[lbl] = idx

        # Convert to numpy array for linkage
        centroid_matrix = np.stack(centroids)

        # Perform Ward hierarchical clustering
        # Ward method minimizes within-cluster variance increase
        Z = linkage(centroid_matrix, method='ward')

        # Cut tree to get max_clusters clusters
        # fcluster returns cluster assignments (1-indexed)
        cluster_assignments = fcluster(Z, t=max_clusters, criterion='maxclust')

        # Map cluster assignments back to original labels
        # Create mapping: original_label -> new_label
        # Use the first label in each new cluster as the representative
        new_label_map = {}
        for orig_idx, new_cluster_id in enumerate(cluster_assignments):
            orig_label = unique_labels[orig_idx]
            # Find the first label assigned to this cluster to use as representative
            if new_cluster_id not in new_label_map:
                new_label_map[new_cluster_id] = orig_label
            # All labels in the same cluster get the same representative label
            target_label = new_label_map[new_cluster_id]
            if target_label != orig_label:
                labels[labels == orig_label] = target_label

        return labels

    def warmup(self, samples: Sequence[str] | None = None) -> WarmupResponse:
        examples = list(samples or ["Warmup sentence."])
        batches = math.ceil(len(examples) / max(1, self.settings.batch_size))
        processed = self.embedder.warmup(examples)
        return WarmupResponse(
            warmed=processed > 0, batches=batches, backend=self.embedder.config.backend
        )

    def _extract_sentences(self, request: EvidenceRequest) -> list[SentenceRecord]:
        sentences: list[SentenceRecord] = []
        tokens_budget = request.constraints.max_tokens_budget
        accumulated_tokens = 0
        total_documents = len(request.documents)
        classifier_stats = request.metadata.classifier if request.metadata else None
        allow_low_confidence_filter = bool(
            classifier_stats and classifier_stats.coverage_ratio >= 0.5
        )
        filter_low_confidence = (
            allow_low_confidence_filter and total_documents > _MIN_DOCUMENTS_PER_GENRE
        )
        skipped_low_confidence = 0
        for document in request.documents:
            confidence = getattr(document, "confidence", None)
            if (
                filter_low_confidence
                and confidence is not None
                and confidence < 0.3
                and (total_documents - (skipped_low_confidence + 1)) >= _MIN_DOCUMENTS_PER_GENRE
            ):
                skipped_low_confidence += 1
                continue
            sentence_counter = 0
            for paragraph_idx, paragraph in enumerate(document.paragraphs):
                for sentence in self._split_paragraph(paragraph):
                    if len(sentence) < 2:
                        continue
                    estimate = self._estimate_tokens(sentence)
                    if accumulated_tokens + estimate > tokens_budget:
                        return sentences
                    if sentence_counter >= self.settings.max_sentences_per_doc:
                        break
                    sentences.append(
                        SentenceRecord(
                            text=sentence,
                            article_id=document.article_id,
                            url=document.source_url if document.source_url else None,
                            paragraph_idx=paragraph_idx,
                            sentence_idx=sentence_counter,
                            lang=document.lang_hint,
                            tokens_estimate=estimate,
                        )
                    )
                    sentence_counter += 1
                    accumulated_tokens += estimate
                if sentence_counter >= self.settings.max_sentences_per_doc:
                    break
        if skipped_low_confidence:
            _LOGGER.debug(
                "pipeline.documents.skipped_low_confidence",
                skipped=skipped_low_confidence,
            )
        return sentences

    def _split_paragraph(self, paragraph: str) -> list[str]:
        # Apply text normalization before sentence splitting
        normalized = normalize_text(
            paragraph,
            enable_sudachi=self.settings.enable_sudachi_preprocessing
        )
        stripped = normalized.strip()
        if not stripped:
            return []
        if "\n" in stripped:
            sentences = [part.strip() for part in stripped.splitlines() if part.strip()]
        else:
            sentences = [
                part.strip() for part in _SENTENCE_SPLIT_RE.split(stripped) if part.strip()
            ]
        return sentences or [stripped]

    def _estimate_tokens(self, sentence: str) -> int:
        return max(1, math.ceil(len(sentence) / 4))

    def _compute_cluster_params(
        self,
        sentence_count: int,
        constraints,
        classifier_stats: Optional[CorpusClassifierStats] = None,
    ) -> tuple[int, int]:
        base = max(2, sentence_count // 20)
        min_cluster_size = constraints.hdbscan_min_cluster_size or base
        min_cluster_size = max(2, min_cluster_size)
        min_samples = constraints.hdbscan_min_samples or max(1, min_cluster_size // 2)
        if classifier_stats:
            if classifier_stats.avg_confidence < 0.35:
                inferred = max(2, sentence_count // 12)
                min_cluster_size = max(min_cluster_size, inferred)
                min_samples = max(1, min_cluster_size // 2)
            elif classifier_stats.avg_confidence > 0.75 and classifier_stats.coverage_ratio > 0.6:
                min_cluster_size = max(2, int(float(min_cluster_size) * 0.75))
                min_samples = max(1, min_samples - 1)
        return min_cluster_size, min_samples

    def _group_clusters(self, labels: np.ndarray) -> tuple[list[int], list[list[int]]]:
        unique_labels = sorted(set(int(label) for label in labels))
        cluster_sentences = [
            [idx for idx, label in enumerate(labels) if int(label) == cluster_id]
            for cluster_id in unique_labels
        ]
        return unique_labels, cluster_sentences

    def _compute_topics(self, corpora: list[str]) -> list[list[str]]:
        if not corpora:
            return []
        if self.process_pool:
            future = self.process_pool.submit(
                topics.extract_topics, corpora, 5, bm25_weighting=True
            )
            return future.result()
        return topics.extract_topics(corpora, bm25_weighting=True)

    def _avg_pairwise_cosine_sim(self, cluster_embeddings: np.ndarray) -> Optional[float]:
        """Calculate average pairwise cosine similarity for cluster embeddings.

        Args:
            cluster_embeddings: Normalized embedding vectors (N, D)

        Returns:
            Average pairwise cosine similarity, or None if < 2 vectors
        """
        if cluster_embeddings.shape[0] < 2:
            return None

        # Embeddings are normalized, so dot product is cosine similarity
        sim_matrix = cluster_embeddings @ cluster_embeddings.T
        numerator = float(sim_matrix.sum() - cluster_embeddings.shape[0])
        denominator = cluster_embeddings.shape[0] * (cluster_embeddings.shape[0] - 1)

        if denominator <= 0:
            return None

        avg_sim = numerator / denominator
        # Clip to valid cosine similarity range [-1, 1]
        return min(1.0, max(-1.0, avg_sim))

    def _lambda_from_avg_sim(self, avg_sim: Optional[float], base_lambda: float) -> float:
        """Calculate adaptive lambda parameter from average similarity.

        Formula: lambda = 0.5 + 0.3 * (1 - avg_sim)
        - High avg_sim (homogeneous cluster) -> lower lambda (more diversity)
        - Low avg_sim (diverse cluster) -> higher lambda (more relevance)

        Args:
            avg_sim: Average pairwise cosine similarity, or None
            base_lambda: Fallback lambda when avg_sim is None

        Returns:
            Lambda parameter in [0.0, 1.0]
        """
        if avg_sim is None:
            return base_lambda

        lambda_param = 0.5 + 0.3 * (1.0 - avg_sim)
        # Clip to valid range
        return min(1.0, max(0.0, lambda_param))

    def _is_valid_representative_text(self, text: str) -> bool:
        """
        Check if text is valid for use as a representative sentence.

        Filters out:
        - Text shorter than 20 characters (API schema requirement)
        - Stack traces and code fragments (heuristic detection)

        Args:
            text: Text to validate

        Returns:
            True if text is valid for representative sentence, False otherwise
        """
        if not text or len(text) < 20:
            return False

        text_lower = text.lower()

        # Check for stack trace / code fragment indicators
        stack_trace_keywords = [
            "stacktrace",
            "traceback",
            "exception",
            'file "',
            "line ",
            "at ",
            "error:",
            "error ",
        ]
        if any(keyword in text_lower for keyword in stack_trace_keywords):
            return False

        # Check for excessive code-like symbols (brackets, braces, semicolons, etc.)
        code_symbols = set("{}[]();=<>")
        symbol_count = sum(1 for char in text if char in code_symbols)
        if len(text) > 0 and symbol_count / len(text) > 0.3:
            return False

        # Check for consecutive code-like patterns (e.g., "}){", "});", etc.)
        code_patterns = [r"\)\s*\{", r"\}\s*\)", r"\}\s*;", r"\)\s*;", r"\{\s*\}"]
        for pattern in code_patterns:
            if re.search(pattern, text):
                return False

        return True

    def _build_clusters(
        self,
        sentences: Sequence[SentenceRecord],
        embeddings: np.ndarray,
        unique_labels: list[int],
        cluster_indices: list[list[int]],
        top_terms: list[list[str]],
        max_sentences_per_cluster: int,
        mmr_lambda: float,
    ) -> tuple[list[EvidenceCluster], int, list[int]]:
        clusters: list[EvidenceCluster] = []
        budget_tokens = 0
        used_articles: set[str] = set()
        all_rep_indices: list[int] = []
        for cluster_offset, (cluster_id, indices) in enumerate(zip(unique_labels, cluster_indices)):
            cluster_embeddings = embeddings[indices]

            # Calculate avg_sim first, then derive adaptive lambda
            avg_sim = self._avg_pairwise_cosine_sim(cluster_embeddings)
            lambda_cluster = self._lambda_from_avg_sim(avg_sim, mmr_lambda)

            selected_local = selectors.mmr_select(
                cluster_embeddings, k=max_sentences_per_cluster, lambda_param=lambda_cluster
            )
            MMR_SELECTED.inc(len(selected_local))
            representatives: list[RepresentativeSentence] = []
            cluster_article_ids: list[str] = []

            def _push_sentence(sentence_idx: int, *, allow_reuse: bool = False) -> int:
                sentence = sentences[sentence_idx]
                if not allow_reuse and sentence.article_id in used_articles:
                    return 0
                # Filter out invalid representative text (too short, code fragments, stack traces)
                if not self._is_valid_representative_text(sentence.text):
                    return 0
                pos = len(representatives)
                representatives.append(
                    RepresentativeSentence(
                        text=sentence.text,
                        lang=sentence.lang,
                        embedding_ref=f"e/{cluster_id}/{pos}",
                        reasons=["centrality", "mmr-diversity"],
                        source=RepresentativeSource(
                            source_id=sentence.article_id,
                            url=sentence.url,
                            paragraph_idx=sentence.paragraph_idx,
                        ),
                    )
                )
                cluster_article_ids.append(sentence.article_id)
                if not allow_reuse:
                    used_articles.add(sentence.article_id)
                return sentence.tokens_estimate

            for local_idx in selected_local:
                sentence_idx = indices[local_idx]
                tokens_added = _push_sentence(sentence_idx)
                if tokens_added:
                    budget_tokens += tokens_added
                    all_rep_indices.append(sentence_idx)
                    if len(representatives) >= max_sentences_per_cluster:
                        break

                for sentence_idx in indices:
                    tokens_added = _push_sentence(sentence_idx)
                    if tokens_added:
                        budget_tokens += tokens_added
                        all_rep_indices.append(sentence_idx)
                        if len(representatives) >= max_sentences_per_cluster:
                            break

            if not representatives and indices:
                # Fallback: try to find any valid sentence from the cluster
                for fallback_idx in indices:
                    tokens_added = _push_sentence(fallback_idx, allow_reuse=True)
                    if tokens_added:
                        budget_tokens += tokens_added
                        all_rep_indices.append(fallback_idx)
                        break

            # avg_sim was already calculated above for lambda adjustment
            # Reuse it here for ClusterStats
            label_terms = top_terms[cluster_offset] if cluster_offset < len(top_terms) else []
            clusters.append(
                EvidenceCluster(
                    cluster_id=int(cluster_id),
                    size=len(indices),
                    label=ClusterLabel(top_terms=label_terms),
                    representatives=representatives,
                    supporting_ids=self._dedup_preserve_order(cluster_article_ids),
                    stats=ClusterStats(
                        avg_sim=avg_sim,
                        token_count=sum(sentences[idx].tokens_estimate for idx in indices),
                    ),
                )
            )
        return clusters, budget_tokens, all_rep_indices

    def _select_genre_highlights(
        self,
        sentences: Sequence[SentenceRecord],
        embeddings: np.ndarray,
        rep_indices: list[int],
        mmr_lambda: float
    ) -> list[RepresentativeSentence]:
        """Select top sentences across all clusters for a genre-level summary."""
        if not rep_indices:
            return []

        rep_embeddings = embeddings[rep_indices]

        # Calculate avg_sim for representative pool and derive adaptive lambda
        avg_sim = self._avg_pairwise_cosine_sim(rep_embeddings)
        lambda_genre = self._lambda_from_avg_sim(avg_sim, mmr_lambda)

        # Apply MMR on the pool of representatives with adaptive lambda
        selected_local_indices = selectors.mmr_select(
            rep_embeddings,
            k=self.settings.max_genre_sentences,
            lambda_param=lambda_genre
        )

        highlights: list[RepresentativeSentence] = []
        for local_idx in selected_local_indices:
            original_idx = rep_indices[local_idx]
            sentence = sentences[original_idx]
            # Filter out invalid representative text (too short, code fragments, stack traces)
            if not self._is_valid_representative_text(sentence.text):
                continue
            highlights.append(
                RepresentativeSentence(
                    text=sentence.text,
                    lang=sentence.lang,
                    embedding_ref=None, # Not inside a cluster
                    reasons=["genre-highlight", "mmr-diversity"],
                    source=RepresentativeSource(
                        source_id=sentence.article_id,
                        url=sentence.url,
                        paragraph_idx=sentence.paragraph_idx,
                    ),
                )
            )
        return highlights


    def _dedup_preserve_order(self, values: list[str]) -> list[str]:
        seen: set[str] = set()
        ordered: list[str] = []
        for value in values:
            if value in seen:
                continue
            seen.add(value)
            ordered.append(value)
        return ordered

    def _adjust_dedup_threshold(
        self,
        base_threshold: float,
        metadata: Optional[CorpusMetadata],
        genre: str,
    ) -> float:
        """
        Adjust deduplication threshold based on genre-specific settings and classifier statistics.

        Priority order:
        1. Request base_threshold (from EvidenceRequest.constraints.dedup_threshold)
        2. Genre-specific override from Settings.genre_dedup_thresholds (if exists)
        3. Classifier-based adjustment (existing logic)
        4. Clamp to [0.0, 0.99]

        Args:
            base_threshold: Base threshold from request constraints
            metadata: Optional corpus metadata with classifier statistics
            genre: Genre name for lookup in genre_dedup_thresholds

        Returns:
            Adjusted threshold value in [0.0, 0.99]
        """
        threshold = base_threshold

        # Step 2: Apply genre-specific override if available
        genre_thresholds = self.settings.genre_dedup_thresholds_dict
        if genre in genre_thresholds:
            threshold = genre_thresholds[genre]
            _LOGGER.debug(
                "Using genre-specific dedup threshold",
                genre=genre,
                threshold=threshold,
                base_threshold=base_threshold,
            )

        # Step 3: Apply classifier-based adjustment (only if genre override didn't apply)
        if metadata and metadata.classifier and genre not in genre_thresholds:
            stats = metadata.classifier
            if stats.avg_confidence < 0.35:
                threshold = min(0.97, threshold + 0.03)
            elif stats.avg_confidence > 0.75 and stats.coverage_ratio > 0.6:
                threshold = max(0.82, threshold - 0.04)

        # Step 4: Clamp to valid range
        return float(min(max(threshold, 0.0), 0.99))
