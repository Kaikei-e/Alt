# Acolyte 実装ステータスレポート — 2026-04-09

## 概要

Knowledge Acolyte は Alt プラットフォームの版管理型レポート生成オーケストレーター。`refine.md` の設計に基づき、Python 3.14+ / Starlette / Connect-RPC / LangGraph で構築した。AIX (Gemma4 26B) に接続し、6 ノードのパイプライン（planner → gatherer → curator → writer → critic → finalizer）でレポートを自動生成し、PostgreSQL 18 に永続化する。

**結論: パイプラインはエンドツーエンドで動作し、日本語レポートを生成・永続化できる状態。ただし scope 伝播と検索クエリの品質に未修正の問題がある。**

---

## 実装済みコンポーネント

### バックエンド (acolyte-orchestrator)

| コンポーネント | 状態 | 詳細 |
|--------------|------|------|
| Proto 定義 | ✅ 完了 | `proto/alt/acolyte/v1/acolyte.proto` — 11 RPC (buf lint pass) |
| Connect-RPC サービス | ✅ 完了 | CreateReport, GetReport, ListReports, ListReportVersions, StartReportRun, GetRunStatus, HealthCheck |
| PostgreSQL 永続化 | ✅ 完了 | `PostgresReportGateway` + `psycopg_pool.AsyncConnectionPool` |
| Atlas マイグレーション | ✅ 完了 | `acolyte-migration-atlas/` — 7 テーブル, 11 SQL statements |
| OllamaGateway | ✅ 完了 | AIX Gemma4 26B 直接接続, ADR-579 `_BASE_OPTIONS` 統一 |
| SearchIndexerGateway | ✅ 完了 | `GET /v1/search` (search-indexer REST API) |
| LangGraph パイプライン | ✅ 完了 | 6 ノード, conditional critic loop (max 2 revisions) |
| Pact CDC テスト | ✅ 完了 | acolyte → news-creator (2), acolyte → search-indexer (2) |
| テスト | ✅ 33 pass | E2E 6 + CDC 4 + Unit 23 |

### フロントエンド (alt-frontend-sv)

| コンポーネント | 状態 | 詳細 |
|--------------|------|------|
| `/acolyte` 一覧ページ | ✅ 完了 | 新聞マストヘッド風, staggered fade-in animation |
| `/acolyte/new` 作成ページ | ✅ 完了 | ラジオカード型 report type 選択, scope textarea |
| `/acolyte/reports/[id]` 詳細ページ | ✅ 完了 | セクションタブ, 版履歴サイドバー, change_kind 表示 |
| Connect-RPC クライアント | ✅ 完了 | REST wrapper (`acolyte.ts`) — proto codegen 前の暫定実装 |
| サイドバーナビ | ✅ 完了 | `Sidebar.svelte` の `baseMenuItems` に追加 (ScrollText icon) |
| フォント | ✅ 完了 | IBM Plex Mono を Google Fonts + CSS 変数で全 UI に適用 |

### インフラ

| コンポーネント | 状態 | 詳細 |
|--------------|------|------|
| `compose/acolyte.yaml` | ✅ 完了 | acolyte-db (PG18) + acolyte-db-migrator (Atlas) + acolyte-orchestrator |
| BFF ルーティング | ✅ 完了 | `server.go` に `/alt.acolyte.v1.AcolyteService/` 追加 + テスト 3 pass |
| BFF env var | ✅ 完了 | `ACOLYTE_CONNECT_URL` in `bff.yaml` |
| AIX 接続変数 | ✅ 完了 | `.env` に `ACOLYTE_LLM_URL`, `ACOLYTE_LLM_HOST`, `ACOLYTE_MODEL` |
| nginx | ✅ 不要 | `/api/v2/` catch-all → SvelteKit proxy → BFF で対応 |

---

## DB スキーマ (7 テーブル)

```
reports                    — mutable current state (current_version integer)
report_versions            — immutable snapshots (change_seq BIGSERIAL)
report_change_items        — field-level change tracking
report_sections            — mutable section state (current_version integer)
report_section_versions    — immutable section content (body TEXT)
report_runs                — execution records
report_jobs                — job queue (SELECT ... FOR UPDATE SKIP LOCKED)
```

---

## パイプライン実行実績

### Run 1 (英語, search-indexer 修正前)

