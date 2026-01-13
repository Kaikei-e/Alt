# MQ Hub

_Last reviewed: January 13, 2026_

**Location:** `mq-hub`

## Role
- Alt プラットフォームのメッセージキューハブサービス
- Redis 8.4 Streams を使用したイベントソーシング
- Connect-RPC によるイベント発行 API

## Architecture & Flow

| Component | Responsibility |
| --- | --- |
| Connect Handler | イベント発行 API (Connect-RPC) |
| Usecase | ストリーム操作のビジネスロジック |
| Port | インターフェース定義 (StreamPort) |
| Gateway | Redis への Anti-corruption layer |
| Driver | Redis Streams クライアント実装 |

```mermaid
flowchart TB
    subgraph Producers
        BE[alt-backend]
        PP[pre-processor]
        TG[tag-generator]
    end

    subgraph MQHub[mq-hub :9500]
        API[Connect-RPC API]
        UC[Usecase Layer]
        GW[Gateway Layer]
        DR[Driver Layer]
    end

    subgraph Redis[redis-streams :6379]
        S1[alt:events:articles]
        S2[alt:events:summaries]
        S3[alt:events:tags]
    end

    subgraph Consumers
        PP2[pre-processor]
        SI[search-indexer]
        TG2[tag-generator]
    end

    BE --> API
    PP --> API
    TG --> API
    API --> UC --> GW --> DR --> Redis
    Redis --> PP2
    Redis --> SI
    Redis --> TG2
```

## Event Types

| Event | Producer | Consumers |
|-------|----------|-----------|
| ArticleCreated | alt-backend | pre-processor, search-indexer, tag-generator |
| SummarizeRequested | alt-backend | pre-processor |
| ArticleSummarized | pre-processor | search-indexer |
| TagsGenerated | tag-generator | search-indexer |

## Stream Keys

| Key | Purpose |
|-----|---------|
| `alt:events:articles` | 記事ライフサイクルイベント |
| `alt:events:summaries` | 要約イベント |
| `alt:events:tags` | タグ生成イベント |

## Endpoints & Behavior
- `GET /health` - ヘルスチェック
- Connect-RPC API (port 9500):
  - イベント発行
  - ストリーム状態クエリ

## Configuration & Env

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | redis://redis-streams:6379 | Redis Streams URL |
| `CONNECT_PORT` | 9500 | Connect-RPC ポート |
| `LOG_LEVEL` | info | ログレベル |

## Redis 8.4 Features
- `XREADGROUP CLAIM` オプションによる効率的なメッセージ処理
- アイドル pending + 新規メッセージを1コマンドで消費
- 障害回復の簡素化
- 30% のスループット向上

## Testing & Tooling
```bash
# テスト実行
go test ./...

# カバレッジ付きテスト
go test -cover ./...

# Proto コード生成
cd ../proto && buf generate --template buf.gen.mq-hub.yaml

# サービス起動
go run main.go

# ヘルスチェック
curl http://localhost:9500/health
```

**テスト層:**
- Usecase: StreamPort をモック、ビジネスロジックテスト
- Gateway: ドライバーをモック、ドメイン変換テスト
- Driver: miniredis でユニットテスト
- Integration: 実 Redis 8.4 インスタンス

## Operational Runbook
1. `docker compose -f compose/mq.yaml up -d` でサービス起動
2. `curl http://localhost:9500/health` でヘルスチェック
3. Redis Streams 確認: `docker compose exec redis-streams redis-cli`
4. ストリーム一覧: `XINFO STREAMS`
5. コンシューマーグループ確認: `XINFO GROUPS alt:events:articles`

## Observability
- 構造化ログ: `log/slog`
- ログには stream_key, event_type, message_id を含む
- rask.group ラベル: `mq-hub`

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Redis connection failed | REDIS_URL とネットワーク確認 |
| BUSYGROUP error | コンシューマーグループ既存 (正常) |
| Slow publishing | Redis latency 確認、バッチ発行検討 |
| Memory issues | Redis maxmemory policy 設定 |

## LLM Notes
- イベント駆動アーキテクチャの中核サービス
- at-least-once 配信を前提に設計 (冪等性必須)
- Clean Architecture (5層) を採用
- Redis Streams の XREADGROUP CLAIM は Redis 8.4 の新機能
