# ポストモーテム: Knowledge Loop SSE silent failure (JWT TTL × nginx × effect race × tile duplicate)

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-045 |
| 発生日時 | 2026-05-27 22:41 (JST) — HAR 取得時点。実際の症状は数週間前から潜在 |
| 復旧日時 | 2026-05-27 23:11 (JST) — 修復コード 6 連 commit + ADR 完了。本番反映はユーザ承認待ち |
| 影響時間 | 顕在化 30 分以内で復旧コード投入。潜在期間は推定 2026-04 以降〜4 週間 |
| 重大度 | SEV-3 (主機能の一部劣化、HTTP エラーは出ない silent failure) |
| 作成者 | オンコール対応 + 修復担当者 |
| レビュアー | 未割当 |
| ステータス | Draft |

## サマリー

2026-05-27 22:41 JST にユーザから「Knowledge Loop で各ボタンが反応しない / ASK・REVISIT のボタンが重複している」との報告を受け、HAR ファイルと alt-backend ログを突き合わせて配信路 4 層の合成不具合を特定。HTTP エラーはゼロ (200×153 / 204×1 / 302×2) だが SSE は body 42 byte の heartbeat だけを返し、`observedProjectionRevision` が client 側で停滞 → REVISIT 連打 → UI 無反応のループに陥っていた。`BACKEND_TOKEN_TTL=5m` と `streamStaleTimeout=30m` の不整合、nginx の Knowledge Loop 専用 location 欠落、`useKnowledgeLoopStream` の catch reconnect race、`LoopEntryTile` の ASK 二重発火という独立 4 欠陥が同じ症状面に重なっていた。30 分以内に切り分け、6 連 commit + ADR-000929 で修復。本番反映は別途承認待ち。

## 影響

- **影響を受けたサービス:** alt-frontend-sv `/loop` ページ全機能、付随する `/loop/transition` / `/loop/ask` 経路、alt-backend `StreamKnowledgeLoopUpdates`。
- **影響を受けたユーザー数/割合:** `/loop` 利用者の全員。Single-tenant 運用のため母数は 1 ユーザだが、Knowledge Loop UX 全体が機能不全。
- **機能への影響:** 部分的劣化。ボタン押下は POST が成功するが UI 反映なし。同一 entry への REVISIT 連打 / ASK 後の transition 二重発火 / 同一ユーザに対し直近 30 分で 5+ 本のストリーム並走。
- **データ損失:** なし (`knowledge_events` への append は無事、projection 再生可)。
- **SLO/SLA違反:** Knowledge Loop SLO は未定義のため formal な違反なし。ただし UX KPI の `time-to-stage-advance` が想定 (≤ 1s) を大幅超過。

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04 中旬 | `BACKEND_TOKEN_TTL=5m` 設定が `compose/auth.yaml` / `compose/compose.staging.yaml` に投入。`streamStaleTimeout=30m` 想定の handler との不整合が潜在化 (この時点では誰も気付かず) |
| 2026-05 上旬 〜 5/26 | alt-backend ログに同一ユーザの `stream_jwt_expired` が 5 分間隔で発生し続けるが、専用 SLI 未整備のため監視で検知できず |
| 2026-05-27 22:41 | **検知** — ユーザが `/loop` で REVISIT を 3 連打しても反応しないことに気付き、Chrome DevTools で HAR を取得して報告 |
| 2026-05-27 22:50 | **対応開始** — HAR ファイル受領、156 entries / 全 2xx-3xx を確認 |
| 2026-05-27 22:55 | HAR 解析で SSE 2 セッション (`13:41:01.905Z` / `13:41:25.548Z`) が共に body 42 byte (= heartbeat 2 件分のみ) と特定。`observedProjectionRevision:1` が 11/12 の transition で停滞 |
| 2026-05-27 23:00 | alt-backend ログで同一ユーザ `93852825-...` の `stream_started` ↔ `stream_jwt_expired` が 5 分周期で何本も並走していることを確認 |
| 2026-05-27 23:02 | **原因特定** — JWT TTL × stale window 不整合、nginx 専用 location 不在、`useKnowledgeLoopStream` の catch race、`LoopEntryTile` の ASK 二重発火という 4 層が同じ症状面に乗っていることを確定 |
| 2026-05-27 23:05 | TDD で `useKnowledgeLoopStream.svelte.test.ts` の RED テストを追加 (cursor 永続化 / 意図的 abort 抑制) |
| 2026-05-27 23:08 | **緩和策適用** — RED → GREEN: TTL 30m、nginx 専用 location、sessionStorage cursor + intentional-abort guard、tile 二重発火撤去を 6 連 commit |
| 2026-05-27 23:10 | **復旧確認 (テスト層)** — `bun run test` 1364/1364 pass, `go test ./connect/v2/knowledge_loop/...` ok, `svelte-check` 0 errors / 0 warnings |
| 2026-05-27 23:11 | ADR-000929 を commit (`a66faf852`)。本番反映 (`git push origin main` → dispatch-deploy) はユーザ承認待ち |

