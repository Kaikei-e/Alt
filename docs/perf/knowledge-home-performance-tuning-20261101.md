# Knowledge Home 表示パフォーマンスチューニング報告書（ログ・トレース・実行計画実測版）

**測定・実施日**: 2026-11-01  
**対象画面 / RPC**:
- フロントエンド: `/home` (`alt-frontend-sv`)
- オーケストレーターRPC: `/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome` (`alt-backend`)
- データオーナーRPC: `/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeHomeItems`, `DeriveTrailClusterCandidates` (`knowledge-sovereign`)
- データベース: `db` (PostgreSQL 17.11), `knowledge-sovereign-db` (PostgreSQL 16.15)

---

## 1. エグゼクティブサマリー

Knowledge Home画面（`/home`）の初期表示において、コールドキャッシュ時に **約30秒のストール**、ウォームキャッシュ時にも **約0.8〜1.1秒のRPCレイテンシ** が発生していた問題を、ClickHouse上のOpenTelemetry分散トレース（`rask_logs.otel_traces`）・構造化ログ（`rask_logs.logs`）・PostgreSQLの活動状況（`pg_stat_activity`）および実行計画（`EXPLAIN (ANALYZE, BUFFERS)`）から実測ベースで特定・解消した。

### チューニング前後の主要指標比較

| 計測指標 | チューニング前 (Before) | チューニング後 (After) | 改善効果 |
| :--- | :--- | :--- | :--- |
| **`GetKnowledgeHome` RPC（コールドキャッシュ時）** | **`30,006.19 ms`** (`tag_hotspot` 30s timeout) | **`60.26 ms`** (HTTP往復 `66.5 ms`) | **約498倍高速化 (-99.8%)** |
| **`GetKnowledgeHome` RPC（ウォームキャッシュ時）** | **`765.64 〜 1,073.49 ms`** | **`51.63 〜 57.49 ms`** (HTTP往復 `52.4 ms`) | **約15〜20倍高速化 (-93%〜-95%)** |
| **`GetKnowledgeHomeItems` SQL (`EXPLAIN ANALYZE`)** | **`321.4 ms`** (`Seq Scan`, `read=47,221` [385 MB]) | **`49.8 ms`** (`Index Only Scan`, `hit=1,230 read=0` [9.6 MB]) | **6.5倍高速化 / バッファ読込 -97.5% / 物理I/O 0** |
| **`DeriveTrailClusterCandidates` SQL (`EXPLAIN ANALYZE`)** | **`399.8 ms`** (`Seq Scan`, `read=48,370` [380 MB]) | **`34.0 ms`** (`Bitmap Index Scan`, `hit=6,359 read=0`) | **11.8倍高速化 / 物理I/O 0** |
| **`knowledge_home_items` テーブルサイズ** | **`379 MB`** (47,221ページ、約73%がデッド領域) | **`97 MB`** (12,371ページ、`relallvisible = 100%`) | **-74.4% 縮小（`shared_buffers` 内に常駐可能）** |
| **`db` / `knowledge-sovereign-db` ハングバックエンド数** | **5プロセス**が50分〜11時間 `ExecuteGather` / `BufferIo` で停滞 | **`0` プロセス** | **完全解消** |

---

## 2. 計測手法とボトルネック特定プロセス

推測による最適化を排し、以下の4段階の観測データ突合によってボトルネックを特定した。

1. **分散トレース分析 (`ClickHouse: rask_logs.otel_traces`)**
   - `GetKnowledgeHome` 親スパンと、`knowledge-sovereign` 側の各子スパン（`GetKnowledgeHomeItems`, `GetRecallCandidates`, `GetTodayDigest`, `CountNeedToKnowItems`, `GetProjectionFreshness`, `ResolveLensFilter`）およびバックグラウンドジョブ（`DeriveTrailClusterCandidates`）の所要時間を時系列で突合した。
2. **構造化ログ分析 (`ClickHouse: rask_logs.logs`)**
   - `30,006 ms` を要したトレースのタイムスタンプと同一時刻の `alt-backend` ログを突合し、`tag_hotspot enrichment failed (non-fatal): context deadline exceeded` が出力されていることを確認した。