| ノード | 所要時間 | 結果 |
|--------|---------|------|
| Planner | 100s | JSON parse 失敗 → fallback 1 section |
| Gatherer | 0s | search-indexer 404 → evidence=0 |
| Writer | 64s | 1,100 chars — 「データを提供してください」 |
| **判定** | — | **NG — evidence なし、メタ発言** |

### Run 2 (英語, search-indexer 修正後)

| ノード | 所要時間 | 結果 |
|--------|---------|------|
| Planner | 24s | `num_predict=512, temperature=0` → fallback 3 sections |
| Gatherer | 0.1s | **20 articles from search-indexer** |
| Curator | 121s | 10 件に絞り込み |
| Writer (×3) | 86s + 98s + 66s | **12,767 chars** — 実質的な分析内容 |
| Critic | 27s | verdict: "accept" |
| Finalizer | 0s | version 1 → PostgreSQL |
| **合計** | 約 7 分 | **OK — 3 セクション, 12,767 文字** |

### Run 3 (日本語)

| ノード | 所要時間 | 結果 |
|--------|---------|------|
| Planner | 25s | fallback 3 sections |
| Gatherer | 0.1s | **20 articles** |
| Curator | 121s | 10 件 |
| Writer (×3) | 104s + 105s + 103s | **5,569 chars — 日本語** |
| Critic | 26s | verdict: "accept" |
| Finalizer | 0s | version 1 → PostgreSQL |
| **合計** | 約 8 分 | **ほぼ OK — 日本語で生成、ただし scope 未伝播** |

---

## 品質評価 (全レポート)

### レポート一覧

| # | タイトル | Ver | セクション | 文字数 | 言語 | 判定 |
|---|---------|-----|-----------|--------|------|------|
| 1 | AI Trend | 1 | 1 | 989 | EN | **F** — メタ発言のみ |
| 2 | AI Market Trends Q2 2026 | 1 | 1 | 1,100 | EN | **F** — メタ発言のみ |
| 3 | AI Semiconductor Supply Chain Analysis Q2 2026 | 1 | 3 | 12,767 | EN | **B** — 実質的な分析あり |
| 4 | 2026年Q2 AI半導体サプライチェーン分析 | 1 | 3 | 5,569 | JP | **B+** — 日本語で実質的な内容 |
| 5 | AI Market Trends Q2 2026 | 0 | 0 | — | — | 未実行 |
| 6 | Weekly AI Briefing - April 2026 | 0 | 0 | — | — | 未実行 |

### レポート 3 (英語, 最高品質) の評価

- **Executive Summary** (4,120 chars): 「2026 Technological Landscape – From Generative Hype to Agentic Integration」。Agentic AI、Domain-specific LLM、ROI の 3 軸で分析
- **Analysis** (5,270 chars): Generative AI からの移行を具体的に論じている。インフラ多角化、人的資本の変容にも言及
- **Conclusion** (3,377 chars): 技術の自律化と人間のスキル再定義を結論

**良い点**: 構造的、具体的トレンド名を引用、権威ある視点 (Gartner, Linus Torvalds)
**問題点**: タイトルの「半導体サプライチェーン」と内容が不一致（汎用テクノロジートレンドに）

### レポート 4 (日本語) の評価

- **Executive Summary** (1,799 chars): 「2026年に向けたテクノロジー・トレンドの展望：AIの自律化、技術の多角化、および人的資本の再定義」
- **Analysis** (2,044 chars): 3 つの柱（AIの進化、テクノロジーの多角化、人的資本の変容）で分析
- **Conclusion** (1,726 chars): 戦略的アプローチの必要性を結論

**良い点**: 日本語で書かれている（70-80%）、論理構造が明確
**問題点**: 冒頭に「トピックが明示されていませんでした」のメタ発言、英語版より短い

---

## 未修正の問題

### P0 (ブロッカー)

| 問題 | 原因 | 影響 | 修正箇所 |
|------|------|------|---------|
| **scope が空で渡される** | `connect_service.py:171` に `scope = {}` がハードコード (TODO コメント) | Writer が「トピックが明示されていません」と出力。Gatherer が常に "technology trends" で検索 | CreateReport で scope を `reports` テーブルに保存し、StartReportRun で取得して pipeline に渡す |

### P1 (品質改善)

