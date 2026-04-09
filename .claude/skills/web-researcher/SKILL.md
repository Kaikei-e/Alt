---
name: web-researcher
description: |
  Web research skill that prioritizes official documentation while covering broad literature.
  Searches, fetches, and synthesizes findings into a structured actionable report.
  Use when:
  - 「調べて」「リサーチして」「公式ドキュメント確認して」
  - "research X", "look up", "find docs for"
  - Evaluating libraries, frameworks, or approaches
  - Investigating errors, migration guides, or breaking changes
  - Needing up-to-date information beyond training data
user-invocable: true
allowed-tools: WebSearch, WebFetch, Read, Write, Agent
argument-hint: <research topic or question> [--depth=shallow|deep] [--lang=en|ja]
---

# Web Researcher

公式ドキュメントを最優先に、幅広い文献をカバーするウェブリサーチスキル。
目的は「大量に検索する」ことではなく、**信頼性の高い情報を構造化して届ける**こと。

原則:

- 公式ドキュメントを最優先。非公式情報は公式で埋まらないギャップにのみ使う
- 検索は段階的に広げる。最初から広く探さない
- 取得した情報は必ず出典付きで構造化する
- 古い情報・矛盾する情報は明示的にフラグを立てる
- 量より質。3 件の高信号ソースは 20 件の低信号ソースに勝る

## 引数の解釈

`$ARGUMENTS` から以下を確定する:

- **調査対象**: 技術名、ライブラリ名、エラーメッセージ、概念など
- **調査目的**: 導入検討、エラー解決、ベストプラクティス確認、移行ガイドなど
- **深さ**: `--depth=shallow` なら Phase 2 までで終了。`--depth=deep` か指定なしなら全 Phase 実行
- **言語**: `--lang=ja` なら日本語ソースも積極的に検索。デフォルトは英語優先

目的が不明確な場合は、まず仮説を立ててから検索を開始する。
検索前に「何を知りたいのか」を 1 行で明文化する。

## Phase 1: 公式ドキュメント検索

**目標:** 公式情報源から正確な情報を得る。

### 手順

1. **公式サイトを特定する**
   - 調査対象の公式ドキュメントサイトのドメインを推定する
   - 例: React → `react.dev`, Go → `go.dev`, Rust → `doc.rust-lang.org`, Python → `docs.python.org`

2. **ドメイン限定検索を実行する**
   - `WebSearch` で公式サイトに絞って検索
   - 検索クエリは具体的に。曖昧な単語の羅列ではなく、ドキュメントに含まれそうなフレーズを使う
   - `site:<official-domain>` をクエリに含めて公式ドメインに絞る

3. **GitHub リポジトリも公式扱いする**
   - ライブラリの場合、GitHub の README, CHANGELOG, Issues, Discussions も公式情報源
   - 対象リポジトリに関する情報を検索

4. **見つかった重要ページを WebFetch で読む**
   - 検索結果の上位 2-3 件を `WebFetch` で取得
   - 取得時は「このページから <調査目的> に関する情報を抽出して」と目的を明確にする

### この Phase で十分な場合

公式ドキュメントだけで調査目的が達成できる場合（API リファレンス確認、設定方法の確認など）、
Phase 2-3 をスキップして Phase 4 に進んでよい。

## Phase 2: 広範な文献検索

**目標:** 公式ドキュメントでカバーされないギャップを埋める。

### 手順

1. **Phase 1 の結果を評価する**
   - 何がわかったか、何がまだ不明か、を整理する
   - 不明点がなければこの Phase をスキップ

2. **コミュニティソースを検索する**
   - `WebSearch` でドメイン制限なしの広範検索を実行
   - 必要に応じて複数のクエリを使う（異なる角度から検索）

3. **情報源の信頼性でフィルタする**

   | Tier | ソース種別 | 扱い方 |
   |------|-----------|--------|
   | S | 公式ドキュメント、RFC、公式ブログ | そのまま採用 |
   | A | GitHub Issues/PR (公式リポ)、著名な技術ブログ | 高信頼、ただし日付を確認 |
   | B | Stack Overflow (高スコア回答)、技術カンファレンス資料 | 参考にするが裏取りする |
   | C | 個人ブログ、Medium 記事、Qiita/Zenn | 複数ソースで裏取りできた場合のみ採用 |
   | D | 未検証フォーラム投稿、古い記事 (2年以上前) | 原則不採用。採用する場合は警告付き |

4. **有望なページを WebFetch で深読みする**
   - Tier A-B のソースから 2-3 件を選んで WebFetch
   - 公式ドキュメントと矛盾する記述がないか確認

### Agent による並列取得

複数の URL を WebFetch する場合、Agent を使って並列化できる:

```
Agent で以下の URL を並列に WebFetch して要約:
- URL1: <目的>
- URL2: <目的>
- URL3: <目的>
```

ただし、5 件以上の並列取得は避ける。質が落ちる。

## Phase 3: 深掘り調査 (deep のみ)

**目標:** エッジケース、既知の問題、代替案を洗い出す。

`--depth=shallow` の場合はスキップ。

### 手順

1. **既知の問題・落とし穴を検索する**
   - `"<技術名> gotcha"`, `"<技術名> pitfall"`, `"<技術名> common mistakes"` で検索
   - GitHub Issues で `label:bug` や `is:issue is:open` を狙う

2. **代替技術・アプローチを検索する**
   - `"<技術名> vs"`, `"<技術名> alternative"` で検索
   - 比較記事から客観的なトレードオフを抽出

3. **最新動向を確認する**
   - リリースノート、ロードマップ、deprecation notice を確認
   - 破壊的変更の予定がないか確認

## Phase 4: レポート作成

**目標:** 発見を構造化し、行動可能な形でまとめる。

### 出力テンプレート

```markdown
## Web Research Report: <調査対象>

### 調査目的
- <1行で目的を記述>

### 要約
<3-5行で調査結果の要約。最も重要な発見を先に書く>

### 公式ドキュメントからの発見
- <発見1> ([出典](URL))
- <発見2> ([出典](URL))

### コミュニティ情報からの発見
- <発見1> ([出典](URL), Tier X)
- <発見2> ([出典](URL), Tier X)

### 注意事項・落とし穴
- <既知の問題や注意点>

### 推奨アクション
1. <具体的な次のステップ>
2. <具体的な次のステップ>

### 情報の鮮度
- 調査日: <date>
- 最も古いソース: <date/URL>
- 鮮度に関する注意: <該当があれば>

### Sources
| # | Title | URL | Tier | Note |
|---|-------|-----|------|------|
| 1 | <タイトル> | <URL> | S | <簡潔な説明> |
| 2 | <タイトル> | <URL> | A | <簡潔な説明> |
| 3 | <タイトル> | <URL> | B | <簡潔な説明> |
```

### レポートの原則

- **公式情報を先に、非公式を後に**配置する
- 矛盾する情報がある場合は両方記載し、どちらが信頼できるか判断を添える
- 「わからなかった」ことも明示する。沈黙より正直な不明が価値がある
- URL は必ず含める。WebSearch の結果から得た URL をそのまま記載する
- Tier 評価を付けることで、読者が信頼度を即座に判断できるようにする

## 守ること

- 検索前に目的を明文化する
- 公式ドキュメントを必ず最初に当たる
- 情報源の信頼性を常に意識する
- 古い情報には日付を明記して警告する
- 出典のない主張は書かない
- 検索結果が不十分でも、無理に情報を捏造しない
- レポートは簡潔に。10 件以上のソースを並べるより、3-5 件の高信号ソースを深く読む
