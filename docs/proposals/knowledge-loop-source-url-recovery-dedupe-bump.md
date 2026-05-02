---
title: Knowledge Loop の act_targets[].source_url を全レガシー entry に充填するため article-url-backfill dedupe namespace を bump して再 emit する
date: 2026-05-02
status: proposed
tags:
  - knowledge-loop
  - knowledge-home
  - projector
  - migration
  - bugfix
aliases:
  - knowledge-loop-source-url-recovery-dedupe-bump
---

# Knowledge Loop ActTarget.source_url 充填のための dedupe namespace bump 提案

## Status

**Proposed** — 本 PR を accept する前にデータ確認 + 運用手順レビューが必要。

## 1. Context

### 1.1 既知の不整合

[[000879]] (ADR-000879) で `act_targets[].source_url` が proto / projector / FE に追加された。`knowledge_url_backfill_usecase` が emit する既存 corrective event `ArticleUrlBackfilled` を Loop projector でも消費するブランチを後追いで実装し (commit `fc0079387`) production に rolling deploy 済み。

ADR 直後の URL backfill 発火 (2026-05-02 03:00:26Z) で:

```
articles_scanned:    29,213
events_appended:     12,091   ← projection に届いた
skipped_duplicate:   17,122   ← article-url-backfill:<id> dedupe 既存で skip
```

`skipped_duplicate` は **過去に knowledge_url_backfill が走った時に dedupe registry に登録された article**。これらに対して `ArticleUrlBackfilled` event は **再 append されない**。Loop projector は payload から URL を読むため、event が来ない以上 source_url を patch できない。

### 1.2 影響を受けるエントリ

DB 観測 (2026-05-02 03:10Z):

```
with_source_url:     ~31,000  (後続の projector 消費で増加中)
without_source_url:  ~22,000+ (この提案のターゲット)
total:               52,725
```

未復旧 ~22,000 entries は今回の backfill では永遠に source_url を貰えない。FE 側で Open ボタンが disable されたまま (commit `fdacb722c` で見た目も改善済み)。

### 1.3 なぜ単純な dedupe テーブル truncate ではダメか

`knowledge_loop_transition_dedupes` 等の dedupe registry は **ingest-only barrier** であり projection ではない (canonical contract §3.8 / [[knowledge-loop-canonical-contract]] §dedupe ≠ projection)。reproject で touch しないのが不変条件。テーブル直 truncate は同条件下で article-created 等の他系統 event も dedupe を失い再 emit を許してしまうため副作用が大きい。

→ **dedupe key namespace を bump** することで「対象 namespace のみ全 article 再 emit」を局所化する。

## 2. Decision (proposed)

### 2.1 採用方針

`DedupeKeyArticleUrlBackfill` の format を `article-url-backfill:%s` から **`article-url-backfill-v2:%s`** に bump する。

```diff
- DedupeKeyArticleUrlBackfill = "article-url-backfill:%s"
+ DedupeKeyArticleUrlBackfill = "article-url-backfill-v2:%s"
```

deploy 後、admin endpoint `EmitArticleUrlBackfill` を再発火すると:

- 旧 namespace `article-url-backfill:<id>` の dedupe は touch されない (keep as-is)
- 新 namespace `article-url-backfill-v2:<id>` で全 article 再 emit が走る (29,213 events)
- Loop projector は payload-only から URL を読み、`act_targets[0].source_url` が空の row に patch (`NOT (act_targets->0 ? 'source_url')` 既存 guard)

### 2.2 なぜ projector でなく producer 側で bump するか

不変条件 `Reproject-safe`:
- projector は payload と stable resource のみから projection を再構築できる
- 同じ event seq の再投影で同じ結果になる必要

projector 内で dedupe を意識した分岐を入れる (e.g. 「ArticleUrlBackfilled は既存 source_url があればスキップ」) のは現コードで既に実装済 (`NOT (act_targets->0 ? 'source_url')`)。projector は idempotent なので **何度同じ event が流れても安全**。

問題は producer 側で「同じ article の corrective event を再生成するか」。producer は dedupe registry を参照するため、dedupe key を変えない限り再 emit されない。**bump は producer の問題、projector はそのまま**。

### 2.3 検討した代替案

#### (却下 1) Loop 専用の新 event type を追加

新規 `LoopActTargetUrlBackfilled` を定義し別 dedupe namespace で emit。

- 利点: home 側の dedupe を完全に touch しない
- 欠点: home projector と Loop projector で event 種別が増え、長期的に保守コストが上がる。事実上 `ArticleUrlBackfilled` と意味が同じ
- 却下理由: 既存 `ArticleUrlBackfilled` は home / loop 両方への corrective として既に semantics を持っており、両 projector が同じ event を消費する設計が clean

