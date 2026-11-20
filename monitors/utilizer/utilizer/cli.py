"""ターミナル版監視CLI"""

from __future__ import annotations

import argparse
import sys
import time
from contextlib import ExitStack
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from .monitors import (
    CpuInfo,
    GpuInfo,
    MemoryInfo,
    ProcessInfo,
    get_cpu_info,
    get_gpu_info,
    get_hanging_processes,
    get_memory_info,
    get_recap_processes,
    get_top_processes,
)

# カラー定義
RED = "\033[0;31m"
GREEN = "\033[0;32m"
YELLOW = "\033[1;33m"
BLUE = "\033[0;34m"
NC = "\033[0m"  # No Color

_HIGH_MEMORY_PERCENT = 90
_HIGH_CPU_PERCENT = 90
_HIGH_HANGING_PROCESS_COUNT = 10
_TOP_PROCESS_LIMIT = 5
_MIB_PER_GIB = 1024
_COMMAND_MAX_CHARS = 60


def _local_now() -> datetime:
    return datetime.now(tz=UTC).astimezone()


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Recap Job リソース監視")
    parser.add_argument("-i", "--interval", type=float, default=2.0, help="監視間隔（秒）")
    parser.add_argument("-l", "--log", type=str, default="", help="ログファイルパス（CSV形式）")
    return parser.parse_args()


def _write_header(interval: float, log_path: str) -> None:
    sys.stdout.write("\033[2J\033[H")
    sys.stdout.write(f"{BLUE}=== Recap Job リソース監視 ==={NC}\n")
    sys.stdout.write(f"監視間隔: {interval}秒\n")
    sys.stdout.write(f"開始時刻: {_local_now().strftime('%Y-%m-%d %H:%M:%S')}\n")
    if log_path:
        sys.stdout.write(f"ログファイル: {log_path}\n")
    sys.stdout.write("\n")


def _format_mem_delta(current_used: int, prev_used: int) -> str:
    if prev_used <= 0:
        return ""
    mem_delta = current_used - prev_used
    if mem_delta > 0:
        return f"{RED}+{mem_delta}GB{NC}"
    if mem_delta < 0:
        return f"{GREEN}{mem_delta}GB{NC}"
    return f"{NC}±0GB{NC}"


def _format_cpu_delta(current_percent: float, prev_percent: float) -> str:
    if prev_percent <= 0:
        return ""
    cpu_delta = current_percent - prev_percent
    if cpu_delta > 0:
        return f"{RED}+{cpu_delta:.1f}%{NC}"
    if cpu_delta < 0:
        return f"{GREEN}{cpu_delta:.1f}%{NC}"
    return f"{NC}±0%{NC}"


def _collect_alerts(
    mem_info: MemoryInfo,
    cpu_info: CpuInfo,
    hanging_count: int,
) -> list[str]:
    alerts = []
    if mem_info.percent > _HIGH_MEMORY_PERCENT:
        alerts.append(f"{RED}⚠ メモリ使用率が高いです！{NC}")
    if hanging_count > _HIGH_HANGING_PROCESS_COUNT:
        alerts.append(f"{RED}⚠ ハングプロセスが多すぎます！{NC}")
    if cpu_info.percent > _HIGH_CPU_PERCENT:
        alerts.append(f"{RED}⚠ CPU使用率が高いです！{NC}")
    return alerts


@dataclass(frozen=True, slots=True)
class _FrameSnapshot:
    timestamp: str
    mem_info: MemoryInfo
    cpu_info: CpuInfo
    gpu_info: GpuInfo
    hanging_count: int
    recap_processes: int
    top_processes: list[ProcessInfo]
    mem_delta_str: str
    cpu_delta_str: str
    alerts: list[str]