| 問題 | 原因 | 修正方法 |
|------|------|---------|
| Planner の `format` が効かない | Gemma4 thinking モードで JSON schema が無視される (ollama/ollama#15260) | フォールバックは機能しているが、3 セクション固定になる。`think` パラメータを明示的に制御するか、プロンプトで JSON 出力を強制 |
| セクションタイトルが英語 | Planner のフォールバックが英語の固定値 | フォールバックを日本語にする、またはプロンプトで日本語セクション名を指示 |
| Writer のメタ発言 | LLM が自分の能力や制約を説明してしまう | プロンプトに「メタ発言禁止」をさらに強く明示 |
| Gatherer のクエリが scope の topic を使わない | scope=空 問題の派生 | P0 修正で解決 |

### P2 (将来改善)

| 問題 | 修正方法 |
|------|---------|
| StreamRunProgress 未実装 | Connect-RPC server-stream で進捗をリアルタイム返却 |
| 版差分 (DiffReportVersions) 未実装 | フロントエンドの diff view 用 |
| RerunSection 未実装 | セクション単位再生成 |
| Proto TypeScript codegen 未実施 | `buf.gen.yaml` に acolyte 追加 → `$lib/gen/` に TS 型生成 |
| JobQueue が in-memory | `PostgresJobGateway` に差し替え（SQL は実装済み） |
| LangGraph checkpointer 未導入 | durable execution（中断復帰）用 |
| 評価基盤なし | ROUGE/LLM-as-Judge でレポート品質を定量評価 |

---

## ADR

- [[000653]] Knowledge Acolyte レポート生成オーケストレーターを導入する (2026-04-09)
- [[000654]] Acolyte LangGraph パイプラインを AIX Gemma4 26B に接続しレポート生成を実現する (2026-04-09)

---

## テスト

| カテゴリ | 件数 | 内容 |
|---------|------|------|
| E2E | 6 | サービス起動, HealthCheck, CreateReport, ListReports, GetReport, unimplemented RPC |
| CDC (Pact) | 4 | acolyte → news-creator (2), acolyte → search-indexer (2) |
| Unit | 23 | Settings, NewsCreatorGW, SearchIndexerGW, CreateReport UC, GetReport UC, VersionBump, Pipeline (6 tests) |
| BFF | 3 | Acolyte routing (unauthorized, success, not registered) |
| Frontend | 2 | navigation.ts (desktop + mobile に Acolyte Reports リンク) |
| **合計** | **38** | 全 pass |

---

## アーキテクチャ

```
Browser → nginx → SvelteKit (/api/v2 proxy) → BFF (alt-butterfly-facade)
  → /alt.acolyte.v1.AcolyteService/* → acolyte-orchestrator :8090

acolyte-orchestrator:
  Starlette + Connect-RPC (connect-python)
  ├── handler/connect_service.py     — RPC handlers
  ├── gateway/postgres_report_gw.py  — DB (psycopg async pool)
  ├── gateway/ollama_gw.py           — AIX Gemma4 26B (Ollama /api/generate)
  ├── gateway/search_indexer_gw.py   — search-indexer REST API
  └── usecase/graph/report_graph.py  — LangGraph StateGraph
        planner → gatherer → curator → writer → critic → finalizer
                                        ↑                    |
                                        └── revise (max 2) ──┘

acolyte-db: PostgreSQL 18 Alpine (dedicated, port 5439)
acolyte-db-migrator: Atlas 0.31 (7 tables, runs before orchestrator)
AIX: Gemma4 26B (Ollama, remote host, port 11436)
```

---

## 技術スタック

| レイヤー | 技術 |
|---------|------|
| 言語 | Python 3.14+ |
| Web | Starlette (ASGI) |
| RPC | connect-python (Connect protocol) |
| DB | PostgreSQL 18, psycopg[binary] 3.2, psycopg-pool |
| Migration | Atlas 0.31 |
| Orchestration | LangGraph 0.4 (StateGraph) |
| LLM | Ollama API → Gemma4 26B (AIX remote) |
| Search | search-indexer REST API (Meilisearch proxy) |
| Config | pydantic-settings |
| Logging | structlog (JSON) |
| Linting | ruff 0.12 (py314, line-length 120) |
| Type check | pyrefly 0.42 |
| Testing | pytest + pytest-asyncio + pact-python |
| Frontend | SvelteKit 2 / Svelte 5 / TailwindCSS v4 |
| Fonts | Playfair Display + Source Sans 3 + IBM Plex Mono (SIL OFL) |
