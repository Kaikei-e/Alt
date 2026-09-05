# Search Indexer

_Last reviewed: September 5, 2026_

**Location:** `search-indexer/app`

## Role
- Go 1.26+ サービスで Meilisearch への記事バッチインデックスと `/v1/search` エンドポイントを提供
- デュアルフェーズインデックスループ (Backfill + Incremental, 200 ドキュメント/バッチ) + 軽量 HTTP ハンドラーで検索クエリ処理
- recap-worker のジャンルも同じデュアルフェーズパターンで `recaps` インデックスへ別途インデックス (`RECAP_WORKER_URL` 設定時のみ有効)
- Clean Architecture レイヤーと共通トークナイザーを使用
- OpenTelemetry (トレース・メトリクス・ログ) による可観測性

## Architecture & Flow

| Layer | Component |
| --- | --- |
| Driver | `driver/backend_api/client.go` (Connect-RPC 経由で alt-data-hub の `DataHubService` を呼び出す。パッケージ名は移行前の名残で、実体は alt-backend ではなく alt-data-hub — [[000954]])、`driver/meilisearch_driver.go` (`articles` インデックス)、`driver/meilisearch_recap_driver.go` (`recaps` インデックス) |
| Gateway | `gateway/article_repository_gateway.go` (バッチ取得)、`gateway/search_engine_gateway.go` (インデックス設定)、`gateway/recap_repository_gateway.go` / `gateway/recap_search_engine_gateway.go` (recap 版) |
| Usecases | `IndexArticlesUsecase` (インデックスループ)、`SearchByUserUsecase` / `SearchArticlesUsecase` (クエリ処理)、`IndexRecapsUsecase` / `SearchRecapsUsecase` (recap 版) |
| Server | `bootstrap/app.go` - `runIndexLoop` + `bootstrap/servers.go` でサーバーオーケストレーション |
| Tokenizer | `tokenize/tokenizer.go` - MeCab ベースのトークナイザー |
| Middleware | `middleware/otel_status_middleware.go` - OTel スパンステータス設定、`middleware/peer_identity_middleware.go` - mTLS peer identity 検証 (`:9443`) |
| Consumer | `consumer/` - Redis Streams コンシューマー (XREADGROUP + XAUTOCLAIM 再配信 + DLQ、下記参照) |

```mermaid
flowchart LR
    DataHub["alt-data-hub\nDataHubService (mTLS :9443)"]
    Loop((runIndexLoop))
    RecapLoop((runRecapIndexLoop))
    Meili[(Meilisearch `articles`)]
    MeiliRecap[(Meilisearch `recaps`)]
    RecapWorker["recap-worker"]
    REST[REST :9300]
    RPC[Connect-RPC :9301]
    Redis[(Redis Streams\nalt:events:articles)]

    DataHub -->|Connect-RPC, ArticleID resolve| Loop --> Meili
    Redis -->|ArticleCreated/ArticleUpdated\n(article_id only)| Loop
    RecapWorker -->|HTTP poll| RecapLoop --> MeiliRecap
    REST -->|/v1/search| Meili
    RPC -->|SearchService| Meili
    RPC -->|SearchService| MeiliRecap
```

## Endpoints & Behavior

| Port | Protocol | Endpoint | Description |
|------|----------|----------|-------------|
| 9300 | HTTP | `/v1/search` | 検索 API (q + user_id 必須) |
| 9300 | HTTP | `/health` | ヘルスチェック |
| 9300 | HTTP | `/health/deep` | Meilisearch 疎通を含む深いヘルスチェック (pass/warn/fail) |
| 9301 | Connect-RPC | `/services.search.v2.SearchService/` | Connect-RPC 検索サービス (v2)。他の Connect-RPC 風パスは `/` の catch-all で受ける |
| 9443 | HTTPS (mTLS) | REST + Connect-RPC | `MTLS_LISTEN=true` の場合のみ。peer-identity 検証済みの east-west トラフィック専用、外部には公開しない |
| 9110 | HTTP | `/metrics` | PKI enrollment + deep health の ops 用メトリクス (`OPS_LISTEN`) |

