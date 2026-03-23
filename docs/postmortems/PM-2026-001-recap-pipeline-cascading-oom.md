# PM-2026-001: recap パイプライン連鎖 OOM 障害

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3 |
| 影響期間 | 2026-03-23 19:25 JST 〜 21:20 JST (約 2 時間) |
| 影響サービス | recap-worker, recap-subworker, recap-db |
| 影響機能 | 3-day Recap 生成（7-day Recap は既存結果の配信で影響なし） |
| 関連 ADR | [[000547]], [[000048]] |
| 関連コミット | `d15ffd0d`, `d29be7c7` |

## サマリー

recap-worker の Recap 生成パイプラインが連鎖的な OOM (Out of Memory) により繰り返し失敗した。直接の原因は recap-worker (1GB 制限) に対する libtorch メモリフラグメンテーションによる OOM だったが、修正の過程で recap-db (256MB 制限) および recap-subworker (512MB 制限) にも OOM が波及し、パイプラインは fetch/genre/dispatch の各ステージで断続的に失敗した。さらに、全分類チャンク失敗時に空の Recap を「成功」として記録するバグ、recap-subworker の冪等キャッシュによるジョブ stuck、および分類プロセスプールの spawn 時 OOM によるハングも発見された。

## 影響

- **3-day Recap 生成**: 新規 Recap が約 2 時間生成されなかった
- **パイプライン実行回数**: 少なくとも 7 回の失敗実行（job ID: `22f1c552`, `9142ffdd`, `ff2daecc`, `b9413aae`, `acc86e5c`, `5b1edc8e`, `e4a933d8`）
- **空 Recap の永続化**: 1 件の空 Recap（`genres_stored: 0`）が正常完了として DB に記録された
- **分類プロセスプール ハング**: recap-subworker の `ClassificationRunner` が `_verify_worker_initialization()` で永久ハング（少なくとも 3 回発生）
- **stuck DB ステート蓄積**: `recap_subworker_runs` に計 45+ 件の `running` 状態の run、`classification_job_queue` に計 9+ 件の `running` ジョブが残留
- **7-day Recap / Evening Pulse**: 既存の生成済み結果からの配信は継続しており、ユーザー向けの読み取りには影響なし
- **データ損失**: なし（パイプラインは冪等で、失敗時は再実行可能）

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 19:25 | recap-worker を OOM 対策 (mem_limit 2048m + jemalloc) で再ビルド・起動。recap-db が Recreate される |
| 19:25 | recap-worker が起動直後に resumable job を検出し、パイプライン即実行開始 |
| 19:26 | recap-subworker が未起動のため、classify-runs が全チャンク接続エラー。全分類失敗するも `genres_stored: 0` で正常完了扱い（バグ） |
| 19:26 | **検知**: ログ調査で `classify-runs POST request failed` の大量エラーと `genres_stored: 0` を確認 |
| 19:30 | **対応開始**: 空結果バグ修正（genre_remote.rs バリデーション + evaluate_job_outcome fallback 修正）を実装 |
| 19:39 | recap-subworker を起動。recap-worker を再ビルド |
| 19:39 | recap-db が Recreate され、リカバリモードに入る。recap-subworker が DB 接続エラー |
| 19:41 | recap-subworker 起動。分類リクエスト受付開始 (202 Accepted) |
| 19:46 | recap-worker 再起動。パイプライン再実行。分類ポーリング中にレスポンスパースエラー発生 |
| 19:47 | recap-subworker のメモリ使用量 503.4MiB / 512MiB (98.3%) を確認。**分類処理が stuck** |
| 19:50 | **原因特定 (1)**: recap-subworker の `mem_limit: 512m` が不足。1536m に引き上げ |
| 19:52 | recap-subworker 再起動。分類 run が recap-subworker DB に `running` で残留し、冪等チェックで新規 run がスキップ |
| 20:11 | classification_job_queue の stuck `running` ジョブをクリーンアップ。recap-worker 再起動 |
| 20:17 | キューが空のため `wait_for_completion()` が即座に `result_count: 0` で完了。バリデーションが `Err` を返し Failed |
| 20:24 | recap-subworker の `recap_subworker_runs` テーブルに run 8540-8550 が `running` で stuck。冪等チェックで分類処理開始されず |
| 20:26 | **原因特定 (2)**: recap-db が OOM killed (`OOMKilled: true`, 256MB 制限) で PostgreSQL クラッシュ → stage state 保存失敗 |
| 20:29 | recap-db の `mem_limit` を 2048m に引き上げ。再起動 |
| 20:32 | recap_subworker_runs の stuck run 11 件を `failed` に更新。recap-subworker 再起動で冪等キャッシュクリア |
| 20:34 | GUI から Recap 手動 kick。分類ポーリングは進むが `status: running` のまま変化せず |
| 20:56 | **原因特定 (3)**: recap-subworker の `ClassificationRunner` プロセスプール (6 ワーカー × spawn) が 1536MB 制限で OOM。`docker top` でワーカープロセス 0 個を確認。`classification.run.process.started` の後 `predict_batch` が永久ハング |
| 21:00 | recap-subworker の `mem_limit` を 8192m に引き上げ。全 stuck ステートをクリーンアップし、全サービス再起動 |
| 21:20 | **復旧**: GUI から手動 kick で分類処理が正常に開始 |

