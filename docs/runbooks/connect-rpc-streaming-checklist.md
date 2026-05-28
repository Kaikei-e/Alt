---
title: Connect-RPC streaming service checklist
date: 2026-05-29
tags:
  - runbook
  - knowledge-loop
  - knowledge-home
  - augur
  - streaming
  - connect-rpc
  - alt-backend
  - nginx
  - auth-hub
related:
  - "[[000874]]"
  - "[[000929]]"
  - docs/postmortems/PM-2026-045-knowledge-loop-sse-silent-failure-jwt-ttl-nginx-effect-race-tile-duplicate.md
---

# Connect-RPC streaming service checklist

PM-2026-045 で 4 週間 silent failure を生んだ「auth TTL × stream stale window × nginx location × client cursor × UI emit ownership」5 軸の不整合を、新規 Connect-RPC streaming service 追加時と既存 service の review 時に **必ず** 通すための checklist。

5 軸のどれか 1 つの不整合では大きな症状が出ないが、複数が揃うと UI が完全に動かなくなる「合成 silent failure」を生む。**1 行でも違反していたら新規 service は landing しない**。

## 5 軸 checklist

| # | 軸 | What | Where |
|---|---|---|---|
| 1 | **auth TTL** | `BACKEND_TOKEN_TTL` (auth-hub) ≥ alt-backend handler の `streamStaleTimeout`。両者が同じ wall-clock を見るため、TTL がストリーム生存時間より短いと 5 分間隔で reconnect storm が起きる | `compose/auth.yaml`、`compose/compose.staging.yaml`、`alt-backend/app/connect/v2/<service>/handler.go` |
| 2 | **nginx SSE location** | `/api/v2/alt\.<service>\.v[0-9]+\..+/Stream` 専用 location が、`proxy_buffering off` + `proxy_request_buffering off` + `proxy_cache off` + `X-Accel-Buffering: no` + `proxy_send_timeout >= streamStaleTimeout` + `proxy_read_timeout >= streamStaleTimeout` を持つ。汎用 `^/api/.*(stream\|sse)` location には buffering off が無いので、upstream CDN で update frame がバッファされる | `nginx/conf.d/default.conf` |
| 3 | **client cursor persist** | FE hook が `(resumeFromSeq, lastSeqHiwater)` を `sessionStorage` に永続化し、SvelteKit invalidateAll / SPA 遷移で hook が remount されても resume seq が 0 に戻らない。`stream_expired` 受領時のみ cursor を破棄する | `alt-frontend-sv/src/lib/hooks/use<Service>Stream.svelte.ts` |
| 4 | **UI emit ownership** | 同一ユーザー意図 (ASK / OPEN / TRANSITION 等) に対し emit する箇所が **1 か所だけ**。tile / page / hook の 3 階層で重複しないよう source-spec test で機械的に gate する | `alt-frontend-sv/src/lib/components/.../*.source.spec.ts`, `alt-frontend-sv/src/routes/(app)/<service>/+page.svelte` |
| 5 | **dedupe key** | server-side で `(user_id, client_transition_id)` を dedupe。UUIDv7 必須、`+5min future / -48h past` をはみ出したら拒否。TTL 48h の dedupe 行で fast-path、TTL 越えは `knowledge_events.dedupe_key` unique index で slow-path 拒否 | `alt-backend/app/usecase/<service>_usecase/*.go`、proto の RPC 定義 |

## 既存 service inventory (2026-05-29 時点)