3. **データベース稼働状態分析 (`pg_stat_activity`)**
   - `db` および `knowledge-sovereign-db` の `pg_stat_activity` を調査し、`state != 'idle'` のまま長時間滞留しているクエリと待機イベント（`wait_event_type`, `wait_event`）を計測した。
4. **物理ページ・バッファ読込分析 (`EXPLAIN (ANALYZE, BUFFERS)`, `pg_class`)**
   - 遅延していたSQLに対して `EXPLAIN (ANALYZE, BUFFERS)` を実行し、論理行数だけでなく `shared hit` / `shared read` ブロック数（1ブロック = 8 KB）と `relpages` / `relallvisible` を計測した。

---

## 3. 特定された5つのボトルネックと根本原因

```
[Before: 初期表示時のクリティカルパス]

Browser (/home)
 ├─ 1. ListLenses (直列待機) ──────────► [ウォーターフォール遅延]
 └─ 2. GetKnowledgeHome
     ├─ getHomeUsecase.Execute
     │   ├─ GetKnowledgeHomeItems ─────► [Seq Scan 385MB read: 600~830ms] (DeriveTrailClusterCandidatesとキャッシュスラッシング)
     │   ├─ GetTodayDigest ────────────► [1~2ms]
     │   ├─ CountNeedToKnowItems ──────► [4~6ms]
     │   ├─ GetProjectionFreshness ────► [1ms]
     │   └─ TagHotspot (TrendingTags) ─► [キャッシュミス時: 7d + 30d 並列重集計 + IPC ExecuteGather 停滞 → 30,000ms Timeout!]
     └─ recallRailUsecase.Execute (getHomeUsecase完了後に直列実行) ─► [5~18ms]
```

### ボトルネック1：`TrendingTagsGateway` のキャッシュミス時における並列重集計とPostgreSQLパラレルワーカーのIPC停滞
- **事象**: `rask_logs.otel_traces` において、`GetKnowledgeHome` が60分（キャッシュTTL）ごとに **`30,006.19 ms`** かかり、`tag_hotspot enrichment failed (non-fatal): context deadline exceeded` が記録されていた。
- **根本原因**:
  1. `TrendingTagsGateway.GetTrendingTags` がキャッシュミス時に「直近7日間」と「直近30日間」の2つのウィンドウに対して、`articles` × `article_tags` × `feed_tags` を結合・集計する `FetchTagArticleCounts` クエリを **2本同時に並列発行** していた。
  2. さらに複数のクライアントリクエストが同時にキャッシュミスすると、同一ユーザーに対して同じ集計クエリ群が重複実行されていた。
  3. `db` コンテナ（当時 `shm_size: 128m`, `shared_buffers: 256MB`）上でPostgreSQLのパラレルクエリワーカーが起動した際、共有メモリおよび動的共有メモリ（DSM）キューの競合により、リーダーとワーカーが `IPC | ExecuteGather` / `IPC | MessageQueueInternal` / `IO | BufferIo` で **50分〜11時間にわたりデッドロック状態で停滞** し、CPUとI/O帯域を圧迫していた。

### ボトルネック2：`GetKnowledgeHomeUsecase` における非必須エンリッチメントの30秒待機
- **事象**: `tag_hotspot` は失敗しても画面表示を継続できる非必須（non-fatal）エンリッチメントとして設計されていたにもかかわらず、`tag_hotspot` の遅延時に `GetKnowledgeHome` 全体が30秒間ブロックされていた。
- **根本原因**: `GetKnowledgeHomeUsecase.Execute` 内の `sync.WaitGroup` が、親コンテキスト（下流ゲートウェイのクライアントタイムアウト30秒）をそのまま非必須エンリッチメントのゴルーチンに渡して `wg.Wait()` していたため、非必須処理のストールがクリティカルパス全体のレイテンシに直結していた。

