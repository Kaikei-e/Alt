---
name: immutable-design-guard
description: |
  Audits code, schema, and migrations for violations of immutable data-model invariants:
  append-first event log, resource/event separation, reproject-safe projectors, disposable
  read models, versioned artifacts, merge-safe upserts, and no business-fact `time.Now()`.
  Use when changing migrations, projectors, event handlers, append-only stores, or any
  read model derived from events; or when the user mentions
  "イミュータブル", "event sourcing", "projector", "reproject", "append-only",
  "projection", "read model", or kawasima's resource/event modeling.
  Applies to any append-only subsystem, not one named service. Not for stateless pure
  functions, TTL caches, or plain CRUD tables with no event log behind them.
allowed-tools: Bash, Read, Glob, Grep
---

# Immutable Design Guard

Append-only event store とそこから派生する projection / read model を持つ任意のサブシステムを
監査する。特定サービスに縛られない — Knowledge Home / Knowledge Loop / Acolyte パイプライン等に
同じ語彙で適用し、固有テーブル名はリファレンスにケーススタディとして閉じ込める。

## コア原則

報告では必ずこの名前で原則を参照する。正規定義は
[references/alt-invariants.md](references/alt-invariants.md)（10 項目、Step 2 で該当分だけ読む）。

- **Append-first event log** — state は event の蓄積。`UPDATE` を増やしたくなったら hidden event を疑う
- **Resource / Event 分離** — 日時を持つのは event のみ。resource の `updated_at` は hidden event のサイン
- **Event-time purity** — business-fact 時刻は event の `occurred_at` 由来。wall clock は debug 用
  `projected_at` のみで、API / proto / metrics label に出さない
- **Reproject-safe projector** — event payload と stable な versioned resource だけから read model を
  再構築できる。latest state や active projection を読まない
- **Disposable projection** — read model は捨てて再生成できる。source of truth に昇格させない。
  write path から直接 mutate しない
- **Versioned artifacts** — summary / tag / lens 等は version append のみ。event は stable な version id
  を参照し、reproject 時に同じ版を再現できる
- **Merge-safe upsert** — projection の更新は monotonic + COALESCE 保持。`GREATEST(0, current + delta)`
  等で負数を防ぐ。SQL `CASE` に business 判定を持ち込まない
- **Single emission** — 同一ユーザ意図で複数 event を出さない。重複は `client_transition_id` 等の
  idempotency key で防ぐ
- **Dedupe ≠ projection** — idempotency barrier は ingest 上流。reproject で touch しない
- **Why as first-class** — 提案 / 選別 / 抑制の理由を構造化 payload として event / projection に持つ。
  後付けで再現できないものは event に書く

## 監査ワークフロー

進行中は次のチェックリストを応答にコピーして埋める。

```
Audit Progress:
- [ ] 1. 対象スコープを 1 行ずつ列挙
- [ ] 2. 適用される不変条件を選ぶ
- [ ] 3. 違反候補を grep / read で抽出
- [ ] 4. kawasima 観点と一般理論で裏取り
- [ ] 5. 違反を分類して報告
- [ ] 6. escape hatch (ADR / canonical contract) を確認
```

1. **対象スコープを特定** — event store table / projection table / projector・reproject 実装 /
   event 発行箇所 (write path) / 影響する migration を 1 行ずつ書き出す。
2. **不変条件をマッピング** — [references/alt-invariants.md](references/alt-invariants.md) の 10 項目
   から今回効くものだけ選ぶ。**全部書かない**。
3. **違反候補を抽出** — [references/check-recipes.md](references/check-recipes.md) の言語別
   grep / ripgrep レシピを、触れた言語 / 層のぶんだけ実行する（Go / Rust / Python / SQL / proto）。
4. **理論で裏取り** — 自然な resource / event 切り分けが崩れていないかは
   [references/kawasima-theory.md](references/kawasima-theory.md)、event sourcing 一般の
   アンチパターンは [references/event-sourcing-patterns.md](references/event-sourcing-patterns.md)。
5. **違反を分類して報告** — 下の出力テンプレで報告する。似た違反の実例と是正手順は
   [references/violation-examples.md](references/violation-examples.md) にケーススタディがある。
6. **escape hatch を確認** — 例外が必要なら、対応する ADR か canonical contract 文書への**明示反映**
   を要求する。skill 単独で例外を許容しない。

## 出力テンプレート

```markdown
## Immutable Design Findings

### 1. [Severity: high|medium|low] <一行サマリ>
- 該当箇所: `path/to/file.go:42` (該当する場合は migration / proto / SQL も)
- 破っている原則: <Append-first | Resource/Event 分離 | Event-time purity | …>
- なぜ危険か: <reproject 不能 / shadow 汚染 / 意味論崩壊 / 監査不能 など>
- 代替案: <event append への置換 / projector fix / contract への明示化>
- 既知の類例: <該当する violation-examples / postmortem があれば>
```

ルール:

- まず違反を指摘する（解説から始めない）
- 代替案は event-first / append-first に寄せる
- 重大度は **再投影できなくなるか** を基準にする:
  - high: replay 順序を変えると結果が変わる / source of truth が壊れる
  - medium: 1 種類の event を後から復元できない / projection に business 判定が入る
  - low: 命名 / 一貫性 / drift しやすい構造（即破綻はしない）