## 根本原因分析

### Five Whys

1. **なぜ Recap 生成が失敗したか？**
   → 3 つのコンテナ (recap-worker, recap-db, recap-subworker) が連鎖的に OOM killed / メモリ不足に陥った

2. **なぜ 3 コンテナ全てが OOM したか？**
   → Docker Compose の `mem_limit` がワークロードに対して過小だった。recap-worker: 1GB (libtorch + 2900 記事処理)、recap-db: 256MB (50 コネクション + JSONB ステート保存)、recap-subworker: 512MB → 1536MB (分類プロセスプール 6 ワーカー × DistilBERT + TF-IDF の spawn に対して)

3. **なぜ適切な mem_limit が設定されていなかったか？**
   → メモリ使用量の見積もりが初期導入時の小規模テスト基準で、本番規模のワークロード (2000-3000 記事/バッチ) および spawn ベースのプロセスプール (6 ワーカー各々がモデルを独立ロード) での計測が行われていなかった

4. **なぜ OOM が連鎖的に発生し、復旧に長時間を要したか？**
   → (a) recap-subworker の冪等チェックが古い `running` run をブロックし新規分類が開始されない、(b) `classification_job_queue` にも `running` ジョブが残留し `wait_for_completion()` が stuck、(c) recap-db のクラッシュ・リカバリが全コネクションの強制切断を引き起こす — これらの状態不整合が重なり、単純な再起動では復旧できなかった

5. **なぜ分類プロセスプールが 1536MB でもハングしたか？**
   → `ClassificationRunner` は `multiprocessing.spawn` で 6 ワーカーを同時スポーンし、各ワーカーが DistilBERT モデルと TF-IDF ベクタライザを独立にロードする (ADR-000048 で CUDA fork 問題回避のため spawn に切り替え)。マスタ ~500MB + 6 ワーカー × ~200MB = ~1700MB で 1536MB を超過し、子プロセスが OOM kill → 親の `_verify_worker_initialization()` が永久ハング

### 寄与要因

- **空結果バグ**: 全分類失敗時に `JobOutcome::Success` を返すバグにより、障害の影響が不可視化された
- **依存方向の非対称性**: recap-subworker は `depends_on: recap-worker` だが、recap-worker を起動しても subworker は起動されない
- **冪等性の副作用**: recap-subworker の冪等チェックはリクエスト重複排除に有効だが、前回のプロセスが kill された場合に stuck を引き起こす (TTL なし)
- **spawn ベースプロセスプールの暗黙的メモリコスト**: fork と異なり spawn は各ワーカーが独立に Python + モデルをロードするため、ワーカー数に比例してメモリが増加する。ADR-000048 で spawn に切り替えた際にこの影響が考慮されていなかった
- **stage state の JSONB サイズ**: 前処理済み 2906 記事の stage state が大きな JSONB として PostgreSQL に保存され、256MB の DB には重すぎた

## 対応の評価

### うまくいったこと

- resumable job メカニズムにより、パイプラインの途中失敗から再開できた
- 既存の `read_process_memory_kb()` 診断機構が RSS の確認に役立った
- OOM の根本原因（libtorch フラグメンテーション）の調査で Web 検索から jemalloc の知見を迅速に適用できた
- 空結果バグの修正が TDD (RED → GREEN) で即座に行えた
- `docker top` によるプロセス確認で分類プロセスプールのハングを特定できた

