"""共通の監視機能"""

from __future__ import annotations

import logging
import re
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

logger = logging.getLogger(__name__)

# Gap between the two /proc/stat samples used to compute instantaneous CPU
# usage (see `_read_proc_stat_times` / `get_cpu_info`).
PROC_STAT_SAMPLE_INTERVAL_SEC = 0.1


@dataclass(frozen=True, slots=True)
class MemoryInfo:
    total: int
    used: int
    available: int
    percent: float


@dataclass(frozen=True, slots=True)
class CpuInfo:
    percent: float


@dataclass(frozen=True, slots=True)
class GpuStat:
    utilization: float
    memory_used: int
    memory_total: int
    temperature: int
    name: str
    memory_percent: float


@dataclass(frozen=True, slots=True)
class GpuInfo:
    available: bool
    gpus: tuple[GpuStat, ...]
    total_gpus: int = 0
    error: str | None = None


@dataclass(frozen=True, slots=True)
class ProcessInfo:
    user: str
    pid: str
    cpu: float
    mem: float
    rss: int
    command: str


def _read_proc_stat_times() -> tuple[int, int]:
    """Read the aggregate `cpu` line from /proc/stat. Returns (total, idle) jiffies."""
    text = Path("/proc/stat").read_text(encoding="utf-8")
    cpu_line = next((line for line in text.splitlines() if line.startswith("cpu ")), None)
    if cpu_line is None:
        raise ValueError("no aggregate cpu line in /proc/stat")
    fields = cpu_line.split()
    total = sum(int(fields[i]) for i in range(1, len(fields)))
    idle = int(fields[4]) + (int(fields[5]) if len(fields) > 5 else 0)
    return total, idle


def get_memory_info() -> MemoryInfo:
    """メモリ使用状況を取得"""
    try:
        result = subprocess.run(
            ["free", "-g"], capture_output=True, text=True, check=True, timeout=5
        )
        lines = result.stdout.strip().split("\n")
        mem_line = next((line for line in lines if line.startswith("Mem:")), None)
        if mem_line is None:
            raise ValueError("no Mem: line in free output")
        parts = mem_line.split()
        total = int(parts[1])
        used = int(parts[2])
        available = int(parts[6])
        percent = (used / total * 100) if total > 0 else 0
        return MemoryInfo(
            total=total,
            used=used,
            available=available,
            percent=round(percent, 1),
        )
    except (
        subprocess.CalledProcessError,
        subprocess.TimeoutExpired,
        FileNotFoundError,
        IndexError,
        ValueError,
    ):
        logger.exception("Failed to get memory info via 'free -g'")
        return MemoryInfo(total=0, used=0, available=0, percent=0)


def get_cpu_info() -> CpuInfo:
    """CPU使用率を取得"""
    try:
        import psutil

        percent = psutil.cpu_percent(interval=1)
        return CpuInfo(percent=round(percent, 1))
    except ImportError:
        # psutilがインストールされていない場合、/proc/statを使用。
        # 単発読みは起動からの累積カウンタなので瞬間使用率にならない
        # ため、2回サンプリングした差分から使用率を求める。
        try:
            total_1, idle_1 = _read_proc_stat_times()
            time.sleep(PROC_STAT_SAMPLE_INTERVAL_SEC)
            total_2, idle_2 = _read_proc_stat_times()
            total_delta = total_2 - total_1
            idle_delta = idle_2 - idle_1
            usage = round((1 - idle_delta / total_delta) * 100, 1) if total_delta > 0 else 0.0
            return CpuInfo(percent=usage)
        except (OSError, IndexError, ValueError):
            logger.exception("Failed to compute CPU usage from /proc/stat")
            # 最後の手段としてtopを使用
            try:
                result = subprocess.run(
                    ["top", "-bn2", "-d", "0.5"],
                    capture_output=True,
                    text=True,
                    check=True,
                    timeout=2,
                )
                cpu_lines = [
                    line for line in result.stdout.split("\n") if "Cpu(s)" in line
                ]
                if cpu_lines:
                    cpu_line = cpu_lines[-1]
                    match = re.search(r"(\d+\.\d+)%id", cpu_line)
                    if match:
                        idle = float(match.group(1))
                        usage = round(100 - idle, 1)
                    else:
                        usage = 0.0
                else:
                    usage = 0.0
                return CpuInfo(percent=usage)
            except (
                subprocess.CalledProcessError,
                FileNotFoundError,
                subprocess.TimeoutExpired,
            ):
                logger.exception("Failed to compute CPU usage from 'top' fallback")
                return CpuInfo(percent=0.0)