### ボトルネック3：`knowledge-sovereign-db` におけるテーブル肥大化と毎回380MB超の `Seq Scan`（バッファキャッシュスラッシング）
- **事象**: `rask_logs.otel_traces` において、`GetKnowledgeHomeItems` が **`595.79 〜 832.16 ms`**、15秒周期のバックグラウンドジョブ `DeriveTrailClusterCandidates` が **`802.27 〜 1,355.43 ms`** を要していた。
- **根本原因**:
  1. **テーブル肥大化（Bloat）**: `knowledge_home_items` は76,908行（実データ約97 MB）であるにもかかわらず、投影更新（UPSERT）の繰り返しによりヒープが **379 MB（47,221ページ、約73%がデッドタプル領域）** に肥大化していた。
  2. **バッファキャッシュスラッシング**: `knowledge-sovereign-db` の `shared_buffers` はデフォルトの `128 MB` であったため、379 MBの `knowledge_home_items` がメモリに収まらず、`GetKnowledgeHomeItems` と `DeriveTrailClusterCandidates` が走るたびに **47,221〜48,370ページ（約380 MB）の物理ディスク読込（`shared read`）** を伴う `Seq Scan` が発生し、互いのキャッシュを追い出し合っていた。
  3. **ワイド行の早期フェッチ**: `GetKnowledgeHomeItems` が全7.5万行のワイドカラム（`title`, `summary_excerpt`, `tags_json`, `why_json` 等、平均約1.5 KB/行）を読み出してから時間減衰スコア（`rank_score`）でソートし、上位21件のみを返していた。

### ボトルネック4：`Handler.GetKnowledgeHome` における `recallRailUsecase` の直列実行
- **事象**: `alt-backend/app/orchestrator/connect/v2/knowledge_home/home_query.go` において、`h.getHomeUsecase.Execute` が完了した後に直列で `h.recallRailUsecase.Execute` を呼び出していた。
- **根本原因**: 両ユースケースは互いにデータ依存がないにもかかわらず直列に実行されており、`GetRecallCandidates` のRPC往復時間がそのまま加算されていた。

### ボトルネック5：`alt-frontend-sv`（`/home`）マウント時の直列ウォーターフォールと冗長な `goto()`
- **事象**: `alt-frontend-sv/src/routes/(app)/home/+page.svelte` の `onMount` において、`await lens.fetchLenses()` の完了を待ってから `home.fetchData()` を呼び出していた。
- **根本原因**:
  1. URLクエリに `?lens=` が指定されていない場合でも、バックエンドの `GetKnowledgeHomeUsecase` は `ResolveLensFilter` によってサーバー側の選択中Lensを自動解決できるにもかかわらず、フロントエンド側で `ListLenses` RPCの完了を直列に待っていた。
  2. また `syncLensQuery` が現在のURLと同一のクエリであっても無条件に `goto()` を呼び出していた。

---

## 4. 実施したチューニング施策の詳細

### 施策1：`knowledge-sovereign` — Late Materialization とカバリングインデックスの導入・テーブルコンパクション

#### (1) `GetKnowledgeHomeItems` の Late Materialization 化
- **対象ファイル**: `knowledge-sovereign/app/driver/sovereign_db/read_projections.go`
- 内側のサブクエリ `ranked` では、主キー（`user_id, item_key, projection_version`）と時間減衰ランキング計算に必要な軽量列（`score, published_at, generated_at`）のみを参照して上位 `$N` 件（21件）に絞り込み、外側のクエリで `knowledge_home_items` の主キーに対して21件だけ `Nested Loop` で結合してワイドカラム（`title`, `summary_excerpt`, `tags_json`, `why_json` 等）を取得する構造に変更した。

```sql
SELECT khi.user_id, khi.tenant_id, khi.item_key, khi.item_type, khi.primary_ref_id,
    khi.title, khi.summary_excerpt, khi.tags_json, khi.why_json, khi.score,
    ranked.rank_score, ranked.rank_as_of,
    khi.freshness_at, khi.published_at, khi.last_interacted_at, khi.generated_at, khi.updated_at,
    khi.dismissed_at, khi.summary_state, COALESCE(khi.url, '') AS url,
    khi.supersede_state, khi.superseded_at, khi.previous_ref_json, khi.projection_version
FROM (
    SELECT khi.user_id, khi.item_key, khi.projection_version, khi.published_at,
        khi.score::double precision / (1 + GREATEST(EXTRACT(EPOCH FROM (now() - COALESCE(khi.published_at, khi.generated_at))) / 86400, 0)) AS rank_score,
        now() AS rank_as_of
    FROM knowledge_home_items khi
    WHERE khi.user_id = $1
      AND khi.projection_version = COALESCE((SELECT version FROM knowledge_projection_versions WHERE status = 'active' ORDER BY version DESC LIMIT 1), 1)
      AND khi.dismissed_at IS NULL
      AND COALESCE(khi.title, '') <> ''
    ORDER BY rank_score DESC, COALESCE(khi.published_at, '-infinity') DESC, khi.item_key DESC
    LIMIT $2
) ranked
JOIN knowledge_home_items khi
  ON khi.user_id = ranked.user_id
 AND khi.item_key = ranked.item_key
 AND khi.projection_version = ranked.projection_version
ORDER BY ranked.rank_score DESC, COALESCE(khi.published_at, '-infinity') DESC, khi.item_key DESC
LIMIT $2;
```