> **ポート定義**: `config/constants.go:14-15` で `HTTP_ADDR=:9300`, `CONNECT_ADDR=:9301` として定義。環境変数で上書き可能。

### Search Handler
- 必須クエリパラメータ: `q` (検索文字列), `user_id`
- 不足時は `400 Bad Request`
- フィルター: `user_id = "<value>"` (エスケープ処理済み)
- レスポンス: `application/json` - id, title, content, tags, score, language, published_at

### Indexing Loop (Dual-Phase)

`bootstrap/app.go` の `runIndexLoop` はデュアルフェーズで動作:

**Phase 1 - Backfill:**
- 既存記事を最新から最古の順にインデックス
- `ExecuteBackfill(ctx, lastCreatedAt, lastID, batchSize)` で 200 件バッチ処理
- インデックス対象が 0 件になるまで継続

**Phase 2 - Incremental:**
- 新規記事のポーリングと削除済み記事の同期
- `ExecuteIncremental(ctx, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, batchSize)` で処理
- 新記事/削除なしの場合は `INDEX_INTERVAL` (デフォルト 5m) スリープ

**共通リトライ:**
- 指数バックオフ (初期 5s, 最大 5m, 倍率 2x) (`cenkalti/backoff/v6`)
- 成功時にバックオフをリセット

`RECAP_WORKER_URL` を設定すると `runRecapIndexLoop` が同じ Backfill/Incremental パターンで
recap-worker のジャンルを `recaps` インデックスへポーリングインデックスする (デフォルト間隔
`RECAP_INDEX_INTERVAL=5m`、バッチサイズ `RECAP_INDEX_BATCH_SIZE=200`)。recap インデックスの
`EnsureIndex` が失敗しても記事インデックスは継続する (non-fatal)。

## Data Access Mode ([[000954]])

3-binary 分割 (ADR-000954) 以降、`driver/backend_api` パッケージが実際に呼ぶ相手は alt-backend
ではなく **alt-data-hub の `services.datahub.v1.DataHubService`** である。パッケージ名は移行前の
名残で残っているだけ。`BACKEND_API_URL` は **必須環境変数**で未設定だと `config.Load()` が起動時
エラーで落ちる (fail-fast) が、実際に使われるのは `MTLS_ENFORCE=false` のときの平文フォールバック
限定で、compose での値は `http://alt-backend:9102` (alt-backend の operator listener、データ
プレーンは持たない)。compose の既定である `MTLS_ENFORCE=true` では `BACKEND_API_MTLS_URL` が
優先され、`https://alt-data-hub:9443` が実際に `DataHubService` へ到達する宛先になる。未設定時に
フォールバックする PostgreSQL 直接接続 (Legacy DB モード) はコードから削除済み。認証はトークン
ファイルではなく mTLS (`BACKEND_MTLS_SERVER_NAME` で SAN を検証) で行う。

### Article Reference (consumer 側は thin only, [[000953]])

`consumer/event_handler.go` は `alt:events:articles` の payload から `article_id` だけを読み、
バッチ (`batchFlushSize=10` または `batchFlushInterval=2s` のどちらか早い方) ごとに alt-data-hub
から本文を再取得する (`ExecuteBatchArticles` → `GetArticleByID`)。参照は "latest" 解決で version
固定はしない。処理対象イベント種別は `ArticleCreated` / `ArticleUpdated` / `IndexArticle` の 3 種
のみ — それ以外は "unknown event type" として即 ACK され、インデックスには影響しない。この
thin 化は 2026-07-31 の ADR-000953 の段階3にあたり、この consumer 側は完了している。**ただし
producer 側 (alt-data-hub) は段階4 (payload から `content`/`tags` を削除する) が未実施で、今も
本文を同梱したまま publish している** — search-indexer 側はそれを単に読まないだけであり、
ストリームの 1 件あたりバイト量自体はまだ縮んでいない (詳細: [[wiki/services/mq-hub]])。

## Meilisearch Indices & Settings