### 改善が必要なこと

- OOM 修正のための `docker compose up --build` が recap-db の Recreate を引き起こし、二次障害を発生させた
- 手動での DB ステート修正（`UPDATE ... SET status = 'failed'`）が 4 回以上必要になり、状態の整合性管理が困難だった
- recap-subworker の冪等チェックが stuck run をブロックする問題は事前にテストされていなかった
- mem_limit を段階的に引き上げた (512m → 1536m → 8192m) ため、復旧が 3 段階に分かれ時間を要した

### 運が良かったこと

- 7-day Recap と Evening Pulse は既存の生成済み結果からの配信で継続しており、ユーザーへの読み取り影響が最小限だった
- 障害がバッチ処理（非同期 Recap 生成）に限定され、リアルタイム API は影響を受けなかった

## 教訓

### 技術的教訓

1. **libtorch + glibc malloc の組み合わせはメモリフラグメンテーションが深刻**: CPU 推論でもピーク RSS が実使用量の 1.5-2 倍に膨張する。jemalloc の導入が必須
2. **Docker Compose の `mem_limit` はワークロード計測に基づいて設定すべき**: 初期のデフォルト値を放置すると、ワークロードの成長とともに OOM が発生する
3. **spawn ベースのプロセスプールはワーカー数 × モデルサイズのメモリが必要**: fork と異なり spawn は COW の恩恵がない。ADR-000048 で spawn に切り替えた時点でメモリ見積もりを更新すべきだった
4. **冪等チェックには TTL または stale 検出が必要**: プロセスが kill された場合に `running` 状態の run が永久にブロックする
5. **`docker compose up --build -d <service>` は依存する DB コンテナも Recreate する可能性がある**: `restart` と `up --build` の使い分けを意識する
6. **メモリ制限は十分なマージンで一発で決める**: 段階的な引き上げは復旧を長期化させる。実測値 + 十分なヘッドルームで設定すべき

### 組織的教訓

1. **メモリ制限はサービス導入時にワークロードプロファイリングを行って設定すべき**: 「とりあえず 256MB」は本番で破綻する
2. **連鎖障害のパターンを想定した復旧手順が必要**: 1 つのコンテナの修正が別のコンテナの障害を引き起こすケースに備える
3. **stuck ステートの一括クリーンアップ手順を runbook として整備すべき**: `recap_subworker_runs` + `classification_job_queue` + `recap_jobs` の 3 テーブルを一貫して処理する手順

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | recap-worker `mem_limit: 2048m` + jemalloc 導入 | 開発担当者 | 2026-03-23 | **完了** |
| P-2 | recap-subworker `mem_limit: 8192m` に引き上げ（spawn プロセスプール 6 ワーカー対応） | 開発担当者 | 2026-03-23 | **完了** |
| P-3 | recap-db `mem_limit: 2048m` に引き上げ | 開発担当者 | 2026-03-23 | **完了** |
| P-4 | 全分類失敗時の `JobOutcome::Success` バグ修正 | 開発担当者 | 2026-03-23 | **完了** |
| P-5 | recap-subworker の冪等チェックに TTL を導入し、一定時間 `running` の run を自動 `failed` にする | 開発担当者 | 2026-04-06 | 未着手 |
| P-6 | classification_job_queue の `running` ジョブに stuck 検出タイムアウトを追加 | 開発担当者 | 2026-04-06 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | コンテナの OOMKilled 状態を監視するアラートを追加 | 開発担当者 | 2026-04-06 | 未着手 |
| D-2 | recap パイプラインの `genres_stored: 0` を異常として検知するアラートを追加 | 開発担当者 | 2026-04-06 | 未着手 |
| D-3 | `ClassificationRunner` プロセスプールの起動失敗を検知するヘルスチェックを追加 | 開発担当者 | 2026-04-06 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | recap-worker 起動時にヘルスチェック対象サービス (recap-subworker, recap-db) の到達確認を行い、不通時は pipeline 開始を遅延させる | 開発担当者 | 2026-04-13 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | recap サービスの復旧 runbook を作成（stuck ジョブの一括クリーンアップ手順: `recap_subworker_runs` + `classification_job_queue` + `recap_jobs` + `recap_stage_state` の 4 テーブル処理、冪等キャッシュリセットのための subworker 再起動手順を含む） | 開発担当者 | 2026-04-06 | 未着手 |
