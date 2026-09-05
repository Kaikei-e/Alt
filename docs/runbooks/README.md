---
title: Runbooks 索引 — カテゴリ・型・鮮度の構造化マップ
date: 2026-09-05
tags:
  - runbook
  - index
  - alt
---

# Runbooks 索引

`docs/runbooks/` 全 35 本の構造化索引。**症状から入るなら下の対応表、横断知識なら [[runbooks/crystallized-knowledge]]** へ。

各ランブックは 3 つの型に分類する:
- **incident** — 症状起点のインシデント対応 (アラート / ユーザー報告 → 調査 → 復旧)
- **operation** — 計画的オペレーション手順 (deploy / reproject / cutover)
- **checklist** — 予防チェックリスト・訓練台本 (PR レビュー / GameDay)

鮮度フラグ: ⚠️ = 既知の陳腐化あり (詳細は「整備課題」節)、⏳ = 時限性 (完了すれば役目終了)、✅ = 時限性で完了済み (`status: archived`)。

## 症状 → ランブック対応表

| 症状 | ランブック |
|---|---|
| Knowledge Home が空 / warming up 多発 | [[knowledge-home-empty-spike]] → [[knowledge-home-projection-recovery]] |
| degraded バナー / burn rate アラート | [[knowledge-home-degraded-mode]] |
| "why" 表示が壊れている | [[knowledge-home-malformed-why-spike]] |
| リアルタイム更新が止まる / stream 切断多発 | [[knowledge-home-stream-disconnect-surge]] + [[connect-rpc-streaming-checklist]] |
| `buf breaking` で PR がブロック | [[knowledge-home-contract-break]] |
| cert 期限切れ / TLS handshake 失敗 / leftover pki-agent / BFF 502 | [[pki-agent-recovery]] |
| Acolyte レポートが止まる / stuck run | [[acolyte-pipeline-recovery]] / [[acolyte-checkpoint-resume]] |
| Acolyte の LLM timeout / JSON 切断 | [[acolyte-llm-timeout]] |
| Acolyte 依存ダウン時の縮退 | [[acolyte-degraded-mode]] |
| レポート再生成したい | [[acolyte-manual-regeneration]] |
| projector が silent stall / 通知が来ない | [[sovereign-projector-notification]] |
| recap の deploy が artefact で落ちる | [[3days-recap-artefact-recovery]] + [[runner-setup]] |
| 新しい compose bind / 静的設定ファイルを足す | [[compose-bind-mount-policy]] |
| 新しい compose サービス / サイドカーを足す | [[ops-surface-budget]] |
| `/health` は緑なのに依存が死んでいる | [[health-deep-contract]] |
| エッジのログイン / Home / 検索が死んでいる | [[synthetic-monitoring]] |
| feeds / login / search の burn-rate / UserJourneyRequestsAbsent | [[user-journey-slo-gameday]] |
| Pact Broker 401 / verification failure / can-i-deploy ブロック | [[pact-broker-ops]] |
| データ喪失・破損 / restore が必要 | [[backup-restore]] |
| Admin 監視画面の異常 | [[admin-observability]] |
| 追加したアラート / scrape job が有効にならない、通知が来ない | [[observability-config-reload]] |

## カテゴリ別索引

### 0. 横断知識
- [[runbooks/crystallized-knowledge]] — **ADR 940 本 / PM 46 本の結晶化知識** (障害パターン百科 / 診断の定石 / Critical Rules 出典 / ADR 時代区分マップ)

### 1. デプロイ & CI/CD (5)
| ランブック | 型 | 一言 |
|---|---|---|
| [[deploy]] | operation | 手動本番デプロイ (Pact gate → c2quay whole-stack converge → smoke) |
| [[pact-broker-ops]] | operation + incident | Broker の起動 / 認証 / バックアップ / failed-verify 調査 |
| [[runner-setup]] | operation | self-hosted runner (alt-builder / alt-prod) の bootstrap |
| [[3days-recap-artefact-recovery]] ⏳⚠️ | incident | rustbert-cache / joblib artefact 復旧で deploy を unblock |
| [[ops-surface-budget]] | checklist | P2-14 OSU freeze（77/63/16、F1–F5）。新 long-running は offset か sunset（[[000979]]） |

