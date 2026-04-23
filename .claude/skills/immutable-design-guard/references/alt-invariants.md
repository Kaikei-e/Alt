# Alt Immutable Data Model — Canonical Invariants

Alt monorepo で append-only event store を持つすべてのサブシステムに通用する
正規不変条件。SKILL.md のコア原則 1 行リストの拡張定義。

> 一次出典: `docs/plan/knowledge-loop-canonical-contract.md` §3 (Canonical
> invariants)。Knowledge Loop のテーブル名で書かれているが、原則は領域非依存。

各項目は次の形で書く。

- **何を言っているか** (1-2 行)
- **典型的な違反**
- **満たす実装パターン**

## Contents

1. Append-first event log
2. Resource / Event separation
3. Event-time purity
4. Reproject-safe projector
5. Disposable projection
6. Versioned artifacts
7. Merge-safe upsert
8. Single emission
9. Dedupe is ingest-only
10. Why as first-class

---

## 1. Append-first event log

**何を言っているか**: 状態遷移は event の追記のみで表現する。event store
テーブルは INSERT only。事実は失われない。

**典型的な違反**:
- `UPDATE … events SET payload = …`
- `DELETE FROM … events WHERE …`（修正イベントを別 event として書かない）
- soft-delete を `deleted_at` 列で event store に持つ
- event の payload を後から書き換える「修正」運用

**満たす実装パターン**:
- 取り消し / 訂正は `XxxCorrected` `XxxReversed` などの新 event
- `dedupe_key` UNIQUE で idempotency
- BIGSERIAL `event_seq` で ordering 担保

## 2. Resource / Event separation

**何を言っているか**: kawasima 理論に従い、resource (日時を持たない名詞)
と event (日時を持つ動詞) を分離する。resource に `updated_at` を生やしたく
なったら、抽出されていない hidden event がある。

**典型的な違反**:
- 全テーブルへの機械的な `created_at` / `updated_at`
- `XxxUpdated` という generic event 名で意味の異なる変更を 1 つに押し込む
- resource 行を上書きして履歴を捨てる
- `status` 列だけで業務遷移を表現する

**満たす実装パターン**:
- 業務的に複数の意味を持つ更新は別 event 種別に切り分け
  (例: `MemberInfoChangedByUser` / `MemberForcedDeactivatedByOperator`
  / `MemberReactivated`)
- resource は append-only event を集約した snapshot
- 更新が業務的に存在しない resource は「日時なし」のまま

## 3. Event-time purity

**何を言っているか**: business-fact を表す時刻は event の `occurred_at` から
派生させる。projector / read model 内で `time.Now()` / `NOW()` を business
fact に使わない。

**典型的な違反**:
- projector が `time.Now()` で `freshness_at` を埋める
- `NOW()` を SQL の DEFAULT にし projector が業務時刻として読む
- session state の `entered_at` が triggering event の `occurred_at` ではなく
  wall-clock

**満たす実装パターン**:
- projection field: `freshness_at = MAX(event.occurred_at)`,
  `source_event_seq`, `projection_seq_hiwater`, `projection_revision`
- wall-clock は **debug 専用の `projected_at`** のみ。API / proto / public
  view / metrics label に出さない
- `updated_at` を分解する: completeness は `projection_seq_hiwater`、
  局所版は `projection_revision`、業務鮮度は `freshness_at`

## 4. Reproject-safe projector

**何を言っているか**: projector は event payload と stable な versioned
resource (immutable な参照) のみから read model を再構築できなければならない。
latest state や active projection を読まない。

**典型的な違反**:
- projector 内で `GetLatestSummary(article_id)` を呼ぶ
- projector 内で active projection 行を読み戻して merge する
- replay 順序が変わると結果が変わる
- swap 後に checkpoint をリセットせず gap を生む（PM-2026-010 参照）

**満たす実装パターン**:
- event payload に **stable な version id** を持たせる
  (例: `summary_version_id`, `tag_set_version_id`)
- projector は event の指す version をそのまま読む
- `SwapReproject()` 系は activation と checkpoint reset を不可分に
- replay 冪等性をテストする: 同一 event 列を任意順序で食わせても結果一致

## 5. Disposable projection

**何を言っているか**: projection / read model は捨てて event log から再構築
できる。source of truth に昇格させない。write path から projection を直接
mutate しない。

**典型的な違反**:
- write path (handler / usecase) が projection table を直接 UPDATE
- projection の値を event 復元せずに直接修正する管理スクリプト
- projection を別サービスが書き戻す
- backfill 系 batch が event を経由せず projection を直接書く

