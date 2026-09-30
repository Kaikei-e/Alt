# Knowledge Sovereign

_Last reviewed: September 5, 2026_

**Location:** `knowledge-sovereign/`
**Ports:**
- 9500 (Connect-RPC main API, `LISTEN_ADDR=:9500`, host port `127.0.0.1:9510`)
- 9501 (Metrics & admin API, `METRICS_ADDR=:9501`, host port `127.0.0.1:9511`)

## Role
- Alt プラットフォームにおける永続知識状態 (durable knowledge state) の唯一のオーナーサービス
- 追記専用 (append-only) のイベントログ (`knowledge_events`, `knowledge_user_events`) をイベントソースとし、インプロセスのプロジェクター群が破棄可能 (disposable) なリードモデル (`knowledge_trail_footprints`, `knowledge_home_items`, `today_digest_view`, `recall_candidate_view` 等) を構築
- **reproject-safe / event-bound** (業務事実に `time.Now()` を使用しない) の原則を厳守
- **現行リードパスは Knowledge Trail**: 2026-06-11 以降、足跡の背骨 (footprint spine) とシステム主導の型付きブランチ提案 (typed branch) を提供。旧 Knowledge Loop (3軸直交モデル、4-bucket UI、`KnowledgeLoopService` RPC) は [[000940]] で retire 済み。旧来のリード RPC・UI・`knowledge_loop_*` プロジェクションテーブルは削除済みだが、過去の `knowledge_loop.*` イベントは追記専用ログとして永続し、Trail プロジェクターが足跡として再射影する

## Architecture & Compose Integration

| Property | Value |
| --- | --- |
| Image | `ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-knowledge-sovereign:${IMAGE_TAG:-main}` (`compose/sovereign.yaml`) |
| Connect-RPC Port | 9500 (host: `127.0.0.1:9510`, `LISTEN_ADDR=:9500`) |
| Metrics/Admin Port | 9501 (host: `127.0.0.1:9511`, `METRICS_ADDR=:9501`, health: `/health`) |
| Dedicated Database | `knowledge-sovereign-db` (PostgreSQL 16, host port `127.0.0.1:5438:5432`) |
| DB User / Name | user `sovereign`, database `knowledge_sovereign` (`compose/sovereign.yaml`) |
| Migrator | `knowledge-sovereign-db-migrator` (Atlas, `knowledge-sovereign/migrations`) |
| Snapshot Volume | ホストバインドマウント (`KNOWLEDGE_SOVEREIGN_SNAPSHOTS_HOST_PATH` → `/tmp/snapshots`)。compose ファイル相対パスはデプロイランナーのジョブ毎チェックアウトに解決され消去されるため絶対ホストパスが必須。nonroot 実行下で image 内 `/tmp/snapshots` (root:root) との権限問題を回避するためバインドを使用 ([[000585]], [[000765]]) |
| Secrets | `sovereign_db_password`, `sovereign_admin_token`, `sovereign_event_token` |
| Auth Gates | `ADMIN_AUTH=disabled` / `EVENT_AUTH=disabled` が明示されない限りトークン不在時は fail-fast で起動失敗する |

他サービスからの共有データベース接続は禁止されており、クライアントは Connect-RPC API (:9500) またはイベント経由でのみアクセスする。

## Event Log Architecture

1. **追記専用パーティションテーブル**:
   - `knowledge_events`: システムイベントログ。月次レンジパーティション (`partition_maintainer` が先回りで自動生成)。
   - `knowledge_user_events`: ユーザー操作ログ。月次レンジパーティション。
2. **ギャップ制御 (`projection_gap`)**:
   - `event_seq` は BIGSERIAL で採番順とコミット順が前後しうるため、プロジェクターのチェックポイントは連続した区間の終端で留め、未コミットの穴を跨いで進めない (トランザクション ID で書き込み中とロールバック欠番を判別)。
3. **プロジェクションスナップショット**:
   - `knowledge_home_items` / `today_digest_view` / `recall_candidate_view` の状態を定期的に `.jsonl.gz` としてスナップショット保存し、高速なリプロジェクションを担保。

## Projectors & Workers

### Knowledge Trail (現行リードパス、[[000940]])
- **`knowledge_trail_projector`** (`app/usecase/knowledge_trail_projector/`): append-only イベントログを `knowledge_trail_footprints` へ折り込む。reproject-safe: 各 footprint は単一イベントの payload のみから導出。
- **`trail_planner`** (`app/usecase/trail_planner/`): `trail.branch_proposed.v1` の唯一の発行元 (system-only)。現在の spine と候補 item から、`relation_kind` / `why` / `evidence_refs_json` / `confidence` の 4 要素を NOT NULL で伴う typed branch を提案。
- **`trail_episodes`** (`app/usecase/trail_episodes/`): footprint のまとまりである「エピソード」を純粋導出 (非永続エンティティ)。同一記事 contact は同一エピソードへ、記事間は `tagclean` で正規化したタグ一致と時間窓の両方を満たす場合のみ連結。
- **`tagclean`** (`app/usecase/tagclean/`): ML 生成タグから不要語・数値断片・URL 残骸・表記揺れを除去。
- **`knowledge_trail_act_outcomes`**: ブランチ追跡結果 (滞在時間等) を記録する追記専用テーブル。

