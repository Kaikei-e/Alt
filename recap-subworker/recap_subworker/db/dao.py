"""Data access helpers for recap-subworker persistence."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Iterable, Optional
from uuid import UUID

from sqlalchemy import (
    BigInteger,
    Column,
    DateTime,
    Float,
    Integer,
    MetaData,
    SmallInteger,
    String,
    Table,
    Text,
    insert,
    select,
    text,
    update,
)
from sqlalchemy.dialects.postgresql import JSONB, UUID as PG_UUID, insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid import uuid4


metadata = MetaData()

runs_table = Table(
    "recap_subworker_runs",
    metadata,
    Column("id", BigInteger, primary_key=True),
    Column("job_id", PG_UUID(as_uuid=True), nullable=False),
    Column("genre", Text, nullable=False),
    Column("status", Text, nullable=False),
    Column("cluster_count", Integer, nullable=False, default=0),
    Column("request_payload", JSONB, nullable=False),
    Column("response_payload", JSONB),
    Column("error_message", Text),
)

clusters_table = Table(
    "recap_subworker_clusters",
    metadata,
    Column("id", BigInteger, primary_key=True),
    Column("run_id", BigInteger, nullable=False),
    Column("cluster_id", Integer, nullable=False),
    Column("size", Integer, nullable=False),
    Column("label", Text),
    Column("top_terms", JSONB, nullable=False),
    Column("stats", JSONB, nullable=False),
)

sentences_table = Table(
    "recap_subworker_sentences",
    metadata,
    Column("id", BigInteger, primary_key=True),
    Column("cluster_row_id", BigInteger, nullable=False),
    Column("source_article_id", Text, nullable=False),
    Column("paragraph_idx", Integer),
    Column("sentence_id", Integer, nullable=False, default=0),
    Column("sentence_text", Text, nullable=False),
    Column("lang", String(8), nullable=False, default="unknown"),
    Column("score", Float, nullable=False, default=0.0),
)

diagnostics_table = Table(
    "recap_subworker_diagnostics",
    metadata,
    Column("run_id", BigInteger, primary_key=True),
    Column("metric", Text, primary_key=True),
    Column("value", JSONB, nullable=False),
)

cluster_evidence_table = Table(
    "recap_cluster_evidence",
    metadata,
    Column("id", BigInteger, primary_key=True),
    Column("cluster_row_id", BigInteger, nullable=False),
    Column("article_id", Text, nullable=False),
    Column("title", Text),
    Column("source_url", Text),
    Column("published_at", DateTime(timezone=True)),
    Column("lang", String(8)),
    Column("rank", SmallInteger, nullable=False, default=0),
    Column("created_at", DateTime(timezone=True), nullable=False, server_default=text("now()")),
)

# Genre evaluation tables (shared with recap-worker)
genre_evaluation_runs_table = Table(
    "recap_genre_evaluation_runs",
    metadata,
    Column("run_id", PG_UUID(as_uuid=True), primary_key=True),
    Column("dataset_path", Text, nullable=False),
    Column("total_items", Integer, nullable=False),
    Column("macro_precision", Float, nullable=False),
    Column("macro_recall", Float, nullable=False),
    Column("macro_f1", Float, nullable=False),
    Column("summary_tp", Integer, nullable=False),
    Column("summary_fp", Integer, nullable=False),
    Column("summary_fn", Integer, nullable=False),
    Column("micro_precision", Float),
    Column("micro_recall", Float),
    Column("micro_f1", Float),
    Column("weighted_f1", Float),
    Column("macro_f1_valid", Float),
    Column("valid_genre_count", Integer),
    Column("undefined_genre_count", Integer),
    Column("created_at", DateTime(timezone=True), nullable=False, server_default=text("now()")),
)

genre_evaluation_metrics_table = Table(
    "recap_genre_evaluation_metrics",
    metadata,
    Column("run_id", PG_UUID(as_uuid=True), primary_key=True),
    Column("genre", Text, primary_key=True),
    Column("tp", Integer, nullable=False),
    Column("fp", Integer, nullable=False),
    Column("fn_count", Integer, nullable=False),
    Column("precision", Float, nullable=False),
    Column("recall", Float, nullable=False),
    Column("f1_score", Float, nullable=False),
)


@dataclass(slots=True)
class NewRun:
    job_id: UUID
    genre: str
    status: str
    request_payload: dict[str, Any]


@dataclass(slots=True)
class RunRecord:
    run_id: int
    job_id: UUID
    genre: str
    status: str
    cluster_count: int
    request_payload: dict[str, Any]
    response_payload: Optional[dict[str, Any]]
    error_message: Optional[str]


@dataclass(slots=True)
class PersistedSentence:
    article_id: str
    paragraph_idx: Optional[int]
    sentence_id: int
    sentence_text: str
    lang: str
    score: float


@dataclass(slots=True)
class PersistedCluster:
    cluster_id: int
    size: int
    label: Optional[str]
    top_terms: list[str]
    stats: dict[str, Any]
    sentences: list[PersistedSentence]
    evidence: list["PersistedEvidence"] = field(default_factory=list)


@dataclass(slots=True)
class PersistedEvidence:
    article_id: str
    title: Optional[str]
    source_url: Optional[str]
    published_at: Optional[datetime]
    lang: Optional[str]
    rank: int


@dataclass(slots=True)
class DiagnosticEntry:
    metric: str
    value: Any


class SubworkerDAO:
    """DAO encapsulating recap-subworker persistence logic."""

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def insert_run(self, run: NewRun) -> int:
        stmt = (
            insert(runs_table)
            .values(
                job_id=run.job_id,
                genre=run.genre,
                status=run.status,
                request_payload=run.request_payload,
            )
            .returning(runs_table.c.id)
        )
        result = await self.session.execute(stmt)
        return int(result.scalar_one())

    async def find_run_by_idempotency(
        self, job_id: UUID, genre: str, idempotency_key: str
    ) -> Optional[RunRecord]:
        request_json = runs_table.c.request_payload
        stmt = (
            select(
                runs_table.c.id,
                runs_table.c.job_id,
                runs_table.c.genre,
                runs_table.c.status,
                runs_table.c.cluster_count,
                runs_table.c.request_payload,
                runs_table.c.response_payload,
                runs_table.c.error_message,
            )
            .where(runs_table.c.job_id == job_id)
            .where(runs_table.c.genre == genre)
            .where(request_json["idempotency_key"].astext == idempotency_key)
            .limit(1)
        )
        result = await self.session.execute(stmt)
        row = result.first()
        if not row:
            return None
        return RunRecord(
            run_id=row.id,
            job_id=row.job_id,
            genre=row.genre,
            status=row.status,
            cluster_count=row.cluster_count,
            request_payload=row.request_payload or {},
            response_payload=row.response_payload,
            error_message=row.error_message,
        )

    async def has_running_run(self, job_id: UUID, genre: str) -> bool:
        stmt = (
            select(runs_table.c.id)
            .where(runs_table.c.job_id == job_id)
            .where(runs_table.c.genre == genre)
            .where(runs_table.c.status == "running")
            .limit(1)
        )
        result = await self.session.execute(stmt)
        return result.first() is not None

    async def mark_run_success(
        self,
        run_id: int,
        cluster_count: int,
        response_payload: dict[str, Any],
        status: str = "succeeded",
    ) -> None:
        stmt = (
            update(runs_table)
            .where(runs_table.c.id == run_id)
            .values(
                status=status,
                cluster_count=cluster_count,
                response_payload=response_payload,
                error_message=None,
            )
        )
        await self.session.execute(stmt)

    async def mark_run_failure(self, run_id: int, status: str, error_message: str) -> None:
        stmt = (
            update(runs_table)
            .where(runs_table.c.id == run_id)
            .values(status=status, error_message=error_message)
        )
        await self.session.execute(stmt)

    async def insert_clusters(self, run_id: int, clusters: Iterable[PersistedCluster]) -> None:
        for cluster in clusters:
            stmt = (
                insert(clusters_table)
                .values(
                    run_id=run_id,
                    cluster_id=cluster.cluster_id,
                    size=cluster.size,
                    label=cluster.label,
                    top_terms=cluster.top_terms,
                    stats=cluster.stats,
                )
                .returning(clusters_table.c.id)
            )
            result = await self.session.execute(stmt)
            cluster_row_id = int(result.scalar_one())
            if cluster.sentences:
                sentence_rows = [
                    {
                        "cluster_row_id": cluster_row_id,
                        "source_article_id": sentence.article_id,
                        "paragraph_idx": sentence.paragraph_idx,
                        "sentence_id": sentence.sentence_id,
                        "sentence_text": sentence.sentence_text,
                        "lang": sentence.lang,
                        "score": sentence.score,
                    }
                    for sentence in cluster.sentences
                ]
                sentence_insert = pg_insert(sentences_table).values(sentence_rows)
                on_conflict = sentence_insert.on_conflict_do_update(
                    index_elements=[
                        sentences_table.c.cluster_row_id,
                        sentences_table.c.source_article_id,
                        sentences_table.c.sentence_id,
                    ],
                    set_={
                        "sentence_text": sentence_insert.excluded.sentence_text,
                        "lang": sentence_insert.excluded.lang,
                        "score": sentence_insert.excluded.score,
                        "paragraph_idx": sentence_insert.excluded.paragraph_idx,
                    },
                )
                await self.session.execute(on_conflict)

            if cluster.evidence:
                evidence_rows = [
                    {
                        "cluster_row_id": cluster_row_id,
                        "article_id": evidence.article_id,
                        "title": evidence.title,
                        "source_url": evidence.source_url,
                        "published_at": evidence.published_at,
                        "lang": evidence.lang,
                        "rank": evidence.rank,
                    }
                    for evidence in cluster.evidence
                ]
                evidence_insert = pg_insert(cluster_evidence_table).values(evidence_rows)
                evidence_conflict = evidence_insert.on_conflict_do_update(
                    index_elements=[
                        cluster_evidence_table.c.cluster_row_id,
                        cluster_evidence_table.c.article_id,
                    ],
                    set_={
                        "title": evidence_insert.excluded.title,
                        "source_url": evidence_insert.excluded.source_url,
                        "published_at": evidence_insert.excluded.published_at,
                        "lang": evidence_insert.excluded.lang,
                        "rank": evidence_insert.excluded.rank,
                    },
                )
                await self.session.execute(evidence_conflict)

    async def upsert_diagnostics(self, run_id: int, entries: Iterable[DiagnosticEntry]) -> None:
        for entry in entries:
            stmt = pg_insert(diagnostics_table).values(
                run_id=run_id,
                metric=entry.metric,
                value=entry.value,
            )
            on_conflict = stmt.on_conflict_do_update(
                index_elements=[diagnostics_table.c.run_id, diagnostics_table.c.metric],
                set_={"value": entry.value},
            )
            await self.session.execute(on_conflict)

    async def fetch_run(self, run_id: int) -> Optional[RunRecord]:
        stmt = select(
            runs_table.c.id,
            runs_table.c.job_id,
            runs_table.c.genre,
            runs_table.c.status,
            runs_table.c.cluster_count,
            runs_table.c.request_payload,
            runs_table.c.response_payload,
            runs_table.c.error_message,
        ).where(runs_table.c.id == run_id)
        result = await self.session.execute(stmt)
        row = result.first()
        if not row:
            return None
        return RunRecord(
            run_id=row.id,
            job_id=row.job_id,
            genre=row.genre,
            status=row.status,
            cluster_count=row.cluster_count,
            request_payload=row.request_payload or {},
            response_payload=row.response_payload,
            error_message=row.error_message,
        )

    async def save_genre_evaluation(
        self,
        dataset_path: str,
        total_items: int,
        macro_precision: float,
        macro_recall: float,
        macro_f1: float,
        summary_tp: int,
        summary_fp: int,
        summary_fn: int,
        micro_precision: float | None = None,
        micro_recall: float | None = None,
        micro_f1: float | None = None,
        weighted_f1: float | None = None,
        macro_f1_valid: float | None = None,
        valid_genre_count: int | None = None,
        undefined_genre_count: int | None = None,
        per_genre_metrics: list[dict[str, Any]] | None = None,
    ) -> UUID:
        """ジャンル評価結果をデータベースに保存。

        Args:
            dataset_path: Golden datasetのパス
            total_items: 総アイテム数
            macro_precision: Macro Precision
            macro_recall: Macro Recall
            macro_f1: Macro F1
            summary_tp: 総True Positives
            summary_fp: 総False Positives
            summary_fn: 総False Negatives
            micro_precision: Micro Precision（オプション）
            micro_recall: Micro Recall（オプション）
            micro_f1: Micro F1（オプション）
            weighted_f1: Weighted F1（オプション）
            macro_f1_valid: Macro F1 (valid genres only)（オプション）
            valid_genre_count: 有効ジャンル数（オプション）
            undefined_genre_count: 未定義ジャンル数（オプション）
            per_genre_metrics: ジャンル別メトリクスのリスト（オプション）

        Returns:
            生成されたrun_id
        """
        run_id = uuid4()

        # Insert run metadata
        stmt = insert(genre_evaluation_runs_table).values(
            run_id=run_id,
            dataset_path=dataset_path,
            total_items=total_items,
            macro_precision=macro_precision,
            macro_recall=macro_recall,
            macro_f1=macro_f1,
            summary_tp=summary_tp,
            summary_fp=summary_fp,
            summary_fn=summary_fn,
            micro_precision=micro_precision,
            micro_recall=micro_recall,
            micro_f1=micro_f1,
            weighted_f1=weighted_f1,
            macro_f1_valid=macro_f1_valid,
            valid_genre_count=valid_genre_count,
            undefined_genre_count=undefined_genre_count,
        )
        await self.session.execute(stmt)

        # Bulk insert per-genre metrics
        if per_genre_metrics:
            metric_rows = [
                {
                    "run_id": run_id,
                    "genre": metric["genre"],
                    "tp": metric["tp"],
                    "fp": metric["fp"],
                    "fn_count": metric["fn"],
                    "precision": metric["precision"],
                    "recall": metric["recall"],
                    "f1_score": metric["f1"],
                }
                for metric in per_genre_metrics
            ]
            if metric_rows:
                metrics_stmt = insert(genre_evaluation_metrics_table).values(metric_rows)
                await self.session.execute(metrics_stmt)

        # 明示的にコミット（他のDAOメソッドと同様に）
        await self.session.commit()
        return run_id


__all__ = [
    "SubworkerDAO",
    "NewRun",
    "RunRecord",
    "PersistedCluster",
    "PersistedEvidence",
    "PersistedSentence",
    "DiagnosticEntry",
]
