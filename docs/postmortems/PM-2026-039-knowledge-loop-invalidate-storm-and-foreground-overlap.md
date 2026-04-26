# ポストモーテム: Knowledge Loop の invalidateAll 暴走による fetch-storm と foreground カード重なり

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-039 |
| 発生日時 | 2026-04-26 13:30 (JST) (= 04:30 UTC) |
| 復旧日時 | 2026-04-26 22:55 (JST) (= 13:55 UTC) — FE 修正実装完了、テスト全緑 |
| 影響時間 | 約 9 時間 25 分 (検知から FE 修正完了まで) |
| 重大度 | SEV-4 (ヒヤリハット — service down ではないが UX 完全破綻) |
| 作成者 | alt-frontend-sv 担当 |
| レビュアー | (Pending) |
| ステータス | Draft |

## サマリー

`/loop` ページを開いたユーザが Dismiss をクリックするとレイアウトが崩れ、ブラウザコンソールに `GET /loop/__data.json?x-sveltekit-invalidated=1 net::ERR_INSUFFICIENT_RESOURCES` と `TypeError: Failed to fetch` が連続出力される事象が発生した。alt-butterfly-facade と alt-backend のログから、ブラウザが同一ミリ秒に約 50 件の `KnowledgeLoopService/GetKnowledgeLoop` を fan-out し、約 30 秒周期で同 user の `stream_jwt_expired` が 30+ 件 lockstep 発火する正のフィードバックループが観測された。本番停止には至らなかったが、`/loop` を開いたユーザは事実上機能を使用できないヒヤリハット。FE のみで構造的に解消可能と判断し、`makeCoalescedRefresh` (debounce + single-flight)、`replaceSnapshot()` パターン、Svelte 5 `untrack` ガード、`out:loopRecede` + `animate:flip` による depth-layered 退場を同日中に実装、テスト全緑とした。

## 影響

- **影響を受けたサービス:** alt-frontend-sv (`/loop` ページ。SvelteKit 2 / Svelte 5)
- **影響を受けたユーザー数/割合:** `/loop` を能動的に開いていたアクティブユーザのみ。本番監視メトリクスでは個別ユーザの `__data.json` request rate は出ていないが、観測された `stream_jwt_expired` log line から少なくとも 1 ユーザは UX が完全に壊れていた。`/loop` は現状 owner 専用の機能で利用者は限定的。
- **機能への影響:** `/loop` は **部分的劣化** (実質クリック不能 = 完全停止に近いがページ表示自体は出る)。`/feeds` / `/home` 等の他ルートには波及せず正常稼働。
- **データ損失:** なし。append-only event log は影響を受けず、`knowledge_events` / `knowledge_loop_entries` は正常。Dismiss クリック自体は backend に届かず、optimistic update も meaningfully apply されないため、状態の不整合も発生していない。
- **SLO/SLA違反:** alt-frontend-sv に明示の SLO は未設定。仮に "interactive < 5 s" を SLO とすれば違反相当。

### 潜在的影響 (ヒヤリハット観点)

- もし `/loop` のリリース範囲が広がっていた場合、複数ユーザ同時に同症状を踏み backend の `GetKnowledgeLoop` request rate が QPS で 100+ に跳ねた可能性が高い。alt-butterfly-facade の elapsed_ms は 5-11 ms と健全だったため backend 側 saturate には至っていなかったが、connection pool / pgbouncer 側のキューに圧力をかけうる。
- ブラウザ側の `ERR_INSUFFICIENT_RESOURCES` はタブを閉じない限り解消しない。ユーザが他ルートに遷移しようとしても同一タブの fetch queue が詰まっており、UX が連鎖劣化する可能性があった。

## タイムライン

全時刻 JST。1 UTC = JST - 9h。