#### (2) カバリング部分インデックスおよび式インデックスの追加
- **対象ファイル**: `knowledge-sovereign/migrations/00033_add_home_ranking_covering_indexes.sql`
- 内側の `ranked` サブクエリがヒープを一切参照せずに `Index Only Scan`（`Heap Fetches: 0`）で完結するよう、`INCLUDE (score, published_at, generated_at, item_key)` を持つカバリング部分インデックス `idx_khi_home_rank_covering`（サイズわずか **8.8 MB / 1,140ページ**）を追加した。
- 併せて、`DeriveTrailClusterCandidates` の30日ウィンドウフィルタ（`COALESCE(published_at, updated_at) >= NOW() - INTERVAL '30 days'`）に完全一致する部分式インデックス `idx_khi_user_ver_coalesce_pub_upd`（サイズ **2.7 MB / 339ページ**）を追加した。

```sql
CREATE INDEX IF NOT EXISTS idx_khi_home_rank_covering
    ON knowledge_home_items (user_id, projection_version)
    INCLUDE (score, published_at, generated_at, item_key)
    WHERE dismissed_at IS NULL AND COALESCE(title, '') <> '';

CREATE INDEX IF NOT EXISTS idx_khi_user_ver_coalesce_pub_upd
    ON knowledge_home_items (user_id, projection_version, COALESCE(published_at, updated_at) DESC)
    WHERE dismissed_at IS NULL AND item_type = 'article';
```

#### (3) ヒープコンパクションと Visibility Map の確立
- `VACUUM (FULL, ANALYZE) knowledge_home_items;` によりヒープサイズを **379 MB → 97 MB（47,221ページ → 12,371ページ）** に縮小した。
- **重要な知見**: PostgreSQLの `VACUUM FULL` はテーブルを新規ヒープファイルに書き直す（`CLUSTER` 相当）ため、書き直し直後は Visibility Map（`pg_class.relallvisible`）が `0` にリセットされる。`relallvisible = 0` の状態ではプランナが「`Index Only Scan` でも100%のヒープフェッチが必要になる」と見積もり、`Seq Scan` を選択してしまう。そのため、`VACUUM FULL` の直後に通常の `VACUUM ANALYZE knowledge_home_items;` を実行して `relallvisible = 12,371 (100%)` を確立し、`Heap Fetches: 0` の `Index Only Scan` が安定して選択されることを確認した。

---

### 施策2：`alt-backend` — `TrendingTagsGateway` の自己競合排除と非必須エンリッチメントのタイムアウト制限・並列化

1. **`TrendingTagsGateway` の Singleflight 化と直列ウィンドウ取得**
   - **対象ファイル**: `alt-backend/app/orchestrator/gateway/trending_tags_gateway/gateway.go`
   - `singleflight.Group` を導入し、同一ユーザーに対する同時キャッシュミス時のDBクエリ発行を1回に統合した。
   - 7日間集計と30日間集計の2つの `FetchTagArticleCounts` 呼び出しを並列から **30日間集計 → 7日間集計の順の直列実行** に変更した。30日間スキャンが直近7日間のデータブロックもすべて `shared_buffers` に読み込むため、続く7日間スキャンは物理ディスクI/Oなしのウォームキャッシュヒットとなり、並列実行時の `BufferIo` 自己競合が解消された。
