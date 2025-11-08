"""Data access helpers for recap-subworker persistence."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Iterable, Optional
from uuid import UUID

from sqlalchemy import BigInteger, Column, Float, Integer, MetaData, String, Table, Text, insert, select, update
from sqlalchemy.dialects.postgresql import JSONB, UUID as PG_UUID, insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession


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
            if not cluster.sentences:
                continue
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


__all__ = [
    "SubworkerDAO",
    "NewRun",
    "RunRecord",
    "PersistedCluster",
    "PersistedSentence",
    "DiagnosticEntry",
]
