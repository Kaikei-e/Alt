"""Tests for system_monitor.py's parsing/aggregation logic.

All external commands (free, ps, nvidia-smi, top) are mocked at the
subprocess.run boundary so these tests exercise only the parsing logic.
"""

import subprocess
from types import SimpleNamespace

import pytest

import system_monitor


def _completed(stdout: str = "", returncode: int = 0) -> SimpleNamespace:
    return SimpleNamespace(stdout=stdout, returncode=returncode)


class TestGetMemoryInfo:
    def test_parses_free_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        free_output = (
            "              total        used        free      shared  buff/cache   available\n"
            "Mem:     67252326400 18317783040 12053950464    23134208 36880592896 48067821568\n"
        )
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(free_output)
        )

        info = system_monitor.get_memory_info()

        assert info.total == 67252326400
        assert info.used == 18317783040
        assert info.available == 48067821568
        assert info.error is None

    def test_returns_error_marker_on_failure(self, monkeypatch: pytest.MonkeyPatch) -> None:
        def _raise(*a, **k):
            raise FileNotFoundError("free not found")

        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        info = system_monitor.get_memory_info()

        assert info.total == 0
        assert info.percent == 0.0
        assert info.error is not None


class TestGetCpuInfo:
    def test_falls_back_to_proc_stat_when_psutil_unavailable(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr(system_monitor, "_psutil", None)

        calls = iter([[100, 0, 50, 800, 10, 0, 0, 0, 0, 0], [110, 0, 60, 830, 10, 0, 0, 0, 0, 0]])
        monkeypatch.setattr(
            system_monitor, "_read_proc_stat_cpu_fields", lambda: next(calls)
        )
        monkeypatch.setattr(system_monitor.time, "sleep", lambda *_: None)

        result = system_monitor.get_cpu_info()

        assert "percent" in result
        assert 0.0 <= result["percent"] <= 100.0

    def test_uses_nonblocking_psutil_when_available(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        class _FakePsutil:
            @staticmethod
            def cpu_percent(interval=None):
                assert interval is None
                return 42.5

        monkeypatch.setattr(system_monitor, "_psutil", _FakePsutil())

        result = system_monitor.get_cpu_info()

        assert result == {"percent": 42.5}

    def test_returns_zero_when_all_methods_fail(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(system_monitor, "_psutil", None)

        def _raise_oserror():
            raise OSError("no /proc/stat")

        monkeypatch.setattr(system_monitor, "_read_proc_stat_cpu_fields", _raise_oserror)

        def _raise_subprocess(*a, **k):
            raise subprocess.SubprocessError("no top")

        monkeypatch.setattr(system_monitor.subprocess, "run", _raise_subprocess)

        result = system_monitor.get_cpu_info()

        assert result == {"percent": 0.0}


class TestGetGpuInfo:
    def test_parses_nvidia_smi_csv_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        csv_output = "0, 45, 2048, 8192, 65, NVIDIA GeForce RTX 4060\n"
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(csv_output)
        )

        info = system_monitor.get_gpu_info()

        assert info["available"] is True
        assert len(info["gpus"]) == 1
        gpu = info["gpus"][0]
        assert gpu["utilization"] == 45.0
        assert gpu["memory_used"] == 2048.0
        assert gpu["memory_percent"] == 25.0

    def test_nvidia_smi_not_found_reports_unavailable(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        def _raise(*a, **k):
            raise FileNotFoundError()

        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        info = system_monitor.get_gpu_info()

        assert info["available"] is False
        assert info["gpus"] == []


class TestCountHangingProcesses:
    def test_counts_spawn_main_and_fork_lines(self, monkeypatch: pytest.MonkeyPatch) -> None:
        ps_output = (
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME spawn_main\n"
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME multiprocessing fork worker\n"
            "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME normal_process\n"
        )
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(ps_output)
        )

        assert system_monitor.count_hanging_processes() == 2

    def test_returns_zero_on_command_failure(self, monkeypatch: pytest.MonkeyPatch) -> None:
        def _raise(*a, **k):
            raise subprocess.TimeoutExpired(cmd="ps", timeout=5)

        monkeypatch.setattr(system_monitor.subprocess, "run", _raise)

        assert system_monitor.count_hanging_processes() == 0


class TestGetTopProcesses:
    def test_parses_ps_aux_output(self, monkeypatch: pytest.MonkeyPatch) -> None:
        ps_output = (
            "USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND\n"
            "root         1 12.5  2.0 100000 20480 ?        Ss   00:00   0:01 python app.py\n"
        )
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed(ps_output)
        )

        processes = system_monitor.get_top_processes(limit=5)

        assert len(processes) == 1
        assert processes[0]["pid"] == "1"
        assert processes[0]["cpu_percent"] == 12.5
        assert processes[0]["memory_mb"] == 20.0

    def test_empty_output_returns_empty_list(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(
            system_monitor.subprocess, "run", lambda *a, **k: _completed("header only\n")
        )

        assert system_monitor.get_top_processes() == []