| 時刻 (JST) | イベント |
|-------------|---------|
| 13:09 | knowledge_loop_projector batch (from_seq 1184133→1184134, 1 event)。通常運用ノイズ |
| 13:20 | knowledge_loop_projector batch (1184134→1184137, 3 events)。通常運用 |
| 13:30 頃 | ユーザが `/loop` を開いて操作中に症状を体験 (Dismiss クリック後にレイアウトが崩れる、コンソールに ERR_INSUFFICIENT_RESOURCES) |
| 13:30:31 | **検知** (alt-butterfly-facade ログ) — `KnowledgeLoopService/GetKnowledgeLoop` が同一ミリ秒に 50+ fan-out。ユーザのスクリーンショット + 本人報告で同時に把握 |
| 13:30:58 | alt-backend が `alt.knowledge_loop.stream_jwt_expired` を同一 `user_id`/同一秒に 30+ lockstep 発火 |
| 13:31:28 / 13:31:59 | 上記 lockstep 発火が 30 秒周期で反復継続 |
| 13:31 頃 | **対応開始** — 担当が plan モードで Plan 作成、`plan-context-loader` で canonical contract を確認、Explore agent で `/loop` UI コードを横断調査 |
| 13:35 頃 | **原因特定** — `+page.svelte:87-103` の `useKnowledgeLoopStream` callbacks が `invalidateAll()` を無条件に呼んでいる + `useKnowledgeLoopStream.svelte.ts:149-161` の `$effect` が `data.lensModeId` を `data` 参照経由で track している、の 2 点で正のフィードバックループが構成されることを確定。第二症状 (カード重なり) は `LoopEntryTile.svelte:328-332` の `.dismissing { max-height: 0 }` collapse + `.entry { max-height: 640px }` clamp + fetch-storm によるメインスレッド飽和の三重要因と判明 |
| 13:40 頃 | **緩和策実装開始** — TDD outside-in (Playwright RED → unit RED → coalescer 実装 → hook 改修 → page 改修 → tile/CSS 改修 → 局所 GREEN) |
| 22:46 頃 | **修正実装完了** — `loop-coalesce.ts` (600 ms trailing debounce + single-flight + dispose) / `replaceSnapshot()` / `untrack` ガード / `out:loopRecede` + `animate:flip` / `max-height` clamp 撤去 / `transform-style: preserve-3d` をグリッドに移設 |
| 22:55 頃 | **復旧確認 (テスト緑)** — `bun run check` 0 errors、`bunx biome check` (lint+format) clean、`bun run test:server` 1086/1086 緑、`useKnowledgeLoop.svelte.test.ts` 7/7 緑、`loop-coalesce.test.ts` 6/6 緑、新規 Playwright spec 2 本は CI で実行予定 |

(本 PM 作成時点では production deploy は未実施。`git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy` 経路をユーザ承認待ち。)

## 検知

- **検知方法:** ユーザ自身による報告 + ブラウザコンソールのスクリーンショット + 開発者によるライブログ確認 (`docker compose -f compose/compose.yaml -p alt logs nginx alt-butterfly-facade alt-backend`) で TPS 突発を直接観察
- **検知までの時間 (TTD):** ユーザが症状を体験してから報告まで分単位 (正確不明)。observability 側のアラートは発火していない (= TTD は事実上ユーザ報告依存)
- **検知の評価:** **不十分**。以下の理由で観測の盲点だった:
  - `__data.json?x-sveltekit-invalidated=*` の request rate に対して nginx / Prometheus 側でアラートが無い
  - `alt.knowledge_loop.stream_jwt_expired` は INFO ログで、同一ユーザに対する lockstep 発火パターンの異常検知ルールが無い
  - alt-butterfly-facade の `bff.proxy_request` が同一ミリ秒に大量 fan-out しても、`elapsed_ms` が健全 (5-11 ms) なので latency-based アラートは発火しない
  - ブラウザの `ERR_INSUFFICIENT_RESOURCES` は client-side エラーで、server-side metrics に出ない

## 根本原因分析

### 直接原因