#### (却下 2) dedupe registry の該当行を物理削除

`DELETE FROM knowledge_event_dedupes WHERE dedupe_key LIKE 'article-url-backfill:%'`

- 利点: namespace 変更不要
- 欠点: ingest barrier を直接 mutate する → 「dedupe ≠ projection」不変条件と "dedupe is ingest-only" 規約に正面から違反
- 却下理由: 例外処理を運用ルールとして許容すると invariant が骨抜きになる

#### (却下 3) projection に対する SQL 直 UPDATE

cross-DB join で alt-db `articles.url` を sovereign-db に取り込み JSONB patch を直接書き込む。

- 利点: 速い (秒単位)
- 欠点: `Disposable projection` 違反 (write path / tracking 以外から projection を直 mutate)。reproject で SQL UPDATE の結果を再現できないため audit/replay が壊れる
- 却下理由: 一度許すと将来の corrective が SQL 直書きに退化する。既に [[feedback_no_sql_logic]] で禁則化されている

#### (却下 4) FE 側で `[id]` から URL を逆引き

reader 側で `?url=` 不在時に article_id → URL を BFF lookup。

- 利点: backfill 不要
- 欠点: proto `ArticleWithTags` に URL field 追加 + BFF endpoint 追加 + FE wiring。blast radius が同等以上で、しかも projection 復旧の根本問題は残る (Loop entry の `source_url` が空のまま FE が hack で補う)
- 却下理由: ADR-000879 で却下済の "reader-side lookup" と同じ案。再採用しない

## 3. 実装計画 (TDD outside-in)

### Phase 0 — E2E 観点

新 spec は不要。既存 `tests/e2e/desktop/loop/act-open-loads-article.spec.ts` が source_url 経路で content fetch を assert しており、bump 後 backfill 経由でも同じ assertion が満たされる。production 確認は admin UI 操作 + DB クエリ (本 ADR §4.4)。

### Phase 1 — CDC

Pact 影響無し。proto 不変、wire shape 不変、dedupe key は alt-backend の **internal 状態**で外部契約に出ない。

### Phase 2 — Unit RED

`alt-backend/app/usecase/knowledge_url_backfill_usecase/usecase_test.go` に 1 ケース追加:

```go
func TestEmit_UsesV2DedupeNamespace(t *testing.T) {
    // The corrective backfill MUST use the bumped dedupe namespace so
    // articles whose v1 corrective is already in the registry get a
    // fresh chance to be re-emitted to the Loop projector.
    require.Equal(t, "article-url-backfill-v2:%s",
        domain.DedupeKeyArticleUrlBackfill)
}
```

`alt-backend/app/domain/knowledge_event_test.go` (新設 or 拡張) で同 const の値を pin。

### Phase 3 — GREEN

`alt-backend/app/domain/knowledge_event.go:80` の 1 文字 (`-v2`) 追加のみ。doc を `v2 — re-emit pass after Loop projector ADR-000879 added source_url consumption (2026-05-02)` に更新。

```go
// DedupeKeyArticleUrlBackfill — v2 since 2026-05-02. v1 was emitted
// before Loop projector consumed ArticleUrlBackfilled; bumping forces
// re-emission so legacy entries pick up source_url. See:
// docs/proposals/knowledge-loop-source-url-recovery-dedupe-bump.md
DedupeKeyArticleUrlBackfill = "article-url-backfill-v2:%s"
```

### Phase 4 — REFACTOR

無し。1 文字変更。

### Phase 5 — CI parity

```bash
cd alt-backend/app && gofmt -l . && go vet ./... && go test ./... -race
```

## 4. Operational Plan

### 4.1 Pre-deploy verification

```sql
-- baseline: 復旧前の without_source_url 数を控える
SELECT COUNT(*) FROM knowledge_loop_entries
WHERE act_targets IS NOT NULL
  AND act_targets->0->>'target_type' = 'article'
  AND NOT (act_targets->0 ? 'source_url');
```

### 4.2 Deploy

通常の `git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy` rolling deploy。alt-backend のみ更新が反映される。

### 4.3 Backfill emit

admin UI `/admin/knowledge-home` の **"Emit ArticleUrlBackfilled"** ボタン (`data-testid=emit-article-url-backfill-button`) を押す。

期待ログ (alt-backend):

```
"emitted article url backfill"
articles_scanned:    29,213
events_appended:     29,213   ← v2 namespace は dedupe 全空、全 article emit
skipped_duplicate:        0
more_remaining:        true   ← page-size 200 で打ち切り、再 click で続き
```

