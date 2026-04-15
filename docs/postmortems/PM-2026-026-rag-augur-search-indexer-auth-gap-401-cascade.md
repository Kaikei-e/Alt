# ポストモーテム: RAG / Augur が search-indexer 認証化により 401 カスケードで機能停止した問題

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-026 |
| 発生日時 | 2026-04-14 深夜 推定（ADR-000722 デプロイ反映後） |
| 検知日時 | 2026-04-15 13:00 JST（ユーザー指摘） |
| 復旧日時 | 2026-04-15 13:20 JST（コンテナ再ビルド完了 + healthy 確認） |
| 影響時間 | 約 11〜13 時間（間欠的に発生、Augur 操作時のみ顕在化） |
| 重大度 | SEV-2（RAG 回答生成が事実上不可、alt-backend 側の検索 UI も全面劣化） |
| 作成者 | インシデント対応担当（本セッション） |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-14 に [[000722]] で search-indexer の REST `/v1/search` と Connect-RPC `SearchService` に `X-Service-Token` 強制が導入された。PM-2026-025 で acolyte-orchestrator の同種バグは修正されたが、**rag-orchestrator と alt-backend の search-indexer 呼び出しは全く追従していなかった**。結果として Augur の retrieval パイプライン（tag search と hybrid BM25 search）は全リクエスト 401 を踏み、空の retrieval コンテキストで LLM を呼び出し続けた。下流では news-creator の GPU キューが飽和し `ollama_chat_stream_failed: status 429 queue full` が二次症状として発生。alt-backend 側でも Tag Trail / Morning Letter / Recap Search の Connect-RPC 検索が全部 401 で失敗していた。ユーザーからの「RAG が死んでいる。Augur が落ちる」という報告を契機にログ解析で 401 を特定し、consumer 側に X-Service-Token 注入 + Pact CDCT を新設して復旧。

## 影響

- **影響を受けたサービス:** rag-orchestrator (Go)、alt-backend (Go)、副次的に news-creator（キュー飽和）
- **影響を受けたユーザー数/割合:** Augur チャット機能を使った全ユーザー。ただし実数は報告者 1 名から観測
- **機能への影響:** 部分的劣化から機能停止
  - Augur ストリームチャット: retrieval コンテキスト 0 件で LLM 生成が続くため回答品質が事実上ゼロ。二次症状として 429 でストリームが切れる
  - alt-backend: `SearchArticles` / `SearchRecapsByTag` / `SearchRecapsByQuery` 全てが 401。Tag Trail と Recap Search の UI が空結果を返し続けた
- **データ損失:** なし。retrieval が 0 件でも LLM は生成を続けるため、低品質なストリーム返答が一時的に生成されただけ。DB への永続書き込みは無し
- **SLO/SLA違反:** 明示 SLO 未定義。

### 特に問題のある点