## 検知

- **検知方法:** ユーザ報告 + HAR 提出。
- **検知までの時間 (TTD):** 推定 4 週間以上 (`BACKEND_TOKEN_TTL=5m` 投入から)。実質的には自動検知できなかった。
- **検知の評価:** 著しく遅い。alt-backend は `alt.knowledge_loop.stream_jwt_expired` を構造化ログで吐いていたが、これを SLI / アラートに繋いでいなかったため、5 分間隔の churn が常態化していた。次のインシデントを未然に防ぐには、ストリーム短命化のメトリクス化が必須。

## 根本原因分析

### 直接原因

1. `compose/auth.yaml:139` の `BACKEND_TOKEN_TTL=5m` が `alt-backend/app/connect/v2/knowledge_loop/handler.go:345` の `streamStaleTimeout = 30 * time.Minute` よりはるかに短い。
2. `nginx/conf.d/default.conf` の Connect-RPC streaming 専用 location 群 (augur / morning_letter / feeds / knowledge_home / admin_monitor) に `knowledge.loop.v1` が欠落しており、汎用 `^/api/.*(stream|sse)` location が `proxy_request_buffering off` と `X-Accel-Buffering: no` を提供しない経路に流れていた。
3. `useKnowledgeLoopStream.svelte.ts` の `connect()` catch ブロックが、自前 `disconnect()` 由来の abort と外部エラー由来の close を区別せず、`scheduleReconnect()` を発火していた。effect の再実行で `stopped=false` に戻った隙間にこの catch が走るとゴーストストリームが生まれる。
4. `LoopEntryTile.svelte:316-318` が ASK 完了後に `buildAskTransitionMetadata` で `onTransition` を再発火しており、親 `+page.svelte:314` の発火と二重になっていた。`await goto()` のレースに救われて本番では稀にしか観測されなかった。

### Five Whys

1. **なぜ REVISIT が UI に反映されなかったのか？** → `observedProjectionRevision:1` のまま transition を送り続けていた。client の cursor が前進していなかった。
2. **なぜ cursor が前進しなかったのか？** → SSE が `update` 系 frame をひとつも届けず、heartbeat のみ流れていた (body 42 byte = 初期 + 10s heartbeat 2 件分のみ)。
3. **なぜ update frame が届かなかったのか？** → `knowledge_events` には event が append されていた (DB 確認で `oriented` 22 件 / `decision_presented` 12 件 / `acted` 10 件 等)。配信路の途中で消失している。
4. **なぜ配信路で消失したのか？** → (a) JWT が 5 分で expire するため stream が再接続を繰り返し、各セッションが実質的に update frame を送る前に終わる。(b) nginx の汎用 SSE location は `proxy_request_buffering off` を持たず、上流 (Cloudflare 等の CDN) で update frame がバッファされる。(c) client 側で intentional abort のはずがゴースト reconnect を生み、同時並走するストリーム同士が cursor を打ち消し合う。
5. **なぜ JWT TTL と stream stale window がずれていたのか？** → `BACKEND_TOKEN_TTL` は auth-hub の `CACHE_TTL` (5m) と揃えるつもりで設定されたが、SSE の生存時間という別観点を考慮した review が無かった。Connect-RPC streaming サービスを追加する際の checklist が docs/runbooks に存在しない。