### 2. mTLS / PKI (2)
| ランブック | 型 | 一言 |
|---|---|---|
| [[mtls-cutover]] | operation + incident | X-Service-Token → mTLS 切替手順 + 事象別対応 + cert rotation |
| [[pki-agent-recovery]] | incident | in-process enrollment の cert 期限切れ / leftover sidecar dual-writer / deploy-rollback 順（[[000978]]） |

### 3. Knowledge Home / Loop インシデント対応 (6)
| ランブック | 型 | 一言 |
|---|---|---|
| [[knowledge-home-degraded-mode]] ⚠️ | incident | degraded レスポンス増加 (projection 鮮度 / DB / burn rate) |
| [[knowledge-home-empty-spike]] ⚠️ | incident | 空レスポンス急増 (projector lag / event store / ingestion) |
| [[knowledge-home-malformed-why-spike]] ⚠️ | incident | 壊れた why 説明 (projector バグ / データ品質) |
| [[knowledge-home-stream-disconnect-surge]] ⚠️ | incident | stream 切断多発 (PgBouncer / LISTEN-NOTIFY / nginx) |
| [[knowledge-home-contract-break]] | incident | proto 契約破壊 CI 失敗の解決 |
| [[knowledge-loop-recall-deprecation]] ⏳ | operation | legacy recall rail 3 RPC の deprecation watch |

### 4. Projection / イベントログ運用 (5)
| ランブック | 型 | 一言 |
|---|---|---|
| [[knowledge-home-projection-recovery]] ⚠️ | incident | event log 健全・read model 破損時の projection リセット |
| [[knowledge-home-reproject-operations]] | operation | `altctl home reproject` (dry_run → compare → swap → rollback) |
| [[knowledge-loop-reproject]] | operation | full reproject + WhyMappingVersion 履歴台帳 (最も新鮮・2026-06-10) |
| [[sovereign-cutover]] ✅ | operation | alt-db → knowledge-sovereign-db の active writer 切替 (完了済み一回性、`status: archived`) |
| [[sovereign-projector-notification]] | incident | knowledge-sovereign 内 in-process ticker projector の stall 診断・復旧 (旧 backend streaming 中継設計は 2026-09-05 時点で未使用と判明、記録として残置) |

### 5. Acolyte 運用 (5)
| ランブック | 型 | 一言 |
|---|---|---|
| [[acolyte-checkpoint-resume]] | operation | LangGraph checkpointer による run resume と制約 |
| [[acolyte-degraded-mode]] ⚠️ | incident | 依存 (DB / news-creator-backend / search-indexer / BFF) ダウン時の縮退 |
| [[acolyte-llm-timeout]] | incident | ReadTimeout / JSON truncation の診断・復旧 |
| [[acolyte-manual-regeneration]] | operation | レポート手動再生成 (full / scope / batch) + 品質 SQL |
| [[acolyte-pipeline-recovery]] | incident | orphaned runs / checkpoint 破損 / stuck job の系統復旧 |

### 6. プラットフォーム横断 (9)
| ランブック | 型 | 一言 |
|---|---|---|
| [[admin-observability]] | operation | Admin UI Observability タブの運用 (flag / trust boundary / metric allowlist) |
| [[observability-config-reload]] | operation | Prometheus / Alertmanager / Grafana 設定の検証 → reload → 反映確認 → ドリフト点検 |
| [[backup-restore]] ⚠️ | operation + incident | 3-2-1 バックアップ、restore 4 シナリオ、DR 訓練 |
| [[connect-rpc-streaming-checklist]] | checklist | 新規 streaming service の 5 軸チェック + 月次 audit |
| [[knowledge-home-gameday-checklist]] ⚠️ | checklist | chaos 訓練 5 シナリオの台本 |
| [[compose-bind-mount-policy]] | operation | file bind 禁止 / `configs:` / `create_host_path: false`（[[000979]]） |
| [[health-deep-contract]] | checklist | `/health/deep` pass\|warn\|fail。compose probe は cheap liveness のまま（[[000980]]） |
| [[synthetic-monitoring]] | checklist | エッジ旅程の provider-agnostic spec。activation は ops gate（[[000980]]） |
| [[user-journey-slo-gameday]] | checklist | feeds / login / search MWMBR と KH `status=`。BFF rebuild が Prometheus reload より先 |