`bootstrap/app.go` の起動時 `EnsureIndex` がインデックスと設定を宣言的に反映する (settings 変更は
既存インデックスにも毎回再適用される)。

| Index | Primary Key | Searchable | Filterable | Notes |
|-------|-------------|------------|------------|-------|
| `articles` | `id` | `title`, `content`, `tags` | `tags`, `user_id`, `published_at`, `language` | Ranking rules は Meilisearch デフォルト (words/typo/proximity/attribute/sort/exactness)。`LocalizedAttributes` で `jpn`/`eng` を `title`/`content`/`tags` に適用 (未設定だと CJK が既定トークナイザーで分割され日本語 BM25 が 0 件になる) |
| `recaps` | `id` | `tags`, `top_terms`, `summary`, `genre` | `genre`, `window_days` | Sortable: `executed_at`。`RECAP_WORKER_URL` 設定時のみ作成 |

`MEILI_SEARCH_CUTOFF_MS=0` は「上限なし」を意味し、`EnsureIndex` は search cutoff の更新自体を
スキップして Meilisearch のデフォルトのままにする。

`MEILI_HYBRID_EMBEDDER` を設定すると `articles` インデックスに Ollama 経由のベクトル embedder
(既定 `bge-m3`, 1024 次元, `knowledge-embedder-local` を利用) を宣言し、`/v1/search` はハイブリッド
検索 (`MEILI_HYBRID_SEMANTIC_RATIO`, 既定 0.5) で応答する。空なら BM25 のみ (embedder 未管理)。

### Meilisearch task-DB 肥大化対策 (PM-2026-047)

2026-07-22、Meilisearch のタスク履歴が全文字通り上限 (~10GiB) に達し、全書き込みが拒否される障害が
発生した。原因は synonyms の全置換 PUT をバッチごとに叩いていたことで、settingsUpdate タスクの
payload が際限なく肥大化して蓄積したこと。恒久対策として 2 つの定期ループが常時走る:

- `runSynonymsFlushLoop` (`MEILI_SYNONYMS_FLUSH_INTERVAL`, 既定 1m): 蓄積した synonyms の union を
  インデックスのたびにではなく固定間隔で 1 回だけ PUT する
- `runTaskPruneLoop` (`MEILI_TASK_PRUNE_INTERVAL`, 既定 6h): `MEILI_TASK_RETENTION` (既定 72h) より
  古い完了済みタスクを削除する。Meilisearch 自身の自動クリーンアップはタスク件数が 100 万件に
  達するまで発火しないため、件数ではなくバイト量で先に飽和するこのワークロードには効かない

`runWarmupLoop` (`MEILI_WARMUP_INTERVAL`, 既定 5m) は embedder を Ollama の常駐セットに留めるための
定期プローブで、task-DB 対策とは無関係。`MEILI_SEARCH_CACHE_SIZE` / `MEILI_SEARCH_CACHE_TTL` は
検索結果の in-memory LRU キャッシュ (既定 1024 件、TTL 5m、0 で無効化)。

## Configuration & Env

### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_API_URL` | - | 必須 (未設定は fail-fast)。`MTLS_ENFORCE=false` のときのみ使われる平文フォールバック — compose の値は `http://alt-backend:9102` (データプレーンなし) |
| `BACKEND_API_MTLS_URL` | - | `MTLS_ENFORCE=true` (compose 既定) のとき `BACKEND_API_URL` より優先される、alt-data-hub `DataHubService` への実際の宛先。compose の値は `https://alt-data-hub:9443` |
| `BACKEND_MTLS_SERVER_NAME` | - | alt-data-hub leaf 証明書の SAN と一致させる TLS ServerName |
| `MEILISEARCH_HOST` | - | Meilisearch ホスト (必須) |
| `MEILISEARCH_API_KEY_FILE` | - | Meilisearch 管理 API キーファイル |
| `MEILISEARCH_API_KEY` | - | 管理 API キーを直接渡す場合 (`_FILE` が未設定のときのフォールバック、Docker Secrets 未使用の環境向け) |
| `MEILISEARCH_SEARCH_API_KEY_FILE` | - | 検索専用 API キーファイル (未設定時は管理キーを流用) |
| `MEILISEARCH_SEARCH_API_KEY` | - | 検索専用 API キーを直接渡す場合 (`_FILE` が未設定のときのフォールバック) |
| `MEILI_MASTER_KEY_FILE` | - | Meilisearch マスターキーファイル |
| `MEILI_HYBRID_EMBEDDER` | - | ハイブリッド検索の embedder 名。空でハイブリッド無効 (BM25 のみ) |
| `MEILI_HYBRID_SEMANTIC_RATIO` | 0.5 | ハイブリッド検索のセマンティック比率 |
| `MEILI_EMBEDDER_MODEL` | `bge-m3` | 宣言する embedder のモデル名 |
| `MEILI_EMBEDDER_URL` | `http://knowledge-embedder-local:11434/api/embed` | Ollama embed エンドポイント |
| `MEILI_EMBEDDER_DIMENSIONS` | 1024 | embedder のベクトル次元数 |
| `MEILI_SEARCH_CUTOFF_MS` | 1500 | クエリごとの Meilisearch 処理時間上限 (0 で無効) |
| `MEILI_SEARCH_CACHE_SIZE` | 1024 | 検索結果 LRU キャッシュのエントリ数上限 (0 で無効) |
| `MEILI_SEARCH_CACHE_TTL` | 5m | キャッシュの有効期限 |
| `MEILI_TASK_PRUNE_INTERVAL` | 6h | 完了済み Meilisearch タスクの削除間隔 |
| `MEILI_TASK_RETENTION` | 72h | タスク履歴の保持期間 (これより古いものが削除対象) |
| `MEILI_WARMUP_INTERVAL` | 5m | embedder ウォームアッププローブの間隔 |
| `MEILI_SYNONYMS_FLUSH_INTERVAL` | 1m | synonyms 全置換 PUT の実行間隔 |
| `RECAP_WORKER_URL` | - | recap-worker の URL。空で recap インデックス無効 |
| `RECAP_INDEX_INTERVAL` | 5m | recap Incremental フェーズのポーリング間隔 |
| `RECAP_INDEX_BATCH_SIZE` | 200 | recap バッチサイズ |
| `INDEX_BATCH_SIZE` | 200 | バッチサイズ |
| `INDEX_INTERVAL` | 5m | インデックスポーリング間隔 (`config/constants.go`) |
| `INDEX_RETRY_INTERVAL` | 1m | リトライ間隔 |
| `DB_TIMEOUT` | 10s | `config/constants.go` に残る定数だが、Legacy DB 直接接続の削除後は読み手がなく現状は未使用 |
| `HTTP_ADDR` | `:9300` | REST HTTP リッスンアドレス |
| `CONNECT_ADDR` | `:9301` | Connect-RPC リッスンアドレス |
| `MEILI_TIMEOUT` | 15s | Meilisearch タイムアウト |
| `MTLS_LISTEN` | `false` | `:9443` で REST + Connect-RPC を mTLS 提供するか |
| `MTLS_PORT` | `9443` | `MTLS_LISTEN=true` のときの mTLS リスナーのポート |
| `MTLS_ENFORCE` | `false` | outbound (alt-data-hub 呼び出し) を mTLS 必須にするか |
| `MTLS_CLIENT_AUTH` | - | `require_and_verify` のときのみ inbound クライアント証明書を検証 |
| `OPS_LISTEN` | `127.0.0.1:9110` | PKI enrollment + deep health 用の ops メトリクスリスナー |

### Redis Streams Consumer

