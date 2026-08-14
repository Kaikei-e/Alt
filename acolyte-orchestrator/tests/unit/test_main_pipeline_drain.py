"""Shutdown ordering for in-flight report pipelines.

A redeploy is the common case, not the rare one. If lifespan tears the DB pool
down while a StartReportRun pipeline is still inside the graph, the pipeline
sees PoolClosed, its own crash handler then tries to write fail_run through the
same dead pool, and the run row is left saying 'running' forever — wedging
has_active_run (the delete_report guard) and GetReport.active_run for that
report until some later process happens to boot and reconcile it.
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager
from unittest.mock import AsyncMock
from uuid import UUID, uuid4

import pytest
from psycopg_pool import PoolClosed

import main as main_module
from acolyte.domain.run import ReportRun


def _running_run(run_id: UUID) -> ReportRun:
    return ReportRun(run_id=run_id, report_id=uuid4(), target_version_no=3, run_status="running")


def _stub_lifespan_io(monkeypatch: pytest.MonkeyPatch) -> None:
    """Neutralise everything lifespan touches except the drain under test.

    get_run included: the drain reads the run before rewriting it, and the real
    gateway would reach for a pool this test never opened.
    """
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", False)
    monkeypatch.setattr(main_module, "_relay", None)
    monkeypatch.setattr(main_module, "_relay_config", None)
    monkeypatch.setattr(main_module._pool, "open", AsyncMock())
    monkeypatch.setattr(main_module._http_client, "aclose", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "list_running_runs", AsyncMock(return_value=[]))
    monkeypatch.setattr(main_module._job_queue, "get_run", AsyncMock(side_effect=_running_run))


@pytest.mark.asyncio
async def test_shutdown_cancels_in_flight_pipeline_while_the_pool_is_still_open(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    run_id = uuid4()
    pool_closed = asyncio.Event()
    pool_open_at_cancel: list[bool] = []
    started = asyncio.Event()

    async def fake_pipeline() -> None:
        started.set()
        try:
            await asyncio.Event().wait()
        except asyncio.CancelledError:
            pool_open_at_cancel.append(not pool_closed.is_set())
            raise

    async def fake_pool_close() -> None:
        pool_closed.set()

    fail_run = AsyncMock()
    _stub_lifespan_io(monkeypatch)
    monkeypatch.setattr(main_module, "_PIPELINE_DRAIN_GRACE_SECONDS", 0.05)
    monkeypatch.setattr(main_module._pool, "close", fake_pool_close)
    monkeypatch.setattr(main_module._job_queue, "fail_run", fail_run)

    app = main_module.create_app()
    service = app.state.connect_service

    async with app.router.lifespan_context(app):
        task = asyncio.create_task(fake_pipeline(), name=f"acolyte-run-{run_id}")
        service._background_tasks.add(task)
        task.add_done_callback(service._background_tasks.discard)
        await asyncio.wait_for(started.wait(), timeout=1.0)

    try:
        assert task.done(), "lifespan returned with the pipeline task still running"
        assert pool_open_at_cancel == [True], "pipeline was cancelled after the pool had already closed"
        fail_run.assert_awaited_once()
        assert fail_run.await_args is not None
        assert fail_run.await_args.args[0] == run_id
    finally:
        task.cancel()


@pytest.mark.asyncio
async def test_shutdown_lets_an_almost_finished_pipeline_land_its_own_terminal_write(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The grace window exists for the FinalizerNode gap.

    A pipeline cancelled between bump_version and complete_run leaves a bumped
    report version behind a run marked failed. Anything that settles inside the
    window must therefore keep its own outcome and never be re-marked.
    """
    run_id = uuid4()
    completed = asyncio.Event()

    async def fake_pipeline() -> None:
        await asyncio.sleep(0)
        completed.set()

    fail_run = AsyncMock()
    _stub_lifespan_io(monkeypatch)
    monkeypatch.setattr(main_module, "_PIPELINE_DRAIN_GRACE_SECONDS", 1.0)
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "fail_run", fail_run)

    app = main_module.create_app()
    service = app.state.connect_service

    async with app.router.lifespan_context(app):
        task = asyncio.create_task(fake_pipeline(), name=f"acolyte-run-{run_id}")
        service._background_tasks.add(task)
        task.add_done_callback(service._background_tasks.discard)

    assert completed.is_set()
    assert not task.cancelled()
    fail_run.assert_not_awaited()


