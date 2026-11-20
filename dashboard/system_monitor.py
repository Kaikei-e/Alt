"""
System Monitoring Module
Refactored to use subprocess-based commands (ps, free, nvidia-smi) instead of psutil
for better reliability in containerized environments.
Adapted from: monitors/utilizer/utilizer/monitors.py
"""

import contextlib
import functools
import logging
import re
import shutil
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

logger = logging.getLogger(__name__)

_PROC_STAT_IOWAIT_INDEX = 4
_NVIDIA_SMI_FIELD_COUNT = 6
_PS_MIN_OUTPUT_LINES = 2
_PS_AUX_COLUMN_COUNT = 11
_PS_COMMAND_INDEX = 10


class _PsutilModule(Protocol):
    def cpu_percent(self, interval: float | None = None) -> float: ...


_psutil: _PsutilModule | None
try:
    import psutil as _psutil_mod

    _psutil = _psutil_mod
except ImportError:  # pragma: no cover - optional dependency
    _psutil = None

# Seed non-blocking cpu_percent baseline so the first SSE sample is meaningful.
if _psutil is not None:
    with contextlib.suppress(OSError, RuntimeError, ValueError):
        _psutil.cpu_percent(interval=None)


@functools.cache
def _resolve_executable(name: str) -> str:
    """Resolve an executable from PATH once and fail loudly if missing."""
    resolved = shutil.which(name)
    if resolved is None:
        msg = f"Required executable '{name}' not found on PATH"
        raise FileNotFoundError(msg)
    return resolved


@dataclass(frozen=True, slots=True)
class MemoryInfo:
    """Memory usage snapshot.

    `error` is set (and the numeric fields are zeroed) when the underlying
    `free` command could not be read, so callers can distinguish a genuine
    0%% reading from a collection failure.
    """

    total: int
    used: int
    available: int
    percent: float
    error: str | None = None


def get_memory_info() -> MemoryInfo:
    """
    Get memory usage statistics using 'free -b'.
    Returns bytes to ensure compatibility with dashboard/app.py.
    """
    try:
        free_bin = _resolve_executable("free")
        # Use -b for bytes
        result = subprocess.run(  # noqa: S603 - fixed command resolved via shutil.which with static arguments
            [free_bin, "-b"], capture_output=True, text=True, check=True, timeout=2
        )
        lines = result.stdout.strip().split("\n")
        # Header:              total        used        free      shared  buff/cache   available
        # Mem:           67252326400  18317783040 12053950464    23134208 36880592896 48067821568
        mem_line = next(line for line in lines if line.startswith("Mem:"))
        parts = mem_line.split()
        total = int(parts[1])
        used = int(parts[2])
        # available is usually the last column in modern 'free'
        available = int(parts[-1])

        # Calculate percent
        percent = (used / total * 100) if total > 0 else 0.0

        return MemoryInfo(total=total, used=used, available=available, percent=round(percent, 1))
    except subprocess.SubprocessError, OSError, IndexError, ValueError:
        logger.exception("Error getting memory info")
        return MemoryInfo(total=0, used=0, available=0, percent=0.0, error="memory_unavailable")


def _read_proc_stat_cpu_fields() -> list[int]:
    """Read the aggregate 'cpu' line from /proc/stat as a list of counters."""
    with Path("/proc/stat").open() as f:
        lines = f.readlines()
    cpu_line = next(line for line in lines if line.startswith("cpu "))
    return [int(f) for f in cpu_line.split()[1:]]


def get_cpu_info() -> dict[str, float]:
    """Get CPU usage statistics (non-blocking where possible)."""
    # 1. Try psutil with interval=None (differential since last call; no 1s block)
    if _psutil is not None:
        try:
            percent = _psutil.cpu_percent(interval=None)
        except (OSError, RuntimeError, ValueError) as e:
            logger.debug("psutil.cpu_percent failed, falling back to /proc/stat", exc_info=e)
        else:
            return {"percent": round(percent, 1)}

    # 2. Try /proc/stat.
    # A single read gives the cumulative average since boot, not current
    # usage, so sample twice with a short interval and diff the counters.
    try:
        before = _read_proc_stat_cpu_fields()
        time.sleep(0.2)
        after = _read_proc_stat_cpu_fields()

        # user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
        deltas = [a - b for a, b in zip(after, before, strict=True)]
        total_delta = sum(deltas)
        idle_delta = deltas[3]
        # Handle Linux 2.6+ iowait as idle
        if len(deltas) > _PROC_STAT_IOWAIT_INDEX:
            idle_delta += deltas[_PROC_STAT_IOWAIT_INDEX]

        usage = 0.0
        if total_delta > 0:
            usage = round((1 - idle_delta / total_delta) * 100, 1)
    except (OSError, IndexError, ValueError) as e:
        logger.debug("Reading /proc/stat failed, falling back to top", exc_info=e)
    else:
        return {"percent": usage}

    # 3. Try top
    try:
        top_bin = _resolve_executable("top")
        result = subprocess.run(  # noqa: S603 - fixed command resolved via shutil.which with static arguments
            [top_bin, "-bn2", "-d", "0.5"],
            capture_output=True,
            text=True,
            check=True,
            timeout=3,
        )
        cpu_lines = [line for line in result.stdout.split("\n") if "Cpu(s)" in line]
        if cpu_lines:
            cpu_line = cpu_lines[-1]
            match = re.search(r"(\d+\.\d+)\s*id", cpu_line)
            if match:
                idle = float(match.group(1))
                usage = round(100 - idle, 1)
                return {"percent": usage}
    except (subprocess.SubprocessError, OSError) as e:
        logger.debug("top command failed", exc_info=e)

    logger.warning("All CPU usage detection methods failed, reporting 0.0%%")
    return {"percent": 0.0}


