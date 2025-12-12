"""Unit tests for SubworkerDAO."""

from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from recap_subworker.db.dao import (
    DiagnosticEntry,
    NewRun,
    PersistedCluster,
    PersistedSentence,
    SubworkerDAO,
)


@pytest.mark.asyncio
async def test_insert_run_returns_generated_id():
    session = AsyncMock()
    result = MagicMock()
    result.scalar_one.return_value = 99
    session.execute.return_value = result
    dao = SubworkerDAO(session)

    run_id = await dao.insert_run(NewRun(job_id=uuid4(), genre="ai", status="running", request_payload={}))

    assert run_id == 99
    session.execute.assert_awaited()


@pytest.mark.asyncio
async def test_find_run_by_idempotency_returns_record():
    job_id = uuid4()
    session = AsyncMock()
    session.execute.return_value.first.return_value = SimpleNamespace(
        id=1,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"idempotency_key": "abc"},
        response_payload=None,
        error_message=None,
    )
    dao = SubworkerDAO(session)

    record = await dao.find_run_by_idempotency(job_id, "ai", "abc")

    assert record is not None
    assert record.run_id == 1
    session.execute.assert_awaited()


@pytest.mark.asyncio
async def test_insert_clusters_inserts_sentences():
    session = AsyncMock()
    cluster_result = MagicMock()
    cluster_result.scalar_one.return_value = 7
    session.execute.side_effect = [cluster_result, MagicMock()]
    dao = SubworkerDAO(session)
    cluster = PersistedCluster(
        cluster_id=0,
        size=2,
        label="topic",
        top_terms=["ai"],
        stats={"avg_sim": 0.9},
        sentences=[
            PersistedSentence(
                article_id="art-1",
                paragraph_idx=0,
                sentence_id=0,
                sentence_text="Sentence with enough length for storage.",
                lang="ja",
                score=0.8,
            )
        ],
    )

    await dao.insert_clusters(42, [cluster])

    assert session.execute.await_count == 2
    first_stmt = session.execute.await_args_list[0].args[0]
    assert "recap_subworker_clusters" in str(first_stmt)


@pytest.mark.asyncio
async def test_upsert_diagnostics_uses_conflict_update():
    session = AsyncMock()
    dao = SubworkerDAO(session)

    await dao.upsert_diagnostics(1, [DiagnosticEntry(metric="embed_ms", value=1.23)])

    session.execute.assert_awaited()
    stmt = session.execute.await_args_list[0].args[0]
    assert "ON CONFLICT" in str(stmt)


@pytest.mark.asyncio
async def test_upsert_run_diagnostics_uses_conflict_update():
    session = AsyncMock()
    dao = SubworkerDAO(session)

    await dao.upsert_run_diagnostics(
        run_id=42,
        cluster_avg_similarity_mean=0.85,
        cluster_avg_similarity_variance=0.01,
        cluster_avg_similarity_p95=0.92,
        cluster_avg_similarity_max=0.95,
        cluster_count=5,
    )

    session.execute.assert_awaited()
    stmt = session.execute.await_args_list[0].args[0]
    assert "ON CONFLICT" in str(stmt)
    assert "recap_run_diagnostics" in str(stmt)
