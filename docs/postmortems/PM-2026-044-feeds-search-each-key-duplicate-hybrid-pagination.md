# PM-2026-044: Archive Desk /feeds/search の Svelte each_key_duplicate クラッシュと「20 件しか表示されない」現象（hybrid pagination boundary）

## メタデータ

| 項目 | 値 |
|------|-----|
| インシデントID | PM-2026-044 |
| 重大度 | SEV-3（Archive Desk `/feeds/search` の infinite scroll が前画面サイズ 20 件で停滞、ブラウザコンソールに Uncaught Error。ユーザー操作で再現するが、データ損失・他機能影響なし。Reference Desk `/search`、フィード一覧、Knowledge Home は無影響） |
| 発生日時 | 2026-05-24 23:45 (JST) — ユーザーが `/feeds/search?q=Rust` で連続 load-more を試した最初の確認時刻 |
| 検知日時 | 2026-05-24 23:55 (JST) 頃 — ユーザーがブラウザコンソールの stack trace と「20 件しかロードされない」現象をチャットで報告 |
| 復旧日時 | 2026-05-25 00:13 (JST) — fix commit `951fda48e` を main にマージ（push 待ち、ローカル動作確認済） |
| 影響期間 | 約 4 週間（2026-04-23 [[000830]] で hybrid 既定 ON 化 〜 2026-05-25 00:13 修正コミットまで）— 実際にユーザーが触ったときに発火する潜伏期間 |
| 影響サービス | alt-frontend-sv（`/feeds/search` Archive Desk のみ） |
| 影響機能 | Archive Desk の infinite scroll による複数ページ取得。1 ページ目（20 件）の表示は無影響 |
| 関連 ADR | [[000915]] グローバル検索の cold-start fix（hybrid を全クエリ既定 ON 維持） / [[000916]] tag prefix functional B-tree |
| 関連 commit | `951fda48e` fix(feeds-search): dedupe hybrid-paginated results / `e1157d143` feat(alt-frontend-sv): redesign Search with Alt-Paper Archive Desk and Reference Desk format |
| 作成者 | オンコール担当者 |
| ステータス | Approved |

## サマリー

2026-05-24 深夜、ユーザーが Archive Desk `/feeds/search?q=Rust` で連続的に load-more（infinite scroll）を試したところ、ブラウザコンソールに `Uncaught Error: https://svelte.dev/e/each_key_duplicate` が出力され、表示件数が 20 件で停滞して新規アイテムが追加されない現象が発生した。直接原因は Meilisearch hybrid search（`semanticRatio=0.7`）の offset pagination が **page boundary で同 article_id を返す** こと。BM25 と vector 類似度を融合した score がページ間で安定せず、境界付近のアイテムが offset=N と offset=N+limit の両方に出現するため、frontend が `feeds = [...feeds, ...newFeeds]` で concat した瞬間に keyed each `(feed.id)` の制約に違反して Svelte が flush を中断、視覚的に「20 件で固まる」症状として顕在化した。データ損失・他機能影響なし。直接 Meilisearch に offset=20 / 40 で同 query を投げると 2 件の article_id が両ページに出現することを確認、frontend 側に `appendUniqueById` を導入する application-layer dedupe で復旧した。

## 影響

