"""Benchmark harness for Irodori-TTS GPU memory and Real-Time Factor (RTF)."""

from __future__ import annotations

import argparse
import contextlib
import http.client
import io
import json
import re
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import wave
from datetime import datetime, timezone
from pathlib import Path


class SetupError(Exception):
    """Raised when environment or arguments fail validation."""


def calculate_percentile(data: list[float], p: float) -> float:
    """Calculate the p-th percentile of data using linear interpolation.

    Matches numpy default (method='linear').
    """
    if not data:
        return 0.0
    if len(data) == 1:
        return float(data[0])

    sorted_data = sorted(data)
    k = (p / 100.0) * (len(sorted_data) - 1)
    idx = int(k)
    fraction = k - idx
    if idx >= len(sorted_data) - 1:
        return float(sorted_data[-1])
    return float(
        sorted_data[idx] + fraction * (sorted_data[idx + 1] - sorted_data[idx])
    )


def calculate_rtf(wall_time: float, audio_seconds: float) -> float:
    """Calculate Real-Time Factor (wall_time / audio_seconds)."""
    if audio_seconds <= 0.0:
        return 0.0
    return wall_time / audio_seconds


def get_wav_duration_seconds(wav_bytes: bytes) -> float:
    """Extract audio duration in seconds from WAV byte stream."""
    try:
        with wave.open(io.BytesIO(wav_bytes), "rb") as wf:
            frames = wf.getnframes()
            rate = wf.getframerate()
            if rate <= 0:
                return 0.0
            return frames / float(rate)
    except (wave.Error, EOFError, OSError):
        return 0.0


def format_speed(speed: float) -> str:
    """Format speed value like 1.0, 1.5, 2.0."""
    formatted = f"{float(speed):.4f}".rstrip("0")
    if formatted.endswith("."):
        formatted += "0"
    return formatted


def parse_compute_apps_csv(output: str) -> dict[int, float]:
    """Parse nvidia-smi --query-compute-apps=pid,used_memory --format=csv,noheader,nounits.

    Returns mapping of pid -> total used_memory in MiB across GPUs.
    Handles '[N/A]', empty lines, and duplicate PIDs.
    """
    result: dict[int, float] = {}
    if not output:
        return result

    for line in output.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = [p.strip() for p in line.split(",")]
        if len(parts) < 2:
            continue
        pid_str, mem_str = parts[0], parts[1]
        if pid_str == "[N/A]" or mem_str == "[N/A]":
            continue
        try:
            pid = int(pid_str)
            mem = float(mem_str)
            result[pid] = result.get(pid, 0.0) + mem
        except ValueError:
            continue
    return result


def parse_gpu_used_csv(output: str) -> float:
    """Parse nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits.

    Returns sum of GPU used memory in MiB across all visible GPUs.
    """
    total = 0.0
    if not output:
        return total

    for line in output.splitlines():
        line = line.strip()
        if not line or line.startswith("#") or line == "[N/A]":
            continue
        try:
            total += float(line)
        except ValueError:
            continue
    return total


def check_nvidia_smi(runner=subprocess.run) -> bool:
    """Verify that nvidia-smi is available and executable."""
    try:
        proc = runner(
            ["nvidia-smi", "-L"],
            capture_output=True,
            text=True,
            check=False,
            timeout=5,
        )
        return proc.returncode == 0
    except (subprocess.SubprocessError, OSError):
        return False