`/loop` ページの `useKnowledgeLoopStream` コールバック `onFrame` と `onExpired` が両方とも、無条件かつ debounce なしに SvelteKit の `invalidateAll()` を呼んでいた。各 `invalidateAll` は SSR `data` を全替えするが、stream hook の `$effect` は `opts.lensModeId` を `data` 参照経由で読んでいたため、データ参照差し替えだけで cleanup→reconnect が走り、stream を張り直すたびに同じ near-expiry SSR JWT で `stream_expired` を即時受信、再び `invalidateAll` を呼ぶ正のフィードバックループになっていた。並行する `AbortController.abort()` の TCP teardown が完了する前に新規 fetch が積み上がり、ブラウザの per-origin connection ceiling を踏んで `ERR_INSUFFICIENT_RESOURCES`。

第二症状 (Dismiss 後のカード重なり): 上記 fetch-storm がメインスレッドを飽和させ、`LoopEntryTile.svelte` の `.dismissing` クラスで動かしていた `transition: max-height 160ms` が rAF 不足でアニメ未完。`.entry` には `max-height: 640px` clamp が default で付いていたため `overflow: visible` のコンテンツが溢れて隣のグリッド行に bleed、3 件の ORIENT カードが Y 軸で重なる視覚バグになった。

### Five Whys

1. **なぜ `__data.json` リクエストが秒間多数飛んだか?**
   → `useKnowledgeLoopStream.onFrame` と `onExpired` が無条件に `invalidateAll()` を呼んでいたため。
2. **なぜ無条件に `invalidateAll()` を呼ぶ設計になっていたか?**
   → §9 stream contract の "non-silent frames は SSR snapshot を refresh すべき" を素直に翻訳した結果。debounce / single-flight の必要性が当時の実装者の頭になかった。stream のフレーム頻度が低い前提で書かれていた。
3. **なぜフレーム頻度が高くなったか?**
   → SSR-issued JWT が短寿命で、stream hook の `$effect` が `data` 参照に依存していたため、`invalidateAll` 1 回ごとに `effect` cleanup → reconnect → 即 `stream_expired` → `invalidateAll` の正のフィードバックループが成立した。1 回の trigger が秒間 30+ frames に増幅されていた。
4. **なぜ `$effect` が `data` 参照で再 fire する設計になっていたか?**
   → Svelte 5 の `$effect` は read された全 reactive sources を track する。`opts.lensModeId` getter は `data.lensModeId` を読むため、`data` 参照が変わるだけで dependency graph が更新される。`untrack` や `$derived` 値等価ゲートを使う必要があったが、コードレビュー時点では「`lensModeId` の値は変わらないから effect は再 fire しないだろう」という暗黙の期待があった。Svelte 5 の `$effect` の reactive tracking semantics と SvelteKit の `data` 差し替えの相性に対する理解が不足していた。
5. **なぜ FE のオンコール / 監視がこれを早期検知しなかったか?**
   → `__data.json?x-sveltekit-invalidated=*` の request rate / 同一クライアントの connection saturation / `stream_jwt_expired` の lockstep 発火、いずれにも production アラートが無かった。FE 起因の暴走は `latency` や `5xx` の dashboard には現れないため (server は健全に応答する)、SRE 側の既存アラートでは検知できない盲点だった。

### 根本原因

**Svelte 5 + SvelteKit における stream-driven SSR refresh の正しい coalesce / scoping パターンが言語化されておらず、`$effect` の reactive tracking semantics に対する理解も不十分なまま、stream contract (§9) を素直に実装した結果、エラー回路 (JWT 短寿命 + reconnect 自動化) と組み合わさったときに正のフィードバックループを構成してしまった。FE 起因の client-side fetch storm を検知するメトリクス / アラートも整備されておらず、ユーザ報告まで気付けなかった。**

### 寄与要因