| Variable | Default | Description |
|----------|---------|-------------|
| `CONSUMER_ENABLED` | `false` | Redis Streams コンシューマー有効化 |
| `REDIS_STREAMS_URL` | `redis://localhost:6379` | Redis 接続 URL |
| `CONSUMER_GROUP` | `search-indexer-group` | コンシューマーグループ名 |
| `CONSUMER_NAME` | `search-indexer-1` | コンシューマー名 |
| `CONSUMER_STREAM_KEY` | `alt:events:articles` | Redis Stream キー |
| `CONSUMER_BATCH_SIZE` | 10 | メッセージ読み取りバッチサイズ |
| `CONSUMER_DLQ_STREAM` | `alt:events:articles:dlq` | 配信上限超過メッセージの送り先ストリーム |
| `CONSUMER_MAX_DELIVERIES` | 5 | この回数を超えて再配信されたメッセージを DLQ へ送る (0 で DLQ 無効) |
| `CONSUMER_REAPER_INTERVAL` | 60s | XAUTOCLAIM による PEL 回収ループの実行間隔 |
| `CONSUMER_DLQ_MAX_LEN` | 10000 | DLQ ストリームの `XADD MAXLEN ~` 上限 |

Redis Streams コンシューマーの契約 (`consumer/redis_consumer.go`, `consumer/dlq.go`):
- `XREADGROUP` (`CONSUMER_GROUP` / `CONSUMER_NAME`) でメッセージを取得し、`XACK` は
  `IndexEventHandler` のバッチ flush が Meilisearch への書き込みを durable に確定した後にのみ発行する
  (受信時 / バッファ投入時の即 ACK は行わない)
- `reclaimLoop` が `ClaimIdleTime` (既定 30s) を超えて未 ACK の PEL エントリを `XAUTOCLAIM` で
  カーソルが `0-0` に戻るまで回収し、ハンドラへ再投入する。このループが無いと `ClaimIdleTime` は
  設定するだけで何も起きない
- 再配信で配信回数が `CONSUMER_MAX_DELIVERIES` を超えたメッセージは `CONSUMER_DLQ_STREAM` へ
  `XADD` (`dlq_reason`, `dlq_delivery_count` 等を付与) してから元ストリームを `XACK` する。DLQ への
  書き込みと ACK は独立して再試行され、DLQ 書き込み済みだが ACK が失敗したメッセージは
  重複コピーせず ACK だけを再試行する

### OpenTelemetry

| Variable | Default | Description |
|----------|---------|-------------|
| `OTEL_ENABLED` | `true` | OTel プロバイダー有効化 |
| `OTEL_SERVICE_NAME` | `search-indexer` | サービス名 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | OTLP HTTP エンドポイント |
| `OTEL_TRACE_SAMPLE_RATIO` | `0.1` | トレースサンプリング比率 (0.0-1.0) |
| `SERVICE_VERSION` | `0.0.0` | サービスバージョン |
| `DEPLOYMENT_ENV` | `development` | デプロイ環境 |

## Testing & Tooling

```bash
# テスト実行
go test ./...

# ヘルスチェック (Docker)
./search-indexer healthcheck

# 検索テスト
curl "http://localhost:9300/v1/search?q=test&user_id=tenant-1"
```

**Driver Tests:**
- `meilisearch-go` の `ServiceManager` インターフェースをフェイクしたユニットテスト (実 Meilisearch には接続しない)
- Pact CDC (consumer/provider contract) テストは `//go:build contract` タグで分離 (`go test -tags=contract ./...`)
- インデックススキーマ変更時は `EnsureIndex` テストを追加

## Operational Runbook

1. 環境変数設定して起動:
   ```bash
   docker compose -f compose/workers.yaml up search-indexer -d
   ```

2. Meilisearch ヘルスチェック:
   ```bash
   curl http://localhost:7700/health
   ```

3. 検索エンドポイントテスト:
   ```bash
   curl "http://localhost:9300/v1/search?q=test&user_id=tenant-1"
   ```

4. インデックスループは 1 プロセスのみ実行 (`INDEX_BATCH_SIZE` + `INDEX_INTERVAL` で調整)

5. 再インデックスが必要な場合はカーソルをリセットしてサービス再起動

## Observability

### 構造化ログ
- バッチごとの `Indexed` / `Deleted` カウント、カーソル更新
- Phase 1 (Backfill) / Phase 2 (Incremental) のステージ表示
- OTel 有効時は `logger/trace_context_handler.go` でトレースコンテキストをログに自動付与
- rask.group ラベル: `search-indexer`
- ヘルスチェック: `/health`

### OTel Metrics