| Service | nginx location | auth TTL 一致 | client cursor persist | single emit | dedupe key |
|---|---|---|---|---|---|
| `alt.knowledge.loop.v1` (StreamKnowledgeLoopUpdates) | ✅ `default.conf:374` | ✅ 30m / 30m (ADR-000929) | ✅ `useKnowledgeLoopStream.svelte.ts` sessionStorage cursor | ✅ ASK は `+page.svelte:314` の単一 emit (ADR-000929 で tile 二重発火撤去) | ✅ `client_transition_id` UUIDv7 |
| `alt.knowledge_home.v1` (StreamKnowledgeHome*) | ✅ `default.conf:405` | ✅ 30m / 30m | ⚠️ Recheck recommended (`useKnowledgeHome.svelte.ts:109` is non-stream pull; `useStreamUpdates.svelte.ts` is the stream side — verify cursor persistence) | ✅ Single emit per kind | ✅ |
| `alt.augur.v2` / `alt.morning_letter.v2` / `alt.feeds.v2` (Stream*) | ✅ `default.conf:335` (shared location) | ⚠️ Verify per-service streamStaleTimeout | ⚠️ Per-hook verify | ⚠️ Per-feature verify | ✅ |
| `alt.admin_monitor.v1` (Watch / Catalog / Snapshot) | ✅ `default.conf:439` (whole service) | ✅ 30m | ⚠️ Watch 経路は FE 自動 rotate (15 分) のため cursor 永続化不要 | N/A (admin 用) | N/A |

⚠️ 印は **次の adjacent PR で verify** する宿題。新 streaming service が landing する前に必ず ✅ に上げる。

## 観測層との連動

このチェックリストは alert / dashboard と同期させる:

- **alert** — `observability/prometheus/rules/knowledge-loop-rules.yml` の `KnowledgeLoopStreamJwtExpiredRate` (TTL 不整合) / `KnowledgeLoopStreamReconnectStorm` (catch-race / nginx 回帰) が発火したら本 checklist の row 1-4 を再点検する
- **dashboard** — `observability/grafana/dashboards/knowledge-loop-projector.json` の "SSE stream lifecycle" / "Stream JWT-expired ratio" / "Stream upstream fetch failures" panels がベースライン乖離なら同じく row 1-4
- **structured logs** — `alt.knowledge_loop.stream_started` / `stream_jwt_expired` / `stream_ended` / `stream_fetch_failed` (`alt-backend/app/connect/v2/knowledge_loop/handler.go`) は構造化 + Prometheus counter (`alt_knowledge_loop_stream_*_total`) の 2 段で出る

新 streaming service を追加する PR は、上記 3 つすべてに新 service 名のシリーズが現れることを確認する。出ていなければ instrumentation 漏れ。

## 新規 Connect-RPC streaming service 追加手順

新規 service `alt.<service>.v<N>.<Service>Service/Stream*` を立てる場合:

1. **proto + handler 設計**
   - canonical contract と OODA 不変条件 (event-source / append-first / reproject-safe) を満たす
   - `client_transition_id` (UUIDv7) を必須化、`(user_id, client_transition_id)` の TTL 48h dedupe を server で実装

2. **auth TTL 確認**
   - `compose/auth.yaml` と `compose/compose.staging.yaml` の `BACKEND_TOKEN_TTL` が新 handler の `streamStaleTimeout` 以上であることを verify
   - 短ければ TTL を伸ばすか、`streamStaleTimeout` を短くする (前者が default)

3. **nginx location 追加**
   - `nginx/conf.d/default.conf` に `^/api/v2/alt\.<service>\.v<N>\..+/Stream` 専用 location を追加
   - 5b'. / 5b. ブロックをコピペ元として使う (knowledge_loop / knowledge_home)
   - 必須 directive: `proxy_buffering off`、`proxy_request_buffering off`、`proxy_cache off`、`gzip off`、`add_header X-Accel-Buffering no always`、`proxy_send_timeout >= streamStaleTimeout`、`proxy_read_timeout >= streamStaleTimeout`

4. **FE hook の cursor persist**
   - `use<Service>Stream.svelte.ts` を新設し、`cursorPersistKey` opt を取り `sessionStorage` キー `<service>-stream:resume:<key>` に `lastSeqHiwater` を読み書き
   - `connect()` 内で `myAbort = new AbortController()` を closure capture、catch 節で `myAbort.signal.aborted` を見て自前 abort 起因なら `return`
   - `stream_expired` 受領時のみ cursor を破棄