- **影響を受けたサービス:** alt-frontend-sv のみ（`/feeds/search` Archive Desk ページに限定）
- **影響を受けたユーザー数/割合:** Archive Desk で同一クエリの infinite scroll を 2 ページ以上スクロールしたユーザーのみ。Reference Desk `/search` は cursor pagination を持たないため影響なし。実数は browser console error monitoring 不在のため未取得（教訓セクション参照）
- **機能への影響:** 部分的劣化 — 1 ページ目（20 件）は正常表示、2 ページ目以降の追加が失敗
- **データ損失:** なし（API 経路は正常、frontend rendering のみクラッシュ）
- **SLO/SLA違反:** 直接 SLO は定義していないが、infinite scroll の UX 期待値（連続スクロールで結果が増えていく）に違反

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-23 | [[000830]] で `OLLAMA_MAX_LOADED_MODELS=2` に変更、Meilisearch hybrid embedder が常駐可能になり semanticRatio=0.7 の本格運用開始（潜伏期間の起点） |
| 2026-05-24 14:42〜14:46 (UTC) | ユーザーが `/feeds/search?q=Rust` を含む複数クエリで連続検索。search-indexer log に `count:20` の SearchArticles が同 query_hash で 6 回連続発生 |
| 2026-05-24 23:55 頃 | **検知** — ユーザーがブラウザコンソールの `each_key_duplicate` stack trace と「20 件しかロードされない」現象をチャットで報告 |
| 2026-05-24 23:56 | **対応開始** — 検索画面の keyed each block 候補を grep で列挙、`ArticleSearchSection` `RecapSearchSection` `TagSearchSection` `feeds/search/+page.svelte` の 4 箇所を特定 |
| 2026-05-24 23:58 | Reference Desk 3 sections は DB GROUP BY / Meilisearch dedup により応答内ユニーク性を確認、Archive Desk `/feeds/search` の concat パターンを疑う |
| 2026-05-25 00:02 | **原因特定** — Meilisearch articles index に直接 `q=Rust user_id=... hybrid={qwen3, 0.7} offset=20/40 limit=5` を投げ、offset=20 と offset=40 で 2 件の同 `id` を確認 |
| 2026-05-25 00:08 | **緩和策適用** — `$lib/domain/feed/dedupe.ts` に `appendUniqueById` を抽出、TDD で 4 ケース GREEN 確認 |
| 2026-05-25 00:11 | `/feeds/search/+page.svelte` の `handleSearch` / `loadMore` で helper を採用、tsgo check + scoped biome lint 緑 |
| 2026-05-25 00:13 | **復旧** — commit `951fda48e` を main にマージ。ローカル `bun run test` で 16/16 green、`bun run check` で 0 errors |

## 検知

- **検知方法:** ユーザー報告（チャット経由でブラウザコンソールの stack trace を共有）
- **検知までの時間 (TTD):** 最初の発火から ~10 分（ユーザーが体感した瞬間に即報告）
- **検知の評価:** 不十分。frontend 側に Sentry や類似のエラートラッキングがなく、ユーザーが報告しなければ気付けない構造になっている。`each_key_duplicate` は Svelte が console.error に出すだけで HTTP エラーや server log には残らないため、observability gap が存在する

## 根本原因分析

### 直接原因

Meilisearch hybrid search（`semanticRatio > 0`）の offset pagination が、page boundary 付近で同一 document id を複数ページに返す。`/feeds/search/+page.svelte` の `loadMore` は `feeds = [...feeds, ...newFeeds]` で結果を concat し、keyed each `{#each feeds as feed, i (feed.id)}` で描画する。同 id が `feeds` 配列に複数現れた時点で Svelte が `each_key_duplicate` を throw し、reactive flush を中断する。

実測（2026-05-25 00:02 JST、`articles` index、`q=Rust`、user filter、`hybrid={qwen3, 0.7}`、`limit=5`）— article id は便宜上 A〜H に匿名化:

```
offset=20: [A, B, C, D, E]
offset=40: [F, D ←重複, G, E ←重複, H]
```

重複した 2 件（D, E）はいずれも offset=20 の末尾近傍にあり、score 融合の微小揺らぎで page 40 側にスライドしたパターン。

### Five Whys

1. **なぜ each_key_duplicate が発火したか？** → `feeds` 配列内に同一 `feed.id` を持つ要素が 2 つ存在し、Svelte 5 のキー一意性制約に違反した
2. **なぜ同 `feed.id` が 2 つ存在したか？** → `loadMore` が返した次ページに前ページと同じ article_id が含まれていたが、frontend は dedupe せず単純 concat で配列に積んだ
3. **なぜ backend が重複 id を返したか？** → Meilisearch hybrid search の offset pagination は disjoint な ID セットを保証しない。BM25 と vector 類似度の score 融合（fusion）は確定的でなく、boundary 付近のアイテムが score の微小変動でページ間を移動する
4. **なぜ hybrid pagination が不安定なことを誰も知らなかったか？** → [[000915]] で hybrid を全クエリで既定 ON にした際の検証は「cold-start 解消」「cache hit rate」「searchCutoffMs 到達率」「article section latency」に集中しており、**pagination stability は検証項目に含まれていなかった**。Meilisearch 公式 docs にも「hybrid scores are not guaranteed stable across paginated requests」という強い警告がなく、運用側も気付かなかった
5. **なぜ frontend が defensive dedupe を持っていなかったか？** → Archive Desk redesign（commit `e1157d143`、2026-04-09 頃）で keyed each `(feed.id)` を導入した際、cursor pagination の backend が「ページ間 disjoint」前提で設計されていた（事実、純 BM25 時代は disjoint だった）。[[000915]] の hybrid 既定 ON 化でこの前提が崩れたが、frontend 側の不変条件レビューはされなかった