def resolve_container_pid(
    container_name: str,
    pid_arg: int | None = None,
    runner=subprocess.run,
) -> int:
    """Resolve target host PID either from direct argument or docker inspect."""
    if pid_arg is not None:
        if pid_arg <= 0:
            raise SetupError(f"PID must be positive integer, got: {pid_arg}")
        return pid_arg

    cmd = ["docker", "inspect", "-f", "{{.State.Pid}}", container_name]
    try:
        proc = runner(cmd, capture_output=True, text=True, check=False, timeout=5)
    except (subprocess.SubprocessError, OSError) as e:
        raise SetupError(
            f"Failed to inspect docker container '{container_name}': {e}"
        ) from e

    if proc.returncode != 0:
        err = proc.stderr.strip() if proc.stderr else f"exit code {proc.returncode}"
        raise SetupError(f"docker inspect failed for '{container_name}': {err}")

    raw_pid = proc.stdout.strip()
    if not raw_pid:
        raise SetupError(f"docker inspect returned empty PID for '{container_name}'")

    try:
        pid = int(raw_pid)
    except ValueError:
        raise SetupError(
            f"docker inspect returned non-integer PID '{raw_pid}' for"
            f" '{container_name}'"
        )

    if pid <= 0:
        raise SetupError(f"Container '{container_name}' is not running (PID={pid})")
    return pid


def run_query_compute_apps(runner=subprocess.run) -> str:
    """Execute nvidia-smi compute-apps query with timeout."""
    cmd = [
        "nvidia-smi",
        "--query-compute-apps=pid,used_memory",
        "--format=csv,noheader,nounits",
    ]
    proc = runner(cmd, capture_output=True, text=True, check=False, timeout=5)
    return proc.stdout if proc.returncode == 0 else ""


def run_query_gpu_used(runner=subprocess.run) -> str:
    """Execute nvidia-smi total GPU used query with timeout."""
    cmd = [
        "nvidia-smi",
        "--query-gpu=memory.used",
        "--format=csv,noheader,nounits",
    ]
    proc = runner(cmd, capture_output=True, text=True, check=False, timeout=5)
    return proc.stdout if proc.returncode == 0 else ""


def start_vram_sampler(
    pid: int,
    interval_ms: int = 100,
    query_apps_fn=None,
    query_gpu_fn=None,
):
    """Start background thread sampling GPU memory periodically.

    Returns stop_fn which terminates sampler and returns:
    (peak_proc_mib, peak_gpu_mib, sample_count, pid_hits)
    """
    q_apps = query_apps_fn or run_query_compute_apps
    q_gpu = query_gpu_fn or run_query_gpu_used
    interval_s = max(0.01, interval_ms / 1000.0)

    stop_event = threading.Event()
    stats = {
        "peak_proc_mib": 0.0,
        "peak_gpu_mib": 0.0,
        "sample_count": 0,
        "pid_hits": 0,
    }
    lock = threading.Lock()

    def sample_step():
        with lock:
            stats["sample_count"] += 1
        with contextlib.suppress(
            subprocess.SubprocessError, OSError, ValueError, TimeoutError
        ):
            apps_csv = q_apps()
            apps = parse_compute_apps_csv(apps_csv)
            gpu_csv = q_gpu()
            gpu_mem = parse_gpu_used_csv(gpu_csv)

            with lock:
                if pid in apps:
                    stats["pid_hits"] += 1
                    proc_mem = apps[pid]
                    stats["peak_proc_mib"] = max(stats["peak_proc_mib"], proc_mem)
                stats["peak_gpu_mib"] = max(stats["peak_gpu_mib"], gpu_mem)

    def worker():
        sample_step()
        while not stop_event.wait(interval_s):
            sample_step()

    thread = threading.Thread(target=worker, daemon=True)
    thread.start()

    def stop():
        stop_event.set()
        thread.join(timeout=2.0)
        sample_step()
        with lock:
            return (
                stats["peak_proc_mib"],
                stats["peak_gpu_mib"],
                stats["sample_count"],
                stats["pid_hits"],
            )

    return stop