**満たす実装パターン**:
- 全変更は event → projector → projection の 1 方向
- projection rebuild の runbook が存在し、定期的に検証
- shadow projection を作って差分検証してから swap

## 6. Versioned artifacts

**何を言っているか**: summary / tag / lens など、後から内容が変わるもの
は version append のみで表現する。event は stable な version id を参照する。

**典型的な違反**:
- `summary` テーブルを上書き更新
- 新 summary を作っても event は `article_id` だけ参照
- "latest" を意味する flag 列を mutable に維持
- `summary_versions` の row を後から書き換える

**満たす実装パターン**:
- `summary_versions` / `tag_set_versions` / `lens_versions` 等が append-only
- event payload に `<artifact>_version_id` を必ず含める
- "latest" は projection 側の disposable view で表現
- version mapping を変えるときは `MappingVersion` を bump して
  full reproject

## 7. Merge-safe upsert

**何を言っているか**: projection の更新は monotonic + COALESCE 保持。
event 差分による増減算で表現し、SQL 内に business 判定を入れない。

**典型的な違反**:
- `UPDATE projection SET counter = current - 1` で負数発生
- SQL `CASE WHEN status='X' THEN …` で business logic を SQL に逃がす
- supersede / dismiss を read model 上の flag だけで表現
- snapshot 上書きで COALESCE すべき値を NULL に戻す

**満たす実装パターン**:
- `GREATEST(0, current + delta)` で負数防止
- `COALESCE(EXCLUDED.x, projection.x)` で既存値を保持
- monotonicity を `seq_hiwater` guard で担保
  (`WHERE EXCLUDED.seq > projection.seq_hiwater`)
- business 判定は Go / Rust / Python 側で行い SQL は単純な merge に留める

## 8. Single emission

**何を言っているか**: 同一ユーザ意図で複数 event を出さない。同一 RPC
呼び出しは 1 event に集約。重複呼び出しは idempotency key で潰す。

**典型的な違反**:
- 同じアクションが UI 系 event と analytics 系 event の 2 本を発行
- retry で同じ event が再発火し double count
- client が optimistic と final の 2 event を出す

**満たす実装パターン**:
- `client_transition_id` (UUIDv7 等) を必須化し server で dedupe
- UUIDv7 embedded timestamp の許容窓を server で検証
- `(user_id, client_transition_id)` で fast-path dedupe + event store
  unique index で slow-path 拒否

## 9. Dedupe is ingest-only

**何を言っているか**: idempotency barrier (dedupe table) は event log の
**上流**。projection ではない。reproject で touch しない。

**典型的な違反**:
- reproject が dedupe table を含めて rebuild
- dedupe table を read model として join
- dedupe row を business signal として使う

**満たす実装パターン**:
- dedupe table は ingest layer のみが書く
- reproject は event log のみを scan、dedupe は無視
- TTL / 窓を持たせ、長期 idempotency は event store unique index で担保

## 10. Why as first-class

**何を言っているか**: 提案 / 選別 / 抑制の理由を構造化 payload として
event / projection に保持する。「なぜそれが選ばれたか」を後から再現
できないものは event に書く。

**典型的な違反**:
- 自由 JSON `why_json` を許可し downstream が解釈不能になる
- ranking score を保存せず順序だけ persist
- recommendation の根拠を log にだけ書き event payload に書かない
- supersede 理由を badge 表示用の文字列でしか持たない

**満たす実装パターン**:
- `WhyPayload { kind: enum, text: 1..N chars, evidence_refs: [] }`
  のような構造化型
- `why_kind` は exhaustive enum、mapping 変更は version bump
- evidence_refs は event id / version id への参照で保持
- score / rule id を payload に同梱して reproducibility を担保

---

## 不変条件と Knowledge Home / Knowledge Loop の対応 (参考)

| Invariant | Knowledge Loop での具体形 |
|---|---|
| Append-first | `knowledge_events` (INSERT only) |
| Event-time purity | `freshness_at = MAX(occurred_at)`, `projected_at` debug only |
| Reproject-safe | `summary_version_id` 経由で stable read |
| Versioned artifacts | `summary_versions`, `tag_set_versions`, `lens_versions` |
| Merge-safe upsert | `GREATEST(0, …)` + `seq_hiwater` guard |
| Single emission | `client_transition_id` (UUIDv7) |
| Dedupe ingest-only | `knowledge_loop_transition_dedupes` |
| Why as first-class | `WhyPayload { kind, text, evidence_refs }` |

固有テーブル名 / 許可コードの詳細は
[violation-examples.md](violation-examples.md) のケーススタディを参照。