完走まで `more_remaining: false` になるまで再 click。29,213 / 200 ≈ 147 page なので **30 回程度**の再 click を想定。

#### 4.3.1 既存 502 timeout の扱い

[[knowledge-loop-canonical-contract]] / 本提案範囲外。FE が 30 秒で 502 を返す既知の表示バグ (alt-backend は完走している) は別 issue で対応。今回の運用では nginx access log の `POST /api/admin/knowledge-home … 502` を「**実は成功**」として読むこと。

### 4.4 Post-deploy verification

projector が消費し終えた後 (1 batch = 100 events / 5 秒 cadence、29,213 events ≈ 25 分):

```sql
-- recovery rate
SELECT 
  COUNT(*) FILTER (WHERE act_targets::text LIKE '%source_url%') AS with_source_url,
  COUNT(*) FILTER (WHERE act_targets IS NOT NULL AND act_targets::text NOT LIKE '%source_url%') AS without_source_url,
  COUNT(*) AS total
FROM knowledge_loop_entries;
```

target: `without_source_url` が 0 (or 極小、article が deleted_at 設定済み等の例外のみ)。

### 4.5 Rollback

dedupe namespace の bump は forward-only。rollback したい場合:

1. 直接 revert は意味が無い (v2 dedupe は既に登録済)
2. もし v2 emission に問題があった場合は **v3 を bump** すれば良い (`article-url-backfill-v3:%s`)
3. projection 自体の rollback は `runbooks/knowledge-loop-reproject.md` 経由

## 5. Consequences

### Pros

- 全 legacy entries の source_url 復旧が **append-first** で完結
- producer 側 1 文字の change で対応、projector は無変更
- ADR-000879 の immutable design 不変条件すべて維持
- `NOT (act_targets->0 ? 'source_url')` guard により既に source_url がある entry は no-op (idempotent)

### Cons / Tradeoffs

- 旧 v1 dedupe 行は登録されたまま (clean-up 不要だが累積)。**dedupe テーブルが将来肥大化する可能性**。長期的には dedupe TTL 機構が望ましい
- backfill emit は同期 endpoint なので 502 timeout 表示が継続発生 (UX の混乱要因。別 issue)
- ~29,000 events を全 projector が再消費するため projector 負荷が一時的に増える (5 分 ~ 30 分)。staging で計測してから本番

### Cons not addressed (out of scope)

- backfill emit の async 化 (FE 502 表示問題)
- dedupe registry TTL / GC 機構

## 6. Decision Criteria

本提案を accept する条件:

1. `articles` テーブルの URL 充填率を確認 (もし URL 空の article が多ければ backfill 効果が薄い)
2. staging で v2 emit + projector 消費を一回通す (timing / DB 負荷計測)
3. 上記運用ログのテンプレ化を `docs/runbooks/` に追加
4. 1 文字変更の PR をレビュー
5. 本番 emit 後の `without_source_url` 数を再計測し効果を検証

すべて満たした上で accept → status を `accepted` に切り替えて新 ADR 番号 (000880) を採番。

## 7. Related ADRs

- [[000879]] Knowledge Loop ActTarget に source_url を追加
- [[000867]] safeArticleHref defense-in-depth chain (ArticleUrlBackfilled corrective event の origin)
- [[000865]] wire schema "url" key normalization (legacy "link" fallback)
- [[knowledge-loop-canonical-contract]] §dedupe ≠ projection / §reproject-safe / §disposable projection

## 8. Open questions

- **Q1**: `articles.url` 空の article は何件？それらは v2 emit でも結局 `events_appended` に入らず `skipped_blocked_scheme` 扱いとなる。実数を計測してから「+α は手動 patch」「諦める」のどちらかを決める。
- **Q2**: dedupe registry TTL/GC の設計は別 ADR とすべきか？それとも本提案のスコープに含めるか (cons #1)。
- **Q3**: `EmitArticleUrlBackfill` admin endpoint を async (background goroutine + status polling) に書き換える ADR は同時に出すか、後回しか。30 回再 click は運用負荷大。

これらは本提案 accept 前に user / レビュアと調整。

---

## Deploy-model 整合性セルフチェック

- [x] 新設 compose service なし
- [x] 新設 named volume なし
- [x] DB schema 変更なし (dedupe key の文字列定数のみ)
- [x] proto / wire 互換性: 維持 (event payload shape 不変)
- [x] **rolling 互換** として pass。`git push origin main` → alt-deploy で alt-backend のみ rolling deploy