### 根本原因

**Backend の pagination contract が「ページ間 disjoint」を暗黙の前提にしていたが、hybrid search の採用でこの contract が静かに破られ、frontend 側の keyed-each 描画が前提依存だったため UI クラッシュとして顕在化した。** 単一原因ではなく、「(a) hybrid 採用で contract が破れた」「(b) backend は contract 違反を検出しない」「(c) frontend は contract に依存していた」の 3 段重なり。

### 寄与要因

- **Browser console error monitoring の欠如**: `each_key_duplicate` は HTTP 4xx/5xx を返さないため、server-side log / metric では検出不可能。ユーザー報告がなければ無限に放置される構造
- **Pagination の e2e test 不在**: `/feeds/search` の Playwright スイートに「2 ページ以上スクロールして要素が単調増加すること」を assert するシナリオがなかった
- **Hybrid 既定 ON の影響範囲レビュー不足**: [[000915]] では Reference Desk `/search` を主スコープにしており、Archive Desk `/feeds/search` で同じ search-indexer driver を使う影響が深掘りされなかった
- **Meilisearch 公式 docs の警告弱**: hybrid pagination stability については API リファレンスに小さく書かれているのみ、operations guide に明示されていない

## 対応の評価

### うまくいったこと

- ユーザーが stack trace 付きで即報告 → 検知から原因特定まで ~15 分
- Meilisearch に直接 `curl` で offset 違いの response を取得して boundary 重複を **2 リクエストで決定的に再現**（推測ではなく実測）
- 修正を `appendUniqueById` という pure helper に切り出し、Svelte component 非依存の 4 unit case で TDD covered
- 修正範囲が application-layer のみ。backend / migration / contract いずれも触らず、deploy リスクが alt-frontend-sv の rebuild に限定
- 同種パターン（`_,..., feeds, ...newFeeds]` + keyed each）は他の検索画面に存在しないことを grep で確認済

### うまくいかなかったこと

- frontend エラートラッキング不在のため、検知がユーザー報告依存
- [[000915]] の hybrid 既定 ON 化レビューで Archive Desk 側影響を見落とした（Reference Desk のみ重点的に検証）
- pagination の e2e 不在（Playwright で連続スクロールの単調増加を assert していない）
- Svelte 5 の keyed each 制約は厳格化されたが、既存 keyed each を audit する作業が hybrid 採用時に走っていなかった

### 運が良かったこと

- `each_key_duplicate` は Svelte が flush を中断するだけで、ページ全体の white screen にはならない（前回サイズで停滞）。完全クラッシュなら影響度は SEV-2 相当だった
- 影響範囲が `/feeds/search` の 2 ページ目以降に限定。1 ページ目（最初の 20 件）は正常表示で、典型的なユーザー導線では気付かない人も多かった可能性
- ユーザーが「20 件しか」という UX 観測と stack trace の両方を渡してくれたため、症状と原因の対応付けが即できた（片方だけだと診断が長引いた可能性）

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | `appendUniqueById` を `/feeds/search/+page.svelte` の `handleSearch` / `loadMore` に導入（commit `951fda48e`） | オンコール | 2026-05-25 | **DONE** |
| 2 | 予防 | 他の `[...prev, ...next]` + keyed each パターンを FE 全体で audit、必要なら同 helper を適用 | フロントエンド担当 | 2026-06-08 | TODO |
| 3 | 予防 | Backend `SearchByUserIDWithPagination` で hybrid 時に deterministic tiebreaker（`id` 等）を追加できないか検討。Meilisearch SDK の `sort` 制約と relevance ranking の trade-off を ADR 化 | search-indexer 担当 | 2026-06-15 | TODO |
| 4 | 検知 | Svelte hydration error をブラウザから収集する error tracking（Sentry 等）の導入。最低限 `each_key_duplicate` / `each_key_invalid` を console.error 経由でフックする stop-gap も検討 | observability 担当 | 2026-07-01 | TODO |
| 5 | 検知 | `/feeds/search` の Playwright e2e に「2 ページ以上スクロールしたとき表示件数が単調増加する」シナリオを追加。Meilisearch の hybrid 応答をモックして boundary 重複を意図的に注入する負方向ケースも併設 | QA 担当 | 2026-06-22 | TODO |
| 6 | 緩和 | `appendUniqueById` の重複検出時に observability log を 1 行出し、頻度を可視化できるようにする（現状は黙って捨てている） | フロントエンド担当 | 2026-06-15 | TODO |
| 7 | プロセス | search 系 ADR の Decision テンプレートに「pagination stability への影響」セクションを追加。今後の hybrid / ranking 変更 ADR で必須項目化 | ADR レビュアー | 2026-06-08 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

### 技術的教訓

- **Hybrid search の pagination は disjoint を保証しない**。BM25 と vector の score 融合は確定的でなく、`offset+limit` で同 doc を再返却しうる。Meilisearch に限らず Elasticsearch の `rrf` rank fusion も同様の性質を持つ — 一般則として「ranking が動的に決まる検索 backend で offset pagination を使う場合、frontend dedupe は必須」
- **Keyed `{#each}` は backend contract に依存する**。Svelte 5 の keyed each は重複 key で flush を中断する厳格設計。「id がユニークである」前提は backend contract として明示し、変更時に必ず影響範囲を audit するルールにする
- **App-layer dedupe は cheap で robust**。`appendUniqueById` の pure 関数 1 つで infinite scroll 全般のロバスト性が上がる。backend 側の deterministic ordering 改修は ADR 化が必要なほど invasive で、まずは frontend で塞ぐ判断は正しかった

### 組織的教訓

- **ADR の影響範囲レビューでは「同じ driver を使う別画面」を必ず洗う**。[[000915]] は Reference Desk / search-indexer SearchArticles を主スコープにしていたが、Archive Desk `/feeds/search` も同じ driver path を通る。「主スコープに含まれない依存先」をチェックリスト化していれば事前に発見できた
- **Frontend エラーの観測性は backend に比べて致命的に低い**。`otel_traces` には HTTP リクエストが残るが、browser の console.error は誰も見ない。Sentry 等の導入は SEV-2 への昇格を防ぐ投資として優先度を上げるべき
- **ユーザー報告の質はインシデント対応速度を決める**。stack trace + UX 症状の両方を一度に共有してもらえると、検知→原因特定が分単位で完了する。バグ報告テンプレート（再現手順 + console error + URL）の整備が次回の TTR を更に縮める

## 参考資料

- 関連 ADR: [[000915]] グローバル検索の cold-start fix（hybrid を全クエリ既定 ON 維持）
- 関連 ADR: [[000916]] tag prefix functional B-tree（同セッションの別 fix）
- 関連 ADR: [[000830]] news-creator-backend OLLAMA_MAX_LOADED_MODELS=2（hybrid 採用の前提）
- 関連 commit: `951fda48e` fix(feeds-search): dedupe hybrid-paginated results to stop each_key_duplicate
- 関連 commit: `e1157d143` feat(alt-frontend-sv): redesign Search with Alt-Paper Archive Desk and Reference Desk format（keyed each 導入元）
- Svelte 5 公式: `https://svelte.dev/e/each_key_duplicate`
- 再現コマンド: `docker compose -p alt exec meilisearch curl -H "Authorization: Bearer $KEY" -X POST http://localhost:7700/indexes/articles/search -d '{"q":"Rust","limit":5,"offset":20,"hybrid":{"embedder":"qwen3","semanticRatio":0.7},"filter":"user_id = \"<uuid>\""}'` を offset=20 と offset=40 の 2 回叩くと boundary 重複が観測される

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