- 症状が「500 エラー」ではなく **「Augur の回答がだんだん薄くなった」** というユーザー体感の劣化で、アラートには発報しなかった
- alt-backend 側は API は 200 を返し続けた（エラーは内部ログのみ）ため、フロントエンドの "結果 0 件" だけが観測される silent failure だった
- news-creator の 429 "queue full" は別原因（queue saturation fix の想定外ケース）と誤認される可能性があった
- 既に同じパターンで PM-2026-025（acolyte）が 24 時間継続した直後だったのに、他 consumer への水平展開チェックが行われなかった

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-14 深夜 推定 | [[000722]] が merge され、search-indexer に X-Service-Token 必須化が反映される |
| 2026-04-14 以降 | rag-orchestrator と alt-backend の search-indexer 呼び出しが全て 401 を返し始める。両サービスは警告ログのみを出し、処理を継続する |
| 2026-04-15 11:58 頃 | rag-orchestrator ログで `ollama_embed_failed: context deadline exceeded`（先行する別要因の GPU 負荷上昇）も観測 |
| 2026-04-15 12:14 | 実ユーザーの Augur 操作で `tag_search_failed: search returned status: 401` と `hybrid_bm25_search_failed: bm25 search returned status: 401` が大量発生。LLM 生成は続き `ollama_chat_stream_failed: status 429 queue full` も出る |
| 2026-04-15 13:00 | **検知** — ユーザーから「RAG が死んでいる。Augur が落ちる。直近の mTLS 対応が原因」の報告 |
| 2026-04-15 13:01 | **対応開始** — `docker logs alt-rag-orchestrator-1` を解析し 401 パターンを特定 |
| 2026-04-15 13:05 | 直近 git log で [[000727]] (mTLS Phase 2) と [[000722]] (search-indexer X-Service-Token 必須化) を確認。PM-2026-025 の acolyte 修正 commit `3c18f5c09` が同クラスのパターンだと認識 |
| 2026-04-15 13:08 | `rag-orchestrator/internal/adapter/rag_http/search_indexer_client.go` で `X-Service-Token` が付いていないことを確定。alt-backend 側も同じ欠落を確認（`NewConnectSearchIndexerDriver(baseURL)` が `http.DefaultClient` を使用） |
| 2026-04-15 13:10 | **原因特定** — 両 consumer とも token 未送信。compose env には `SERVICE_TOKEN_FILE` / `SERVICE_SECRET_FILE` が到達済み・config にもロード済みで、クライアント層だけが欠落という診断 |
| 2026-04-15 13:11–13:17 | **緩和策適用** — Pact CDCT RED→GREEN の順で consumer テストを追加してから実装修正（rag-orchestrator: ヘッダ注入 / alt-backend: preprocessor_connect と同じ `serviceTokenTransport` パターン移植）。search-indexer に provider verification を新設 |
| 2026-04-15 13:18 | `docker compose up --build -d alt-backend rag-orchestrator` 実行 |
| 2026-04-15 13:20 | **復旧確認** — 再ビルド後ログに `connect_rpc_service_token_configured` が記録、401 系のログが消失。alt-backend healthy |
| 2026-04-15 13:25 | `./scripts/pact-check.sh`: 6 passed / 0 failed で全契約が通ることを確認 |

## 検知

- **検知方法:** ユーザー報告（"RAG が死んでいる" 系の自然言語による症状訴え）
- **検知までの時間 (TTD):** 約 11〜13 時間（実際に 401 が起き始めてから最初の報告まで）
- **検知の評価:** 不十分。下記の複数の理由で silent failure 化しており、モニタリングは一切発報しなかった
  - rag-orchestrator の 401 は `WARN` レベルで、`ERROR` 以上をトリガーにするアラートに引っかからなかった
  - alt-backend は search-indexer 呼び出し失敗時にもハンドラとしては 200 を返す（結果 0 件）ため、エラーレート系アラートでは検出できなかった
  - ユーザー体験の「retrieval が薄い」は個別ユーザーの主観であり、合成モニタリングがない
  - PM-2026-025 が acolyte で起きた直後だったが、search-indexer を呼ぶ他サービスへの同種チェックが行われなかった

## 根本原因分析

### 直接原因

rag-orchestrator の `SearchIndexerClient.Search()` / `SearchBM25()` と alt-backend の `NewConnectSearchIndexerDriver()` が、search-indexer の `RequireServiceAuth` ミドルウェアに必須の `X-Service-Token` ヘッダを送らずに呼び出していた。

### Five Whys

1. **なぜ rag-orchestrator の retrieval が 401 になるのか？**
   → search-indexer が `X-Service-Token` なしのリクエストを 401 で拒否するようになっていた（[[000722]]）のに、rag-orchestrator の HTTP クライアントは同ヘッダを付けていなかったから。
2. **なぜヘッダを付けていなかったのか？**
   → [[000722]] が search-indexer（provider 側）の middleware を追加しただけで、consumer 側の全コードパスへの伝搬が作業範囲に含まれていなかったから。
3. **なぜ provider 変更時に全 consumer の追従が必須化されなかったのか？**
   → provider の要件強化を「自サービスの変更」として扱い、consumer 一斉更新を強制する playbook（tdd-workflow スキルの Phase 0）が「認証ヘッダの昇格」をトリガーとして列挙していなかったから。
4. **なぜ Pact CDCT が検出できなかったのか？**
   → `pacts/` に `rag-orchestrator-search-indexer.json` および `alt-backend-search-indexer.json` が**そもそも存在しなかった**。contract-unprotected な consumer が 2 件あり、search-indexer 側にも provider verification テストが存在しなかったため、契約レベルでの保護が一切働かなかった。
5. **なぜ contract-unprotected な consumer が 2 件あることに誰も気付かなかったのか？**
   → tdd-workflow スキルの CDC 対応表が「consumer → provider」の方向を `search-indexer | alt-backend, recap-worker, mq-hub` のように**逆向きに読める形**で記載されており、provider 視点の逆引きテーブルも欠けていた。結果として「search-indexer を consumer として呼ぶ alt-backend / rag-orchestrator の pact が無い」という現状が可視化されなかった。

