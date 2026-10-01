"""Tests verifying route handlers offload blocking calls off the event loop thread."""

from __future__ import annotations

import threading
from unittest.mock import MagicMock

import pytest

from recap_subworker.app.routers import admin, evidence
from recap_subworker.domain.models import (
    EvidenceBudget,
    EvidenceResponse,
    WarmupResponse,
)
from tests.conftest import make_evidence_request


@pytest.mark.asyncio
async def test_evidence_cluster_runs_off_event_loop_thread():
    """In inprocess mode (runner=None), pipeline.run must execute on a worker thread."""
    loop_thread_id = threading.get_ident()
    called_thread_id = None

    fake_pipeline = MagicMock()

    def fake_run(payload):
        nonlocal called_thread_id
        called_thread_id = threading.get_ident()
        return EvidenceResponse(
            job_id=payload.job_id,
            genre=payload.genre,
            clusters=[],
            evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
        )

    fake_pipeline.run.side_effect = fake_run

    payload = make_evidence_request(n_docs=2)

    result = await evidence.cluster_evidence(
        payload=payload,
        pipeline=fake_pipeline,
        runner=None,
    )

    assert result is not None
    assert called_thread_id is not None
    assert called_thread_id != loop_thread_id, (
        f"pipeline.run ran on event loop thread ({called_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )


@pytest.mark.asyncio
async def test_admin_warmup_runs_off_event_loop_thread():
    """In inprocess mode (runner=None), pipeline.warmup must execute on a worker thread."""
    loop_thread_id = threading.get_ident()
    called_thread_id = None

    fake_pipeline = MagicMock()

    def fake_warmup(samples=None):
        nonlocal called_thread_id
        called_thread_id = threading.get_ident()
        return WarmupResponse(
            warmed=True,
            batches=1,
            backend="test",
        )

    fake_pipeline.warmup.side_effect = fake_warmup

    result = await admin.warmup(
        pipeline=fake_pipeline,
        runner=None,
    )

    assert result is not None
    assert called_thread_id is not None
    assert called_thread_id != loop_thread_id, (
        f"pipeline.warmup ran on event loop thread ({called_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )
