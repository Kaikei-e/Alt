"""ターミナル版監視CLI"""

from __future__ import annotations

import os
import sys
import time
from datetime import datetime

from .monitors import (
    get_cpu_info,
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


def main() -> None:
    """メインループ"""
    import argparse

    parser = argparse.ArgumentParser(description="Recap Job リソース監視")
    parser.add_argument(
        "-i", "--interval", type=float, default=2.0, help="監視間隔（秒）"
    )
    parser.add_argument(
        "-l", "--log", type=str, default="", help="ログファイルパス（CSV形式）"
    )
    args = parser.parse_args()

    # ログファイルの初期化
    log_file = None
    if args.log:
        log_file = open(args.log, "w")
        log_file.write("時刻,メモリ使用(GB),メモリ使用率(%),CPU使用率(%),ハングプロセス数,総プロセス数\n")
        log_file.flush()

    # 前回の値を保存
    prev_mem = 0
    prev_cpu = 0.0

    # ヘッダー表示
    os.system("clear")
    print(f"{BLUE}=== Recap Job リソース監視 ==={NC}")
    print(f"監視間隔: {args.interval}秒")
    print(f"開始時刻: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    if log_file:
        print(f"ログファイル: {args.log}")
    print()

    try:
        while True:
            timestamp = datetime.now().strftime("%H:%M:%S")

            # 情報取得
            mem_info = get_memory_info()
            cpu_info = get_cpu_info()
            hanging_count = get_hanging_processes()
            recap_processes = get_recap_processes()
            top_processes = get_top_processes(5)

            # 変化率計算
            mem_delta_str = ""
            if prev_mem > 0:
                mem_delta = mem_info["used"] - prev_mem
                if mem_delta > 0:
                    mem_delta_str = f"{RED}+{mem_delta}GB{NC}"
                elif mem_delta < 0:
                    mem_delta_str = f"{GREEN}{mem_delta}GB{NC}"
                else:
                    mem_delta_str = f"{NC}±0GB{NC}"

            cpu_delta_str = ""
            if prev_cpu > 0:
                cpu_delta = cpu_info["percent"] - prev_cpu
                if cpu_delta > 0:
                    cpu_delta_str = f"{RED}+{cpu_delta:.1f}%{NC}"
                elif cpu_delta < 0:
                    cpu_delta_str = f"{GREEN}{cpu_delta:.1f}%{NC}"
                else:
                    cpu_delta_str = f"{NC}±0%{NC}"

            # アラート判定
            alerts = []
            if mem_info["percent"] > 90:
                alerts.append(f"{RED}⚠ メモリ使用率が高いです！{NC}")
            if hanging_count > 10:
                alerts.append(f"{RED}⚠ ハングプロセスが多すぎます！{NC}")
            if cpu_info["percent"] > 90:
                alerts.append(f"{RED}⚠ CPU使用率が高いです！{NC}")

            # 画面クリア（最初の3行を保持）
            print(f"\033[4H\033[J", end="")

            # 情報表示
            print(f"{BLUE}時刻: {NC}{timestamp}")
            print(
                f"{GREEN}メモリ: {NC}{mem_info['used']}GB / {mem_info['total']}GB ({mem_info['percent']}%) {mem_delta_str}"
            )
            print(f"{GREEN}利用可能: {NC}{mem_info['available']}GB")
            print(
                f"{YELLOW}CPU使用率: {NC}{cpu_info['percent']}% {cpu_delta_str}"
            )
            print(f"{YELLOW}ハングプロセス: {NC}{hanging_count}個")
            print(f"{YELLOW}Recap関連プロセス: {NC}{recap_processes}個")
            if alerts:
                print(" ".join(alerts))
            print()
            print(f"{BLUE}--- トップ5プロセス（メモリ使用量） ---{NC}")
            for proc in top_processes:
                if "recap" in proc["command"].lower() or "gunicorn" in proc["command"].lower() or "spawn_main" in proc["command"]:
                    print(
                        f"{proc['user']:8s} {proc['cpu']:6.1f}% {proc['rss']:8d}MB {proc['command'][:60]}"
                    )

            # ログファイルに記録
            if log_file:
                log_file.write(
                    f"{datetime.now().strftime('%Y-%m-%d %H:%M:%S')},{mem_info['used']},{mem_info['percent']},{cpu_info['percent']},{hanging_count},{recap_processes}\n"
                )
                log_file.flush()

            # 前回の値を更新
            prev_mem = mem_info["used"]
            prev_cpu = cpu_info["percent"]

            time.sleep(args.interval)

    except KeyboardInterrupt:
        print(f"\n{YELLOW}監視を終了します...{NC}")
    finally:
        if log_file:
            log_file.close()


if __name__ == "__main__":
    main()
