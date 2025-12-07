# Utilizer - Real-time Resource Monitor

Recap Job実行中のメモリとCPU使用状況をリアルタイムで監視するツールです。

## インストール

```bash
cd monitors/utilizer
uv sync
```

## 実行コマンド

### ターミナル版

```bash
# 基本実行（2秒間隔）
cd monitors/utilizer
uv run utilizer

# カスタム間隔（例: 5秒）
uv run utilizer -i 5

# ログファイルに記録
uv run utilizer -l /tmp/monitor.log

# ヘルプ表示
uv run utilizer --help
```

### Web版

```bash
# Webサーバー起動
cd monitors/utilizer
uv run utilizer-web

# ブラウザで http://localhost:8889 にアクセス
```

### ルートディレクトリから実行

プロジェクトルートから実行する場合：

```bash
# ターミナル版
cd monitors/utilizer && uv run utilizer

# Web版
cd monitors/utilizer && uv run utilizer-web
```

## 依存関係

- `fastapi>=0.104.0` - Webフレームワーク
- `uvicorn[standard]>=0.24.0` - ASGIサーバー（WebSocketサポート含む）
- `websockets>=12.0` - WebSocketライブラリ
- `psutil>=5.9.0` - システム情報取得（CPU使用率の正確な取得）

## 機能

- **メモリ監視**: 総容量、使用量、利用可能量、使用率
- **CPU監視**: リアルタイムCPU使用率（psutilまたは/proc/statを使用）
- **ハングプロセス検出**: `spawn_main` / `multiprocessing-fork` プロセスの監視
- **プロセス一覧**: メモリ使用量の多いプロセスを表示
- **アラート**: メモリ/CPU使用率が高い場合に警告
- **ログ出力**: CSV形式でログを記録（ターミナル版）
- **モダンなWeb UI**: シックなダークテーマのダッシュボード

## 開発

```bash
# 依存関係をインストール
cd monitors/utilizer
uv sync

# 開発モードで実行
uv run utilizer
uv run utilizer-web
```