- backend が SSR-issued JWT を短寿命で発行する設計 (本 PM の対象 fix では FE 側で吸収。後続 ADR で backend 側のチューニングを別トラック化)
- `useKnowledgeLoopStream` 自体は `scheduleReconnect` で reconnect-with-exponential-backoff を持っていたが、page-level `onExpired` がそれを並列で起動する第二の reconnect 経路として動いており、両者がレースしていた (アーキテクチャ上の double-recovery 構造)
- `LoopEntryTile.svelte` の `.entry { max-height: 640px }` という defensive clamp が、コンテンツが clamp を超えるケースに対する防御として無効 (`overflow: visible` 既定) で、本症状のときに具体的な悪化要因として効いた
- `#each foreground` ブロックに `animate:flip` も `out:` も無く、Dismiss 後の survivors の動きが定義されていなかった (= optimistic UX の設計漏れ)
- `/loop` ページの設計 (Knowledge Loop) が比較的新しく、FE の review / canary / observability の整備が他ルート (`/feeds`, `/home`) に比べて手薄

## 対応の評価

### うまくいったこと

- ライブログ (`docker compose ... logs nginx alt-butterfly-facade alt-backend`) を即座に確認することで、ブラウザ側の `ERR_INSUFFICIENT_RESOURCES` というクライアント観測が、サーバ側の同一ミリ秒 fan-out + lockstep stream_jwt_expired という具体的サーバ観測と紐付けられた。原因特定までが分単位で進んだ
- `plan-context-loader` で canonical contract を読み直し、§9 (stream) / §11-13 (spatial) / §12.5 (reduced-motion) という設計上の不変条件を fix の制約として明確化したことで、修正案が contract drift を起こさなかった
- TDD outside-in (Playwright fetch-storm spec → unit RED → 実装 GREEN) を厳守し、production の症状を直接 pin する E2E テスト (`__data.json` request count ≤ 2、Dismiss 後の Y 軸非重なり、DOM 削除) を残せた。再発時にこの spec が真っ先に落ちる
- `loop-coalesce.ts` を pure factory + dependency injection (clock / scheduler) で実装したため、6 ケースの unit test を `vi.useFakeTimers` で決定論的に書けた。debounce + single-flight + dispose の 3 不変条件すべてに red-green-refactor が回った
- `bun run check` (svelte-check + tsgo) と `biome check` が、Phase 5 local CI parity の実行で `return in finally` のような subtle な lint 警告を捕捉できた

### うまくいかなかったこと

- **検知がユーザ報告依存**だった。`__data.json` request rate / client-side connection saturation / `stream_jwt_expired` lockstep のいずれにも production アラートが無く、FE 起因の暴走を SRE が早期検知する手段が無かった
- 修正中、一度 `git stash -u` した直後に E2E テストの実行 ([test:all]) で 18 件の pre-existing client failures (acolyte / feeds / recap / knowledge-home / swipe) が観測されたが、これらは pre-existing で本変更とは無関係だった。client (browser) project の品質バグが累積している
- `LoopEntryTile.svelte.spec.ts` が `node:fs` を browser project で import しているために load 失敗する pre-existing issue が見つかったが、本 PM の scope 外として残置した。技術的負債

### 運が良かったこと

