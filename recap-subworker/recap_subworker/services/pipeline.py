"""Evidence pipeline orchestrating preprocessing, clustering, and selection."""

from __future__ import annotations

import math
import re
import time
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
_MIN_DOCUMENTS_PER_GENRE = 10


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
            request.constraints.dedup_threshold, request.metadata
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
        cluster_result = self.clusterer.cluster(
            embeddings,
            min_cluster_size=cluster_params[0],
            min_samples=cluster_params[1],
        )
        hdbscan_duration = time.perf_counter() - cluster_start
        HDBSCAN_SECONDS.observe(hdbscan_duration)
        response.diagnostics.hdbscan_ms = hdbscan_duration * 1000

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
        clusters, budget_tokens = self._build_clusters(
            sentences,
            embeddings,
            unique_labels,
            cluster_indices,
            top_terms,
            request.constraints.max_sentences_per_cluster,
            request.constraints.mmr_lambda,
        )

        response.clusters = clusters
        response.evidence_budget = EvidenceBudget(
            sentences=sum(len(cluster.representatives) for cluster in clusters),
            tokens_estimated=budget_tokens,
        )
        response.diagnostics.dedup_pairs = removed
        response.diagnostics.umap_used = cluster_result.used_umap
        response.diagnostics.hdbscan = cluster_result.params
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
        stripped = paragraph.strip()
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

    def _build_clusters(
        self,
        sentences: Sequence[SentenceRecord],
        embeddings: np.ndarray,
        unique_labels: list[int],
        cluster_indices: list[list[int]],
        top_terms: list[list[str]],
        max_sentences_per_cluster: int,
        mmr_lambda: float,
    ) -> tuple[list[EvidenceCluster], int]:
        clusters: list[EvidenceCluster] = []
        budget_tokens = 0
        for cluster_offset, (cluster_id, indices) in enumerate(zip(unique_labels, cluster_indices)):
            cluster_embeddings = embeddings[indices]
            selected_local = selectors.mmr_select(
                cluster_embeddings, k=max_sentences_per_cluster, lambda_param=mmr_lambda
            )
            MMR_SELECTED.inc(len(selected_local))
            representatives = []
            for pos, local_idx in enumerate(selected_local):
                sentence_idx = indices[local_idx]
                sentence = sentences[sentence_idx]
                budget_tokens += sentence.tokens_estimate
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
            avg_sim = None
            if cluster_embeddings.shape[0] > 1:
                sim_matrix = cluster_embeddings @ cluster_embeddings.T
                numerator = float(sim_matrix.sum() - cluster_embeddings.shape[0])
                denominator = cluster_embeddings.shape[0] * (cluster_embeddings.shape[0] - 1)
                if denominator > 0:
                    avg_sim = numerator / denominator

            label_terms = top_terms[cluster_offset] if cluster_offset < len(top_terms) else []
            clusters.append(
                EvidenceCluster(
                    cluster_id=int(cluster_id),
                    size=len(indices),
                    label=ClusterLabel(top_terms=label_terms),
                    representatives=representatives,
                    supporting_ids=sorted({sentences[idx].article_id for idx in indices}),
                    stats=ClusterStats(
                        avg_sim=avg_sim,
                        token_count=sum(sentences[idx].tokens_estimate for idx in indices),
                    ),
                )
            )
        return clusters, budget_tokens

    def _adjust_dedup_threshold(
        self,
        base_threshold: float,
        metadata: Optional[CorpusMetadata],
    ) -> float:
        threshold = base_threshold
        if metadata and metadata.classifier:
            stats = metadata.classifier
            if stats.avg_confidence < 0.35:
                threshold = min(0.97, threshold + 0.03)
            elif stats.avg_confidence > 0.75 and stats.coverage_ratio > 0.6:
                threshold = max(0.82, threshold - 0.04)
        return float(min(max(threshold, 0.0), 0.99))
