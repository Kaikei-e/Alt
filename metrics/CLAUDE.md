# Alt Metrics - CLAUDE.md

## Overview

Alt システム健全性アナライザー。ClickHouseに蓄積されたログ・トレースデータを分析し、日本語Markdownレポートを生成します。

## Quick Commands

```bash
# 依存関係インストール
uv sync

# テスト実行
uv run pytest -v

# Linting
uv run ruff check src/ tests/
uv run ruff format src/ tests/

# 分析実行（ClickHouse接続が必要）
uv run python -m alt_metrics analyze --hours 24 --verbose

# 接続テスト
uv run python -m alt_metrics validate
```

## Directory Structure

```
metrics/
├── pyproject.toml          # プロジェクト設定
├── uv.lock
├── CLAUDE.md               # このファイル
├── src/
│   └── alt_metrics/
│       ├── __init__.py
│       ├── __main__.py     # CLIエントリーポイント
│       ├── cli.py          # CLIコマンド処理
│       ├── config.py       # 設定管理（閾値含む）
│       ├── models.py       # Pydanticデータモデル
│       ├── analysis.py     # ヘルススコア計算
│       ├── exceptions.py   # カスタム例外
│       ├── collectors/     # ClickHouseデータ収集
│       │   ├── base.py     # レガシーlogs
│       │   ├── traces.py   # OTelトレース
│       │   ├── logs.py     # OTelログ
│       │   ├── http.py     # HTTPメトリクス
│       │   └── sli.py      # SLI/SLO
│       └── reports/
│           ├── japanese.py # 日本語レポート生成
│           └── templates/
│               └── report_ja.md.j2
└── tests/
    ├── conftest.py         # 共通フィクスチャ
    ├── test_analysis.py
    ├── test_config.py
    ├── test_collectors/
    └── test_reports/
```

## Environment Variables

### ClickHouse接続
- `APP_CLICKHOUSE_HOST` (default: localhost)
- `APP_CLICKHOUSE_PORT` (default: 8123)
- `APP_CLICKHOUSE_USER` (default: default)
- `APP_CLICKHOUSE_PASSWORD` or `APP_CLICKHOUSE_PASSWORD_FILE`
- `APP_CLICKHOUSE_DATABASE` (default: rask_logs)

### 閾値設定（オプション）
- `METRICS_THRESHOLD_ERROR_RATE_CRITICAL` (default: 10.0)
- `METRICS_THRESHOLD_LATENCY_CRITICAL_MS` (default: 10000)
- etc.

### レポート設定
- `METRICS_REPORT_LANGUAGE` (default: ja)
- `METRICS_OUTPUT_DIR` (default: ./scripts/reports)

## TDD Workflow

```bash
# 1. RED: 失敗するテストを書く
# 2. GREEN: 最小限の実装でパス
# 3. REFACTOR: 品質改善

uv run pytest -v --cov=alt_metrics
```

## Key Patterns

- **Pydantic Models**: 型安全なデータモデル
- **Structlog**: 構造化ログ
- **Jinja2 Templates**: レポート生成
- **Custom Exceptions**: `CollectorError`, `ConfigurationError`, etc.

## Health Score Calculation

```
スコア = 100 - エラー率ペナルティ - レイテンシペナルティ - ログ欠落ペナルティ

エラー率:
  > 10%: -40点, > 5%: -25点, > 1%: -10点, > 0.5%: -5点

レイテンシ (p95):
  > 10s: -30点, > 5s: -20点, > 1s: -10点, > 500ms: -5点

ログ欠落:
  > 10分: -30点, > 5分: -15点
```