5. **UI emit ownership**
   - 同一ユーザー意図に対する emit 経路を 1 か所に固定 (tile / page / hook の重複 emit を source-spec で機械的に gate)
   - 例: `<Tile>.source.spec.ts` で import グレップして重複 import が無いことを確認

6. **観測層への配線**
   - `alt-backend/app/connect/v2/<service>/metrics.go` を新設し、`<namespace>_<service>_stream_started_total` / `_ended_total{reason=...}` / `_fetch_failed_total` を promauto で export
   - handler.go の `slog.InfoContext("..stream_started")` 直後で counter.Inc() を呼ぶ
   - `observability/prometheus/rules/<service>-rules.yml` に `JwtExpiredRate` / `ReconnectStorm` alert を追加
   - `observability/grafana/dashboards/<service>.json` に lifecycle / jwt_expired_ratio / fetch_failed panel を追加

7. **このチェックリストの inventory を更新**
   - 新 service の行を上の table に追加、5 列すべて ✅ に揃ってから merge

## 月次 health audit 手順 (推奨)

PM-2026-045 が「ログは出ていたが SLI / alert が無く 4 週間放置」だった反省として、月初に 30 分の audit window を取る:

```bash
# 1. 直近 30 日の HAR 取得 (ユーザフロー再現)
#    Chrome DevTools → /loop / /knowledge-home / augur conversation 開く →
#    Network tab Save all as HAR → tmp/<date>.har

# 2. SSE body size が heartbeat 以上か (PM-2026-045 で body 42 byte = heartbeat のみ)
jq -r '.log.entries[] | select(.request.url | test("/Stream")) | "\(.response.bodySize)\t\(.request.url)"' tmp/<date>.har | sort -n | head -20

# 3. 同一ユーザの stream_started を 1 分 buckets で集計
docker compose -f compose/compose.yaml -p alt logs alt-backend --since 30d \
  | grep alt.knowledge_loop.stream_started \
  | awk '{print substr($0,1,16)}' | sort | uniq -c | sort -rn | head -20

# 4. stream_jwt_expired の頻度 (BACKEND_TOKEN_TTL=30m なら 0 が期待値)
docker compose -f compose/compose.yaml -p alt logs alt-backend --since 30d \
  | grep -c alt.knowledge_loop.stream_jwt_expired

# 5. Grafana board の "SSE stream lifecycle" / "Stream JWT-expired ratio" を目視
#    過去 30 日でベースライン (started ≈ ended{ctx_done}, jwt_expired ≈ 0) から
#    逸脱があれば該当 service の 5 軸 checklist を再点検

# 6. inventory table の ⚠️ 列を 1 つでも ✅ に上げる作業を 1 PR で進める
```

audit 結果 (異常の有無 / 直した内容) は `docs/daily/<date>.md` に 1 行記録する。

## 参考資料

- [[000929]] Knowledge Loop SSE 配信路 4 層修復 — auth TTL / nginx / client lifecycle / tile dedup
- [[000874]] Knowledge Loop stream cadence env tunable 化
- PM-2026-045 (`docs/postmortems/PM-2026-045-knowledge-loop-sse-silent-failure-jwt-ttl-nginx-effect-race-tile-duplicate.md`) — root cause 4 層と action items #2〜#5
- `nginx/conf.d/default.conf:374-402` — Knowledge Loop SSE canonical location (copy template)
- `alt-backend/app/connect/v2/knowledge_loop/handler.go:345-413` — handler streamStaleTimeout / jwtCheckTicker / counter increment 配線
- `alt-frontend-sv/src/lib/hooks/useKnowledgeLoopStream.svelte.ts` — cursorPersistKey + intentional-abort guard reference impl