### 根本原因

**「ストリーミング配信路のレイヤ整合性を保証する仕組みが Alt のプロセスに無い」** ── auth TTL、nginx location、client lifecycle (Svelte 5 $effect)、UI transition emit という 4 つの独立レイヤがそれぞれ別の決定論で動いており、互いの整合を機械的に検証する仕組み (SLI / runbook / source-spec) が無かった。結果、どれか 1 つの不整合では大きな症状が出ない (5 分 churn だけなら回復するし、tile の二重 emit は projector の冪等性で吸収される) が、4 つが揃うと UI が完全に動かなくなる「合成 silent failure」を生む。

### 寄与要因

- **過去 ADR の死角**: 000874〜000928 の Loop 関連 ADR はすべて projection を「生成する」側 (source_url pin / CitationKind / DI wiring / surface_planner heartbeat 等) を順番に潰してきた。projection を「配信する」側を扱う ADR は今回 (000929) が初。
- **観測の死角**: `alt.knowledge_loop.stream_jwt_expired` ログを SLI 化しておらず、5 分 churn の常態化を誰も気付けなかった。
- **TDD の死角**: `useKnowledgeLoopStream` には catch race / cursor 永続化のテストが無かった。`LoopEntryTile` のソースガードも ASK 二重発火を捕捉していなかった。
- **silent failure の性質**: HTTP は全部 200。Sentry も発火しない。ユーザが HAR を出して初めて見える種類のバグ。

## 対応の評価

### うまくいったこと

- ユーザ提供の HAR ファイルを起点に 30 分以内で root cause 4 層を切り分けられた。
- HAR の `bodySize=42` という小さな数値に違和感を持ち、heartbeat 計算 (初期 + 10s 周期で 2 件) と一致することを早期に突き止め、「SSE が物理的に届いていない」事実を定量化できた。
- alt-backend の構造化ログ (`stream_started` / `stream_jwt_expired`) が trace_id 付きで残っており、HAR の SSE セッションと 1:1 で紐付けられた。
- TDD で RED → GREEN を分割し、`useKnowledgeLoopStream.svelte.test.ts` の cursor 永続化テストと `LoopEntryTile.source.spec.ts` の duplicate-emit ガードを再発防止アセットとして残せた。
- 過去 ADR 000874〜000928 を時系列で総点検したことで「projection 生成側は十分修復、配信側は未着手」というギャップを発見できた。
- 修復を 6 connit + ADR commit に意味別に分け、レビューしやすい歴史を残せた。

### うまくいかなかったこと

- 4 週間以上 silent failure を放置していた。alert / SLI が無かった。
- ユーザ報告まで気付けなかった。観測機構の不足が露呈。
- `BACKEND_TOKEN_TTL=5m` という設定変更時に SSE 生存時間との整合をレビューする仕組みが無かった。
- Connect-RPC streaming サービス追加時の checklist (auth TTL / nginx location / client cursor / UI emit ownership) が docs/runbooks に存在せず、レビューが網羅的でない。

### 運が良かったこと

