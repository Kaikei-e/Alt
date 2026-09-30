---
title: "Alt Postmortems"
date: 2026-09-30
tags:
  - postmortem
---

# Alt Postmortems

Alt のポストモーテム（事後検証記録）リポジトリです。各ドキュメントは `PM-YYYY-NNN`（西暦年と通番）の形式で採番され、障害やヒヤリハット事象から得られた知見を将来の設計・実装・運用に活かすため、非難を排した blameless format に従って記録されています。

横断的な構造的傾向や再発パターンの分析については、[[postmortem-structural-analysis-2026-08-16]] を参照してください。

## インシデント一覧

| インシデント | 発生日 | 重大度 | 影響サービス | 根本原因 |
|:---|:---:|:---:|:---|:---|
| [PM-2026-001](PM-2026-001-recap-pipeline-cascading-oom.md) | 2026-03-23 | SEV-3 | recap-worker, recap-subworker, recap-db | recap-worker に対する libtorch メモリフラグメンテーションによる OOM が recap-db, recap-subworker に連鎖波及 |
| [PM-2026-002](PM-2026-002-unsummarized-infinite-enqueue-loop.md) | 2026-03-19 | SEV-4 | pre-processor, alt-backend | 要約不適（短すぎ/長すぎ）記事がスキップ時に永続化されずキューに無限再エンキューされる構造 |
| [PM-2026-003](PM-2026-003-visual-preview-summary-mismatch.md) | 2026-03-23 | SEV-3 | alt-frontend-sv, alt-backend | フロントエンド要約ストリーミングコールバックの stale-response guard 欠如による race condition |
| [PM-2026-004](PM-2026-004-streamsummarize-524-and-streaming-failure.md) | 2026-03-24 | SEV-3 | alt-backend, pre-processor, alt-frontend-sv, news-creator | REST/Connect-RPC 呼び出し不整合・Cloudflare アイドルタイムアウト・プロキシバッファリング等の複合障害 |
| [PM-2026-005](PM-2026-005-distributed-be-gpu-underutilization.md) | 2026-03-25 | SEV-4 | news-creator, rag-orchestrator | 古い推論ランタイムによる CPU フォールバック動作および設定変更に伴うコンテナ再作成時のモデルコールドスタート |
| [PM-2026-006](PM-2026-006-ask-augur-gpu-starvation.md) | 2026-03-25 | SEV-3 | rag-orchestrator, news-creator | rag-orchestrator が推論サーバーに直接接続し優先度セマフォをバイパスしたためバッチ要約によるスロット飢餓が発生 |
| [PM-2026-007](PM-2026-007-recap-subworker-numba-deadlock-and-be-model-mismatch.md) | 2026-03-25 | SEV-3 | recap-subworker, recap-worker, news-creator | Numba threading layer の workqueue フォールバックによる並行実行デッドロックおよび BE 要約モデル設定ミス |
| [PM-2026-008](PM-2026-008-ask-augur-ollama-parameter-mismatch-model-reload.md) | 2026-03-25 | SEV-3 | news-creator | chat とバッチ要約で options パラメータが異なり推論サーバーでモデルアンロード・再ロードのピンポンが発生 |
| [PM-2026-009](PM-2026-009-ask-augur-invalid-utf8-stream-hang.md) | 2026-03-26 | SEV-3 | rag-orchestrator | Citation メタデータの UTF-8 サニタイズ漏れによる protobuf シリアライゼーション失敗および JSON 内引用符のエスケープ不備 |
| [PM-2026-010](PM-2026-010-knowledge-home-reproject-checkpoint-gap.md) | 2026-03-23 | SEV-4 | alt-backend, knowledge-sovereign | V3 Reproject swap 時に KnowledgeProjector のチェックポイントがリセットされず新イベントが投影されなかった |
| [PM-2026-011](PM-2026-011-knowledge-home-open-silent-noop.md) | 2026-03-26 | SEV-4 | alt-frontend-sv | 旧イベント発行パスで ArticleCreated イベントに link フィールドが含まれず read model で空 link となり FE でサイレント return |
| [PM-2026-012](PM-2026-012-visual-swipe-summarize-semaphore-slot-leak.md) | 2026-03-27 | SEV-3 | news-creator, alt-backend | HybridPrioritySemaphore のプリエンプション連鎖で出身プール追跡が欠如し RT スロットが BE プールに永久移行 |
| [PM-2026-013](PM-2026-013-ask-augur-follow-up-timeout-clarification-drop.md) | 2026-03-28 | SEV-3 | rag-orchestrator, news-creator | ConversationPlanner の過剰 clarification 判定、RPC ハンドラーの Done イベントドロップ、セマフォスロットリークの 3 重畳障害 |
| [PM-2026-014](PM-2026-014-news-creator-slow-summarization-recap-degradation.md) | 2026-03-28 | SEV-3 | news-creator, pre-processor, recap-worker, recap-subworker | HybridPrioritySemaphore スロット消失、pre-processor 旧バイナリによる COLD_START、numpy 2.0 互換性等の複合障害 |
| [PM-2026-015](PM-2026-015-semaphore-cancelled-waiter-slot-leak.md) | 2026-03-29 | SEV-3 | news-creator | waiter へのスロット転送後にクライアント切断等で CancelledError が発生した際に転送済みスロットが回収されず永久消失 |
| [PM-2026-016](PM-2026-016-ask-augur-model-name-mismatch-eof.md) | 2026-04-03 | SEV-3 | rag-orchestrator, news-creator | モデル移行コミット後に rag-orchestrator がリビルドされず旧モデル名を送信し推論ランタイムから即時 EOF 返却 |
| [PM-2026-017](PM-2026-017-quality-check-guard-deadlock-be-summarization-stall.md) | 2026-04-02 | SEV-4 | pre-processor | 品質チェックが低品質要約を削除した後にキューの完了ジョブが残り HasRecentSuccessfulJob ガードが再エンキューをブロック |
| [PM-2026-018](PM-2026-018-rag-html-chunk-contamination-search-quality-degradation.md) | 2026-04-03 | SEV-3 | rag-orchestrator | chunker がテキスト抽出を行わずに改行分割のみ適用したため fulltext-fetch 記事の生 HTML タグやボイラープレートが混入 |
| [PM-2026-019](PM-2026-019-feed-mark-read-latency-spike.md) | 2026-04-06 | SEV-3 | alt-backend | CachedFeedListUsecase の N+1 fan-out クエリが SharedCache TTL expire 時にコネクションプールを枯渇させた |
| [PM-2026-020](PM-2026-020-ask-augur-embedder-down-total-retrieval-failure.md) | 2026-04-11 | SEV-3 | rag-orchestrator | 外部推論停止時にリトリーバルパイプラインが embedding 失敗を fatal 扱いし BM25 正常稼働中にもかかわらず即座に中断 |
| [PM-2026-021](PM-2026-021-ask-augur-cross-language-retrieval-failure.md) | 2026-04-11 | SEV-4 | rag-orchestrator, search-indexer, news-creator | 日本語クエリが英語記事に到達できない複数パス（クエリ展開・Meilisearch・embedding）の分散設計不備 |
| [PM-2026-022](PM-2026-022-image-proxy-gzip-decode-failures.md) | 2026-04-12 | SEV-3 | alt-backend | ImageFetchGateway が Accept-Encoding: gzip を手動付与し Go http.Transport の自動透過解凍が無効化された |
| [PM-2026-023](PM-2026-023-search-indexer-container-absent-reference-desk-degraded.md) | 2026-04-12 | SEV-3 | search-indexer, alt-backend | search-indexer コンテナが存在せず restart: always と alt-backend の depends_on が未設定だった |
| [PM-2026-024](PM-2026-024-morning-update-stuck-in-batch-recap-resume-loop.md) | 2026-04-13 | SEV-3 | recap-worker | morning_update ジョブがクラッシュ後 boot-time resume loop に巻き込まれステータスが zombie 化 |
| [PM-2026-025](PM-2026-025-acolyte-search-indexer-auth-gap-empty-reports.md) | 2026-04-14 | SEV-3 | acolyte-orchestrator, search-indexer | search-indexer の認証強制導入に対し acolyte-orchestrator が認証トークンを送信せず全検索 401 となり空レポート生成 |
| [PM-2026-026](PM-2026-026-rag-augur-search-indexer-auth-gap-401-cascade.md) | 2026-04-14 | SEV-2 | rag-orchestrator, alt-backend, news-creator | search-indexer 認証強制に対して rag-orchestrator と alt-backend が追従せず 401 カスケードと推論キュー飽和が発生 |
| [PM-2026-027](PM-2026-027-pre-processor-summarize-retry-storm-false-dead-letter.md) | 2026-04-15 | SEV-3 | pre-processor, news-creator | pre-processor の ResponseHeaderTimeout (20s) が news-creator 要約時間より短く切断リトライが重複処理と false dead_letter を誘発 |
| [PM-2026-028](PM-2026-028-mtls-cert-expiry-knowledge-home-outage.md) | 2026-04-15 | SEV-2 | alt-backend, alt-butterfly-facade | cert-renewer サイドカーが稼働しておらず証明書期限切れ到達 + cert-init が既存ファイルを無条件信用して再発行しなかった |
| [PM-2026-029](PM-2026-029-nginx-tls-sidecar-stale-cert-acolyte-outage.md) | 2026-04-16 | SEV-3 | acolyte-orchestrator, nginx | nginx TLS sidecar が起動時にロードした証明書をメモリに保持し続けディスク上の更新を反映しなかった |
| [PM-2026-030](PM-2026-030-pki-agent-stale-netns-acolyte-502.md) | 2026-04-17 | SEV-3 | acolyte-orchestrator, pki-agent | acolyte-orchestrator が recreate された際 network_mode: service 共有の pki-agent が旧 netns に取り残され幽霊化 |
| [PM-2026-031](PM-2026-031-mtls-cutover-residual-tasks-acolyte-502-3days-recap-404.md) | 2026-04-14 | SEV-3 | acolyte-orchestrator, recap-worker, alt-backend | pki-agent の cascading recreate 漏れ再発および alt-backend mTLS リスナーへの REST ルート登録漏れ |
| [PM-2026-032](PM-2026-032-mtls-client-cert-stale-in-memory-3days-recap-failure.md) | 2026-04-18 | SEV-3 | recap-worker, alt-backend | recap-worker の HTTP クライアントが起動時に証明書をメモリ固定しディスク上の新証明書ローテーションに追従しなかった |
| [PM-2026-033](PM-2026-033-mtls-server-side-gap-recap-subworker-news-creator-3days-recap-failure.md) | 2026-04-14 | SEV-3 | recap-worker, recap-subworker, news-creator | recap-worker を https_only にした一方下流の subworker と news-creator が mTLS 化されておらず URL scheme not allowed で失敗 |
| [PM-2026-034](PM-2026-034-pki-agent-self-exit-zombie-and-netns-orphan-4th-recurrence.md) | 2026-04-19 | SEV-3 | acolyte-orchestrator, pki-agent | 親コンテナ recreate 時に pki-agent が netns 孤立し、プローブが旧 netns loopback を叩いて orphan を検知できず zombie 化 |
| [PM-2026-035](PM-2026-035-recap-subworker-learning-machine-artifacts-missing.md) | 2026-04-20 | SEV-3 | recap-subworker, recap-worker | classification_backend デフォルトが learning_machine になっていたがホスト側に artifacts が配置されておらず 0 結果返却 |
| [PM-2026-036](PM-2026-036-recap-subworker-joblib-bind-mount-empty-directory-8day-outage.md) | 2026-04-14 | SEV-3 | recap-subworker, recap-worker | docker-compose の file-scoped bind mount がホスト側の消失ファイルを空ディレクトリとしてマウントしたため IsADirectoryError で失敗 |
| [PM-2026-037](PM-2026-037-recap-subworker-init-container-never-ran-rolling-deploy-mismatch.md) | 2026-04-22 | SEV-4 | recap-subworker | rolling deploy で --no-deps が付与され depends_on で参照される init container が一度も起動されなかった |
| [PM-2026-038](PM-2026-038-recap-worker-rustbert-cache-empty-silent-keyword-only-fallback.md) | 2026-04-20 | SEV-3 | recap-worker | rust-bert キャッシュディレクトリが空のまま ro bind-mount され subgenre-splitter が silent に keyword-only fallback |
| [PM-2026-039](PM-2026-039-knowledge-loop-invalidate-storm-and-foreground-overlap.md) | 2026-04-26 | SEV-4 | alt-frontend-sv | SvelteKit の invalidateAll 暴走による fetch-storm と stream_jwt_expired の lockstep 発火ループ |
| [PM-2026-040](PM-2026-040-knowledge-home-start-reproject-jsonb-not-null-violation-latent-since-projection-versions.md) | 2026-04-27 | SEV-4 | alt-backend, knowledge-sovereign | ReprojectRun の JSON フィールドが nil のまま送信され PostgreSQL の NOT NULL constraint 違反で 502 |
| [PM-2026-041](PM-2026-041-knowledge-home-reproject-empty-link-projector-payload-tag-drift.md) | 2026-04-28 | SEV-4 | alt-backend, knowledge-sovereign | event producer の json:"url" と projector consumer の json:"link" のタグ不一致により link カラムが全件空文字化 |
| [PM-2026-042](PM-2026-042-staging-projector-content-type-loop-disk-fill.md) | 2026-05-04 | SEV-3 | alt-backend | Connect-RPC の content-type 不整合による永久リトライループで約 148GB のコンテナログが生成されホストディスクが逼迫 |
| [PM-2026-043](PM-2026-043-inoreader-oauth-token-empty-3day-silent-outage.md) | 2026-05-06 | SEV-3 | pre-processor-sidecar, auth-token-manager | ディスク逼迫による共有 volume の OAuth トークンファイル空化と観測フック不在によるサイレント停止 |
| [PM-2026-044](PM-2026-044-feeds-search-each-key-duplicate-hybrid-pagination.md) | 2026-05-24 | SEV-3 | alt-frontend-sv | Meilisearch hybrid search の offset pagination がページ境界で同一 ID を返し Svelte keyed each 制約違反クラッシュ |
| [PM-2026-045](PM-2026-045-knowledge-loop-sse-silent-failure-jwt-ttl-nginx-effect-race-tile-duplicate.md) | 2026-05-27 | SEV-3 | alt-frontend-sv, alt-backend | JWT TTL と streamStaleTimeout 不整合、nginx location 欠落、reconnect race、UI 二重発火の 4 層複合障害 |
| [PM-2026-046](PM-2026-046-search-indexer-e2e-network-race-reclaim-concurrent-matrix.md) | 2026-05-29 | SEV-3 | search-indexer, ci | reclaim-network-pool.sh が並行 matrix job の作成直後ネットワークを container attach 前に削除していた |
| [PM-2026-047](PM-2026-047-meilisearch-task-db-lockout-search-indexer-crash-loop.md) | 2026-07-16 | SEV-2 | meilisearch, search-indexer | Meilisearch 内部タスク履歴 DB の肥大化による書き込みロックアウトで search-indexer が EnsureIndex でクラッシュループ |
| [PM-2026-048](PM-2026-048-meilisearch-search-key-drift-dual-checkout-outage.md) | 2026-07-22 | SEV-2 | search-indexer, meilisearch | 対話用とデプロイランナー用の二重チェックアウトで secrets が分岐し、ランナー側に古い検索キーが取り残され 403 で全滅 |
| [PM-2026-049](PM-2026-049-acolyte-hollow-report-silent-persistence.md) | 2026-07-22 | SEV-3 | acolyte-orchestrator | 上流検索の 403 失敗時にフォールバックが空の section claim を受け入れ空洞版レポートを無警告で永続化 |
| [PM-2026-050](PM-2026-050-canonical-config-migration-cascading-outage.md) | 2026-07-24 | SEV-2 | acolyte-orchestrator, search-indexer, db-migrators | 正準設定移行スクリプトが鮮度比較なしに古いデプロイランナー側残置コピーを昇格させ多段障害を誘発 |
| [PM-2026-051](PM-2026-051-log-collector-glibc-mismatch-latent-defect.md) | 2026-07-18 | SEV-3 | rask-log-aggregator, rask-log-forwarder | ビルダー側の新しい glibc とランタイム側の古い glibc の不整合によりコンテナ再作成時に起動即死 |
| [PM-2026-052](PM-2026-052-recap-3days-multilayer-outage.md) | 2026-07-08 | SEV-2 | recap-subworker, recap-worker, alt-butterfly-facade | scikit-learn 更新に伴う classifier アーティファクト互換性不一致での fail-closed 拒否および日次アラート不在 |
| [PM-2026-053](PM-2026-053-adr954-split-release-pipeline-and-dataplane-outage.md) | 2026-08-01 | SEV-2 | alt-backend, alt-harvester, alt-data-hub | GitHub Compare API の 300 件上限で変更リストが無警告切り捨てされ consumer が欠落 + 凍結 Pact によるデッドロック |
| [PM-2026-054](PM-2026-054-pki-agent-swept-by-disk-cleanup-mtls-expiry-outage.md) | 2026-08-10 | SEV-2 | alt-data-hub, alt-backend, pki-agent | ディスク掃除スクリプトの無差別 prune で pki-agent サイドカーが消失し証明書が自動更新されず失効 |
| [PM-2026-055](PM-2026-055-go-1266-h2c-readheadertimeout-stream-teardown.md) | 2026-08-15 | SEV-3 | alt-backend, alt-butterfly-facade | Go 1.26.6 の ReadHeaderTimeout 挙動変更により ReadTimeout: 0 の h2c 接続がちょうど 10 秒で破棄され全ストリーム切断 |
| [PM-2026-056](PM-2026-056-pre-processor-health-gate-contract-break-summarization-outage.md) | 2026-08-18 | SEV-2 | pre-processor, news-creator | news-creator の /health 契約変更により models 配列が空となり pre-processor の health gate が開かず自動要約ジョブが未登録 |
| [PM-2026-057](PM-2026-057-rag-embedding-wipe-silent-bm25-degradation.md) | 2026-08-02 | SEV-2 | rag-db, rag-orchestrator | embedding モデル移行の migration で全 embedding を NULL 化したが対応する backfill が実行されず約1ヶ月間 BM25 単独に縮退 |
| [PM-2026-058](PM-2026-058-lockstep-auth-hardening-contract-gate-deadlock.md) | 2026-09-21 | SEV-3 | acolyte-orchestrator, release-pipeline | 4 サービス同時の認証強化が契約ゲートの先行検証に阻まれ鶏卵構造のデッドロックが発生 |