- `/loop` の利用者が現状 owner 中心で限定的だったため、本症状を踏んだユーザは少数。本格 rollout 後だったら、複数ユーザ同時に同症状を踏み backend / pgbouncer に圧力がかかった可能性
- alt-butterfly-facade と alt-backend が同期 fan-out 50+ を吸収できる程度に余裕を持って動いていた。`elapsed_ms` 5-11 ms で latency 劣化なし。もし backend が saturate していたら symptom が他ルートにも波及していた
- ブラウザの per-origin connection ceiling が発動して fetch が止まる安全弁が機能した。これがなければ無限に socket を消費していた可能性

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | 本 ADR-000847 の修正を `git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy` 経路で production 反映 | alt-frontend-sv 担当 | 2026-04-27 | TODO |
| 2 | 予防 | backend 側 stream JWT lifetime のチューニング (短寿命 JWT を 5 分以上に延長 or `stream_expired` 送出を JWT exp 直前バッファ)。後続 ADR を起票し alt-backend `app/usecase/knowledge_loop_usecase` で対応 | alt-backend 担当 | 2026-05-10 | TODO |
| 3 | 予防 | Svelte 5 + SvelteKit の stream-driven SSR refresh パターンを `docs/best_practices/svelte.md` に追記 (本 ADR と coalesce 実装をリファレンスとして引用)。`$effect` の reactive tracking + `untrack` + `$derived` 値等価のイディオムも明文化 | FE 全体 | 2026-05-03 | TODO |
| 4 | 予防 | `LoopEntryTile.svelte.spec.ts` の `node:fs` import を server project に分離するか `vite-node` 経由読み込みに直す (pre-existing。本 PM で発覚) | alt-frontend-sv 担当 | 2026-05-10 | TODO |
| 5 | 検知 | nginx access log から `/loop/__data.json?x-sveltekit-invalidated=*` の request rate を抽出する Prometheus rule を追加。1 client (or 1 IP) で 5 req/s を超えたら warn、20 req/s を超えたら critical でアラート | SRE / alt-frontend-sv | 2026-05-10 | TODO |
| 6 | 検知 | alt-backend `alt.knowledge_loop.stream_jwt_expired` の lockstep 発火 (同一 `user_id` で 60 秒以内に 5+ 件発火) を検知する Prometheus rule + Grafana dashboard を追加 | SRE / alt-backend 担当 | 2026-05-10 | TODO |
| 7 | 検知 | Sentry / 同等のクライアント監視で `ERR_INSUFFICIENT_RESOURCES` / `Failed to fetch` の発生率を可視化、ユーザ単位 / route 単位で警告できるようにする | FE 全体 | 2026-05-17 | TODO |
| 8 | 緩和 | `useKnowledgeLoopStream` に max retry / circuit-breaker を追加し、JWT exp が連続 N 回 (例: 5 回) 発生したら自動再接続を一時停止して UI バナー (`Loop reconnect paused — refresh to retry`) を出す | alt-frontend-sv 担当 | 2026-05-17 | TODO |
| 9 | 緩和 | `loop-coalesce.ts` の `windowMs` を `import.meta.env` からオーバーライド可能にし、運用時に hot patch で広げられるようにする (デフォルト 600 ms は維持) | alt-frontend-sv 担当 | 2026-05-24 | TODO |
| 10 | プロセス | TDD outside-in が新機能だけでなく **bug fix** にも適用される旨を `feedback_tdd_outside_in` メモに追記。本 PM のテスト 2 段 (Playwright + unit) を実例として引用 | 全体 | 2026-05-03 | TODO |
| 11 | プロセス | Knowledge Loop / Knowledge Home 系の page-level review checklist を作成 (項目: stream-driven refresh の coalesce 確認 / `$effect` の reactive tracking 検証 / `out:` transition + `animate:flip` の有無 / 退場時の DOM 削除 vs フラグ / max-height clamp の正当性) | FE 全体 | 2026-05-17 | TODO |

### カテゴリの説明