### 根本原因

Pact CDCT の運用体系に、**「provider 側が要件を強める変更に対して、全 consumer の pact が存在し、かつ provider verify にかかっているか」を強制する仕組みが無かった**。結果、[[000722]] の変更は provider-only の変更として完結し、consumer 未追従が暗黙に許容された。

### 寄与要因

- **類似 incident の教訓を水平展開していない:** PM-2026-025（acolyte-orchestrator）の修正 commit `3c18f5c09` は 1 日前に merge されていたが、search-indexer を呼ぶ他の consumer（rag-orchestrator / alt-backend）への同種監査は行われなかった。
- **silent failure に対する監視の弱さ:** 「X-Service-Token 401 を WARN でログ出し」のみで、アラート化も Grafana / dashboard 化もされていなかった。
- **Connect-RPC クライアントの初期化ボイラープレートが重複:** preprocessor_connect には既に `serviceTokenTransport` を持つ正解実装があったが、search_indexer_connect では使われていなかった（リファレンス実装の参照規約が無い）。
- **pact-check.sh に search-indexer provider verification が未登録:** そもそも search-indexer が provider として契約を verify していなかったため、任意の consumer pact 欠落がゲートで止まらない運用だった。

## 対応の評価

### うまくいったこと

- ログから 401 パターンを 5 分以内に特定できた（構造化 JSON ログの `category` / `error` フィールドが機能した）
- 直前の commit `3c18f5c09`（PM-2026-025 の修正）がリファレンス実装として完全に使える状態にあり、実装パターンをそのまま移植できた
- alt-backend の preprocessor_connect `serviceTokenTransport` が既存しており、コピー元として即利用可能だった
- 修正を Pact CDCT RED→GREEN 順で行ったため、修正と同時に再発防止が契約レベルで入った
- `scripts/pact-check.sh` が既に file-based モードで全契約を 1 コマンドで検証できる状態にあり、回帰チェックを即時実行できた

### うまくいかなかったこと

- PM-2026-025 の水平展開チェックを省略したことで、同種の 2 件が 1 日遅れで顕在化した
- silent failure の検知を完全にユーザー報告に依存しており、TTD が 11〜13 時間まで伸びた
- rag-orchestrator の 401 が WARN で出ていたのに、「WARN 頻度の急増」を発火条件にしたアラートが存在しなかった
- Pact の CDC 対応表の方向性が曖昧で、provider 視点の監査が成立しなかった

### 運が良かったこと

- ユーザー自身が「直近の mTLS 対応が原因」と的確な仮説付きで報告してくれたため、調査時間が大幅に短縮された
- 実害は「回答品質劣化」にとどまり、データ整合性・認証境界の破綻には波及しなかった（search-indexer 側の fail-closed 設計が効いた）
- 修正対象が compose 経由で env 到達済み / config ロード済みだったため、env 追加や secret 発行が不要だった

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | rag-orchestrator の `SearchIndexerClient` に `X-Service-Token` 注入（pact 付き） | 実装担当 | 2026-04-15 | DONE |
| 2 | 予防 | alt-backend `NewConnectSearchIndexerDriver` に `serviceTokenTransport` 導入（pact 付き） | 実装担当 | 2026-04-15 | DONE |
| 3 | 予防 | search-indexer に Go provider verification (`provider_test.go`) を新設し `scripts/pact-check.sh` に登録 | 実装担当 | 2026-04-15 | DONE |
| 4 | 予防 | tdd-workflow スキルに "Phase 0b: Provider-adds-requirement Playbook" を追加し、CDC 対応表を A→B 方向統一 + provider 逆引き表を追加 | 実装担当 | 2026-04-15 | DONE |
| 5 | 予防 | acolyte-orchestrator の consumer pact (`acolyte-orchestrator-search-indexer.json`) を X-Service-Token 固定に更新し、search-indexer provider verify に再組み込み | Acolyte 担当 | 2026-04-22 | TODO |
| 6 | 予防 | search-indexer を consumer に持つ他サービス（mq-hub など）の pact に `X-Service-Token` が固定されているか監査 | SRE / platform | 2026-04-22 | TODO |
| 7 | 検知 | `scripts/pact-check.sh` を pre-push フック / CI 前段に必須ステップとして組み込み、provider 要件変更が consumer pact 未更新で merge されないようにする | SRE / platform | 2026-04-22 | TODO |
| 8 | 検知 | rag-orchestrator / alt-backend の 401 WARN ログに対するアラート（直近 5 分で N 回以上）を Grafana / Prometheus に追加 | SRE | 2026-04-29 | TODO |
| 9 | 検知 | retrieval が 0 件 + LLM ストリームが生成されたケースを「Augur low-context stream」メトリクスとして露出し、合成モニタリングで監視 | RAG 担当 | 2026-04-29 | TODO |
| 10 | 緩和 | rag-orchestrator に「retrieval 結果が全経路で 0 件なら LLM 呼び出しを抑止して insufficient-context を返す」guard を追加 | RAG 担当 | 2026-04-29 | TODO |
| 11 | 緩和 | search-indexer の `RequireServiceAuth` の 401 WARN に TLS peer identity（CN/SAN）を含める（[[000727]] の `peer_identity_http_middleware` と整合） | search-indexer 担当 | 2026-04-29 | TODO |
| 12 | プロセス | 認証/認可/mTLS の provider 側変更時は「Phase 0b Playbook」を PR テンプレートのチェック項目として必須化 | SRE / platform | 2026-04-22 | TODO |
| 13 | プロセス | インシデント対応時、類似 incident （直近 30 日）が存在する場合は水平展開監査を必ず 1 ステップとして実施する。PM-2026-025 のような先行 PM が居たら search-indexer を呼ぶ全 consumer の監査を同一 PR 内で実行する | SRE / platform | 2026-04-22 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

