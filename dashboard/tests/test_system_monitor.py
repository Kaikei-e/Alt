"""Tests for system_monitor.py's parsing/aggregation logic.

All external commands (free, ps, nvidia-smi, top) are mocked at the
subprocess.run boundary so these tests exercise only the parsing logic.
"""

import io
import subprocess
from types import SimpleNamespace
from typing import NoReturn

import pytest

import system_monitor

_MAX_CPU_PERCENT = 100.0


def _completed(stdout: str = "", returncode: int = 0) -> SimpleNamespace:
    return SimpleNamespace(stdout=stdout, returncode=returncode)


class TestGetMemoryInfo:
    def test_parses_free_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        expected_total = 67252326400
        expected_used = 18317783040
        expected_available = 48067821568
        free_output = (
            "              total        used        free      shared  buff/cache   available\n"
            f"Mem:     {expected_total} {expected_used} 12053950464    23134208 36880592896 {expected_available}\n"
        )
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(free_output)
        )

        info = system_monitor.get_memory_info()

        assert info.total == expected_total
        assert info.used == expected_used
        assert info.available == expected_available
        assert info.error is None

    def test_returns_error_marker_on_failure(self, monkeypatch: pytest.MonkeyPatch) -> None:
        def _raise(*a: object, **k: object) -> NoReturn:
            msg = "free not found"
            raise FileNotFoundError(msg)

        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        info = system_monitor.get_memory_info()

        assert info.total == 0
        assert info.percent == 0.0
        assert info.error is not None

    def test_returns_error_marker_when_mem_line_missing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        free_output = (
            "              total        used        free      shared  buff/cache   available\n"
            "Swap:             0           0           0\n"
        )
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(free_output)
        )

        info = system_monitor.get_memory_info()

        assert info == system_monitor.MemoryInfo(
            total=0, used=0, available=0, percent=0.0, error="memory_unavailable"
        )


