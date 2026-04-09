---
name: plan-context-loader
description: |
  設計・計画タスク時に、Obsidian vault から関連 ADR・contract・review・runbook を
  必要最小限だけ集めて計画の精度を上げる。
  次のときに使う:
  - 「計画を立てて」「設計して」「プランを作って」
  - ADR や過去の決定を確認したい
  - Knowledge Home や reproject-safe / immutable 設計が絡む
user-invocable: true
allowed-tools: Bash, Read, Glob, Grep, Agent, mcp__obsidian__view, mcp__obsidian__get_workspace_files
argument-hint: <計画・設計の対象>
---

# Plan Context Loader

設計前に vault から正規コンテキストを集めるスキル。  
目的は「たくさん読む」ことではなく、**正しい文書を少数読む**こと。

Claude Code の原則に合わせる:

- まず探索し、その後に計画する
- 必要な文書だけ読む
- 長い引用ではなく短いブリーフに圧縮する
- 実装判断が割れる点だけを前面に出す

## ワークフロー

### 1. タスクを 1 行で言い換える

$ARGUMENTS から次を確定する。

- 影響サービス
- 対象ドメイン
- 主要な不変条件
- 調べるべき論点

この段階では推測しすぎない。まだ結論は出さない。

### 2. 先に正規 contract を当てる

最初に以下から必要なものだけ開く。

| 対象 | 参照先 |
|---|---|
| Knowledge Home 全般 | `docs/plan/knowledge-home-phase0-canonical-contract.md` |
| イミュータブルデータモデル | `docs/plan/IMPL_BASE.md` |
| 全体フェーズ計画 | `docs/plan/alt_knowledge_home_phase_plan.md` |
| フェーズ詳細 | `docs/plan/IMPL_PHASE1.md` - `docs/plan/IMPL_PHASE6.md` |

ルール:

- 全文を読むのではなく、対象論点の節だけを開く
- contract と食い違う既存案があるかを先に見る

### 3. ADR を少数読む

ADR は 2-6 件の高信号なものに絞る。  
`accepted` を優先し、`superseded` / `deprecated` は後続 ADR を確認してから使う。

検索例:

```bash
grep -rl "affected_services:.*<service>" docs/ADR/ | sort | tail -10
grep -rl "tags:.*<tag>" docs/ADR/ | sort | tail -10
grep -rl "<keyword>" docs/ADR/ | sort | tail -10
```

各 ADR から拾うのは次だけ。

- なぜその判断が必要だったか
- 何を固定したか
- 今回の計画に効く制約は何か

### 4. review / runbook / daily を補助的に拾う

必要な場合だけ追加で確認する。

- `docs/review/`:
  既知の未達、是正指示、監査結果
- `docs/runbooks/`:
  reproject、障害復旧、degraded mode などの運用制約
- `docs/daily/`:
  直近 1-2 日の作業文脈

運用文書は「設計を縛る事実」があるときだけ使う。  
作業メモ全体を読み込まない。

### 5. 衝突を明示する

次の衝突があれば必ず書く。

- 既存案が canonical contract と矛盾する
- read model を source of truth 扱いしている
- reproject-safe を壊す副作用更新がある
- feature flag の意図と恒久設計が混線している

### 6. 短いコンテキストブリーフを出す

出力は 1 画面で読める長さを優先する。

```markdown
## 計画コンテキストブリーフ

### 対象
- 何を決めるタスクか

### 関連 ADR
- [[000NNN]] タイトル — 今回効く判断だけ

### 適用される不変条件
- append-first
- reproject-safe
- versioned projection

### 参照すべき contract / plan
- 文書名と該当セクション

### 運用制約
- runbook / review 由来の制約だけ

### 潜在的な衝突
- 今回の設計で踏みやすい地雷
```

## 守ること

- 先に探索、あとで計画
- 読む文書は少なく、要約はさらに短く
- 引用の羅列ではなく、意思決定に必要な差分だけ残す
- 不変条件を満たさない既存案は、その場で明示して止める
