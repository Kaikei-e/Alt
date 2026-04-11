#!/usr/bin/env python3
"""
Docker Composeログ解析スクリプト

Recap関連サービスのログからエラーパターンと警告を抽出する。
"""

import re
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple


def parse_log_line(line: str) -> Tuple[Optional[str], Optional[datetime], Optional[str]]:
    """ログ行をパースしてサービス名、タイムスタンプ、メッセージを抽出"""
    # Docker Compose log format: service_name | timestamp | message
    # Example: recap-worker | 2025-01-15T10:30:45.123Z | ERROR: something went wrong

    parts = line.split(" | ", 2)
    if len(parts) < 3:
        return None, None, line

    service_name = parts[0].strip()
    timestamp_str = parts[1].strip()
    message = parts[2].strip()

    # Parse timestamp
    timestamp = None
    try:
        # Try ISO format
        timestamp = datetime.fromisoformat(timestamp_str.replace('Z', '+00:00'))
    except:
        try:
            # Try other formats
            for fmt in ["%Y-%m-%dT%H:%M:%S.%f", "%Y-%m-%d %H:%M:%S"]:
                try:
                    timestamp = datetime.strptime(timestamp_str, fmt)
                    break
                except:
                    continue
        except:
            pass

    return service_name, timestamp, message


def extract_errors_and_warnings(lines: List[str]) -> Dict[str, List[Dict[str, str]]]:
    """エラーと警告を抽出"""
    errors = defaultdict(list)
    warnings = defaultdict(list)

    error_patterns = [
        r'ERROR',
        r'error',
        r'Error',
        r'Exception',
        r'panic',
        r'failed',
        r'Failed',
        r'FAILED',
        r'timeout',
        r'Timeout',
        r'deadline exceeded',
    ]

    warning_patterns = [
        r'WARN',
        r'warning',
        r'Warning',
        r'threshold',
        r'below threshold',
        r'fallback',
        r'insufficient',
        r'skipped',
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service:
            continue

        # Check for errors
        for pattern in error_patterns:
            if re.search(pattern, message, re.IGNORECASE):
                errors[service].append({
                    "timestamp": timestamp.isoformat() if timestamp else None,
                    "message": message
                })
                break

        # Check for warnings
        for pattern in warning_patterns:
            if re.search(pattern, message, re.IGNORECASE):
                warnings[service].append({
                    "timestamp": timestamp.isoformat() if timestamp else None,
                    "message": message
                })
                break

    return {
        "errors": dict(errors),
        "warnings": dict(warnings)
    }


def extract_genre_classification_issues(lines: List[str]) -> List[Dict[str, str]]:
    """ジャンル分類関連の問題を抽出"""
    issues = []

    patterns = [
        (r'genre.*threshold', 'threshold'),
        (r'fallback.*genre', 'fallback'),
        (r'classified.*other', 'other_classification'),
        (r'genre.*failed', 'classification_failure'),
        (r'rocchio', 'rocchio_mention'),
        (r'graph.*propagation', 'graph_propagation'),
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service or service not in ['recap-worker', 'recap-subworker']:
            continue

        for pattern, issue_type in patterns:
            if re.search(pattern, message, re.IGNORECASE):
                issues.append({
                    "service": service,
                    "timestamp": timestamp.isoformat() if timestamp else None,
                    "issue_type": issue_type,
                    "message": message
                })
                break

    return issues


def extract_clustering_issues(lines: List[str]) -> List[Dict[str, str]]:
    """クラスタリング関連の問題を抽出"""
    issues = []

    patterns = [
        (r'umap', 'umap_mention'),
        (r'hdbscan', 'hdbscan_mention'),
        (r'cluster.*size', 'cluster_size'),
        (r'noise', 'noise_mention'),
        (r'dbcv', 'dbcv_mention'),
        (r'min_cluster_size', 'min_cluster_size'),
        (r'embedding', 'embedding_mention'),
    ]

    for line in lines:
        service, timestamp, message = parse_log_line(line)

        if not service or service != 'recap-subworker':
            continue

        for pattern, issue_type in patterns:
            if re.search(pattern, message, re.IGNORECASE):
                issues.append({
                    "service": service,
                    "timestamp": timestamp.isoformat() if timestamp else None,
                    "issue_type": issue_type,
                    "message": message
                })
                break

    return issues


def analyze_log_file(log_file: Path) -> Dict:
    """ログファイルを解析"""
    if not log_file.exists():
        print(f"Error: Log file not found: {log_file}")
        return {}

    print(f"Reading log file: {log_file}")

    with open(log_file, 'r', encoding='utf-8', errors='ignore') as f:
        lines = f.readlines()

    print(f"Read {len(lines)} lines")

    # Extract errors and warnings
    print("Extracting errors and warnings...")
    errors_warnings = extract_errors_and_warnings(lines)

    # Extract genre classification issues
    print("Extracting genre classification issues...")
    genre_issues = extract_genre_classification_issues(lines)

    # Extract clustering issues
    print("Extracting clustering issues...")
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
            "clustering_issues_count": len(clustering_issues)
        }
    }