- **"Pact がある" は "CDCT が効いている" ではない。** consumer pact が存在しない consumer、provider verification に登録されていない pact、どちらか一方でもあると CDCT は機能不全に陥る。pact の **有無** を定期的に棚卸しする仕組み（provider 視点の逆引きテーブル + `scripts/pact-check.sh` の必須化）が本体。
- **Provider 側が要件を強める変更は "consumer の契約を一斉更新しないと完了しない変更"** である。[[000722]] のような認証ヘッダ追加は、provider の middleware 追加だけでは終わらない。Phase 0b Playbook を tdd-workflow スキルに明文化したのはこの教訓から。
- **類似 incident の水平展開監査は即時実施** すべき。PM-2026-025 を修正した時点で、search-indexer を呼ぶ他の consumer を即座に点検していれば PM-2026-026 は回避できた。インシデント対応の標準手順に「類似パターンの grep」を組み込む。
- **silent failure は "200 を返し続けること" で実現される。** HTTP 5xx / panic のような目立つ失敗だけではなく、「結果 0 件」「WARN ログのみ」といった劣化パターンは積極的にメトリクス化しないと、必ずユーザー報告まで見えない。
- **"既存実装のパターンをそのまま踏襲できるリファレンス実装" は再発防止コストを一桁下げる。** preprocessor_connect の `serviceTokenTransport` が完全動作していたおかげで alt-backend の修正は 10 分で完了した。逆に言えばリファレンス実装の参照規約（CLAUDE.md / skill）に明記されていれば、そもそもバグは発生しなかった可能性がある。

## 参考資料

- ADR: [[000735]] search-indexer を呼ぶ全 consumer に X-Service-Token を強制し Pact で回帰を封じる（本 incident の fix ADR）
- ADR: [[000722]] search-indexer に X-Service-Token 必須化（本 incident のトリガー）
- ADR: [[000717]] alt-backend 内部 API の shared secret 認証モデル
- ADR: [[000727]] mTLS Phase 2 client-side enforcement
- 先行 postmortem: [[PM-2026-025-acolyte-search-indexer-auth-gap-empty-reports]]（同クラス incident）
- 修正コード:
  - `rag-orchestrator/internal/adapter/rag_http/search_indexer_client.go`
  - `rag-orchestrator/internal/di/container.go`
  - `alt-backend/app/driver/search_indexer_connect/client.go`
  - `alt-backend/app/di/infra_module.go`
- 新規 Pact 契約:
  - `rag-orchestrator/internal/adapter/contract/search_indexer_consumer_test.go`
  - `alt-backend/app/driver/search_indexer_connect/contract/consumer_test.go`
  - `search-indexer/app/driver/contract/provider_test.go`
- Pact 公式リファレンス:
  - https://docs.pact.io/provider/handling_auth
  - https://docs.pact.io/pact_broker/advanced_topics/pending_pacts
  - https://docs.pact.io/pact_broker/webhooks

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
