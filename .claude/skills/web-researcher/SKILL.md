---
name: web-researcher
description: |
  Researches a technical question on the web, official documentation first, and returns a structured
  report with tiered sources, contradictions flagged, freshness noted, and recommended next actions.
  Use when the user says 「調べて」「リサーチして」「公式ドキュメント確認して」, "research X",
  "look up", "find docs for", when evaluating a library / framework / approach, when investigating an
  error, migration guide or breaking change, or when the answer needs information newer than training
  data. Prefer plan-context-loader when the material lives in this repo's Obsidian vault rather than
  on the web, and the claude-api skill for Claude/Anthropic model, pricing and SDK questions.
allowed-tools: WebSearch, WebFetch, Read, Write, Agent
argument-hint: <research topic or question> [--depth=shallow|deep] [--lang=en|ja]
---

# Web Researcher

目的は「大量に検索する」ことではなく、**信頼性の高い情報を構造化して届ける**こと。
3 件の高信号ソースは 20 件の低信号ソースに勝る。

原則:

- 公式ドキュメントを最優先。非公式情報は公式で埋まらないギャップにだけ使う
- 検索前に「何を知りたいのか」を 1 行で明文化する
- 出典のない主張は書かない。古い情報・矛盾する情報は明示的にフラグを立てる
- 検索結果が不十分なら「わからなかった」と書く。埋め合わせに推測を書かない

`$ARGUMENTS` から調査対象・調査目的・深さ（`--depth=shallow` なら Phase 2 で打ち切り、
既定は deep）・言語（`--lang=ja` なら日本語ソースも積極的に探す。既定は英語優先）を確定する。

## Phase 1: 公式ドキュメント

公式ドメインを推定し（React → `react.dev`、Go → `go.dev`、Rust → `doc.rust-lang.org` など）、
`site:<official-domain>` 付きの `WebSearch` から始める。ライブラリなら GitHub の README /
CHANGELOG / Issues / Discussions も公式扱い。上位 2-3 件を `WebFetch` するときは、
「このページから <調査目的> に関する情報を抽出して」と目的を添える。

API リファレンスや設定方法の確認のように、公式だけで目的が達成できたなら Phase 4 に飛ぶ。

## Phase 2: 広範な文献

Phase 1 で埋まらなかったギャップだけを、ドメイン制限なしの検索で埋める。角度を変えた複数クエリを
使い、拾ったソースは Tier で扱いを変える。

| Tier | ソース種別 | 扱い方 |
|------|-----------|--------|
| S | 公式ドキュメント、RFC、公式ブログ | そのまま採用 |
| A | GitHub Issues/PR (公式リポ)、著名な技術ブログ | 高信頼、ただし日付を確認 |
| B | Stack Overflow (高スコア回答)、カンファレンス資料 | 参考にするが裏取りする |
| C | 個人ブログ、Medium、Qiita/Zenn | 複数ソースで裏取りできた場合のみ採用 |
| D | 未検証フォーラム投稿、2 年以上前の記事 | 原則不採用。使うなら警告付き |

Tier A-B から 2-3 件を `WebFetch` で深読みし、公式と矛盾する記述がないか確認する。複数 URL を
まとめて読むときは Agent で並列化してよいが、5 件以上の並列取得は質が落ちるので避ける。

## Phase 3: 深掘り（deep のみ）

エッジケース・既知の問題・代替案を洗い出す。`"<技術名> gotcha"` / `"pitfall"` /
`"common mistakes"` の検索、GitHub Issues の `label:bug` / `is:issue is:open`、`"<技術名> vs"` /
`"alternative"` による比較、そしてリリースノート・ロードマップ・deprecation notice の確認。

## Phase 4: レポート

公式情報を先に、非公式を後に置く。矛盾があれば両論を併記し、どちらが信頼できるかの判断を添える。

```markdown
## Web Research Report: <調査対象>

### 調査目的
- <1行で目的を記述>

### 要約
<3-5行。最も重要な発見を先に書く>

### 公式ドキュメントからの発見
- <発見> ([出典](URL))

### コミュニティ情報からの発見
- <発見> ([出典](URL), Tier X)

### 注意事項・落とし穴
- <既知の問題や注意点>

### 推奨アクション
1. <具体的な次のステップ>

### 情報の鮮度
- 調査日: <date>
- 最も古いソース: <date/URL>
- 鮮度に関する注意: <該当があれば>

### Sources
| # | Title | URL | Tier | Note |
|---|-------|-----|------|------|
| 1 | <タイトル> | <URL> | S | <簡潔な説明> |
```