### Knowledge Home Projector ([[000944]])
- **`knowledge_home_projector`** (`app/usecase/knowledge_home_projector/`): append-only イベントログを `knowledge_home_items`, `today_digest_view`, `recall_candidate_view` にインプロセスで折り込む。alt-backend から移設され RPC 往復を除去。`SummaryVersionCreated` / `TagSetVersionCreated` はイベント自身が payload を持つため、alt-db への読み返しが不要。

### システム運用ワーカー
- **`partition_maintainer`**: イベントテーブルの月次パーティションを定期的に自動作成。
- **`projection_health`**: イベント種別ごとの最終出現時刻から経過秒を測定し、プロデューサーの liveness ゲージを公開。

## RPC Surface & APIs

### Connect-RPC (Port 9500, Bearer 認証: `EVENT_TOKEN_FILE` / `EVENT_AUTH`)
- **イベント管理 (`rpc_events.go`)**:
  - `AppendKnowledgeEvent`, `AppendKnowledgeUserEvent`, `ListKnowledgeEvents`, `GetLatestEventSeq`
- **プロジェクション参照 (`rpc_projections.go`)**:
  - `GetKnowledgeHomeItems`, `GetTodayDigest`, `GetRecallCandidates`, `ListDistinctUserIDs`, `CountNeedToKnowItems`
- **ミューテーション (`sovereign_handler.go`)**:
  - `ApplyProjectionMutation`, `ApplyRecallMutation`, `ApplyCurationMutation`
- **Knowledge Trail (`rpc_trail.go`)**:
  - `GetTrailFootprints`, `GetTrailBranchesForAnchor`
  - (ブランチ提案はインプロセスの `trail_planner` が `trail.branch_proposed.v1` を発行)
- **プロジェクション基盤 & インフラ (`rpc_infra.go`, `rpc_reproject_backfill.go`)**:
  - プロジェクションバージョン: `GetActiveProjectionVersion`, `ListProjectionVersions`, `CreateProjectionVersion`, `ActivateProjectionVersion`
  - チェックポイント・鮮度・遅延: `GetProjectionCheckpoint`, `UpdateProjectionCheckpoint`, `GetProjectionFreshness`, `GetProjectionLag`
  - リプロジェクション & 監査: `GetReprojectRun`, `ListReprojectRuns`, `CreateReprojectRun`, `UpdateReprojectRun`, `CompareProjections`, `ListProjectionAudits`, `CreateProjectionAudit`
  - バックフィル: `GetBackfillJob`, `ListBackfillJobs`, `CreateBackfillJob`, `UpdateBackfillJob`
  - レンズ可視性: `AreArticlesVisibleInLens`
- **Lens 選択 (`rpc_lens.go`)**:
  - `ListLenses`, `GetLens`, `CreateLens`, `CreateLensVersion`, `SelectCurrentLens`, `ClearCurrentLens`, `ArchiveLens`, `ResolveLensFilter`, `GetCurrentLensSelection`
- **Recall 信号 (`rpc_recall_signals.go`)**:
  - `ListRecallSignals`, `AppendRecallSignal`
- **イベントストリーミング (`rpc_watch.go`)**:
  - `WatchProjectorEvents` (server-streaming。RPC 定義として存在するが、実運用のプロジェクター群はインプロセス ticker で駆動)

### Admin & Metrics (Port 9501, Bearer 認証: `ADMIN_TOKEN_FILE` / `ADMIN_AUTH`)
- `GET /health`: 認証不要ヘルスチェック
- `GET /health/deep`: 要認証の詳細ヘルスチェックおよび合成プローブ
- スナップショット (`snapshot_handler.go`):
  - `POST /admin/snapshots/create`: スナップショット作成
  - `GET /admin/snapshots/list`: スナップショット一覧取得
  - `GET /admin/snapshots/latest`: 最新スナップショット取得
- プロジェクション再構築 (`projection_rebuild.go`):
  - `POST /admin/projections/rebuild`: 再構築実行
  - `GET /admin/projections/rebuild/targets`: 再構築対象一覧
- 保持期間管理 (`retention_handler.go`):
  - `POST /admin/retention/run`: リテンション実行
  - `GET /admin/retention/status`: 実行ステータス取得
  - `GET /admin/retention/eligible`: 削除対象パーティション一覧
- ストレージ統計 (`storage_handler.go`):
  - `GET /admin/storage/stats`: ストレージ統計取得

## Consumers

