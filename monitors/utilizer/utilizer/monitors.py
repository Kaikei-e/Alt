"""共通の監視機能"""

from __future__ import annotations

import subprocess
from typing import Any


def get_memory_info() -> dict[str, Any]:
    """メモリ使用状況を取得"""
    try:
        result = subprocess.run(
            ["free", "-g"], capture_output=True, text=True, check=True
        )
        lines = result.stdout.strip().split("\n")
        mem_line = [l for l in lines if l.startswith("Mem:")][0]
        parts = mem_line.split()
        total = int(parts[1])
        used = int(parts[2])
        available = int(parts[6])
        percent = (used / total * 100) if total > 0 else 0
        return {
            "total": total,
            "used": used,
            "available": available,
            "percent": round(percent, 1),
        }
    except Exception:
        return {"total": 0, "used": 0, "available": 0, "percent": 0}


def get_cpu_info() -> dict[str, Any]:
    """CPU使用率を取得"""
    try:
        import psutil

        percent = psutil.cpu_percent(interval=1)
        return {"percent": round(percent, 1)}
    except ImportError:
        # psutilがインストールされていない場合、/proc/statを使用
        try:
            with open("/proc/stat", "r") as f:
                lines = f.readlines()
                cpu_line = [l for l in lines if l.startswith("cpu ")][0]
                fields = cpu_line.split()
                total = sum(int(fields[i]) for i in range(1, len(fields)))
                idle = int(fields[4]) + (int(fields[5]) if len(fields) > 5 else 0)
                usage = round((1 - idle / total) * 100, 1) if total > 0 else 0.0
                return {"percent": usage}
        except Exception:
            # 最後の手段としてtopを使用
            try:
                result = subprocess.run(
                    ["top", "-bn2", "-d", "0.5"],
                    capture_output=True,
                    text=True,
                    check=True,
                    timeout=2,
                )
                cpu_lines = [l for l in result.stdout.split("\n") if "Cpu(s)" in l]
                if cpu_lines:
                    cpu_line = cpu_lines[-1]
                    import re

                    match = re.search(r"(\d+\.\d+)%id", cpu_line)
                    if match:
                        idle = float(match.group(1))
                        usage = round(100 - idle, 1)
                    else:
                        usage = 0.0
                else:
                    usage = 0.0
                return {"percent": usage}
            except Exception:
                return {"percent": 0.0}


def get_hanging_processes() -> int:
    """ハングプロセス数を取得"""
    try:
        result = subprocess.run(
            ["ps", "aux"], capture_output=True, text=True, check=True
        )
        count = len(
            [
                l
                for l in result.stdout.split("\n")
                if "spawn_main" in l or "multiprocessing-fork" in l
            ]
        )
        return max(0, count - 1)  # grep自身を除外
    except Exception:
        return 0


def get_recap_processes() -> int:
    """Recap関連プロセス数を取得"""
    try:
        result = subprocess.run(
            ["ps", "aux"], capture_output=True, text=True, check=True
        )
        count = len(
            [l for l in result.stdout.split("\n") if "recap" in l or "gunicorn" in l]
        )
        return max(0, count - 1)
    except Exception:
        return 0


def get_gpu_info() -> dict[str, Any]:
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
            return {"available": False, "gpus": []}

        gpus = []
        for line in lines:
            if not line.strip():
                continue
            parts = [p.strip() for p in line.split(",")]
            if len(parts) >= 5:
                try:
                    gpus.append({
                        "utilization": float(parts[0]),
                        "memory_used": int(parts[1]),
                        "memory_total": int(parts[2]),
                        "temperature": int(parts[3]),
                        "name": parts[4],
                        "memory_percent": round((int(parts[1]) / int(parts[2]) * 100) if int(parts[2]) > 0 else 0, 1),
                    })
                except (ValueError, IndexError):
                    continue

        return {
            "available": True,
            "gpus": gpus,
            "total_gpus": len(gpus),
        }
    except FileNotFoundError:
        # nvidia-smiがインストールされていない
        return {"available": False, "gpus": [], "error": "nvidia-smi not found"}
    except subprocess.TimeoutExpired:
        return {"available": False, "gpus": [], "error": "timeout"}
    except Exception as e:
        return {"available": False, "gpus": [], "error": str(e)}


def get_top_processes(limit: int = 5) -> list[dict[str, Any]]:
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
                        {
                            "user": parts[0],
                            "pid": parts[1],
                            "cpu": float(parts[2]),
                            "mem": float(parts[3]),
                            "rss": int(parts[5]) // 1024,  # KBをMBに変換
                            "command": parts[10] if len(parts) > 10 else "",
                        }
                    )
                except (ValueError, IndexError) as e:
                    # 個別のプロセス行のパースエラーは無視して続行
                    continue
        return processes
    except subprocess.TimeoutExpired:
        return []
    except Exception as e:
        # エラーをログに記録（本番環境では適切なロガーを使用）
        print(f"Error getting top processes: {e}", flush=True)
        return []