## 整備課題（2026-09-05 更新）

全 35 本を全文読査した結果の既知の陳腐化。修正するまでフラグを残す。

1. **Sovereign cutover 前後の DB 参照先不整合** — knowledge-home 系 4 本 (degraded / empty / malformed-why / projection-recovery) + gameday が `alt-db` の `knowledge_events` を直接参照するが、cutover 後の authority は knowledge-sovereign-db ([[sovereign-cutover]] / [[knowledge-loop-reproject]] は新参照)。**実インシデント時に旧 DB を掘るリスクがあり最優先**
2. **[[knowledge-home-stream-disconnect-surge]]** に PM-2026-045 / ADR-000929 の SSE 5 軸が未反映 — [[connect-rpc-streaming-checklist]] と相互リンクすべき
3. **acolyte-db / knowledge-sovereign-db / pre-processor-db が backup 対象外** — [[backup-restore]] 2026-09-05 更新で pg_dump にも Restic volume snapshot にも掛かっていないことを明記したが、`backup-all.sh` の `databases=()` / `compose/backup.yaml` の volume mount / `backup-all.sh` の `volumes=()` の追加はいずれも未着手のまま。データ保護の実害リスクは残存
4. **[[acolyte-degraded-mode]]** が search-indexer の health を 7700 (Meilisearch のポート) で確認する誤記あり
5. **時限性ランブックの寿命管理** — [[knowledge-loop-recall-deprecation]] / [[3days-recap-artefact-recovery]] は完了状態の追記がなく、生きているか終わったか判別不能。[[sovereign-cutover]] は 2026-09-05 に `status: archived` を追記済み。完了時は冒頭に `status: archived` を追記する規約とする
6. **宣言済み未作成** — `distribution-paths.md` が [[3days-recap-artefact-recovery]] 内で作成予定と明記されたまま未着手。`compose-bind-mount-policy.md` は 2026-08-18 に追加 (PM-036 AI #10)

## カバレッジギャップ（ランブックが無い頻出領域）

優先度順。作成時はこの索引にも追記する。

1. **mq-hub / Redis Streams consumer 障害** — Critical Rule 10 がルール化済みなのに consumer 停止・pending 滞留・DLQ 溢れの手順が無い
2. **auth 層 (Kratos / auth-hub) 障害** — 全サービスの前提なのに session 大量失効 / Kratos down / JWT 検証失敗の手順が無い
3. **DB / PgBouncer / Atlas migration 障害** — pgbouncer userlist、Atlas drift (`migrate set`)、接続枯渇が断片散在
4. **記事 ingestion パイプライン** — pre-processor 停滞・Inoreader rate limit / token 失効 (PM-2026-043) の専用手順が無い
5. **news-creator / Ollama 基盤ダウン** — Acolyte 視点以外の LLM 共通コンシューマ (recap / tag / RAG) の対応が無い
6. **Meilisearch / search-indexer の index 復旧** — 再インデクシング手順が無い
7. **rask logging パイプライン自体の障害** — /log-seeker が依存する観測基盤 (ClickHouse disk / forwarder 停止) の手順が無い
8. **フロントエンド deploy 後の `_app/immutable/*` 404** — 三点セットの知見 (ADR-000898/000902) が runbook 化されていない
9. **単一ホストのリソース枯渇** — disk full (PM-2026-042/043) / OOM / port rebind の horizontal な手順が無い

## 執筆規約

- frontmatter 必須 (`title` / `date` / `tags`)、ADR 参照は `[[000NNN]]`、PM 参照は `PM-2026-NNN`
- 1 本 1 症状ドメイン。横断知識は [[runbooks/crystallized-knowledge]] へ、手順はここへ (wiki には手順を書かない)
- 冒頭に TL;DR (最短復旧コマンド列)、次にトリガー条件、その後に調査ツリー
- 時限性 (cutover / deprecation watch) は完了時に `status: archived` を frontmatter に追記
- 新規作成・アーカイブ時は本索引を更新する