2. **`GetKnowledgeHomeUsecase` の非必須エンリッチメントへのタイムアウト上限設定**
   - **対象ファイル**: `alt-backend/app/orchestrator/usecase/get_knowledge_home_usecase/usecase.go`
   - 非必須エンリッチメント（`tagHotspotPort`, `needToKnowCountPort`, `freshnessPort`）に対して `defaultEnrichmentTimeout = 2 * time.Second` のサブコンテキストタイムアウトを設定し、万が一バックエンドDBが高負荷になっても `GetKnowledgeHome` のレスポンスが最大2秒以上ブロックされないようにした。
3. **`Handler.GetKnowledgeHome` における `recallRailUsecase` の並列実行**
   - **対象ファイル**: `alt-backend/app/orchestrator/connect/v2/knowledge_home/home_query.go`
   - `h.getHomeUsecase.Execute` と `h.recallRailUsecase.Execute` を並列に起動し、クリティカルパスから `recallRailUsecase` の直列待ち時間を削減した。

---

### 施策3：`alt-frontend-sv` — マウント時のウォーターフォール解消と冗長な `goto` 抑止

- **対象ファイル**: `alt-frontend-sv/src/routes/(app)/home/+page.svelte`
- `onMount` 内で `lens.fetchLenses()` の完了を待たずに `home.fetchData(true, urlLensId)` を即座に並列開始するよう変更した。
- URL上に `?lens=` が存在する場合のみ `lens.selectLens(urlLensId)` でクライアント状態を同期し、URL上に `?lens=` がなくサーバー側の `activeLensId` が返ってきた場合は `syncLensQuery` でURLを更新する（すでに同一URLの場合は `goto()` をスキップする）ことで、初期表示時の直列RTTと不要なナビゲーション再評価を排除した。

---

### 施策4：PostgreSQL コンテナ設定の適正化（`db` / `knowledge-sovereign-db`）

- **対象ファイル**: `docker/postgres/postgresql.conf`, `compose/db.yaml`, `compose/sovereign.yaml`, `compose/auth.yaml`
- `db` および `knowledge-sovereign-db` の共有メモリとバッファ設定を拡張した（`mem_limit: 1g`, `shm_size: "512m"`, `shared_buffers=256MB`, `work_mem=16MB`, `effective_cache_size=768MB`）。
- コンテナ環境下でDSM/IPCデッドロック（`ExecuteGather` / `MessageQueueInternal`）を引き起こしていたパラレルワーカーおよびJIT（`max_parallel_workers_per_gather=0`, `max_parallel_workers=0`, `jit=off`）を無効化した。
- AlpineベースのPostgreSQLイメージにおいて `gosu postgres` がコンテナ起動時にスピンする事象を防ぐため、`user: postgres` を明示して `postgres` プロセスを直接 `exec` する構成に統一した。

---

## 5. チューニング前後の実行計画（`EXPLAIN (ANALYZE, BUFFERS)`）とトレース実測比較

### 5.1 `GetKnowledgeHomeItems` の実行計画比較

#### Before（`Seq Scan` + 全件ワイド行ソート：321.4 ms / 47,221ブロック読込 [385 MB]）
```text
Limit  (cost=48797.50..48797.55 rows=21 width=1516) (actual time=321.178..321.183 rows=21 loops=1)
  Buffers: shared hit=2115 read=47221
  ->  Sort  (cost=48797.50..48985.48 rows=75191 width=1516) (actual time=321.176..321.179 rows=21 loops=1)
        Sort Method: top-N heapsort  Memory: 127kB
        Buffers: shared hit=2115 read=47221
        ->  Seq Scan on knowledge_home_items khi  (cost=0.00..46770.26 rows=75191 width=1516) (actual time=0.042..294.756 rows=75575 loops=1)
              Filter: ((dismissed_at IS NULL) AND (user_id = '00000000-0000-0000-0000-000000000001'::uuid) AND (projection_version = 1))
              Rows Removed by Filter: 1333
              Buffers: shared hit=2113 read=47221
Execution Time: 321.412 ms
```

