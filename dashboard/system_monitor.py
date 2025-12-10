"""
System Monitoring Module
Refactored to use subprocess-based commands (ps, free, nvidia-smi) instead of psutil
for better reliability in containerized environments.
Adapted from: monitors/utilizer/utilizer/monitors.py
"""

import subprocess
import shutil
import time
import re
from typing import Dict, List, Any

def get_memory_info() -> Dict[str, Any]:
    """
    Get memory usage statistics using 'free -b'.
    Returns bytes to ensure compatibility with dashboard/app.py.
    """
    try:
        # Use -b for bytes
        result = subprocess.run(
            ["free", "-b"],
            capture_output=True,
            text=True,
            check=True,
            timeout=2
        )
        lines = result.stdout.strip().split("\n")
        # Header:              total        used        free      shared  buff/cache   available
        # Mem:           67252326400  18317783040 12053950464    23134208 36880592896 48067821568
        mem_line = [l for l in lines if l.startswith("Mem:")][0]
        parts = mem_line.split()
        total = int(parts[1])
        used = int(parts[2])
        # available is usually the last column in modern 'free'
        available = int(parts[-1])

        # Calculate percent
        percent = (used / total * 100) if total > 0 else 0.0

        return {
            "total": total,
            "used": used,
            "available": available,
            "percent": round(percent, 1)
        }
    except Exception as e:
        print(f"Error getting memory info: {e}")
        return {"total": 0, "used": 0, "available": 0, "percent": 0.0}

def get_cpu_info() -> Dict[str, float]:
    """Get CPU usage statistics"""
    # 1. Try psutil (preferred if works)
    try:
        import psutil
        percent = psutil.cpu_percent(interval=1)
        return {"percent": round(percent, 1)}
    except ImportError:
        pass
    except Exception:
        pass

    # 2. Try /proc/stat
    try:
        with open("/proc/stat", "r") as f:
            lines = f.readlines()
            cpu_line = [l for l in lines if l.startswith("cpu ")][0]
            fields = cpu_line.split()
            # fields[0] is 'cpu'
            # user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
            values = [int(f) for f in fields[1:]]
            total_time = sum(values)
            idle_time = values[3]
            # Handle Linux 2.6+ iowait as idle
            if len(values) > 4:
                idle_time += values[4]

            usage = 0.0
            if total_time > 0:
                usage = round((1 - idle_time / total_time) * 100, 1)
            return {"percent": usage}
    except Exception:
        pass

    # 3. Try top
    try:
        result = subprocess.run(
            ["top", "-bn2", "-d", "0.5"],
            capture_output=True,
            text=True,
            check=True,
            timeout=3,
        )
        cpu_lines = [l for l in result.stdout.split("\n") if "Cpu(s)" in l]
        if cpu_lines:
            cpu_line = cpu_lines[-1]
            match = re.search(r"(\d+\.\d+)\s*id", cpu_line)
            if match:
                idle = float(match.group(1))
                usage = round(100 - idle, 1)
                return {"percent": usage}
    except Exception:
        pass

    return {"percent": 0.0}

def get_gpu_info() -> Dict[str, Any]:
    """Get GPU info using nvidia-smi"""
    try:
        # Added 'index' to query to match previous system_monitor.py interface
        result = subprocess.run(
            ["nvidia-smi", "--query-gpu=index,utilization.gpu,memory.used,memory.total,temperature.gpu,name", "--format=csv,noheader,nounits"],
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
            if len(parts) >= 6:
                try:
                    mem_used = int(parts[2])
                    mem_total = int(parts[3])
                    mem_percent = round((mem_used / mem_total * 100) if mem_total > 0 else 0, 1)

                    gpus.append({
                        "index": parts[0],
                        "utilization": float(parts[1]),
                        "memory_used": float(mem_used),
                        "memory_total": float(mem_total),
                        "temperature": int(parts[4]),
                        "name": parts[5],
                        "memory_percent": mem_percent,
                    })
                except (ValueError, IndexError):
                    continue

        return {
            "available": True,
            "gpus": gpus,
            # 'total_gpus': len(gpus) # optional
        }
    except FileNotFoundError:
        return {"available": False, "gpus": [], "error": "nvidia-smi not found", "message": "nvidia-smi command not found. GPU may not be available or NVIDIA drivers not installed."}
    except subprocess.TimeoutExpired:
        return {"available": False, "gpus": [], "error": "timeout", "message": "nvidia-smi command timed out."}
    except subprocess.CalledProcessError as e:
        return {"available": False, "gpus": [], "error": f"nvidia-smi failed: {e}", "message": f"nvidia-smi command failed (exit code {e.returncode}). GPU may not be accessible."}
    except Exception as e:
        return {"available": False, "gpus": [], "error": str(e), "message": f"Error getting GPU info: {str(e)}"}

def count_hanging_processes() -> int:
    """Count potential hanging processes"""
    try:
        result = subprocess.run(
            ["ps", "aux"], capture_output=True, text=True, check=True
        )
        count = 0
        for l in result.stdout.split("\n"):
            if "spawn_main" in l or ("multiprocessing" in l and "fork" in l):
                count += 1

        # Subtract grep itself if it appears (though we scan lines, grep is usually not in output of ps aux unless piped)
        # But safest is to just count.
        # Adjusted logic: 'ps aux' shows all processes.
        return max(0, count)
    except Exception:
        return 0

def get_top_processes(limit: int = 15) -> List[Dict[str, Any]]:
    """Get top N processes by CPU usage"""
    try:
        result = subprocess.run(
            ["ps", "aux", "--sort=-%cpu"],
            capture_output=True,
            text=True,
            check=True,
            timeout=5,
        )
        lines = result.stdout.strip().split("\n")
        if len(lines) < 2:
            return []

        # lines[0] is header
        process_lines = lines[1 : limit + 1]
        processes = []
        for line in process_lines:
            if not line.strip():
                continue
            # USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
            parts = line.split(None, 10)
            if len(parts) >= 11:
                try:
                    # Mapping to interface expected by app.py:
                    # 'pid', 'name', 'memory_mb', 'cpu_percent'
                    cmd = parts[10] if len(parts) > 10 else "unknown"

                    processes.append({
                        "pid": parts[1], # string is fine, app.py uses st.dataframe
                        "name": cmd,     # Use full command or first part? app.py just displays it.
                        "cpu_percent": float(parts[2]),
                        "memory_mb": int(parts[5]) / 1024, # RSS in KB -> MB
                    })
                except (ValueError, IndexError):
                    continue
        return processes
    except subprocess.TimeoutExpired:
        print(f"Error getting top processes: ps command timed out after 5 seconds")
        return []
    except subprocess.CalledProcessError as e:
        print(f"Error getting top processes: ps command failed with exit code {e.returncode}")
        return []
    except Exception as e:
        print(f"Error getting top processes: {e}")
        return []
