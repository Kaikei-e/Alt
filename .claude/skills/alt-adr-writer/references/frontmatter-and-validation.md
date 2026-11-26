# ADR Frontmatter Schema, Allowed Tags & DocDag Validation

Load this reference when populating frontmatter/tags (Phase 1 `--draft` & Phase 2 `--finalize`) and when running or troubleshooting `docdag validate --touching` / `--immutable-since`.

## 1. Frontmatter (§2.2)

| フィールド | 値の決め方 |
|---|---|
| `title` | 動詞始まりの行動指向の一文。ADR 番号は含めない |
| `date` | `YYYY-MM-DD`（当日） |
| `status` | 実装前ドラフト（Phase 1 `--draft`）は **`proposed`**（本文 `## Status` は `Proposed`）、実装・テスト green 後の確定昇格（Phase 2 `--finalize`）で **`accepted`**（本文は `Accepted`）。新 ADR 自身を `superseded` にしない（置換される側の status はグラフ投影）。**取り下げ（後継 ADR を書かずに撤回）は `withdrawn`** — `superseded` にすると誰も置換していないので `superseded_orphan` エラーが永久に残る（このリポジトリでは error） |
| `tags` | 下の許可タグから最大 5 個 |
| `affected_services` | 正準のサービス名・コンポーネント識別子のみを列挙（例: `[alt-backend, knowledge-sovereign]`。言語注記 `(Go)` や変更概要の散文は含めず、変更概要は本文の `## Affected Services` セクションに書く） |
| `aliases` | `ADR-NNN` と `ADR-000NNN` の 2 形式を必ず両方入れる（Obsidian のリンク解決用） |
| `supersedes` | 本 ADR が既存 ADR の決定を**完全置換**する場合のみ、旧 ADR 番号（6 桁）を列挙。置き換えないならキーごと省略する（空の `supersedes: -` stub は `empty_edge` エラー）。新 ADR 側にだけ書き、逆辺は DocDag が算出する。記載前に `depends-on` と同様に各 ID を `docdag context <id>`（必要に応じて `--section Consequences` 等）で直接読み `Decision` を確認すること。**重要（`status_drift` 規則）**: 下の「既存 binding ADR を置換するときの 2 段階ルール」に従い、`proposed` ドラフト段階では省略し、`accepted` 昇格時に旧 ADR の `status: superseded` と同一コミットで追加する |
| `depends-on` | 本 ADR の決定が既存 ADR の決定を**前提にする・拡張する・部分的に修正する**場合に、旧 ADR 番号（6 桁）を列挙。記載する各 ID は題名や要約だけで選ばず、辺を追加する前に必ず 1 本ずつ `docdag context <id>`（必要に応じて `--section Consequences` 等）で `Decision` を読み、新 ADR との緊張関係があれば `## Context` または `## Decision` に明記する。本文で「〜を前提に」と書いた ADR は必ずここにも列挙する（本文と frontmatter の食い違いは書き手の責任で防ぐ）。単なる言及・背景・対比・却下した代替案は `[[000NNN]]` wikilink のみでよい。省略可、空の `depends-on: -` stub は `empty_edge` エラー |

### 既存 binding ADR を置換するときの 2 段階ルール（`status_drift` 回避）

`docdag.yaml` の `status_drift` 規則は「inbound `supersedes:` エッジを持つ ADR の `status` が `superseded` でない」場合に **error** を返す。新 ADR が `status: proposed` であっても `supersedes: ["000NNN"]` を付けると、まだ `accepted` の旧 ADR `000NNN` で `status_drift` が発火する。

そのため、既存 binding ADR `000NNN` を置換する場合は必ず次を守る:

1. **承認**: コードや ADR を書く前に停止し、衝突する `000NNN` と理由をユーザへ提示して合意を得る。
2. **Phase 1（`status: proposed` ドラフト）**: 新 ADR 本文（Context / Decision / Related ADRs）で `[[000NNN]]` を明記し、**frontmatter から `supersedes:` キーを省略する**。旧 ADR `docs/ADR/000NNN.md` はまだ編集しない。
3. **Phase 2（`status: accepted` 昇格）**: 実装とテストがすべて green になった後、新 ADR に `status: accepted` と `supersedes: ["000NNN"]` を設定し、**同一コミットで** `docs/ADR/000NNN.md` の frontmatter のみ `status: superseded` へ更新する（本文の Decision は改変しない。また `alt-harness adr-gate` により `supersedes` 対象は新 ADR 本文にも `[[000NNN]]` として出現している必要がある）。

## 2. 許可タグ (§2.4)

```
architecture, clean-architecture, connect-rpc, performance, security,
database, migration, pgbouncer, frontend, backend, api, rss, search,
caching, authentication, docker, networking, ci-cd, testing, refactoring,
bugfix, monitoring, logging, ai, rag, recap, nats, queue, 3d-graphics
```

この外のタグを増やしたくなったら、ADR ではなく `docs/CLAUDE.md` を先に更新する。

## 3. 書き込みと DocDag 検証 (§3)

Write / Edit ツールで `docs/ADR/NNNNNN.md` を作成・更新する（heredoc や `cat > ...` は使わない）。
書いたら必ず検証する（`supersedes` を書いたときだけではない）:

```bash
docdag validate --touching docs/ADR/NNNNNN.md
# Phase 2（確定昇格・コミット前）では不変条件も必ず検証:
docdag validate --immutable-since origin/main
```

**validate → fix → repeat ループ:**
`docdag validate --touching docs/ADR/NNNNNN.md` を実行し、findings があれば修正して再検証を繰り返す。
`--touching` はコーパス全体を検査したうえで、そのファイルと、そこから典型 edge 1 ホップでつながる文書に関する findings だけを表示する。終了コードはコーパス全体で判定されるので、絞り込んでも壊れたリポジトリが緑になることはない。

検出されるエラー一覧（このリポジトリではすべて warning ではなく error）:
- 循環（`supersedes` と `depends-on` の和集合をまたぐ循環も含む）
- dangling 参照
- `empty_edge`（`supersedes:` / `depends-on:` と書いて中身が空）
- `status_drift`（inbound `supersedes` があるのに旧 ADR が `superseded` でない）
- `missing_section` / `section_order`（`## Status` → `## Context` → `## Decision` → `## Consequences` 4 見出しの欠落・順序違い）
- `unmanaged_file`（`docs/ADR/` 直下の野良 Markdown）
- `superseded_orphan`（誰も置き換えていない文書の `status: superseded`）
- `accepted_depends_on_withdrawn`（`accepted` ADR が `withdrawn` ADR に `depends-on` している）
- 本文の壊れた `[[000NNN]]` リンク

Claude Code では git 追跡済みの `.claude/settings.json` の `PostToolUse` フックが、`docs/ADR/*.md` 編集時に `docdag validate --touching` を、`docdag.yaml` 編集時に `docdag lint --strict --config` を自動実行し、違反時は exit `2` で差し戻す（`agy-cli` でも `Stop` フックの `agy-hook stop` が未コミットの `docs/ADR/`・`docdag.yaml` を `docdag validate` で検証する）。手で回す `docdag validate --touching` / `--immutable-since origin/main` は、フックが無い環境やコミット前の最終確認として必ず実行する。

置き換え対象の旧 ADR の `status` は Phase 2 の `accepted` 昇格と**同じ commit** で `superseded` に揃える（status 投影の例外）。