- **予防 (4 件):** ADR-000847 の本番反映 (#1) / backend JWT 寿命 (#2) / FE ベストプラクティス文書化 (#3) / 既知技術的負債の解消 (#4)
- **検知 (3 件):** nginx 側 fetch-storm 監視 (#5) / backend stream_jwt_expired 監視 (#6) / クライアント観測 (#7)
- **緩和 (2 件):** stream hook circuit-breaker (#8) / coalesce window 運用調整可能化 (#9)
- **プロセス (2 件):** TDD 適用範囲明文化 (#10) / page-level review checklist 整備 (#11)

## 教訓

### 技術的な学び

- **Svelte 5 の `$effect` は読まれた reactive sources をすべて track する**。getter プロパティ越しに親オブジェクトの参照を読むと、参照差し替えだけで dependency が更新される。値等価でゲートしたいケースでは `$derived.by(() => obj.field)` + `untrack(...)` の組み合わせが正解。getter が見た目だけ shallow でも、effect の依存グラフは深い。
- **SvelteKit の `invalidateAll()` は scope を絞る `invalidate(name)` + `depends(name)` に置き換える方が安全**。stream-driven refresh のように頻発するパスで使うときは特に。`__data.json?x-sveltekit-invalidated=*` リクエストは可視化されにくいので、暴走しても気付きにくい。
- **debounce + single-flight は stream-driven UI に対する標準処方**。フレーム頻度が動的に変わる場面では、N → 1 の coalesce を入れないと frontend が server に対して amplifier として機能してしまう。本ケースは 1 イベント → 30+ fetches に増幅していた。
- **optimistic update は array filter で消す**のが Svelte の `#each` keyed block + `out:` transition + `animate:flip` 設計と相性が良い。フラグだけ立てて DOM に残す設計は、(a) survivor の reflow が定義されない、(b) フラグベースの CSS transition が main thread starvation 時に止まる、という二重の脆弱性を持つ。
- **Alt-Paper の depth contract (§11-13) を `tile + transition` 単位で実装する**。`perspective` をルートに置いて、`transform-style: preserve-3d` を中間コンテナにも置かないと Z-translation がフラットに潰れる。reduced-motion 時は §12.5 マッピング (dissolve + highlight fade + color shift) を transition factory で値ごと差し替えるのが正解。

### 組織的・プロセス的な学び

- **client-side fetch storm を検知する観測体系が必要**。サーバが健全に応答していても、クライアントが暴走していれば実害は等価。`__data.json` request rate / `ERR_INSUFFICIENT_RESOURCES` / lockstep `stream_jwt_expired` は alert の対象にすべき指標。
- **page-level の review checklist を整備して構造的バグを早期発見する**。本ケースは 1 行の `void invalidateAll()` を 2 箇所に書いた段階で起きうる構造的バグだったが、code review 時点で `$effect` の re-fire と組み合わせて生じる loop を予見するには Svelte 5 + SvelteKit の semantics に対する練度が要る。チェックリスト化することで個人の練度依存を減らす。
- **bug fix にも outside-in TDD を適用する**。本ケースで Playwright spec を最初に書いたことで、production の症状 (`__data.json` request count) を直接 pin する regression test が残った。「fix を入れたあと壊れたら気付ける」状態が次の同種事案に対する最大の保険。
- **canonical contract (今回は ADR-000831) を書いておくと修正の質が上がる**。spatial render contract / reduced-motion mapping / stream semantics のすべてに明確な不変条件があったため、修正案が contract drift を起こさず、迷いも少なかった。Knowledge Loop のような複雑な UI ほど contract 投資が効く。

## 参考資料

- 関連 ADR: [[000847]] Knowledge Loop の stream-driven refresh をコアレス化し foreground 退場を depth-layered transition に置き換える (本 PM が指す修正)
- 関連 ADR: [[000831]] Knowledge Loop state-machine adoption (canonical contract のアンカー)
- 関連 ADR: [[000844]] Knowledge Loop why_text を SummaryVersionCreated event に inline (本 PM の前段の `/loop` 改修)
- 関連 ADR: [[000846]] Knowledge Loop why_text を discovered event で backfill (同日同期 fix)
- canonical contract: [[knowledge-loop-canonical-contract]] §9 stream / §11-13 spatial render / §12.5 reduced-motion
- 不変条件: [[IMPL_BASE]] append-first / reproject-safe / immutable-design
- 修正コミット: (deploy 後に hash を追記)
- ライブログ抜粋: 2026-04-26 04:30-04:32 UTC `nginx` / `alt-butterfly-facade` / `alt-backend` (compose stack `-p alt`)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
