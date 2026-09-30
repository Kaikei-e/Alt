---
name: plan-context-loader
description: |
  設計・計画の前に Obsidian vault (`docs/`) から関連 ADR・canonical contract・review・runbook を必要最小限だけ集め、適用される不変条件と潜在的な衝突を 1 画面のブリーフにまとめる。ユーザが「計画を立てて」「設計して」「プランを作って」「過去の ADR 確認して」と言ったとき、Knowledge Trail / Knowledge Home や reproject-safe / immutable 設計が絡むとき、append-first projection まわりの修正 PR を始める前に使う。作成済みのプランを対話的に問い詰めて検証したいときは grill-with-docs を使う。こちらは文脈を集めて返すだけで、ユーザに質問を浴びせるスキルではない。
allowed-tools: Bash, Read, Glob, Grep
argument-hint: "<計画・設計の対象>"
---

# Plan Context Loader

vault は `docs/` 配下の通常のファイル群なので、Read / Grep / Glob で読む。
目的は「たくさん読む」ことではなく、**正しい文書を少数読む**こと。先に探索し、あとで計画する。

### ワークフロー

- [ ] 1. タスク特定: 影響サービス、対象ドメイン、主要な不変条件を整理
- [ ] 2. 正規 contract 照合: 対象領域の canonical contract・不変条件（Trail / Home 等）を確認
- [ ] 3. ADR 探索: `docdag context <id>` や `docdag query` を使い、現行の確定決定（`status: accepted` かつ inbound `supersedes` なし）を 2〜6 件特定
- [ ] 4. 運用・是正文書確認: `docs/review/`、`docs/runbooks/`、`docs/postmortems/`、`docs/daily/` から制約事項を抽出
- [ ] 5. 衝突検証: canonical contract との矛盾、read model の誤用、reproject 破壊がないか確認
- [ ] 6. ブリーフ出力: 1 画面に収まる計画コンテキストブリーフを出力

## 1. タスクを 1 行で言い換える

`$ARGUMENTS` から、影響サービス / 対象ドメイン / 主要な不変条件 / 調べるべき論点 を確定する。
この段階では推測しすぎない。まだ結論は出さない。

## 2. 先に正規 contract を当てる

| 計画対象 | 参照先 |
|---|---|
| Knowledge Trail | `docs/plan/knowledge-trail-core-concept.md`, `docs/plan/knowledge-trail-implementation-plan.md`, `docs/wiki/architecture/knowledge-trail.md` |
| Knowledge Home（価値・入口） | `docs/plan/knowledge-home-value-position-plan.md`, `docs/wiki/architecture/immutable-data-model.md` |
| イミュータブルデータモデル | `docs/wiki/architecture/immutable-data-model.md`, Trail core-concept §C |
| Projector / Reproject | `docs/services/knowledge-sovereign.md`, `docs/runbooks/` の reproject 系 |
| 是正・未達事項（historical audit） | `docs/review/` 配下（`docs/review/README.md`） |
| Acolyte 全般・パイプライン | `docs/services/acolyte-orchestrator.md`, `docs/case-studies/acolyte-design-evolution.md`, Acolyte 関連 ADR は `docdag query` またはタイトル検索で特定 |
| 全体ナビゲーション地図 | `docs/wiki/HOME.md` |
| 運用手順・復旧 | `docs/runbooks/README.md` |
| 障害分析 | `docs/postmortems/README.md` |
| 直近の作業文脈 | `docs/daily/` の最新エントリ |
| Knowledge Loop / Home phase0 / IMPL_* | **historical** — 現行契約として開かない（[[000940]]） |

- 全文ではなく、対象論点の節だけを開く
- contract と食い違う既存案があるかを先に見る
- Trail plan は core-concept + implementation-plan の **2 文書上限**（3 つ目を作らない）

## 3. ADR を少数読む

2-6 件の高信号なものに絞る。**現行契約**は `status: accepted` かつ inbound `supersedes` が
無いものだけ。`superseded` / 置換済み / Knowledge Loop・IMPL_*（[[000940]]）は historical として扱い、
現行契約として開かない。Related は must-read にしない（参考のみ）。

グラフに聞けることをディレクトリ走査で代替しない。ADR を 1 件掴んだら、そこから周辺を広げる。

```bash
# 起点 ADR とその近傍を、各文書の Decision 冒頭つきで読む（まずこれ）
docdag context 000929 --depth 2 --budget 1500

# 現行契約の一覧（status: accepted かつ inbound supersedes 無し）
docdag query --binding --fields id,title,status,path

# この ADR が前提にしている側 = 何の上に成り立っているか
# （既定 --descendants。depends-on / supersedes を辿る）
docdag query 000929 --fields id,title

# この ADR に依存している側 = 変更が波及する先（depends-on の inbound も拾う）
docdag query 000929 --ancestors --fields id,title

docdag resolve 000929   # → 000940 （葉 = 現行後継を確認。後継が proposed など
                        #   non-binding ならそこで止まり、stderr に
                        #   note: X supersedes Y but is proposed; not yet binding
                        #   （JSON なら pending 配列）。まだ確定していないと扱う）
docdag validate         # status / stub / cycle（supersedes+depends-on の和集合も）/ dangling
```

キーワードや影響サービスで**入口の ADR を探す**ときだけテキスト検索を使う:

```bash
grep -l -- "- <service>" docs/ADR/*.md | sort | tail -10   # affected_services はブロックリスト
grep -rl "<keyword>" docs/ADR/ | sort | tail -10
```

`grep -rl "affected_services:.*<service>"` は使わない。`affected_services` はほぼ全 ADR で
ブロックリスト（`affected_services:` の次行から `- ...`）なので、同一行にサービス名が来る
インライン形式しか拾えず、実際に該当する ADR の 3 割弱を静かに取りこぼす。
`tags:` も同じ理由で行内 grep が効かないため、タグでの絞り込みは諦めて上のグラフ問い合わせを使う。

各 ADR から拾うのは 3 点だけ — なぜその判断が必要だったか / 何を固定したか / 今回の計画に効く制約。

## 4. review / runbook / daily を補助的に拾う

- `docs/review/` (`docs/review/README.md`) — 既知の未達、是正指示、監査結果
- `docs/runbooks/` (`docs/runbooks/README.md`) — reproject、障害復旧、degraded mode などの運用制約
- `docs/postmortems/` (`docs/postmortems/README.md`) — 障害再発防止・ポストモーテム
- `docs/daily/` — 直近 1-2 日の作業文脈

運用文書は「設計を縛る事実」があるときだけ開く。作業メモ全体を読み込まない。

## 5. 衝突を明示する

次に当てはまるものがあれば必ずブリーフに書く。不変条件を満たさない既存案は、その場で明示して止める。

- 既存案が canonical contract と矛盾する
- read model を source of truth 扱いしている
- reproject-safe を壊す副作用更新がある
- feature flag の意図と恒久設計が混線している
- Knowledge Loop の語彙（4-bucket / primary surface `/loop`）を現行として扱っている（[[000940]] で廃止）

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
