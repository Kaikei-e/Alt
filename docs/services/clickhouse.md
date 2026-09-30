# ClickHouse

_Last reviewed: September 5, 2026_

**Location:** `clickhouse/`
**Port:** 8123 (HTTP, host bound `127.0.0.1`), 9009 (host `127.0.0.1`) → 9000 (container, native protocol)

## Role
- Alt プラットフォーム全体の集約ログおよび OpenTelemetry トレースデータの格納・分析基盤
- ClickHouse 25.9 を使用したカラム指向データストア
- `metrics` CLI やログ検索ツール (`rask` スタック) によるヘルス分析および SLI 指標集計

## Architecture & Compose Integration

| Property | Value |
| --- | --- |
| Image | `clickhouse/clickhouse-server:25.9` (`compose/db.yaml`) |
| HTTP Port | 8123 (host: `127.0.0.1:8123`, health `/ping`) |
| Native Port | 9000 (host: `127.0.0.1:9009`) |
| Volume | `clickhouse_data` (`/var/lib/clickhouse`) |
| Secrets | `clickhouse_password` |
| Database | `rask_logs` (環境変数 `CLICKHOUSE_DB`) |
| User | `rask_user` (環境変数 `CLICKHOUSE_USER`) |

## Schema Management & Migrations

Atlas による管理ではなく、`clickhouse/migrations/*.sql` に配置された冪等な生 SQL を使用する。

1. **コンテナ起動時**: `entrypoint-wrapper.sh` が `migrations/*.sql` を全件順次実行する。
2. **デプロイ時**: コンテナが再作成されないロール時にも DDL を確実に適用するため、ワンショットサービス `clickhouse-migrator` (`compose/db.yaml`) が先行して `apply` モードで同一のマイグレーションスクリプトを実行する。

## Database & Tables

データベース `rask_logs` に以下の 7 テーブルを保持 (`sli_metrics` 以外は TTL 1 日、`sli_metrics` は 90 日):

| Table | Type | Source / Purpose | Retention / TTL |
| --- | --- | --- | --- |
| `logs` | MergeTree | legacy NDJSON (rask-log-forwarder より) | 1 日 |
| `otel_logs` | MergeTree | OpenTelemetry 構造化ログ (OTLP) | 1 日 |
| `otel_traces` | MergeTree | OpenTelemetry 分散トレース (OTLP) | 1 日 |
| `http_logs` | MergeTree | `logs` から MV (`http_logs_mv`) 経由で抽出した HTTP アクセスログ | 1 日 |
| `otel_http_requests` | MergeTree | `otel_logs` から MV (`otel_http_requests_mv`) 経由で抽出した HTTP リクエストログ | 1 日 |
| `otel_error_logs` | MergeTree | `otel_logs` から MV (`otel_error_logs_mv`) 経由で抽出したエラーログ | 1 日 |
| `sli_metrics` | MergeTree | `otel_logs` から MV (`sli_error_rate_mv`, `sli_log_throughput_mv`) 経由で集計した SLI/SLO メトリクス | 90 日 |

## Primary Consumers & Probing

- **書込**: `rask-log-aggregator` (`compose/logging.yaml`) がログ・トレースをバッチ投入
- **読取**:
  - `metrics` CLI (`metrics/`): システムヘルスレポート生成、SLI 集計
  - 運用時のアドホックログ調査・トレース検索

## Operational Runbook & Health Check

```bash
# HTTP ping によるヘルスチェック
curl http://localhost:8123/ping

# クエリ実行 (HTTP)
curl -u rask_user:password "http://localhost:8123/?query=SELECT+count()+FROM+rask_logs.logs"

# スキーママイグレーションの手動適用
docker compose -f compose/db.yaml run --rm clickhouse-migrator apply
```

## Known Failure Patterns & Invariants

Cross-cutting incident patterns are catalogued in [[runbooks/crystallized-knowledge]].

- **RowBinary / Native protocol 接続時の型厳格性**: ClickHouse のバイナリプロトコル接続では、スキーマ定義とクライアント側の型が完全に一致している必要がある (`FixedString(N)` は正確なバイト長、`Enum8` は i8、`DateTime64` は i64)。不一致時は接続エラーまたは不正なデータ投入となる ([[000074]])。
- **Retention と Compaction 遅延**: TTL を設定していても、バックグラウンドの merge/compaction が走るまで実ディスク上から即時削除されない場合がある。ディスク逼迫時には注意を要する。
- **ログへの機微情報混入防止**: ClickHouse はログ分析基盤であるため、各サービスおよびログフォワーダー側でマスキング・redaction を徹底し、資格情報やトークンを投入しない。
