"""Unit tests for utilizer.monitors.

Focus of this suite: the collector functions must only swallow the specific,
expected failure modes of the external commands they shell out to (missing
binary, non-zero exit, timeout, malformed output) and must let any other
exception propagate instead of being silently converted into a zero/empty
default. Before the exception-narrowing fix, every collector used a bare
``except Exception`` here, which made "the external tool genuinely failed"
indistinguishable from "our code has a bug" (see CLAUDE.md rule 8, no silent
fallback).
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path
from typing import Any
from unittest.mock import Mock

import pytest

from utilizer import monitors
from utilizer.monitors import (
    CpuInfo,
    GpuInfo,
    GpuStat,
    MemoryInfo,
    ProcessInfo,
)


class _UnexpectedError(Exception):
    """A failure mode the collectors have never been designed to expect.

    Used to prove that a collector's except clause is narrow: if it were
    still a bare ``except Exception``, this would be swallowed and a zero
    default returned instead of propagating.
    """


def _fake_completed(stdout: str) -> Mock:
    return Mock(stdout=stdout)


def _patch_proc_stat(monkeypatch: pytest.MonkeyPatch, texts: list[str]) -> None:
    """Stub Path('/proc/stat').read_text for the Path-based reader."""
    samples = iter(texts)
    real_read_text = Path.read_text

    def fake_read_text(self: Path, *args: Any, **kwargs: Any) -> str:
        if str(self) == "/proc/stat":
            return next(samples)
        return real_read_text(self, *args, **kwargs)

    monkeypatch.setattr(Path, "read_text", fake_read_text)


def _patch_proc_stat_error(monkeypatch: pytest.MonkeyPatch, exc: Exception) -> None:
    real_read_text = Path.read_text

    def fake_read_text(self: Path, *args: Any, **kwargs: Any) -> str:
        if str(self) == "/proc/stat":
            raise exc
        return real_read_text(self, *args, **kwargs)

    monkeypatch.setattr(Path, "read_text", fake_read_text)


# ---------------------------------------------------------------------------
# get_memory_info
# ---------------------------------------------------------------------------


def test_get_memory_info_parses_free_output(monkeypatch: pytest.MonkeyPatch) -> None:
    stdout = (
        "              total        used        free      shared  buff/cache   available\n"
        "Mem:             31          10           5           1          15          20\n"
    )
    monkeypatch.setattr(monitors.subprocess, "run", lambda *a, **k: _fake_completed(stdout))

    assert monitors.get_memory_info() == MemoryInfo(
        total=31,
        used=10,
        available=20,
        percent=32.3,
    )


@pytest.mark.parametrize(
    "exc",
    [
        subprocess.CalledProcessError(1, ["free", "-g"]),
        FileNotFoundError("free: command not found"),
    ],
)
def test_get_memory_info_returns_zero_default_on_expected_failures(
    monkeypatch: pytest.MonkeyPatch, exc: Exception
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise exc

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_memory_info() == MemoryInfo(total=0, used=0, available=0, percent=0)


def test_get_memory_info_propagates_unexpected_errors(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("disk on fire")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_memory_info()


# ---------------------------------------------------------------------------
# get_hanging_processes / get_recap_processes
# ---------------------------------------------------------------------------


def test_get_hanging_processes_counts_matching_lines(monkeypatch: pytest.MonkeyPatch) -> None:
    stdout = "\n".join(
        [
            "user1 1 ... spawn_main",
            "user2 2 ... multiprocessing-fork",
            "user3 3 ... normal_process",
            "",
        ]
    )
    monkeypatch.setattr(monitors.subprocess, "run", lambda *a, **k: _fake_completed(stdout))

    assert monitors.get_hanging_processes() == 2


@pytest.mark.parametrize(
    "exc",
    [
        subprocess.CalledProcessError(1, ["ps", "aux"]),
        FileNotFoundError("ps: command not found"),
    ],
)
def test_get_hanging_processes_returns_zero_on_expected_failures(
    monkeypatch: pytest.MonkeyPatch, exc: Exception
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise exc

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_hanging_processes() == 0


def test_get_hanging_processes_propagates_unexpected_errors(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("ps blew up")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_hanging_processes()


def test_get_recap_processes_counts_matching_lines(monkeypatch: pytest.MonkeyPatch) -> None:
    stdout = "\n".join(
        [
            "user1 1 ... recap-worker",
            "user2 2 ... gunicorn",
            "user3 3 ... normal_process",
            "",
        ]
    )
    monkeypatch.setattr(monitors.subprocess, "run", lambda *a, **k: _fake_completed(stdout))

    assert monitors.get_recap_processes() == 2


@pytest.mark.parametrize(
    "exc",
    [
        subprocess.CalledProcessError(1, ["ps", "aux"]),
        FileNotFoundError("ps: command not found"),
    ],
)
def test_get_recap_processes_returns_zero_on_expected_failures(
    monkeypatch: pytest.MonkeyPatch, exc: Exception
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise exc

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_recap_processes() == 0


def test_get_recap_processes_propagates_unexpected_errors(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("ps blew up")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_recap_processes()


# ---------------------------------------------------------------------------
# get_gpu_info
# ---------------------------------------------------------------------------


def test_get_gpu_info_parses_nvidia_smi_csv(monkeypatch: pytest.MonkeyPatch) -> None:
    stdout = "45, 1024, 8192, 60, NVIDIA GeForce RTX 3080\n"
    monkeypatch.setattr(
        monitors.subprocess,
        "run",
        lambda *a, **k: _fake_completed(stdout),
    )

    result = monitors.get_gpu_info()

    assert result == GpuInfo(
        available=True,
        gpus=(
            GpuStat(
                utilization=45.0,
                memory_used=1024,
                memory_total=8192,
                temperature=60,
                name="NVIDIA GeForce RTX 3080",
                memory_percent=12.5,
            ),
        ),
        total_gpus=1,
    )


def test_get_gpu_info_reports_missing_binary(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise FileNotFoundError("nvidia-smi not found")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_gpu_info() == GpuInfo(
        available=False,
        gpus=(),
        error="nvidia-smi not found",
    )


def test_get_gpu_info_reports_timeout(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise subprocess.TimeoutExpired(cmd=["nvidia-smi"], timeout=5)

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_gpu_info() == GpuInfo(
        available=False,
        gpus=(),
        error="timeout",
    )


def test_get_gpu_info_reports_non_zero_exit(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise subprocess.CalledProcessError(1, ["nvidia-smi"], stderr="driver error")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    result = monitors.get_gpu_info()

    assert result.available is False
    assert result.gpus == ()
    assert result.error is not None


def test_get_gpu_info_propagates_unexpected_errors(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("gpu driver segfaulted")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_gpu_info()


# ---------------------------------------------------------------------------
# get_top_processes
# ---------------------------------------------------------------------------


def test_get_top_processes_parses_ps_aux_output(monkeypatch: pytest.MonkeyPatch) -> None:
    stdout = "\n".join(
        [
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND",
            "alice 1234 12.3 45.6 100000 204800 pts/0 R+ 10:00 0:01 python3 my_script.py --flag",
        ]
    )
    monkeypatch.setattr(monitors.subprocess, "run", lambda *a, **k: _fake_completed(stdout))

    assert monitors.get_top_processes(limit=5) == [
        ProcessInfo(
            user="alice",
            pid="1234",
            cpu=12.3,
            mem=45.6,
            rss=200,
            command="python3 my_script.py --flag",
        )
    ]


def test_get_top_processes_returns_empty_on_header_only_output(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    stdout = "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\n"
    monkeypatch.setattr(monitors.subprocess, "run", lambda *a, **k: _fake_completed(stdout))

    assert monitors.get_top_processes() == []


@pytest.mark.parametrize(
    "exc",
    [
        subprocess.CalledProcessError(1, ["ps", "aux", "--sort=-%mem"]),
        FileNotFoundError("ps: command not found"),
        subprocess.TimeoutExpired(cmd=["ps", "aux"], timeout=5),
    ],
)
def test_get_top_processes_returns_empty_on_expected_failures(
    monkeypatch: pytest.MonkeyPatch, exc: Exception
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise exc

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_top_processes() == []


def test_get_top_processes_propagates_unexpected_errors(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("ps blew up")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_top_processes()


# ---------------------------------------------------------------------------
# get_cpu_info
# ---------------------------------------------------------------------------


def test_get_cpu_info_uses_psutil_when_available(monkeypatch: pytest.MonkeyPatch) -> None:
    import psutil

    monkeypatch.setattr(psutil, "cpu_percent", lambda interval: 37.0)

    assert monitors.get_cpu_info() == CpuInfo(percent=37.0)


def _force_psutil_import_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setitem(sys.modules, "psutil", None)


def test_get_cpu_info_falls_back_to_proc_stat(monkeypatch: pytest.MonkeyPatch) -> None:
    """Usage must come from the delta between two samples, not a single
    cumulative-since-boot read (which would report the lifetime average)."""
    _force_psutil_import_error(monkeypatch)
    monkeypatch.setattr(monitors.time, "sleep", lambda _seconds: None)

    # Sample 1: total=1000, idle=800. Sample 2: total=1200, idle=900.
    # delta: total=200, idle=100 -> usage = (1 - 100/200) * 100 = 50.0
    _patch_proc_stat(
        monkeypatch,
        [
            "cpu  100 0 100 800 0 0 0 0 0 0\n",
            "cpu  150 0 150 900 0 0 0 0 0 0\n",
        ],
    )

    assert monitors.get_cpu_info() == CpuInfo(percent=50.0)


def test_get_cpu_info_proc_stat_propagates_unexpected_errors(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _force_psutil_import_error(monkeypatch)
    _patch_proc_stat_error(monkeypatch, _UnexpectedError("procfs corrupted"))

    with pytest.raises(_UnexpectedError):
        monitors.get_cpu_info()


def test_get_cpu_info_falls_back_to_top_when_proc_stat_unavailable(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _force_psutil_import_error(monkeypatch)
    _patch_proc_stat_error(monkeypatch, OSError("/proc is not mounted"))
    monkeypatch.setattr(
        monitors.subprocess,
        "run",
        lambda *a, **k: _fake_completed("Cpu(s): 75.0%id 25.0%us\n"),
    )

    assert monitors.get_cpu_info() == CpuInfo(percent=25.0)


def test_get_cpu_info_top_fallback_recovers_from_timeout(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _force_psutil_import_error(monkeypatch)
    _patch_proc_stat_error(monkeypatch, OSError("/proc is not mounted"))

    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise subprocess.TimeoutExpired(cmd=["top"], timeout=2)

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    assert monitors.get_cpu_info() == CpuInfo(percent=0.0)


def test_get_cpu_info_top_fallback_propagates_unexpected_errors(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _force_psutil_import_error(monkeypatch)
    _patch_proc_stat_error(monkeypatch, OSError("/proc is not mounted"))

    def fake_run(*_a: Any, **_k: Any) -> Mock:
        raise _UnexpectedError("top blew up")

    monkeypatch.setattr(monitors.subprocess, "run", fake_run)

    with pytest.raises(_UnexpectedError):
        monitors.get_cpu_info()