def get_hanging_processes() -> int:
    """ハングプロセス数を取得"""
    try:
        result = subprocess.run(
            ["ps", "aux"], capture_output=True, text=True, check=True, timeout=5
        )
        return len(
            [
                line
                for line in result.stdout.split("\n")
                if "spawn_main" in line or "multiprocessing-fork" in line
            ]
        )
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError):
        logger.exception("Failed to count hanging processes via 'ps aux'")
        return 0


def get_recap_processes() -> int:
    """Recap関連プロセス数を取得"""
    try:
        result = subprocess.run(
            ["ps", "aux"], capture_output=True, text=True, check=True, timeout=5
        )
        return len(
            [
                line
                for line in result.stdout.split("\n")
                if "recap" in line or "gunicorn" in line
            ]
        )
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError):
        logger.exception("Failed to count recap processes via 'ps aux'")
        return 0


def get_gpu_info() -> GpuInfo:
    """GPU使用状況を取得（nvidia-smiを使用）"""
    try:
        result = subprocess.run(
            ["nvidia-smi", "--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu,name", "--format=csv,noheader,nounits"],
            capture_output=True,
            text=True,
            check=True,
            timeout=5,
        )
        lines = result.stdout.strip().split("\n")
        if not lines or not lines[0]:
            return GpuInfo(available=False, gpus=())

        gpus = []
        for line in lines:
            if not line.strip():
                continue
            parts = [p.strip() for p in line.split(",")]
            if len(parts) >= 5:
                try:
                    memory_total = int(parts[2])
                    gpus.append(
                        GpuStat(
                            utilization=float(parts[0]),
                            memory_used=int(parts[1]),
                            memory_total=memory_total,
                            temperature=int(parts[3]),
                            name=parts[4],
                            memory_percent=round(
                                (int(parts[1]) / memory_total * 100) if memory_total > 0 else 0,
                                1,
                            ),
                        )
                    )
                except (ValueError, IndexError):
                    continue

        return GpuInfo(available=True, gpus=tuple(gpus), total_gpus=len(gpus))
    except FileNotFoundError:
        # nvidia-smiがインストールされていない
        return GpuInfo(available=False, gpus=(), error="nvidia-smi not found")
    except subprocess.TimeoutExpired:
        return GpuInfo(available=False, gpus=(), error="timeout")
    except subprocess.CalledProcessError as err:
        logger.exception("nvidia-smi returned a non-zero exit code")
        return GpuInfo(available=False, gpus=(), error=str(err))


def get_top_processes(limit: int = 5) -> list[ProcessInfo]:
    """メモリ使用量の多いプロセスを取得"""
    try:
        result = subprocess.run(
            ["ps", "aux", "--sort=-%mem"],
            capture_output=True,
            text=True,
            check=True,
            timeout=5,  # タイムアウトを追加
        )
        lines = result.stdout.strip().split("\n")
        if len(lines) < 2:  # ヘッダー行のみの場合
            return []

        # ヘッダーを除外して、指定された数のプロセスを取得
        process_lines = lines[1 : limit + 1]
        processes = []
        for line in process_lines:
            if not line.strip():
                continue
            parts = line.split(None, 10)  # 最大11個のフィールドに分割（コマンドは最後にまとめる）
            if len(parts) >= 11:
                try:
                    processes.append(
                        ProcessInfo(
                            user=parts[0],
                            pid=parts[1],
                            cpu=float(parts[2]),
                            mem=float(parts[3]),
                            rss=int(parts[5]) // 1024,  # KBをMBに変換
                            command=parts[10] if len(parts) > 10 else "",
                        )
                    )
                except (ValueError, IndexError):
                    # 個別のプロセス行のパースエラーは無視して続行
                    continue
        return processes
    except subprocess.TimeoutExpired:
        return []
    except (subprocess.CalledProcessError, FileNotFoundError):
        logger.exception("Failed to get top processes via 'ps aux --sort=-%mem'")
        return []