#### After（`Index Only Scan` + 上位21件のみ主キー結合：49.8 ms / 1,230ブロックヒット [9.6 MB], 物理読込 0）
```text
Limit  (cost=9975.51..10145.10 rows=21 width=1470) (actual time=49.654..49.656 rows=21 loops=1)
  Buffers: shared hit=1230
  ->  Incremental Sort  (cost=9975.51..10145.10 rows=21 width=1470) (actual time=49.653..49.655 rows=21 loops=1)
        Buffers: shared hit=1230
        ->  Nested Loop  (cost=9967.07..10144.16 rows=21 width=1470) (actual time=49.516..49.609 rows=21 loops=1)
              Buffers: shared hit=1218
              ->  Limit  (cost=9966.65..9966.71 rows=21 width=97) (actual time=49.477..49.480 rows=21 loops=1)
                    Buffers: shared hit=1134
                    ->  Sort  (cost=9951.35..10139.33 rows=75192 width=97) (actual time=49.476..49.477 rows=21 loops=1)
                          Sort Method: top-N heapsort  Memory: 29kB
                          Buffers: shared hit=1134
                          ->  Index Only Scan using idx_khi_home_rank_covering on knowledge_home_items khi_1  (cost=0.42..7924.06 rows=75192 width=97) (actual time=0.043..42.277 rows=75575 loops=1)
                                Index Cond: ((user_id = '00000000-0000-0000-0000-000000000001'::uuid) AND (projection_version = COALESCE($0, 1)))
                                Heap Fetches: 0
                                Buffers: shared hit=1134
              ->  Index Scan using knowledge_home_items_pkey on knowledge_home_items khi  (cost=0.42..8.44 rows=1 width=1500) (actual time=0.005..0.005 rows=1 loops=21)
                    Index Cond: ((user_id = khi_1.user_id) AND (item_key = khi_1.item_key) AND (projection_version = khi_1.projection_version))
                    Buffers: shared hit=84
Execution Time: 49.820 ms
```

---

### 5.2 `DeriveTrailClusterCandidates` の実行計画比較

- **Before**: `Seq Scan on knowledge_home_items` により **48,370ブロック（約380 MB）の物理読込** が発生し、**`399.8 ms`**（コンテンション時は最大 `1,355 ms`）を要していた。
- **After**: `Bitmap Index Scan on idx_khi_user_ver_coalesce_pub_upd` により、インデックス7ブロック＋ヒープ372ブロック（`shared hit=449`, `read=0`）のアクセスに縮小され、クエリ全体が **`34.0 ms`** で完了した。

```text
->  Bitmap Heap Scan on knowledge_home_items khi  (cost=890.15..14079.82 rows=12532 width=273) (actual time=1.131..2.055 rows=703 loops=1)
      Recheck Cond: ((user_id = '00000000-0000-0000-0000-000000000001'::uuid) AND (projection_version = $2) AND (COALESCE(published_at, updated_at) >= (now() - '30 days'::interval)) AND (dismissed_at IS NULL) AND (item_type = 'article'::text))
      Heap Blocks: exact=372
      Buffers: shared hit=449
      ->  Bitmap Index Scan on idx_khi_user_ver_coalesce_pub_upd  (cost=0.00..767.30 rows=25190 width=0) (actual time=0.087..0.087 rows=784 loops=1)
            Index Cond: ((user_id = '00000000-0000-0000-0000-000000000001'::uuid) AND (projection_version = $2) AND (COALESCE(published_at, updated_at) >= (now() - '30 days'::interval)))
            Buffers: shared hit=7
...
Execution Time: 34.000 ms
```

---

### 5.3 エンドツーエンド分散トレース実測（`rask_logs.otel_traces`）

#### Before (`rask_logs.otel_traces`)
```text
2026-11-01 13:01:02.313  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome                     30006.19 ms
2026-11-01 13:00:33.030  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome                      1073.49 ms
2026-11-01 13:00:33.039  knowledge-sovereign  services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeHomeItems             832.16 ms
2026-11-01 13:00:32.334  knowledge-sovereign  DeriveTrailClusterCandidates                                                     1355.43 ms
```

#### After (`rask_logs.otel_traces`)
```text
2026-11-01 13:44:47.676  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome (Run 1: Cold Cache)    60.26 ms
2026-11-01 13:44:47.737  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome (Run 2: Warm Cache)    57.49 ms
2026-11-01 13:44:47.796  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome (Run 3: Warm Cache)    51.88 ms
2026-11-01 13:44:47.849  alt-backend          alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome (Run 4: Warm Cache)    51.63 ms
```

---

## 6. 変更ファイル一覧

