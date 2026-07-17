"""Tests for utilizer.web covering the XSS and event-loop-blocking fixes.

- The dashboard's client-side JS must never concatenate untrusted process /
  GPU data (user, command, gpu name) into ``innerHTML``; it must build rows
  via the DOM API and assign values through ``textContent`` instead.
- ``collect_snapshot`` must offload the blocking ``psutil`` /
  ``subprocess`` calls to worker threads (``asyncio.to_thread``) so the
  event loop stays free to service other coroutines while a snapshot is
  being collected.
"""

from __future__ import annotations

import asyncio
import time

import pytest

from utilizer import web


# ---------------------------------------------------------------------------
# XSS: process list / GPU name must be inserted via DOM API, not innerHTML
# ---------------------------------------------------------------------------


async def test_dashboard_html_builds_process_rows_via_dom_api_not_innerhtml() -> None:
    response = await web.index()
    html = response.body.decode()

    script = html.split("<script>", 1)[1]

    # The two historically-vulnerable assignments must be gone: they used to
    # splice `p.user` / `p.command` / `gpu.name` straight into innerHTML.
    assert "tbody.innerHTML =" not in script
    assert "gpuGrid.innerHTML = data.gpu.gpus.map" not in script

    # Rows/cards must instead be built with the DOM API and textContent.
    assert "createElement('tr')" in script
    assert "createElement('td')" in script
    assert script.count("textContent") >= 2
    # HTTPS pages must use wss://; plain http keeps ws://.
    assert "window.location.protocol === 'https:' ? 'wss:' : 'ws:'" in script


async def test_dashboard_html_process_cell_values_use_text_content() -> None:
    response = await web.index()
    script = response.body.decode().split("<script>", 1)[1]

    # The block that renders `p.user`/`p.command` into table cells must
    # assign through `cell.textContent`, never through innerHTML.
    process_block = script.split("data.top_processes.forEach", 1)[1]
    process_block = process_block.split("// 最終更新時刻", 1)[0]
    assert "innerHTML" not in process_block
    assert "cell.textContent = cellText" in process_block


async def test_dashboard_html_gpu_card_values_use_text_content() -> None:
    response = await web.index()
    script = response.body.decode().split("<script>", 1)[1]

    gpu_block = script.split("data.gpu.gpus.forEach", 1)[1]
    gpu_block = gpu_block.split("// プロセス一覧", 1)[0]
    assert "innerHTML" not in gpu_block
    assert "gpu.name" in gpu_block
    assert "heading.textContent" in gpu_block


# ---------------------------------------------------------------------------
# collect_snapshot must not block the event loop
# ---------------------------------------------------------------------------


async def test_collect_snapshot_offloads_blocking_calls_to_worker_threads(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    blocking_seconds = 0.3

    def blocking_cpu_info() -> web.CpuPayload:
        time.sleep(blocking_seconds)
        return web.CpuPayload(percent=42.0)

    monkeypatch.setattr(
        web,
        "get_memory_info",
        lambda: web.MemoryPayload(total=1, used=1, available=1, percent=1),
    )
    monkeypatch.setattr(web, "get_cpu_info", blocking_cpu_info)
    monkeypatch.setattr(
        web,
        "get_gpu_info",
        lambda: web.GpuPayload(available=False, gpus=[]),
    )
    monkeypatch.setattr(web, "get_hanging_processes", lambda: 0)
    monkeypatch.setattr(web, "get_top_processes", lambda limit=10: [])

    tick_count = 0

    async def ticker() -> None:
        nonlocal tick_count
        for _ in range(20):
            await asyncio.sleep(0.01)
            tick_count += 1

    start = time.monotonic()
    _, snapshot = await asyncio.gather(ticker(), web.collect_snapshot())
    elapsed = time.monotonic() - start

    assert snapshot.cpu.percent == 42.0
    assert tick_count == 20
    # The ticker's 20 x 10ms sleeps (~0.2s) must interleave with the 0.3s
    # blocking call instead of running after it: if `get_cpu_info` were
    # called synchronously on the event loop (no asyncio.to_thread), the
    # ticker couldn't advance at all until the blocking call returned and
    # total elapsed time would be roughly additive (~0.5s).
    assert elapsed < blocking_seconds + 0.15
