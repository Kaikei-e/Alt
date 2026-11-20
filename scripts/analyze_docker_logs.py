#!/usr/bin/env python3
"""
Docker Composeログ解析スクリプト

Recap関連サービスのログからエラーパターンと警告を抽出する。
"""

import json
import re
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any

LOG_LINE_PART_COUNT = 3


def parse_log_line(line: str) -> tuple[str | None, datetime | None, str | None]:
    """ログ行をパースしてサービス名、タイムスタンプ、メッセージを抽出"""
    # Docker Compose log format: service_name | timestamp | message
    # Example: recap-worker | 2025-01-15T10:30:45.123Z | ERROR: something went wrong

    parts = line.split(" | ", 2)
    if len(parts) < LOG_LINE_PART_COUNT:
        return None, None, line

    service_name = parts[0].strip()
    timestamp_str = parts[1].strip()
    message = parts[2].strip()

    # Parse timestamp
    timestamp = None
    try:
        # Try ISO format
        timestamp = datetime.fromisoformat(timestamp_str.replace("Z", "+00:00"))
        if timestamp.tzinfo is None:
            timestamp = timestamp.astimezone()
    except ValueError:
        try:
            # Try other formats
            for fmt in ["%Y-%m-%dT%H:%M:%S.%f", "%Y-%m-%d %H:%M:%S"]:
                try:
                    timestamp = datetime.strptime(timestamp_str, fmt).astimezone()
                    break
                except ValueError:
                    continue
        except ValueError:
            pass

    return service_name, timestamp, message