| Consumer | Interface | Purpose |
| --- | --- | --- |
| `alt-backend` | Connect-RPC (:9500) | `SovereignClient` 経由で Knowledge Home / Trail 画面のデータ取得および Lens 操作 |
| `rag-orchestrator` | Connect-RPC (:9500) | `augur.conversation_linked.v1` イベント発行 (`RAG_ORCHESTRATOR_KNOWLEDGE_EVENT_EMIT` で有効化) |
| `recap-worker` | Connect-RPC (:9500) | `recap.topic_snapshotted.v1` イベント発行 |
| `altctl` | Admin API (:9501) / alt-backend (:9102) | `altctl home` サブコマンド群 (snapshot, retention, storage は sovereign :9501 を直接呼び出す。reproject, audit, backfill は alt-backend :9102 経由、slo も backend URL を使用) |

> **Note**: `X-Alt-Tenant-Id` HTTP ヘッダーは knowledge-sovereign には中継されず到達しない（テナント ID はイベント payload や RPC リクエストメッセージのフィールドとして渡される）。

## Configuration & Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:9500` | Connect-RPC リスナーアドレス |
| `METRICS_ADDR` | `:9501` | メトリクス / Admin リスナーアドレス |
| `DATABASE_URL` | - | PostgreSQL 接続 URL (`compose/sovereign.yaml` のラッパーで生成) |
| `SNAPSHOT_DIR` | `/data/snapshots` (コード) / `/tmp/snapshots` (compose) | スナップショット出力先ディレクトリ |
| `KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL` | `5s` | プロジェクター実行間隔 |
| `KNOWLEDGE_SOVEREIGN_BRANCH_PLANNER_TICK_INTERVAL` | `30s` | Trail Planner 実行間隔 |
| `KNOWLEDGE_SOVEREIGN_PROJECTION_HEALTH_TICK_INTERVAL` | `60s` | ヘルス監視実行間隔 |
| `KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_BATCH_SIZE` | `500` | Trail プロジェクターのバッチサイズ |
| `KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_MAX_BATCHES_PER_TICK` | `4` | 1 tick あたりの最大バッチ数 |
| `KNOWLEDGE_SOVEREIGN_HOME_PROJECTOR_BATCH_SIZE` | `500` | Home プロジェクターのバッチサイズ |
| `KNOWLEDGE_SOVEREIGN_HOME_PROJECTOR_MAX_BATCHES_PER_TICK` | `4` | 1 tick あたりの最大バッチ数 |
| `KNOWLEDGE_SOVEREIGN_TRAIL_MAX_BRANCHES_PER_USER` | `5` | ユーザーごとの最大アクティブブランチ数 |
| `ADMIN_TOKEN_FILE` | - | Admin / Metrics API の Bearer トークンファイル |
| `EVENT_TOKEN_FILE` | - | Connect-RPC API の Bearer トークンファイル |

## Core Invariants

1. **プロジェクターはイベントペイロードのみを参照する (Reproject-safe)**: 現在状態のクエリや外部 DB の参照を禁止し、過去のイベントログのみから決定論的に同一のリードモデルを再構築可能にする。
2. **業務事実に `time.Now()` を使用しない**: すべてのタイムスタンプはイベントに記録された `occurred_at` を起点とする。
3. **プロデューサーとプロジェクターの責務分離**: プロデューサー (`trail_planner` 等) は現在状態をクエリして何を提案・発行するか判断してよいが、発行されたイベントを畳み込むプロジェクター側は pure fold (payload only) を維持する。

## Known Failure Patterns

Cross-cutting incident patterns are catalogued in [[runbooks/crystallized-knowledge]].

- **Projector Lag**: イベント発行速度に対してプロジェクター処理が追いつかない場合、リードモデルの鮮度が低下する。`projection_health` およびチェックポイントの乖離を監視し、必要に応じてバッチパラメータをチューニングする。なお、Prometheus のアラートルール (`observability/prometheus/rules/knowledge-loop-rules.yml`) は、退役済みの旧 Loop プロジェクター (`knowledge_loop_projector_*`) を現在もターゲットとしている。
- **event_seq の欠番進行**: コミット順と採番順の不一致により、未コミットのシーケンス番号を跨いでチェックポイントを進めてしまうと、対象イベントがリードモデルから欠落する (`projection_gap` で防御)。
- **DEFAULT パーティションへの蓄積**: パーティションの事前作成 (`partition_maintainer`) が停止するとデータが DEFAULT パーティションに退避され、後日の切り離し・クリーンアップに追加のマイグレーション作業が必要になる。

## Operational Runbook & Health Check

```bash
# ヘルスチェック (Host Port 9511, Container Port 9501)
curl http://localhost:9511/health

# 詳細ヘルスチェック (要 Bearer Token)
curl -H "Authorization: Bearer $(cat /path/to/token)" http://localhost:9511/health/deep

# テスト実行
go test ./...

# サービス再起動
docker compose -f compose/compose.yaml -p alt up -d --build knowledge-sovereign
```
