---
name: immutable-design-guard
description: |
  Audits code, schema, and migrations for violations of immutable data-model invariants:
  append-first event log, reproject-safe projectors, disposable read models, versioned
  artifacts, merge-safe upserts, and no business-fact `time.Now()`.
  Use when changing migrations, projectors, event handlers, append-only stores, or any
  read model derived from events; or when the user mentions
  "イミュータブル", "event sourcing", "projector", "reproject", "append-only",
  "projection", "read model", or kawasima's resource/event modeling.
user-invocable: true
allowed-tools: Bash, Read, Glob, Grep, mcp__obsidian__view
paths:
  - "migrations-atlas/migrations/**/*.sql"
  - "**/*projector*.go"
  - "**/*projector*.rs"
  - "**/*projector*.py"
  - "**/*event*store*.go"
  - "**/*event*store*.rs"
  - "**/*reproject*.go"
  - "**/*_repository.go"
---

# Immutable Design Guard

Append-only event store とそこから派生する projection / read model を持つ任意の
サブシステムに対して、イミュータブルデータモデル不変条件を監査する。

このスキルは特定サービスに縛られない。Knowledge Home / Knowledge Loop /
Acolyte パイプライン / その他の event-sourced 領域すべてに同じ語彙で適用する。
固有テーブル名やドメイン語彙はリファレンスにケーススタディとして閉じ込める。

## いつ使うか

- migration / projector / event handler / read model コードを変更したとき
- レビュー対象が CQRS / event sourcing 風のサブシステムに触れているとき
- ユーザが「イミュータブル」「reproject」「projection」「event sourcing」
  「append-only」「read model」と言ったとき
- 新しい event 種別 / projection table / version 管理を追加するとき
- 別サービスやサブモジュールに同じ観点を移植したいとき

## いつ使わないか

- ステートレスな pure function / 計算ロジックの review
- キャッシュ層（TTL ベース）の review（reproject の概念と独立）
- 単純な CRUD アプリで append-only の前提がない領域

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

### Step 1: 対象スコープを特定

影響する以下を 1 行ずつ書き出す。

- event store table
- projection / read model table
- projector / reproject 実装ファイル
- event 発行箇所（write path）
- 影響する migration

### Step 2: 不変条件をマッピング

[references/alt-invariants.md](references/alt-invariants.md) の 10 項目から
今回の変更に該当するものを選ぶ。**全部書かない**。今回効くものだけ。

### Step 3: 違反候補を抽出

[references/check-recipes.md](references/check-recipes.md) の言語別
grep / ripgrep レシピを実行。Go / Rust / Python / SQL / proto それぞれにレシピがある。

### Step 4: 理論で裏取り

- 自然な resource / event 切り分けが崩れていないか →
  [references/kawasima-theory.md](references/kawasima-theory.md)
- event sourcing 一般のアンチパターンを踏んでいないか →
  [references/event-sourcing-patterns.md](references/event-sourcing-patterns.md)

### Step 5: 違反を分類して報告

下の出力テンプレで報告する。`どの原則を破っているか` は次節の名前で参照する。

### Step 6: escape hatch を確認

例外が必要であれば、対応する ADR か canonical contract 文書への
**明示反映**を要求する。skill 単独で例外を許容しない。

## コア原則 (1 行で)

これらは `references/alt-invariants.md` で詳細化される正規定義。
報告では必ずこの名前で参照する。

- **Append-first event log** — state は event の蓄積。`UPDATE` を増やしたく
  なったら hidden event を疑う。
- **Resource / Event 分離** — 日時を持つのは event のみ。resource に
  `updated_at` を生やすのは hidden event のサイン。
- **Event-time purity** — business-fact 時刻は event の `occurred_at` から
  派生。`time.Now()` / `NOW()` は debug の `projected_at` のみ、API 非露出。
- **Reproject-safe projector** — projector は event payload と stable な
  versioned resource だけから read model を再構築できる。latest state や
  active projection を読まない。
- **Disposable projection** — read model は捨てて再生成できる。source of
  truth に昇格させない。write path から read model を直接 mutate しない。
- **Versioned artifacts** — summary / tag / lens 等は version append のみ。
  event は stable な version id を参照し、reproject 時に同じ版を再現できる。
- **Merge-safe upsert** — projection の更新は monotonic + COALESCE 保持。
  `GREATEST(0, current + delta)` 等で負数を防ぐ。SQL `CASE` ロジックで
  business 判定を持ち込まない。
- **Single emission** — 同一ユーザ意図で複数 event を出さない。重複は
  `client_transition_id` 等の idempotency key で防ぐ。
- **Dedupe ≠ projection** — idempotency barrier は ingest 上流。
  reproject で touch しない。
- **Why as first-class** — 提案 / 選別 / 抑制の理由を構造化 payload として
  event / projection に保持する。後付けで再現できないものは event に書く。

## 出力テンプレート

違反は次の形式で報告する。

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
- どの原則に反するかを上記の名前で示す
- 代替案は event-first / append-first に寄せる
- 例外が必要なら ADR か canonical contract への明示反映を求める
- 重大度は **再投影できなくなるか** を基準にする:
  - high: replay 順序を変えると結果が変わる / source of truth が壊れる
  - medium: 1 種類の event を後から復元できない / projection に business
    判定が入る
  - low: 命名 / 一貫性 / drift しやすい構造（即破綻はしない）

## 参照

進行中、必要なものだけ読む。**全部読まない**。

- [references/alt-invariants.md](references/alt-invariants.md)
  — Alt の 10 個の正規 invariant 定義
- [references/kawasima-theory.md](references/kawasima-theory.md)
  — Resource/Event 分離と隠れた event の発見
- [references/event-sourcing-patterns.md](references/event-sourcing-patterns.md)
  — Pat Helland / Greg Young / Property Sourcing 系アンチパターン
- [references/check-recipes.md](references/check-recipes.md)
  — 言語別 grep / ripgrep レシピ
- [references/violation-examples.md](references/violation-examples.md)
  — 既知違反パターンと是正例
