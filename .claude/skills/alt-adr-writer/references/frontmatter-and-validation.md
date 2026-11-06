# ADR Frontmatter Schema, Allowed Tags & DocDag Validation

Load this reference at **Step 3** when populating frontmatter/tags and at **Step 4** when running or troubleshooting `docdag validate --touching`.

## 1. Frontmatter (§2.2)

| フィールド | 値の決め方 |
|---|---|
| `title` | 動詞始まりの行動指向の一文。ADR 番号は含めない |
| `date` | `YYYY-MM-DD`（当日） |
| `status` | 原則 `accepted`。新 ADR 自身を `superseded` にしない（置換される側の status はグラフ投影）。**取り下げ（後継 ADR を書かずに撤回）は `withdrawn`** — `superseded` にすると誰も置換していないので `superseded_orphan` エラーが永久に残る（このリポジトリでは error） |
| `tags` | 下の許可タグから最大 5 個 |
| `affected_services` | 正準のサービス名・コンポーネント識別子のみを列挙（例: `[alt-backend, knowledge-sovereign]`。言語注記 `(Go)` や変更概要の散文は含めず、変更概要は本文の `## Affected Services` セクションに書く） |
| `aliases` | `ADR-NNN` と `ADR-000NNN` の 2 形式を必ず両方入れる（Obsidian のリンク解決用） |
| `supersedes` | 本 ADR が既存 ADR の決定を**完全置換**する場合のみ、旧 ADR 番号（6 桁）を列挙。置き換えないならキーごと省略する（空の `supersedes: -` stub は `empty_edge` エラー）。新 ADR 側にだけ書き、逆辺は DocDag が算出する |
| `depends-on` | 本 ADR の決定が既存 ADR の決定を**前提にする・拡張する・部分的に修正する**場合に、旧 ADR 番号（6 桁）を列挙。本文で「〜を前提に」と書いた ADR は必ずここにも列挙する（本文と frontmatter の食い違いは書き手の責任で防ぐ）。単なる言及・背景・対比・却下した代替案は `[[000NNN]]` wikilink のみでよい。省略可、空の `depends-on: -` stub は `empty_edge` エラー |

## 2. 許可タグ (§2.4)

```
architecture, clean-architecture, connect-rpc, performance, security,
database, migration, pgbouncer, frontend, backend, api, rss, search,
caching, authentication, docker, networking, ci-cd, testing, refactoring,
bugfix, monitoring, logging, ai, rag, recap, nats, queue, 3d-graphics
```

この外のタグを増やしたくなったら、ADR ではなく `docs/CLAUDE.md` を先に更新する。

## 3. 書き込みと DocDag 検証 (§2.6)

Write ツールで `docs/ADR/NNNNNN.md` を作る（heredoc や `cat > ...` は使わない）。
書いたら必ず検証する（`supersedes` を書いたときだけではない）:

```bash
docdag validate --touching docs/ADR/NNNNNN.md
```

**validate → fix → repeat ループ:**
`docdag validate --touching docs/ADR/NNNNNN.md` を実行し、findings があれば修正して再検証を繰り返す。
`--touching` はコーパス全体を検査したうえで、そのファイルと、そこから典型 edge 1 ホップでつながる文書に関する findings だけを表示する。終了コードはコーパス全体で判定されるので、絞り込んでも壊れたリポジトリが緑になることはない。

検出されるエラー一覧（このリポジトリではすべて warning ではなく error）:
- 循環（`supersedes` と `depends-on` の和集合をまたぐ循環も含む）
- dangling 参照
- `empty_edge`（`supersedes:` / `depends-on:` と書いて中身が空）
- status ドリフト
- `missing_section` / `section_order`（`## Status` → `## Context` → `## Decision` → `## Consequences` 4 見出しの欠落・順序違い）
- `unmanaged_file`（`docs/ADR/` 直下の野良 Markdown）
- `superseded_orphan`（誰も置き換えていない文書の `status: superseded`）
- 本文の壊れた `[[000NNN]]` リンク

DocDag プラグインの `PostToolUse` フックが入っていれば、`docs/ADR/` 配下への Write のたびに同じチェックが自動で走る。fail-closed 設計なので、docdag が想定外の終了コードを返すとフック自体が exit 2 で失敗し docdag の stderr をそのまま転送する（`jq` / `docdag` が見つからない場合だけ、enforcement が OFF である旨の非ブロッキングな notice を stderr に出して exit 0）。手で回す `docdag validate --touching` はその保険で、フックが無い・OFF のときも自分で確認する。

置き換え対象の旧 ADR の `status` は同じ commit で `superseded` に揃える（status 投影の例外）。