| レイヤ / サービス | ファイルパス | 変更内容 |
| :--- | :--- | :--- |
| `knowledge-sovereign` (Driver) | `knowledge-sovereign/app/driver/sovereign_db/read_projections.go` | `buildKnowledgeHomeQuery` を Late Materialization クエリに変更 |
| `knowledge-sovereign` (Test) | `knowledge-sovereign/app/driver/sovereign_db/read_projections_test.go` | Late Materialization SQL構造の単体テスト追加 |
| `knowledge-sovereign` (Migration) | `knowledge-sovereign/migrations/00033_add_home_ranking_covering_indexes.sql` | `idx_khi_home_rank_covering`, `idx_khi_user_ver_coalesce_pub_upd` 追加 |
| `knowledge-sovereign` (Migration) | `knowledge-sovereign/migrations/atlas.sum` | Atlasマイグレーションハッシュ更新 |
| `alt-backend` (Gateway) | `alt-backend/app/orchestrator/gateway/trending_tags_gateway/gateway.go` | `singleflight.Group` による重複排除と 30d → 7d の直列フェッチ化 |
| `alt-backend` (Test) | `alt-backend/app/orchestrator/gateway/trending_tags_gateway/gateway_test.go` | 同時リクエスト時のSingleflight・直列フェッチ検証テスト追加 |
| `alt-backend` (Usecase) | `alt-backend/app/orchestrator/usecase/get_knowledge_home_usecase/usecase.go` | 非必須エンリッチメントへの2秒サブコンテキストタイムアウト導入 |
| `alt-backend` (Test) | `alt-backend/app/orchestrator/usecase/get_knowledge_home_usecase/usecase_test.go` | エンリッチメント遅延時の上限タイムアウト検証テスト追加 |
| `alt-backend` (Connect Handler) | `alt-backend/app/orchestrator/connect/v2/knowledge_home/home_query.go` | `getHomeUsecase` と `recallRailUsecase` の並列実行化 |
| `alt-backend` (Test) | `alt-backend/app/orchestrator/connect/v2/knowledge_home/handler_test.go` | `recallRailUsecase` 並列実行の単体テスト追加 |
| `alt-frontend-sv` (Route) | `alt-frontend-sv/src/routes/(app)/home/+page.svelte` | `onMount` 時の `fetchLenses` / `fetchData` 並列化と冗長な `goto` 抑止 |
| `alt-frontend-sv` (Test) | `alt-frontend-sv/src/routes/(app)/home/page.svelte.test.ts` | マウント時の並列呼び出し・冗長な `goto` 抑止のブラウザ単体テスト追加 |
| PostgreSQL / Compose | `docker/postgres/postgresql.conf`, `compose/db.yaml`, `compose/sovereign.yaml`, `compose/auth.yaml` | `shm_size` / `shared_buffers` 拡張、パラレルワーカー・JIT無効化、`user: postgres` 指定 |

---

## 7. 継続的運用のための設計教訓（Lessons Learned）

1. **動的時間減衰ランキングにおける Late Materialization とカバリングインデックスの組み合わせ**
   - `now()` に依存する減衰スコア（`score / (1 + age_days)`）はB-Treeインデックスに事前ソート順として格納できないが、ランキング計算に必要な列だけを `INCLUDE` したカバリング部分インデックス（8.8 MB）を作成し、上位 `$N` 件の主キーのみを抽出してから本体行（97 MB）に `JOIN` することで、バッファ参照量を **97.5%削減（385 MB → 9.6 MB）** できる。
2. **`VACUUM FULL` 実行後の Visibility Map リセットへの対処**
   - `VACUUM FULL` はテーブルを再構築するため `pg_class.relallvisible` が `0` になる。直後に通常の `VACUUM ANALYZE` を実行して Visibility Map ビットを立てないと、`Index Only Scan` のコスト見積もりに100%のヒープフェッチコストが加算され、`Seq Scan` にフォールバックする点に注意が必要である。
3. **非必須（Non-Fatal）エンリッチメントのタイムアウト境界分離**
   - メインストリームの表示を阻害しない設計の補助データ取得（トレンドタグや鮮度インジケータ等）は、親RPCのコンテキストをそのまま共有せず、必ず短いデッドライン（2秒等）を持つ派生コンテキストで囲むことで、下流障害時のTail Latency爆発を防げる。