- `knowledge_events` への append は単独で完結していたため、4 週間分の event log が無事に残っており、reproject の必要が無かった。
- ASK 二重発火は `client_transition_id` の冪等性で server 側で吸収されており、データ的な汚染は無かった。
- 影響範囲が単一テナント (1 ユーザ) だったため、ユーザ体験の毀損は深刻だが対外的な信用毀損は無かった。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | 本 ADR-000929 の 4 層修復を本番反映 (`git push origin main` → dispatch-deploy → alt-frontend-sv / auth-hub / nginx の rolling restart 確認) | Kaikei | 2026-05-28 | TODO |
| 2 | 予防 | Connect-RPC streaming サービス追加 runbook を `docs/runbooks/connect-rpc-streaming-checklist.md` に新設 (auth TTL / nginx location / client cursor 永続化 / UI emit ownership の 4 項目チェック) | Kaikei | 2026-06-10 | TODO |
| 3 | 検知 | rask-log-aggregator (ClickHouse `otel_logs`) に「同一 (tenant, user) の `alt.knowledge_loop.stream_started` を 1 分窓で 2 回以上」検出する SLI を追加し、Grafana ダッシュボードに載せる | Kaikei | 2026-06-05 | TODO |
| 4 | 検知 | `alt.knowledge_loop.stream_jwt_expired` の rate を 5 分窓で集計し、`> 3 件/5min` を warning にする Prometheus alert を追加 | Kaikei | 2026-06-05 | TODO |
| 5 | 検知 | Streaming SSE エンドポイント (knowledge_loop / knowledge_home / augur 等) の body size / duration / reconnect 間隔の 3 指標を Grafana に載せる | Kaikei | 2026-06-20 | TODO |
| 6 | 緩和 | `useKnowledgeLoopStream` の cursor 永続化キーに userId を含める (現状は lensModeId のみ)。logout → 再ログイン同一タブのケースで stale cursor を防ぐ | Kaikei | 2026-06-15 | TODO |
| 7 | 緩和 | alt-backend `StreamKnowledgeLoopUpdates` handler に「ctx.Done() を即時にトリガする graceful shutdown」のヘルスチェック単体テスト追加 (nginx 切断時の伝播確認) | Kaikei | 2026-06-15 | TODO |
| 8 | プロセス | Knowledge Home / Loop / Augur 等の SSE 系エンドポイントを定期 (月次) で HAR + alt-backend log で health audit する手順を runbook に追加 | Kaikei | 2026-06-30 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

- **silent failure は HTTP ステータスでは検知できない**。SSE の body size、ストリーム数、reconnect 間隔のような「正常路の指標」を SLI 化しないと、4 週間放置の悪夢が再発する。
- **Streaming endpoint は 4 層の整合を機械的に検証する必要がある**: auth TTL、proxy buffering、client lifecycle (Svelte 5 $effect の cleanup / abort semantics)、UI emit ownership。どれか 1 つの不整合は無害でも、全部揃うとユーザ体験が破壊される。
- **過去 ADR の chronological narrative は強力なツール**。「過去に X を直したのに今回 X が再発した」のではなく、「過去に X (projection 生成) を直してきたが Y (projection 配信) は未着手」というギャップを発見できた。
- **HAR の body size 数値は侮れない**。`bodySize=42` という小さな数値が「heartbeat 2 件分」と一致したことから物理的な配信失敗を確信できた。HAR 解析時は status code だけでなく content size も読む。
- **Svelte 5 $effect の cleanup と非同期 for-await の組み合わせは race を生みやすい**。AbortController を closure capture し、catch でも `signal.aborted` を見る defensive pattern を default にすべき。
- **TDD の RED commit は将来の自分への手紙**。`buildAskTransitionMetadata` を tile から外す source-spec ガードは、別の誰か (あるいは半年後の自分) が「便利だから」と再追加するのを防ぐ。

## 参考資料

- [[000929]] Knowledge Loop SSE 配信路を JWT TTL・nginx location・client ライフサイクル・tile 二重発火の 4 層で固める
- [[000874]] Knowledge Loop stream cadence env tunable 化
- [[000914]] intent_signal を proto に昇格
- [[000923]] Recall を Home に統合 (dual stream 解消)
- [[000924]] Knowledge Loop 5 Pillar 修復
- [[000925]] source_url drift 修復
- [[000927]] RAG citation hydration
- [[000928]] HybridSearchRepository DI を unconditional 化
- 修復 commit chain: `91a6a6392` (TTL), `799f8df61` (nginx), `3ef0fcb8e` (RED tests), `52c6dea97` (GREEN stream), `ed94c796b` (RED ASK), `c3a889b45` (GREEN ASK), `a66faf852` (ADR)
- HAR: `curionoah.com.har` (2026-05-27 22:41 JST 取得, 156 entries)
- 関連ログ: alt-backend `alt.knowledge_loop.stream_started` / `stream_jwt_expired` 構造化ログ (2026-05-27 11:41-13:48 UTC)
- 関連 plan: `/home/koko/.claude/plans/har-knowledge-loop-ask-revisit-web-adr-reactive-nebula.md`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