@pytest.mark.asyncio
async def test_shutdown_does_not_rewrite_a_run_that_already_reached_a_terminal_state(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A cancellation can land after complete_run committed but before the task returns.

    fail_run is an unconditional UPDATE, so re-marking that run would pair a
    bumped report version with a failed run — the exact corruption the drain
    exists to prevent.
    """
    run_id = uuid4()
    started = asyncio.Event()

    async def fake_pipeline() -> None:
        started.set()
        await asyncio.Event().wait()

    fail_run = AsyncMock()
    _stub_lifespan_io(monkeypatch)
    monkeypatch.setattr(main_module, "_PIPELINE_DRAIN_GRACE_SECONDS", 0.05)
    monkeypatch.setattr(main_module._pool, "close", AsyncMock())
    monkeypatch.setattr(main_module._job_queue, "fail_run", fail_run)
    monkeypatch.setattr(
        main_module._job_queue,
        "get_run",
        AsyncMock(
            return_value=ReportRun(
                run_id=run_id,
                report_id=uuid4(),
                target_version_no=3,
                run_status="succeeded",
            )
        ),
    )

    app = main_module.create_app()
    service = app.state.connect_service

    async with app.router.lifespan_context(app):
        task = asyncio.create_task(fake_pipeline(), name=f"acolyte-run-{run_id}")
        service._background_tasks.add(task)
        task.add_done_callback(service._background_tasks.discard)
        await asyncio.wait_for(started.wait(), timeout=1.0)

    try:
        assert task.done()
        fail_run.assert_not_awaited()
    finally:
        task.cancel()


@pytest.mark.asyncio
async def test_shutdown_still_closes_the_pool_when_marking_a_run_failed_blows_up(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Best-effort bookkeeping must never hold the teardown hostage.

    The run bookkeeping talks to the same Postgres that may already be gone —
    which is how this whole failure mode started. If it raises, the pool and the
    HTTP client still have to close, or a redeploy leaks both.
    """
    run_id = uuid4()
    started = asyncio.Event()

    async def fake_pipeline() -> None:
        started.set()
        await asyncio.Event().wait()

    pool_close = AsyncMock()
    _stub_lifespan_io(monkeypatch)
    monkeypatch.setattr(main_module, "_PIPELINE_DRAIN_GRACE_SECONDS", 0.05)
    monkeypatch.setattr(main_module._pool, "close", pool_close)
    monkeypatch.setattr(main_module._job_queue, "get_run", AsyncMock(side_effect=PoolClosed("pool is already closed")))

    app = main_module.create_app()
    service = app.state.connect_service

    async with app.router.lifespan_context(app):
        task = asyncio.create_task(fake_pipeline(), name=f"acolyte-run-{run_id}")
        service._background_tasks.add(task)
        task.add_done_callback(service._background_tasks.discard)
        await asyncio.wait_for(started.wait(), timeout=1.0)

    try:
        pool_close.assert_awaited_once()
        main_module._http_client.aclose.assert_awaited_once()  # type: ignore[missing-attribute]
    finally:
        task.cancel()


@pytest.mark.asyncio
async def test_shutdown_drains_pipelines_before_the_checkpointer_closes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The grace window is worthless if the checkpoint connection is already gone.

    Production runs with CHECKPOINT_ENABLED=true and durability="sync", so every
    super-step writes through the single psycopg connection create_checkpointer
    owns. Draining after that context exits means the pipeline we are giving
    time to land its terminal write cannot actually write.
    """
    _stub_lifespan_io(monkeypatch)
    monkeypatch.setattr(main_module.settings, "checkpoint_enabled", True)

    checkpointer_closed = asyncio.Event()
    checkpointer_open_at_drain: list[bool] = []
    started = asyncio.Event()

    @asynccontextmanager
    async def _stub_checkpointer(_dsn: object) -> AsyncGenerator[object]:
        try:
            yield object()
        finally:
            checkpointer_closed.set()

    monkeypatch.setattr(main_module, "create_checkpointer", _stub_checkpointer)
    monkeypatch.setattr(main_module, "_compile_graph", lambda **_kw: object())

    async def _pipeline() -> None:
        started.set()
        try:
            await asyncio.Event().wait()
        except asyncio.CancelledError:
            checkpointer_open_at_drain.append(not checkpointer_closed.is_set())
            raise

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        task = asyncio.create_task(_pipeline(), name="report-pipeline")
        main_module._job_queue.get_run.side_effect = _running_run
        app.state.connect_service._background_tasks.add(task)
        await started.wait()

    assert checkpointer_open_at_drain == [True], (
        "the pipeline must get its grace window while the checkpointer connection is still live"
    )


@pytest.mark.asyncio
async def test_shutdown_still_closes_the_pool_when_a_pipeline_dies_uncancellably(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A drained task settling with anything but CancelledError must not escape.

    _run_pipeline_locked can raise RuntimeError outside its own except clause;
    if that propagates out of the drain it skips the client and pool closes
    below it, leaking both across every redeploy.
    """
    _stub_lifespan_io(monkeypatch)
    pool_close = AsyncMock()
    http_aclose = AsyncMock()
    monkeypatch.setattr(main_module._pool, "close", pool_close)
    monkeypatch.setattr(main_module._http_client, "aclose", http_aclose)
    monkeypatch.setattr(main_module, "_PIPELINE_DRAIN_GRACE_SECONDS", 0.05)
    started = asyncio.Event()

    async def _pipeline() -> None:
        # Outlives the grace window, so the drain cancels it — and then it
        # settles with something other than CancelledError, the way
        # _run_pipeline_locked does when mark_running raises outside its own
        # except clause.
        started.set()
        try:
            await asyncio.Event().wait()
        except asyncio.CancelledError:
            msg = "Pipeline graph not configured"
            raise RuntimeError(msg) from None

    app = main_module.create_app()
    async with app.router.lifespan_context(app):
        task = asyncio.create_task(_pipeline(), name="report-pipeline")
        app.state.connect_service._background_tasks.add(task)
        await started.wait()

    pool_close.assert_awaited()
    http_aclose.assert_awaited()