def _render_frame(frame: _FrameSnapshot) -> None:
    # 画面クリア（最初の3行を保持）
    sys.stdout.write("\033[4H\033[J")

    # 情報表示
    sys.stdout.write(f"{BLUE}時刻: {NC}{frame.timestamp}\n")
    sys.stdout.write(
        f"{GREEN}メモリ: {NC}{frame.mem_info.used}GB / {frame.mem_info.total}GB "
        f"({frame.mem_info.percent}%) {frame.mem_delta_str}\n"
    )
    sys.stdout.write(f"{GREEN}利用可能: {NC}{frame.mem_info.available}GB\n")
    sys.stdout.write(f"{YELLOW}CPU使用率: {NC}{frame.cpu_info.percent}% {frame.cpu_delta_str}\n")
    # GPU情報表示
    if frame.gpu_info.available and frame.gpu_info.gpus:
        for idx, gpu in enumerate(frame.gpu_info.gpus):
            sys.stdout.write(
                f"{YELLOW}GPU {idx} ({gpu.name}): {NC}使用率 {gpu.utilization}% | "
                f"メモリ {gpu.memory_percent}% ({gpu.memory_used // _MIB_PER_GIB}GB/{gpu.memory_total // _MIB_PER_GIB}GB) | "
                f"温度 {gpu.temperature}°C\n"
            )
    sys.stdout.write(f"{YELLOW}ハングプロセス: {NC}{frame.hanging_count}個\n")
    sys.stdout.write(f"{YELLOW}Recap関連プロセス: {NC}{frame.recap_processes}個\n")
    if frame.alerts:
        sys.stdout.write(f"{' '.join(frame.alerts)}\n")
    sys.stdout.write("\n")
    sys.stdout.write(f"{BLUE}--- トップ5プロセス（メモリ使用量） ---{NC}\n")
    for proc in frame.top_processes:
        if (
            "recap" in proc.command.lower()
            or "gunicorn" in proc.command.lower()
            or "spawn_main" in proc.command
        ):
            sys.stdout.write(
                f"{proc.user:8s} {proc.cpu:6.1f}% {proc.rss:8d}MB {proc.command[:_COMMAND_MAX_CHARS]}\n"
            )


def main() -> None:
    """メインループ"""
    args = _parse_args()

    # 前回の値を保存
    prev_mem = 0
    prev_cpu = 0.0

    with ExitStack() as stack:
        # ログファイルの初期化
        log_file = None
        if args.log:
            log_file = stack.enter_context(Path(args.log).open("w"))
            log_file.write(
                "時刻,メモリ使用(GB),メモリ使用率(%),CPU使用率(%),ハングプロセス数,総プロセス数\n"
            )
            log_file.flush()

        _write_header(args.interval, args.log)

        try:
            while True:
                timestamp = _local_now().strftime("%H:%M:%S")

                # 情報取得
                mem_info = get_memory_info()
                cpu_info = get_cpu_info()
                gpu_info = get_gpu_info()
                hanging_count = get_hanging_processes()
                recap_processes = get_recap_processes()
                top_processes = get_top_processes(_TOP_PROCESS_LIMIT)

                # 変化率計算
                mem_delta_str = _format_mem_delta(mem_info.used, prev_mem)
                cpu_delta_str = _format_cpu_delta(cpu_info.percent, prev_cpu)

                # アラート判定
                alerts = _collect_alerts(mem_info, cpu_info, hanging_count)

                _render_frame(
                    _FrameSnapshot(
                        timestamp=timestamp,
                        mem_info=mem_info,
                        cpu_info=cpu_info,
                        gpu_info=gpu_info,
                        hanging_count=hanging_count,
                        recap_processes=recap_processes,
                        top_processes=top_processes,
                        mem_delta_str=mem_delta_str,
                        cpu_delta_str=cpu_delta_str,
                        alerts=alerts,
                    )
                )

                # ログファイルに記録
                if log_file:
                    log_file.write(
                        f"{_local_now().strftime('%Y-%m-%d %H:%M:%S')},{mem_info.used},{mem_info.percent},{cpu_info.percent},{hanging_count},{recap_processes}\n"
                    )
                    log_file.flush()

                # 前回の値を更新
                prev_mem = mem_info.used
                prev_cpu = cpu_info.percent

                time.sleep(args.interval)

        except KeyboardInterrupt:
            sys.stdout.write(f"\n{YELLOW}監視を終了します...{NC}\n")


if __name__ == "__main__":
    main()