class TestGetCpuInfo:
    def test_falls_back_to_proc_stat_when_psutil_unavailable(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr(system_monitor, "_psutil", None)

        calls = iter([[100, 0, 50, 800, 10, 0, 0, 0, 0, 0], [110, 0, 60, 830, 10, 0, 0, 0, 0, 0]])
        monkeypatch.setattr(system_monitor, "_read_proc_stat_cpu_fields", lambda: next(calls))
        monkeypatch.setattr(system_monitor.time, "sleep", lambda *_: None)

        result = system_monitor.get_cpu_info()

        assert "percent" in result
        assert 0.0 <= result["percent"] <= _MAX_CPU_PERCENT

    def test_uses_nonblocking_psutil_when_available(self, monkeypatch: pytest.MonkeyPatch) -> None:
        expected_percent = 42.5

        class _FakePsutil:
            @staticmethod
            def cpu_percent(interval: float | None = None) -> float:
                assert interval is None
                return expected_percent

        monkeypatch.setattr(system_monitor, "_psutil", _FakePsutil())

        result = system_monitor.get_cpu_info()

        assert result == {"percent": expected_percent}

    def test_returns_zero_when_all_methods_fail(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(system_monitor, "_psutil", None)

        def _raise_oserror() -> NoReturn:
            msg = "no /proc/stat"
            raise OSError(msg)

        monkeypatch.setattr(system_monitor, "_read_proc_stat_cpu_fields", _raise_oserror)

        def _raise_subprocess(*a: object, **k: object) -> NoReturn:
            msg = "no top"
            raise subprocess.SubprocessError(msg)

        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", _raise_subprocess)

        result = system_monitor.get_cpu_info()

        assert result == {"percent": 0.0}

    def test_falls_back_to_top_when_proc_stat_cpu_line_missing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        expected_cpu_percent = 25.0
        monkeypatch.setattr(system_monitor, "_psutil", None)
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(
            system_monitor.Path,
            "open",
            lambda _self, *a, **k: io.StringIO("cpu0 100 0 50 800\nintr 123\n"),
        )
        top_calls: list[list[str]] = []

        def _fake_top(cmd: list[str], **_kwargs: object) -> SimpleNamespace:
            top_calls.append(cmd)
            return _completed("%Cpu(s):  5.0 us,  5.0 sy,  0.0 ni, 75.0 id,  0.0 wa\n")

        monkeypatch.setattr(system_monitor.subprocess, "run", _fake_top)

        result = system_monitor.get_cpu_info()

        assert len(top_calls) == 1
        assert top_calls[0][0] == "top"
        assert result == {"percent": expected_cpu_percent}


class TestGetGpuInfo:
    def test_parses_nvidia_smi_csv_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        expected_util = 45.0
        expected_mem_used = 2048.0
        expected_mem_total = 8192.0
        expected_mem_percent = round(expected_mem_used / expected_mem_total * 100, 1)
        csv_output = (
            f"0, {int(expected_util)}, {int(expected_mem_used)}, "
            f"{int(expected_mem_total)}, 65, NVIDIA GeForce RTX 4060\n"
        )
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(csv_output)
        )

        info = system_monitor.get_gpu_info()

        assert info["available"] is True
        assert len(info["gpus"]) == 1
        gpu = info["gpus"][0]
        assert gpu["utilization"] == expected_util
        assert gpu["memory_used"] == expected_mem_used
        assert gpu["memory_percent"] == expected_mem_percent

    def test_nvidia_smi_not_found_reports_unavailable(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        def _raise(*a: object, **k: object) -> NoReturn:
            raise FileNotFoundError

        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        info = system_monitor.get_gpu_info()

        assert info["available"] is False
        assert info["gpus"] == []


class TestCountHangingProcesses:
    def test_counts_spawn_main_and_fork_lines(self, monkeypatch: pytest.MonkeyPatch) -> None:
        hanging_lines = [
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME spawn_main",
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME multiprocessing fork worker",
        ]
        normal_lines = ["USER PID %CPU %MEM VSZ RSS TTY STAT START TIME normal_process"]
        ps_output = "\n".join([*hanging_lines, *normal_lines]) + "\n"
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", lambda *a, **k: _completed(ps_output))

        assert system_monitor.count_hanging_processes() == len(hanging_lines)

    def test_returns_zero_on_command_failure(self, monkeypatch: pytest.MonkeyPatch) -> None:
        def _raise(*a: object, **k: object) -> NoReturn:
            raise subprocess.TimeoutExpired(cmd="ps", timeout=5)

        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        assert system_monitor.count_hanging_processes() == 0

    def test_logs_error_and_returns_zero_when_ps_missing(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        system_monitor._resolve_executable.cache_clear()
        monkeypatch.setattr(system_monitor.shutil, "which", lambda _name: None)

        try:
            with caplog.at_level("ERROR"):
                result = system_monitor.count_hanging_processes()
        finally:
            system_monitor._resolve_executable.cache_clear()

        assert result == 0
        assert "Error counting hanging processes" in caplog.text
        assert any(r.levelname == "ERROR" and r.exc_info is not None for r in caplog.records)


class TestGetTopProcesses:
    def test_parses_ps_aux_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        expected_cpu = 12.5
        rss_kb = 20480
        expected_memory_mb = rss_kb / 1024
        ps_output = (
            "USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND\n"
            f"root         1 {expected_cpu}  2.0 100000 {rss_kb} ?        Ss   00:00   0:01 python app.py\n"
        )
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(system_monitor.subprocess, "run", lambda *a, **k: _completed(ps_output))

        processes = system_monitor.get_top_processes(limit=5)

        assert len(processes) == 1
        assert processes[0]["pid"] == "1"
        assert processes[0]["cpu_percent"] == expected_cpu
        assert processes[0]["memory_mb"] == expected_memory_mb

    def test_empty_output_returns_empty_list(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(system_monitor, "_resolve_executable", lambda name: name)
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed("header only\n")
        )

        assert system_monitor.get_top_processes() == []

    def test_logs_error_and_returns_empty_when_ps_missing(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        system_monitor._resolve_executable.cache_clear()
        monkeypatch.setattr(system_monitor.shutil, "which", lambda _name: None)

        try:
            with caplog.at_level("ERROR"):
                result = system_monitor.get_top_processes()
        finally:
            system_monitor._resolve_executable.cache_clear()

        assert result == []
        assert "Error getting top processes" in caplog.text
        assert any(r.levelname == "ERROR" and r.exc_info is not None for r in caplog.records)