def default_http_request(
    url: str,
    method: str = "GET",
    headers: dict[str, str] | None = None,
    data: bytes | None = None,
    timeout: float = 60.0,
) -> tuple[int, bytes, dict[str, str]]:
    """Standard library HTTP request implementation."""
    req = urllib.request.Request(url, data=data, headers=headers or {}, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            status = resp.status
            body = resp.read()
            resp_headers = {k.lower(): v for k, v in resp.headers.items()}
            return status, body, resp_headers
    except urllib.error.HTTPError as e:
        body = e.read()
        resp_headers = {k.lower(): v for k, v in e.headers.items()}
        return e.code, body, resp_headers
    except urllib.error.URLError as e:
        return 0, str(e.reason).encode("utf-8"), {}
    except (http.client.HTTPException, OSError, TimeoutError) as e:
        return 0, str(e).encode("utf-8"), {}


def send_speech_request(
    base_url: str,
    api_key: str,
    text: str,
    voice: str,
    speed: float = 1.0,
    seed: int | None = None,
    http_fn=None,
    timeout: float = 120.0,
) -> tuple[int, bytes, float]:
    """Send POST /v1/audio/speech request and measure elapsed wall time."""
    fn = http_fn or default_http_request
    url = f"{base_url.rstrip('/')}/v1/audio/speech"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    irodori_cfg: dict[str, int | bool] = {"chunking_enabled": False}
    if seed is not None:
        irodori_cfg["seed"] = seed

    payload = {
        "model": "irodori-tts",
        "input": text,
        "voice": voice,
        "speed": speed,
        "response_format": "wav",
        "irodori": irodori_cfg,
    }
    data = json.dumps(payload).encode("utf-8")

    t0 = time.perf_counter()
    status, body, _ = fn(
        url, method="POST", headers=headers, data=data, timeout=timeout
    )
    wall_time = time.perf_counter() - t0
    return status, body, wall_time


def reset_cuda_memory_probe(
    base_url: str,
    api_key: str,
    http_fn=None,
    timeout: float = 10.0,
) -> tuple[int, dict]:
    """POST /internal/cuda-memory/reset-peak."""
    fn = http_fn or default_http_request
    url = f"{base_url.rstrip('/')}/internal/cuda-memory/reset-peak"
    headers = {"Authorization": f"Bearer {api_key}"}
    status, body, _ = fn(url, method="POST", headers=headers, timeout=timeout)
    data = {}
    if body:
        with contextlib.suppress(json.JSONDecodeError, UnicodeDecodeError):
            data = json.loads(body.decode("utf-8"))
    return status, data


def get_cuda_memory_probe(
    base_url: str,
    api_key: str,
    http_fn=None,
    timeout: float = 10.0,
) -> tuple[int, dict]:
    """GET /internal/cuda-memory."""
    fn = http_fn or default_http_request
    url = f"{base_url.rstrip('/')}/internal/cuda-memory"
    headers = {"Authorization": f"Bearer {api_key}"}
    status, body, _ = fn(url, method="GET", headers=headers, timeout=timeout)
    data = {}
    if body:
        with contextlib.suppress(json.JSONDecodeError, UnicodeDecodeError):
            data = json.loads(body.decode("utf-8"))
    return status, data


def evaluate_gates(
    effective_peak_mib: float | None,
    budget_mib: float,
    budget_ratio: float,
    rtf_p95: float | None,
    rtf_max: float,
    failures: int,
    *,
    pid_hits: int,
) -> tuple[bool, bool, bool]:
    """Evaluate budget (on effective_peak_mib) and RTF pass/fail conditions."""
    budget_limit = budget_mib * budget_ratio
    budget_pass = (
        (effective_peak_mib is not None)
        and (effective_peak_mib <= budget_limit)
        and (failures == 0)
        and (pid_hits > 0)
    )
    rtf_pass = (rtf_p95 is not None) and (rtf_p95 <= rtf_max) and (failures == 0)
    all_passed = budget_pass and rtf_pass
    return budget_pass, rtf_pass, all_passed


def build_result_dict(
    label: str,
    timestamp_utc: str,
    requested: int,
    completed: int,
    warmup: int,
    sample_count: int,
    pid_hits: int,
    failures: int,
    failure_detail: dict | None,
    rtf_stats: dict[str, float | None],
    latency_stats: dict[str, float | None],
    peak_process_mib: float,
    peak_reserved_mib: float | None,
    peak_allocated_mib: float | None,
    context_overhead_mib: float,
    effective_peak_mib: float | None,
    peak_total_gpu_used_mib: float,
    budget_mib: float,
    budget_ratio: float,
    rtf_max: float,
    speed: float = 1.0,
) -> dict:
    """Build unified benchmark result dictionary."""
    budget_limit_mib = budget_mib * budget_ratio
    budget_pass, rtf_pass, all_passed = evaluate_gates(
        effective_peak_mib=effective_peak_mib,
        budget_mib=budget_mib,
        budget_ratio=budget_ratio,
        rtf_p95=rtf_stats.get("p95"),
        rtf_max=rtf_max,
        failures=failures,
        pid_hits=pid_hits,
    )

    return {
        "label": label,
        "speed": speed,
        "timestamp_utc": timestamp_utc,
        "counts": {
            "requested": requested,
            "completed": completed,
            "warmup": warmup,
        },
        "sample_count": sample_count,
        "pid_hits": pid_hits,
        "failures": failures,
        "failure_detail": failure_detail,
        "rtf": {
            "mean": round(rtf_stats["mean"], 4)
            if rtf_stats.get("mean") is not None
            else None,
            "p50": round(rtf_stats["p50"], 4)
            if rtf_stats.get("p50") is not None
            else None,
            "p95": round(rtf_stats["p95"], 4)
            if rtf_stats.get("p95") is not None
            else None,
            "max": round(rtf_stats["max"], 4)
            if rtf_stats.get("max") is not None
            else None,
        },
        "latency_seconds": {
            "p50": round(latency_stats["p50"], 4)
            if latency_stats.get("p50") is not None
            else None,
            "p95": round(latency_stats["p95"], 4)
            if latency_stats.get("p95") is not None
            else None,
        },
        "peak_process_mib": round(peak_process_mib, 2),
        "peak_reserved_mib": round(peak_reserved_mib, 2)
        if peak_reserved_mib is not None
        else None,
        "peak_allocated_mib": round(peak_allocated_mib, 2)
        if peak_allocated_mib is not None
        else None,
        "context_overhead_mib": round(context_overhead_mib, 2),
        "effective_peak_mib": round(effective_peak_mib, 2)
        if effective_peak_mib is not None
        else None,
        "peak_total_gpu_used_mib": round(peak_total_gpu_used_mib, 2),
        "budget_mib": budget_mib,
        "budget_ratio": budget_ratio,
        "budget_limit_mib": round(budget_limit_mib, 2),
        "budget_pass": budget_pass,
        "rtf_max": rtf_max,
        "rtf_pass": rtf_pass,
        "all_passed": all_passed,
    }


def format_summary(result: dict) -> str:
    """Format single-screen summary of benchmark results."""
    label = result.get("label", "unknown")
    counts = result.get("counts", {})
    completed = counts.get("completed", 0)
    requested = counts.get("requested", 0)
    warmup = counts.get("warmup", 0)
    failures = result.get("failures", 0)
    sample_count = result.get("sample_count", 0)
    pid_hits = result.get("pid_hits", 0)

    rtf = result.get("rtf", {})
    r_mean = f"{rtf['mean']:.4f}" if rtf.get("mean") is not None else "N/A"
    r_p50 = f"{rtf['p50']:.4f}" if rtf.get("p50") is not None else "N/A"
    r_p95 = f"{rtf['p95']:.4f}" if rtf.get("p95") is not None else "N/A"
    r_max = f"{rtf['max']:.4f}" if rtf.get("max") is not None else "N/A"

    lat = result.get("latency_seconds", {})
    l_p50 = f"{lat['p50']:.3f}s" if lat.get("p50") is not None else "N/A"
    l_p95 = f"{lat['p95']:.3f}s" if lat.get("p95") is not None else "N/A"

    peak_proc = result.get("peak_process_mib", 0.0)
    alloc_mib = result.get("peak_allocated_mib")
    alloc_str = f"{alloc_mib:.1f} MiB" if isinstance(alloc_mib, (int, float)) else "N/A"
    res_mib = result.get("peak_reserved_mib")
    res_str = f"{res_mib:.1f} MiB" if isinstance(res_mib, (int, float)) else "N/A"
    overhead_mib = result.get("context_overhead_mib", 0.0)
    eff_peak = result.get("effective_peak_mib")
    eff_str = f"{eff_peak:.1f} MiB" if isinstance(eff_peak, (int, float)) else "N/A"
    gpu_total = result.get("peak_total_gpu_used_mib", 0.0)

    budget_lim = result.get("budget_limit_mib", 0.0)
    budget_mib = result.get("budget_mib", 0.0)
    budget_rat = result.get("budget_ratio", 0.0)

    b_pass = "PASS" if result.get("budget_pass") else "FAIL"
    r_pass = "PASS" if result.get("rtf_pass") else "FAIL"
    o_pass = "PASS" if result.get("all_passed") else "FAIL"

    speed = result.get("speed", 1.0)
    speed_str = (
        f"{format_speed(speed)}x" if isinstance(speed, (int, float)) else str(speed)
    )

    lines = [
        "=" * 66,
        f"Irodori-TTS Benchmark Summary: {label}",
        "=" * 66,
        f"Speed:                 {speed_str}",
        (
            f"Requests: {completed}/{requested} completed, {failures} failures"
            f" ({warmup} warmup)"
        ),
        f"Telemetry: {sample_count} samples, {pid_hits} pid hits",
        f"Latency:  p50 = {l_p50}, p95 = {l_p95}",
        f"RTF:      mean = {r_mean}, p50 = {r_p50}, p95 = {r_p95}, max = {r_max}",
        f"VRAM Host Peak:        {peak_proc:.1f} MiB",
        (f"VRAM Probe Peak:       Allocated = {alloc_str}, Reserved = {res_str}"),
        f"Context Overhead:      {overhead_mib:.1f} MiB",
        (
            f"Effective Peak:        {eff_str}"
            f" (Limit: {budget_lim:.1f} MiB = {budget_mib:.0f} * {budget_rat:.2f})"
        ),
        f"GPU Peak Total Used:   {gpu_total:.1f} MiB",
        "-" * 66,
        (
            "Gate: Budget Pass (Effective Peak <="
            f" {budget_lim:.1f} MiB & 0 fail): {b_pass}"
        ),
        (
            "Gate: RTF Pass (RTF p95 <="
            f" {result.get('rtf_max', 1.0):.2f} & 0 fail):                 {r_pass}"
        ),
        f"Overall Outcome:                                           {o_pass}",
        "=" * 66,
    ]
    return "\n".join(lines)


def run_benchmark(
    label: str,
    base_url: str,
    api_key: str,
    voice: str,
    input_lines: list[str],
    budget_mib: float,
    count: int = 100,
    warmup: int = 3,
    pid: int = 1,
    sample_interval_ms: int = 100,
    budget_ratio: float = 0.9,
    seed: int | None = None,
    rtf_max: float = 1.0,
    speed: float = 1.0,
    http_fn=None,
    query_apps_fn=None,
    query_gpu_fn=None,
) -> dict:
    """Execute complete benchmark flow against target server."""
    if not (0.0 < speed <= 4.0):
        raise SetupError(f"Invalid speed {speed}. Must be > 0 and <= 4.0")

    # Take result timestamp at start of run
    timestamp_utc = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")

    # 1. Warmup
    warmup_failure: dict | None = None
    for i in range(warmup):
        text = input_lines[i % len(input_lines)]
        status, body, _ = send_speech_request(
            base_url,
            api_key,
            text,
            voice,
            speed=speed,
            seed=seed,
            http_fn=http_fn,
        )
        if status == 0:
            err_msg = body.decode("utf-8", errors="replace")[:200]
            raise SetupError(
                f"Server connection failed during warmup (status 0: {err_msg})"
            )
        if status != 200:
            err_msg = body.decode("utf-8", errors="replace")[:200]
            warmup_failure = {
                "stage": "warmup",
                "warmup_index": i,
                "status": status,
                "error": err_msg,
            }
            break
        dur = get_wav_duration_seconds(body)
        if dur <= 0.0:
            warmup_failure = {
                "stage": "warmup",
                "warmup_index": i,
                "status": 200,
                "error": "unparseable WAV",
            }
            break

    if warmup_failure is not None:
        return build_result_dict(
            label=label,
            speed=speed,
            timestamp_utc=timestamp_utc,
            requested=count,
            completed=0,
            warmup=warmup,
            sample_count=0,
            pid_hits=0,
            failures=1,
            failure_detail=warmup_failure,
            rtf_stats={"mean": None, "p50": None, "p95": None, "max": None},
            latency_stats={"p50": None, "p95": None},
            peak_process_mib=0.0,
            peak_reserved_mib=0.0,
            peak_allocated_mib=0.0,
            context_overhead_mib=0.0,
            effective_peak_mib=0.0,
            peak_total_gpu_used_mib=0.0,
            budget_mib=budget_mib,
            budget_ratio=budget_ratio,
            rtf_max=rtf_max,
        )

    # 2. Reset peak probe after warmup
    status_reset, _ = reset_cuda_memory_probe(base_url, api_key, http_fn=http_fn)
    if status_reset != 200:
        raise SetupError(
            f"Reset peak probe returned status {status_reset} (expected 200)"
        )

    # Sample process MiB and probe reserved_bytes immediately after reset
    q_apps = query_apps_fn or run_query_compute_apps
    try:
        raw_apps = q_apps()
    except (subprocess.SubprocessError, OSError) as e:
        raise SetupError(
            f"Failed to query GPU compute apps for context overhead: {e}"
        ) from e
    initial_apps = parse_compute_apps_csv(raw_apps)
    initial_process_mib = initial_apps.get(pid, 0.0)

    status_init_probe, init_probe_data = get_cuda_memory_probe(
        base_url, api_key, http_fn=http_fn
    )
    if status_init_probe != 200:
        raise SetupError(
            "CUDA memory probe failed immediately after reset (status"
            f" {status_init_probe})"
        )
    initial_reserved_bytes = init_probe_data.get("reserved_bytes", 0)
    initial_reserved_mib = initial_reserved_bytes / (1024.0 * 1024.0)
    context_overhead_mib = max(0.0, initial_process_mib - initial_reserved_mib)

    # 3. Start background sampler
    stop_sampler = start_vram_sampler(
        pid=pid,
        interval_ms=sample_interval_ms,
        query_apps_fn=query_apps_fn,
        query_gpu_fn=query_gpu_fn,
    )

    # 4. Sequentially send requests
    latencies: list[float] = []
    rtfs: list[float] = []
    failure_detail: dict | None = None
    failures = 0

    for i in range(count):
        text = input_lines[i % len(input_lines)]
        status, body, wall_time = send_speech_request(
            base_url,
            api_key,
            text,
            voice,
            speed=speed,
            seed=seed,
            http_fn=http_fn,
        )
        if status != 200:
            failures = 1
            err_msg = body.decode("utf-8", errors="replace")[:200]
            failure_detail = {
                "request_index": i,
                "status": status,
                "error": err_msg,
                "input": text,
            }
            break

        audio_secs = get_wav_duration_seconds(body)
        if audio_secs <= 0.0:
            failures = 1
            failure_detail = {
                "request_index": i,
                "status": status,
                "error": "unparseable WAV",
                "input": text,
            }
            break

        rtf = calculate_rtf(wall_time, audio_secs)
        latencies.append(wall_time)
        rtfs.append(rtf)

    # 5. Stop sampler and read probe
    peak_proc, peak_gpu, sample_count, pid_hits = stop_sampler()
    status_final_probe, probe_data = get_cuda_memory_probe(
        base_url, api_key, http_fn=http_fn
    )
    if status_final_probe != 200:
        if failures > 0:
            peak_allocated_mib = None
            peak_reserved_mib = None
            effective_peak_mib = peak_proc
        else:
            raise SetupError(
                f"CUDA memory probe failed at end of run (status {status_final_probe})"
            )
    else:
        peak_allocated_bytes = probe_data.get("max_allocated_bytes", 0)
        peak_reserved_bytes = probe_data.get("max_reserved_bytes", 0)
        peak_allocated_mib = peak_allocated_bytes / (1024.0 * 1024.0)
        peak_reserved_mib = peak_reserved_bytes / (1024.0 * 1024.0)
        effective_peak_mib = max(peak_proc, peak_reserved_mib + context_overhead_mib)

    # 6. Aggregate calculations
    if rtfs:
        rtf_stats = {
            "mean": sum(rtfs) / len(rtfs),
            "p50": calculate_percentile(rtfs, 50),
            "p95": calculate_percentile(rtfs, 95),
            "max": max(rtfs),
        }
    else:
        rtf_stats = {"mean": None, "p50": None, "p95": None, "max": None}

    if latencies:
        latency_stats = {
            "p50": calculate_percentile(latencies, 50),
            "p95": calculate_percentile(latencies, 95),
        }
    else:
        latency_stats = {"p50": None, "p95": None}

    return build_result_dict(
        label=label,
        timestamp_utc=timestamp_utc,
        requested=count,
        completed=len(latencies),
        warmup=warmup,
        sample_count=sample_count,
        pid_hits=pid_hits,
        failures=failures,
        failure_detail=failure_detail,
        rtf_stats=rtf_stats,
        latency_stats=latency_stats,
        peak_process_mib=peak_proc,
        peak_reserved_mib=peak_reserved_mib,
        peak_allocated_mib=peak_allocated_mib,
        context_overhead_mib=context_overhead_mib,
        effective_peak_mib=effective_peak_mib,
        peak_total_gpu_used_mib=peak_gpu,
        budget_mib=budget_mib,
        budget_ratio=budget_ratio,
        rtf_max=rtf_max,
        speed=speed,
    )


def parse_args(args=None):
    """Parse CLI arguments for bench_vram_rtf.py."""
    parser = argparse.ArgumentParser(
        description=(
            "Benchmark harness measuring VRAM usage and Real-Time Factor of"
            " Irodori-TTS."
        )
    )
    parser.add_argument(
        "--label",
        required=True,
        help="Configuration label (e.g. C1..C4, free text)",
    )
    parser.add_argument(
        "--base-url",
        default="http://127.0.0.1:8088",
        help="Base URL of Irodori-TTS server",
    )
    parser.add_argument(
        "--api-key-file",
        required=True,
        help="Path to file containing Bearer auth token",
    )
    parser.add_argument(
        "--voice", default="default", help="Voice ID (default: 'default')"
    )
    parser.add_argument(
        "--inputs",
        default=str(Path(__file__).resolve().parent / "inputs" / "worst_case_ja.txt"),
        help="Path to text input file (one sentence per line)",
    )
    parser.add_argument(
        "--count",
        type=int,
        default=100,
        help="Number of benchmark requests (default: 100)",
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=3,
        help="Number of warmup requests (default: 3)",
    )
    parser.add_argument(
        "--container",
        default="irodori-tts",
        help="Container name to resolve host PID",
    )
    parser.add_argument(
        "--pid",
        type=int,
        default=None,
        help="Host PID of server process (overrides --container)",
    )
    parser.add_argument(
        "--sample-interval-ms",
        type=int,
        default=100,
        help="VRAM sample interval in ms (default: 100)",
    )
    parser.add_argument(
        "--budget-mib",
        type=float,
        required=True,
        help="VRAM budget in MiB for the TTS container, decided by the operator",
    )
    parser.add_argument(
        "--budget-ratio",
        type=float,
        default=0.9,
        help="Budget ratio ceiling (default: 0.9)",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=None,
        help="Optional fixed seed for reproducibility",
    )
    parser.add_argument(
        "--out-dir",
        default=str(Path(__file__).resolve().parent / "results"),
        help="Directory to store JSON benchmark result",
    )
    parser.add_argument(
        "--rtf-max",
        type=float,
        default=1.0,
        help="Maximum allowed RTF p95 gate threshold (default: 1.0)",
    )
    parser.add_argument(
        "--speed",
        type=float,
        default=1.0,
        help="Playback speed multiplier (default: 1.0, must be > 0 and <= 4.0)",
    )
    return parser.parse_args(args)


def main(args=None) -> int:
    """Entry point for bench_vram_rtf."""
    parsed = parse_args(args)

    if not re.fullmatch(r"[A-Za-z0-9_.-]+", parsed.label):
        sys.stderr.write(
            f"Setup error: Invalid label '{parsed.label}'. Must match"
            " ^[A-Za-z0-9_.-]+$\n"
        )
        return 2

    if not (0.0 < parsed.speed <= 4.0):
        sys.stderr.write(
            f"Setup error: Invalid speed {parsed.speed}. Must be > 0 and <= 4.0\n"
        )
        return 2

    # 1. Validate API key file
    key_path = Path(parsed.api_key_file)
    if not key_path.is_file():
        sys.stderr.write(
            f"Setup error: API key file not found: {parsed.api_key_file}\n"
        )
        return 2

    try:
        api_key = key_path.read_text(encoding="utf-8").strip()
    except OSError as e:
        sys.stderr.write(f"Setup error: Failed to read API key file: {e}\n")
        return 2

    if not api_key:
        sys.stderr.write("Setup error: API key file is empty\n")
        return 2

    # 2. Validate input sentences
    input_path = Path(parsed.inputs)
    if not input_path.is_file():
        sys.stderr.write(f"Setup error: Inputs file not found: {parsed.inputs}\n")
        return 2

    try:
        lines = [
            line.strip()
            for line in input_path.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
    except OSError as e:
        sys.stderr.write(f"Setup error: Failed to read inputs file: {e}\n")
        return 2

    if not lines:
        sys.stderr.write("Setup error: Inputs file contains no non-empty lines\n")
        return 2

    # 3. Validate nvidia-smi
    if not check_nvidia_smi():
        sys.stderr.write("Setup error: nvidia-smi is missing or failed execution\n")
        return 2

    # 4. Resolve PID
    try:
        pid = resolve_container_pid(container_name=parsed.container, pid_arg=parsed.pid)
    except SetupError as e:
        sys.stderr.write(f"Setup error: {e}\n")
        return 2

    # 5. Run benchmark
    try:
        result = run_benchmark(
            label=parsed.label,
            base_url=parsed.base_url,
            api_key=api_key,
            voice=parsed.voice,
            input_lines=lines,
            count=parsed.count,
            warmup=parsed.warmup,
            pid=pid,
            sample_interval_ms=parsed.sample_interval_ms,
            budget_mib=parsed.budget_mib,
            budget_ratio=parsed.budget_ratio,
            seed=parsed.seed,
            rtf_max=parsed.rtf_max,
            speed=parsed.speed,
        )
    except SetupError as e:
        sys.stderr.write(f"Setup error: {e}\n")
        return 2

    # 6. Save result JSON
    out_dir = Path(parsed.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    speed_formatted = format_speed(result.get("speed", parsed.speed))
    filename = f"{parsed.label}-x{speed_formatted}-{result['timestamp_utc']}.json"
    result_file = out_dir / filename
    result_file.write_text(
        json.dumps(result, indent=2, ensure_ascii=False), encoding="utf-8"
    )

    measured_phase_ran = not (
        result.get("failure_detail")
        and result["failure_detail"].get("stage") == "warmup"
    )
    if result["pid_hits"] == 0 and measured_phase_ran and result["failures"] == 0:
        sys.stderr.write(
            f"Setup error: PID {pid} was never observed in GPU telemetry; check"
            " container or PID\n"
        )
        return 2

    # 7. Print summary and return exit code
    summary = format_summary(result)
    print(summary)
    print(f"Result written to: {result_file}")

    return 0 if result["all_passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
