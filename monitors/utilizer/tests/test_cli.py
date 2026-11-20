"""Characterization tests for utilizer.cli."""

from __future__ import annotations

import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path

import pytest

from utilizer import cli
from utilizer.monitors import CpuInfo, GpuInfo, GpuStat, MemoryInfo, ProcessInfo

_EXPECTED_LOG_LINES = 2


class _StopLoop(KeyboardInterrupt):
    """Test sentinel inheriting KeyboardInterrupt to exit cli.main() cleanly."""


@dataclass(frozen=True, slots=True)
class _MonitorSamples:
    memories: list[MemoryInfo]
    cpus: list[CpuInfo]
    gpus: list[GpuInfo] = field(default_factory=list)
    hanging_counts: list[int] = field(default_factory=list)
    recap_counts: list[int] = field(default_factory=list)
    top_processes: list[list[ProcessInfo]] = field(default_factory=list)


def _make_sleep_stopper(max_iterations: int) -> Callable[[float], None]:
    calls = 0

    def fake_sleep(_interval: float) -> None:
        nonlocal calls
        calls += 1
        if calls >= max_iterations:
            raise _StopLoop

    return fake_sleep


def _stub_monitors(monkeypatch: pytest.MonkeyPatch, samples: _MonitorSamples) -> None:
    mem_iter = iter(samples.memories)
    cpu_iter = iter(samples.cpus)
    gpu_iter = iter(samples.gpus or [GpuInfo(available=False, gpus=()) for _ in samples.memories])
    hang_iter = iter(samples.hanging_counts or [0 for _ in samples.memories])
    recap_iter = iter(samples.recap_counts or [0 for _ in samples.memories])
    proc_iter = iter(samples.top_processes or [[] for _ in samples.memories])

    monkeypatch.setattr(cli, "get_memory_info", lambda: next(mem_iter))
    monkeypatch.setattr(cli, "get_cpu_info", lambda: next(cpu_iter))
    monkeypatch.setattr(cli, "get_gpu_info", lambda: next(gpu_iter))
    monkeypatch.setattr(cli, "get_hanging_processes", lambda: next(hang_iter))
    monkeypatch.setattr(cli, "get_recap_processes", lambda: next(recap_iter))
    monkeypatch.setattr(cli, "get_top_processes", lambda _limit=5: next(proc_iter))


def test_cli_outputs_deltas_alerts_gpu_and_filtered_processes(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    long_cmd = "recap-worker " + ("x" * 70)
    procs = [
        ProcessInfo(user="alice", pid="1", cpu=12.3, mem=4.0, rss=100, command=long_cmd),
        ProcessInfo(user="bob", pid="2", cpu=1.0, mem=1.0, rss=50, command="Gunicorn app"),
        ProcessInfo(user="carol", pid="3", cpu=0.5, mem=0.5, rss=20, command="python spawn_main"),
        ProcessInfo(user="dave", pid="4", cpu=99.0, mem=10.0, rss=500, command="unrelated-proc"),
    ]
    gpu = GpuInfo(
        available=True,
        gpus=(
            GpuStat(
                utilization=55.0,
                memory_used=2048,
                memory_total=8192,
                temperature=62,
                name="RTX 3080",
                memory_percent=25.0,
            ),
        ),
        total_gpus=1,
    )

    _stub_monitors(
        monkeypatch,
        _MonitorSamples(
            memories=[
                MemoryInfo(total=32, used=10, available=22, percent=31.2),
                MemoryInfo(total=32, used=12, available=20, percent=95.0),
                MemoryInfo(total=32, used=11, available=21, percent=34.4),
                MemoryInfo(total=32, used=11, available=21, percent=34.4),
            ],
            cpus=[
                CpuInfo(percent=20.0),
                CpuInfo(percent=95.5),
                CpuInfo(percent=50.0),
                CpuInfo(percent=50.0),
            ],
            gpus=[
                GpuInfo(available=False, gpus=()),
                gpu,
                GpuInfo(available=True, gpus=()),
                GpuInfo(available=False, gpus=()),
            ],
            hanging_counts=[0, 11, 0, 0],
            recap_counts=[1, 2, 3, 3],
            top_processes=[procs, [], [], []],
        ),
    )
    monkeypatch.setattr(cli.time, "sleep", _make_sleep_stopper(4))
    monkeypatch.setattr(sys, "argv", ["utilizer", "-i", "1.5"])

    cli.main()

    out = capsys.readouterr().out
    assert "=== Recap Job リソース監視 ===" in out
    assert "監視間隔: 1.5秒" in out
    assert "ログファイル:" not in out

    # Positive, negative, and zero deltas for memory and CPU
    assert "+2GB" in out
    assert "-1GB" in out
    assert "±0GB" in out
    assert "+75.5%" in out
    assert "-45.5%" in out
    assert "±0%" in out

    # All three alerts
    assert "メモリ使用率が高いです！" in out
    assert "ハングプロセスが多すぎます！" in out
    assert "CPU使用率が高いです！" in out

    # GPU line (MiB -> GiB floor division)
    assert "GPU 0 (RTX 3080): " in out
    assert "使用率 55.0%" in out
    assert "メモリ 25.0% (2GB/8GB)" in out
    assert "温度 62°C" in out

    # Process filtering and 60-char truncation
    assert f"{long_cmd[:60]}\n" in out
    assert "Gunicorn app" in out
    assert "python spawn_main" in out
    assert "unrelated-proc" not in out

    # Graceful exit message
    assert "監視を終了します..." in out


def test_cli_writes_csv_log_file(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    log_path = tmp_path / "metrics.csv"
    _stub_monitors(
        monkeypatch,
        _MonitorSamples(
            memories=[MemoryInfo(total=16, used=4, available=12, percent=25.0)],
            cpus=[CpuInfo(percent=12.5)],
            hanging_counts=[2],
            recap_counts=[7],
        ),
    )
    monkeypatch.setattr(cli.time, "sleep", _make_sleep_stopper(1))
    monkeypatch.setattr(sys, "argv", ["utilizer", "-l", str(log_path)])

    cli.main()

    out = capsys.readouterr().out
    assert f"ログファイル: {log_path}" in out

    lines = log_path.read_text(encoding="utf-8").splitlines()
    assert len(lines) == _EXPECTED_LOG_LINES
    assert (
        lines[0] == "時刻,メモリ使用(GB),メモリ使用率(%),CPU使用率(%),ハングプロセス数,総プロセス数"
    )
    assert lines[1].endswith(",4,25.0,12.5,2,7")