def main():
    """メイン処理"""
    import json

    # ログファイルのパスを取得
    if len(sys.argv) > 1:
        log_file = Path(sys.argv[1])
    else:
        # デフォルト: 最新のログファイルを探す
        script_dir = Path(__file__).parent
        logs_dir = script_dir / "logs"

        if logs_dir.exists():
            log_files = sorted(logs_dir.glob("recap-logs-*.txt"), key=lambda p: p.stat().st_mtime, reverse=True)
            if log_files:
                log_file = log_files[0]
            else:
                print("Error: No log files found in logs/ directory")
                print("Usage: python analyze_docker_logs.py <log_file_path>")
                print("Or place log files in scripts/logs/ directory")
                return
        else:
            print("Error: logs/ directory not found")
            print("Usage: python analyze_docker_logs.py <log_file_path>")
            return

    # ログを解析
    result = analyze_log_file(log_file)

    if not result:
        return

    # 結果をJSONで保存
    output_dir = Path(__file__).parent / "reports"
    output_dir.mkdir(exist_ok=True)
    output_file = output_dir / f"log-analysis-{datetime.now().strftime('%Y%m%d-%H%M%S')}.json"

    with open(output_file, "w", encoding="utf-8") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)

    print(f"\nResults saved to {output_file}")

    # サマリを表示
    print("\n" + "="*80)
    print("ログ解析結果サマリ")
    print("="*80)

    summary = result["summary"]
    print(f"総ログ行数: {summary['total_lines']}")
    print(f"総エラー数: {summary['total_errors']}")
    print(f"総警告数: {summary['total_warnings']}")
    print(f"ジャンル分類関連問題: {summary['genre_issues_count']}")
    print(f"クラスタリング関連問題: {summary['clustering_issues_count']}")

    if result["errors"]:
        print("\nエラー（サービス別）:")
        for service, errors in result["errors"].items():
            print(f"  {service}: {len(errors)}件")
            # 最初の3件を表示
            for err in errors[:3]:
                print(f"    - {err['message'][:100]}")

    if result["warnings"]:
        print("\n警告（サービス別）:")
        for service, warnings in result["warnings"].items():
            print(f"  {service}: {len(warnings)}件")
            # 最初の3件を表示
            for warn in warnings[:3]:
                print(f"    - {warn['message'][:100]}")

    if result["genre_classification_issues"]:
        print("\nジャンル分類関連問題（最初の5件）:")
        for issue in result["genre_classification_issues"][:5]:
            print(f"  [{issue['service']}] {issue['issue_type']}: {issue['message'][:80]}")

    if result["clustering_issues"]:
        print("\nクラスタリング関連問題（最初の5件）:")
        for issue in result["clustering_issues"][:5]:
            print(f"  [{issue['service']}] {issue['issue_type']}: {issue['message'][:80]}")

    print("="*80)


if __name__ == "__main__":
    main()