def get_gpu_info() -> dict[str, Any]:
    """Get GPU info using nvidia-smi"""
    try:
        nvidia_smi_bin = _resolve_executable("nvidia-smi")
        # Added 'index' to query to match previous system_monitor.py interface
        result = subprocess.run(  # noqa: S603 - fixed command resolved via shutil.which with static arguments
            [
                nvidia_smi_bin,
                "--query-gpu=index,utilization.gpu,memory.used,memory.total,temperature.gpu,name",
                "--format=csv,noheader,nounits",
            ],
            capture_output=True,
            text=True,
            check=True,
            timeout=5,
        )
        lines = result.stdout.strip().split("\n")
        if not lines or not lines[0]:
            return {"available": False, "gpus": []}

        gpus = []
        for line in lines:
            if not line.strip():
                continue
            parts = [p.strip() for p in line.split(",")]
            # Expecting 6 parts
            if len(parts) >= _NVIDIA_SMI_FIELD_COUNT:
                try:
                    mem_used = int(parts[2])
                    mem_total = int(parts[3])
                    mem_percent = round((mem_used / mem_total * 100) if mem_total > 0 else 0, 1)

                    gpus.append(
                        {
                            "index": parts[0],
                            "utilization": float(parts[1]),
                            "memory_used": float(mem_used),
                            "memory_total": float(mem_total),
                            "temperature": int(parts[4]),
                            "name": parts[5],
                            "memory_percent": mem_percent,
                        }
                    )
                except ValueError, IndexError:
                    continue
    except FileNotFoundError:
        return {
            "available": False,
            "gpus": [],
            "error": "nvidia-smi not found",
            "message": "nvidia-smi command not found. GPU may not be available or NVIDIA drivers not installed.",
        }
    except subprocess.TimeoutExpired:
        return {
            "available": False,
            "gpus": [],
            "error": "timeout",
            "message": "nvidia-smi command timed out.",
        }
    except subprocess.CalledProcessError as e:
        logger.exception("nvidia-smi failed with exit code %s", e.returncode)
        return {
            "available": False,
            "gpus": [],
            "error": "nvidia_smi_failed",
            "message": "nvidia-smi command failed. GPU may not be accessible.",
        }
    except OSError:
        logger.exception("Error getting GPU info")
        return {
            "available": False,
            "gpus": [],
            "error": "gpu_info_unavailable",
            "message": "Error getting GPU info.",
        }
    else:
        return {
            "available": True,
            "gpus": gpus,
        }


def count_hanging_processes() -> int:
    """Count potential hanging processes"""
    try:
        ps_bin = _resolve_executable("ps")
        result = subprocess.run(  # noqa: S603 - fixed command resolved via shutil.which with static arguments
            [ps_bin, "aux"], capture_output=True, text=True, check=True, timeout=5
        )
        count = 0
        for line in result.stdout.split("\n"):
            if "spawn_main" in line or ("multiprocessing" in line and "fork" in line):
                count += 1

        # Subtract grep itself if it appears (though we scan lines, grep is usually not in output of ps aux unless piped)
        # But safest is to just count.
        # Adjusted logic: 'ps aux' shows all processes.
        return max(0, count)
    except subprocess.SubprocessError, OSError:
        logger.exception("Error counting hanging processes")
        return 0


def get_top_processes(limit: int = 15) -> list[dict[str, Any]]:
    """Get top N processes by CPU usage"""
    try:
        ps_bin = _resolve_executable("ps")
        result = subprocess.run(  # noqa: S603 - fixed command resolved via shutil.which with static arguments
            [ps_bin, "aux", "--sort=-%cpu"],
            capture_output=True,
            text=True,
            check=True,
            timeout=5,
        )
        lines = result.stdout.strip().split("\n")
        if len(lines) < _PS_MIN_OUTPUT_LINES:
            return []

        # lines[0] is header
        process_lines = lines[1 : limit + 1]
        processes = []
        for line in process_lines:
            if not line.strip():
                continue
            # USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
            parts = line.split(None, 10)
            if len(parts) >= _PS_AUX_COLUMN_COUNT:
                try:
                    # Mapping to interface expected by app.py:
                    # 'pid', 'name', 'memory_mb', 'cpu_percent'
                    cmd = parts[_PS_COMMAND_INDEX] if len(parts) > _PS_COMMAND_INDEX else "unknown"

                    processes.append(
                        {
                            "pid": parts[1],  # string is fine, app.py uses st.dataframe
                            "name": cmd,  # Use full command or first part? app.py just displays it.
                            "cpu_percent": float(parts[2]),
                            "memory_mb": int(parts[5]) / 1024,  # RSS in KB -> MB
                        }
                    )
                except ValueError, IndexError:
                    continue
    except subprocess.TimeoutExpired:
        logger.error("Error getting top processes: ps command timed out after 5 seconds")  # noqa: TRY400 - expected condition; traceback adds noise
        return []
    except subprocess.CalledProcessError as e:
        logger.error(  # noqa: TRY400 - expected condition; traceback adds noise
            "Error getting top processes: ps command failed with exit code %s",
            e.returncode,
        )
        return []
    except OSError:
        logger.exception("Error getting top processes")
        return []
    else:
        return processes