`utils/otel/metrics.go` で定義。メーター名: `search-indexer`。

| Metric | Type | Description | Attributes |
|--------|------|-------------|------------|
| `search_indexer_indexed_total` | Counter | インデックスされた記事の総数 | `phase` (backfill/incremental) |
| `search_indexer_deleted_total` | Counter | インデックスから削除された記事の総数 | `phase` (backfill/incremental) |
| `search_indexer_errors_total` | Counter | エラーの総数 | `operation` (backfill/incremental/search) |
| `search_indexer_batch_duration_seconds` | Histogram | バッチ処理所要時間 (秒) | `phase` (backfill/incremental) |
| `search_indexer_search_duration_seconds` | Histogram | 検索リクエスト所要時間 (秒) | - |

### OTel Traces
- `middleware/otel_status_middleware.go` で HTTP リクエストにスパンを自動生成
- 5xx レスポンスで `codes.Error` ステータスを設定 (OTel HTTP セマンティック規約準拠)
- W3C Trace Context + Baggage プロパゲーション
- `ParentBased` サンプラー (デフォルト 10% サンプリング)

### OTel Logs
- OTLP HTTP エクスポーター (`/v1/logs`) でログを送信
- バッチプロセッサー (5s 間隔, 最大 512 件/バッチ)

## Known failure patterns

Cross-cutting incident patterns are catalogued in [[crystallized-knowledge]].

- Container vanished and stayed down for 33 hours, Reference Desk silently degraded → no `restart: always` and no `depends_on` from alt-backend; graceful degradation shielded users but sent no operator signal. DNS `no such host` means "container absent" — a different category from `connection refused`. Degradation occurrence itself must be an observable metric → PM-2026-023, [[000703]].
- Consumers received 401 and produced empty output after `X-Service-Token` enforcement → the provider strengthened requirements without enumerating consumers; consumers without a Pact are not protected by CDC. Provider-adds-requirement changes need consumer enumeration + Pact CDC RED first (Critical Rule 7) → PM-2026-025 / PM-2026-026, [[000735]].
- Infinite scroll crashed / stalled at 20 items on `/feeds/search` → Meilisearch hybrid search offset pagination is not disjoint across pages (dynamic ranking re-returns ids at page boundaries); frontends consuming this API must dedupe by id, and keyed `{#each}` consumers depend on that contract → PM-2026-044.
- Japanese queries could not find English articles → Meilisearch CJK locale restriction combined multiplicatively with two other services' gaps; cross-language search breaks as a product of layers, so layer-local tests are insufficient → PM-2026-021.
- CI e2e legs failed with "network not found" → the e2e reclaim helper prefix-matched all `alt-staging-*` networks and deleted sibling jobs' created-but-not-yet-attached networks; `docker network rm` protection only applies after attach, and concurrent CI helpers must touch nothing outside their own project → PM-2026-046.
- Meilisearch task-DB hit its byte-size ceiling (~10GiB) and refused all writes, crash-looping `EnsureIndex` on every search-indexer restart → a synonyms full-replace PUT fired on every index cycle, and each `settingsUpdate` task snapshot the whole dictionary, so the task history grew unbounded; Meilisearch's own task-count cleanup (triggers at 1M tasks) never fires against a byte-bound workload. Fixed by batching the synonyms PUT onto a fixed interval (`runSynonymsFlushLoop`) and pruning completed tasks older than a retention window (`runTaskPruneLoop`) → PM-2026-047.

## LLM Notes
- インデックスロジック変更時は `usecase`, `gateway`, `driver` のどこに属するか明示
- 定数 (`INDEX_BATCH_SIZE`, `INDEX_INTERVAL`) は `config/constants.go` で集中管理
- ポート: REST 9300 (`HTTP_ADDR`), Connect-RPC 9301 (`CONNECT_ADDR`) - `config/constants.go:14-15`
- MeCab トークナイザーでインデックス/クエリスコアリングを統一
- OTel メトリクス計装は `utils/otel/metrics.go`、記録は `bootstrap/app.go` の `recordBatch` / `recordError` と `rest/handler.go`