def extract_errors_and_warnings(lines: list[str]) -> dict[str, list[dict[str, str]]]:
    """エラーと警告を抽出"""
    errors = defaultdict(list)
    warnings = defaultdict(list)

    error_patterns = [
        r"ERROR",
        r"Exception",
        r"panic",
        r"failed",
        r"timeout",
        r"deadline exceeded",
    ]

    warning_patterns = [
        r"WARN",
        r"warning",
        r"threshold",
        r"below threshold",
        r"fallback",
        r"insufficient",
        r"skipped",
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service:
            continue

        # Check for errors
        for pattern in error_patterns:
            if re.search(pattern, message, re.IGNORECASE):
                errors[service].append(
                    {"timestamp": timestamp.isoformat() if timestamp else None, "message": message}
                )
                break

        # Check for warnings
        for pattern in warning_patterns:
            if re.search(pattern, message, re.IGNORECASE):
                warnings[service].append(
                    {"timestamp": timestamp.isoformat() if timestamp else None, "message": message}
                )
                break

    return {"errors": dict(errors), "warnings": dict(warnings)}


def extract_genre_classification_issues(lines: list[str]) -> list[dict[str, str]]:
    """ジャンル分類関連の問題を抽出"""
    issues = []

    patterns = [
        (r"genre.*threshold", "threshold"),
        (r"fallback.*genre", "fallback"),
        (r"classified.*other", "other_classification"),
        (r"genre.*failed", "classification_failure"),
        (r"rocchio", "rocchio_mention"),
        (r"graph.*propagation", "graph_propagation"),
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service or service not in ["recap-worker", "recap-subworker"]:
            continue

        for pattern, issue_type in patterns:
            if re.search(pattern, message, re.IGNORECASE):
                issues.append(
                    {
                        "service": service,
                        "timestamp": timestamp.isoformat() if timestamp else None,
                        "issue_type": issue_type,
                        "message": message,
                    }
                )
                break

    return issues


def extract_clustering_issues(lines: list[str]) -> list[dict[str, str]]:
    """クラスタリング関連の問題を抽出"""
    issues = []

    patterns = [
        (r"umap", "umap_mention"),
        (r"hdbscan", "hdbscan_mention"),
        (r"cluster.*size", "cluster_size"),
        (r"noise", "noise_mention"),
        (r"dbcv", "dbcv_mention"),
        (r"min_cluster_size", "min_cluster_size"),
        (r"embedding", "embedding_mention"),
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service or service != "recap-subworker":
            continue

        for pattern, issue_type in patterns:
            if re.search(pattern, message, re.IGNORECASE):
                issues.append(
                    {
                        "service": service,
                        "timestamp": timestamp.isoformat() if timestamp else None,
                        "issue_type": issue_type,
                        "message": message,
                    }
                )
                break

    return issues


def analyze_log_file(log_file: Path) -> dict[str, Any]:
    """ログファイルを解析"""
    if not log_file.exists():
        sys.stdout.write(f"Error: Log file not found: {log_file}\n")
        return {}

    sys.stdout.write(f"Reading log file: {log_file}\n")

    with log_file.open(encoding="utf-8", errors="ignore") as f:
        lines = f.readlines()

    sys.stdout.write(f"Read {len(lines)} lines\n")

    # Extract errors and warnings
    sys.stdout.write("Extracting errors and warnings...\n")
    errors_warnings = extract_errors_and_warnings(lines)

    # Extract genre classification issues
    sys.stdout.write("Extracting genre classification issues...\n")
    genre_issues = extract_genre_classification_issues(lines)

    # Extract clustering issues
    sys.stdout.write("Extracting clustering issues...\n")
    clustering_issues = extract_clustering_issues(lines)

    return {
        "errors": errors_warnings["errors"],
        "warnings": errors_warnings["warnings"],
        "genre_classification_issues": genre_issues,
        "clustering_issues": clustering_issues,
        "summary": {
            "total_lines": len(lines),
            "total_errors": sum(len(v) for v in errors_warnings["errors"].values()),
            "total_warnings": sum(len(v) for v in errors_warnings["warnings"].values()),
            "genre_issues_count": len(genre_issues),
            "clustering_issues_count": len(clustering_issues),
        },
    }


def _resolve_log_file() -> Path | None:
    """Resolve log file path from CLI arg or latest file under logs/."""
    if len(sys.argv) > 1:
        return Path(sys.argv[1])

    script_dir = Path(__file__).parent
    logs_dir = script_dir / "logs"

    if not logs_dir.exists():
        sys.stdout.write("Error: logs/ directory not found\n")
        sys.stdout.write("Usage: python analyze_docker_logs.py <log_file_path>\n")
        return None

    log_files = sorted(
        logs_dir.glob("recap-logs-*.txt"), key=lambda p: p.stat().st_mtime, reverse=True
    )
    if not log_files:
        sys.stdout.write("Error: No log files found in logs/ directory\n")
        sys.stdout.write("Usage: python analyze_docker_logs.py <log_file_path>\n")
        sys.stdout.write("Or place log files in scripts/logs/ directory\n")
        return None

    return log_files[0]


def _print_service_entries(
    heading: str, entries_by_service: dict[str, list[dict[str, Any]]]
) -> None:
    """Print per-service error or warning samples."""
    if not entries_by_service:
        return
    sys.stdout.write(f"\n{heading}:\n")
    for service, entries in entries_by_service.items():
        sys.stdout.write(f"  {service}: {len(entries)}件\n")
        for entry in entries[:3]:
            sys.stdout.write(f"    - {entry['message'][:100]}\n")


def _print_issue_samples(heading: str, issues: list[dict[str, Any]]) -> None:
    """Print first five genre or clustering issue samples."""
    if not issues:
        return
    sys.stdout.write(f"\n{heading}（最初の5件）:\n")
    for issue in issues[:5]:
        sys.stdout.write(f"  [{issue['service']}] {issue['issue_type']}: {issue['message'][:80]}\n")


def _print_analysis_summary(result: dict[str, Any]) -> None:
    """Display human-readable summary of log analysis results."""
    sys.stdout.write("\n" + "=" * 80 + "\n")
    sys.stdout.write("ログ解析結果サマリ\n")
    sys.stdout.write("=" * 80 + "\n")

    summary = result["summary"]
    sys.stdout.write(f"総ログ行数: {summary['total_lines']}\n")
    sys.stdout.write(f"総エラー数: {summary['total_errors']}\n")
    sys.stdout.write(f"総警告数: {summary['total_warnings']}\n")
    sys.stdout.write(f"ジャンル分類関連問題: {summary['genre_issues_count']}\n")
    sys.stdout.write(f"クラスタリング関連問題: {summary['clustering_issues_count']}\n")

    _print_service_entries("エラー（サービス別）", result["errors"])
    _print_service_entries("警告（サービス別）", result["warnings"])
    _print_issue_samples("ジャンル分類関連問題", result["genre_classification_issues"])
    _print_issue_samples("クラスタリング関連問題", result["clustering_issues"])

    sys.stdout.write("=" * 80 + "\n")


def main() -> None:
    """メイン処理"""
    log_file = _resolve_log_file()
    if log_file is None:
        return

    # ログを解析
    result = analyze_log_file(log_file)

    if not result:
        return

    # 結果をJSONで保存
    output_dir = Path(__file__).parent / "reports"
    output_dir.mkdir(exist_ok=True)
    output_file = (
        output_dir / f"log-analysis-{datetime.now().astimezone().strftime('%Y%m%d-%H%M%S')}.json"
    )

    with output_file.open("w", encoding="utf-8") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)

    sys.stdout.write(f"\nResults saved to {output_file}\n")

    _print_analysis_summary(result)


if __name__ == "__main__":
    main()
