---
name: plan-context-loader
description: |
  設計・計画の前に Obsidian vault (`docs/`) から関連 ADR・canonical contract・review・runbook を
  必要最小限だけ集め、適用される不変条件と潜在的な衝突を 1 画面のブリーフにまとめる。
  次のときに使う:
  - 「計画を立てて」「設計して」「プランを作って」「過去の ADR 確認して」
  - Knowledge Trail / Knowledge Home や reproject-safe / immutable 設計が絡むとき
  - append-first projection まわりの修正 PR を始める前（canonical contract の読み直し）
  既にあるプランを問い詰めて検証したいときは grill-with-docs を使う。
  こちらは文脈を集めて返すだけで、ユーザに質問を浴びせるスキルではない。
allowed-tools: Bash, Read, Glob, Grep, Agent
argument-hint: <計画・設計の対象>
---

# Plan Context Loader

vault は `docs/` 配下の通常のファイル群なので、Read / Grep / Glob で読む。
目的は「たくさん読む」ことではなく、**正しい文書を少数読む**こと。先に探索し、あとで計画する。

## 1. タスクを 1 行で言い換える

`$ARGUMENTS` から、影響サービス / 対象ドメイン / 主要な不変条件 / 調べるべき論点 を確定する。
この段階では推測しすぎない。まだ結論は出さない。

## 2. 先に正規 contract を当てる

| 対象 | 参照先 |
|---|---|
| Knowledge Trail | `docs/plan/knowledge-trail-core-concept.md` |
| Trail 実装 wave | `docs/plan/knowledge-trail-implementation-plan.md` |
| イミュータブルデータモデル | `docs/wiki/architecture/immutable-data-model.md` + Trail §C |
| Knowledge Home（価値・入口） | `docs/plan/knowledge-home-value-position-plan.md` |
| 現在地の地図 | `docs/wiki/HOME.md` |
| Loop / Home phase0 / IMPL_* | **historical** — 現行契約として開かない（[[000940]]） |

- 全文ではなく、対象論点の節だけを開く
- contract と食い違う既存案があるかを先に見る
- Trail plan は core-concept + implementation-plan の **2 文書上限**（3 つ目を作らない）

## 3. ADR を少数読む

2-6 件の高信号なものに絞る。**現行契約**は `status: accepted` かつ inbound `supersedes` が
無いものだけ。`superseded` / 置換済み / Loop・IMPL_*（[[000940]]）は historical として扱い、
現行契約として開かない。Related は must-read にしない（参考のみ）。

```bash
grep -rl "affected_services:.*<service>" docs/ADR/ | sort | tail -10
grep -rl "tags:.*<tag>" docs/ADR/ | sort | tail -10
grep -rl "<keyword>" docs/ADR/ | sort | tail -10
docdag resolve 000929   # → 000940 （葉 = 現行後継を確認）
docdag validate         # status / stub / cycle
```

各 ADR から拾うのは 3 点だけ — なぜその判断が必要だったか / 何を固定したか / 今回の計画に効く制約。

## 4. review / runbook / daily を補助的に拾う

- `docs/review/` — 既知の未達、是正指示、監査結果
- `docs/runbooks/` — reproject、障害復旧、degraded mode などの運用制約
- `docs/daily/` — 直近 1-2 日の作業文脈

運用文書は「設計を縛る事実」があるときだけ開く。作業メモ全体を読み込まない。

## 5. 衝突を明示する

次に当てはまるものがあれば必ずブリーフに書く。不変条件を満たさない既存案は、その場で明示して止める。

- 既存案が canonical contract と矛盾する
- read model を source of truth 扱いしている
- reproject-safe を壊す副作用更新がある
- feature flag の意図と恒久設計が混線している
- Loop 時代の語彙（4-bucket / primary surface `/loop`）を現行として扱っている

## 6. 短いコンテキストブリーフを出す

引用の羅列ではなく、意思決定に必要な差分だけを残す。1 画面で読める長さを優先する。

```markdown
## 計画コンテキストブリーフ

### 対象
- 何を決めるタスクか

### 関連 ADR
- [[000NNN]] タイトル — 今回効く判断だけ

### 適用される不変条件
- append-first / reproject-safe / versioned projection のうち該当するもの

### 参照すべき contract / plan
- 文書名と該当セクション

### 運用制約
- runbook / review 由来の制約だけ

### 潜在的な衝突
- 今回の設計で踏みやすい地雷
```
