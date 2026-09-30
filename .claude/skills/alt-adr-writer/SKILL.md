---
name: alt-adr-writer
description: Alt の Architecture Decision Record を日本語で `docs/ADR/NNNNNN.md` に書き起こす。6 桁の番号採番、frontmatter（title/date/status/tags/affected_services/aliases/supersedes/depends-on）、Context・Decision・Consequences の書き分け、`[[000NNN]]` wikilink、OSS 公開向けの情報衛生を扱う。ユーザが「ADR書いて」「ADRにまとめて」「ADRに記録して」「実装が終わったのでドキュメントに」と言ったとき、または設計判断を伴う変更が一段落したときに使う。障害の事後分析には postmortem-writer を使う（ADR = 決定の記録、postmortem = 障害の記録）。
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: "[決定の対象] [--only-docs]"
---

# Alt ADR Writer

**§1 実装確認 → §2 ADR 執筆** の順に実行する。このスキルはデプロイをしない（§4）。

### ワークフロー

- [ ] 1. 実装確認: 対象サービスのテスト実行（green を確認、または `--only-docs` でスキップ）
- [ ] 2. 採番: `docdag new "<ADR タイトル>" --dry-run --format json` で空き番号とパスを取得
- [ ] 3. 執筆: `docs/ADR/template.md` の見出し順に従い `docs/ADR/NNNNNN.md` を作成
- [ ] 4. 検証: `docdag validate --touching docs/ADR/NNNNNN.md`（エラーが出たら修正して再検証）
- [ ] 5. 旧 ADR 更新: `supersedes` 対象があれば、その旧 ADR の `status` を `superseded` に更新
- [ ] 6. コミット: コードと ADR を同一コミットに記録（英語 1 行メッセージ、push はしない）
- [ ] 7. 完了報告: パス、タイトル、テスト結果、validate 結果を報告

## §1. 実装確認

ADR は「動いた状態」を固定する記録なので、先に最低限のテストを green にする。
コンテナの再ビルド・再起動はしない。

| 変更の種類 | 回すコマンド |
|---|---|
| Go service | `cd alt-backend/app && go test ./...`（alt-backend / harvester / notifier / datahub 一括）または各サービスディレクトリで `go test ./...` |
| Rust service | `cargo test` |
| TypeScript / Svelte (alt-frontend-sv) | `cd alt-frontend-sv && bun run check && bun run test` |
| Python (news-creator) | `cd news-creator/app && uv run pytest` |
| Python (tag-generator) | `cd tag-generator/app && uv run pytest` |
| ドキュメント・scripts のみ | 該当テストだけ（例: `bash tests/scripts/run.sh`） |

テストが落ちていたら ADR は書かず、原因を報告して止まる。ADR は動いた実装の決定記録であり、
憶測を書く場所ではない。

`--only-docs`、または「ADR だけ書いて」「docs だけ」と言われた場合は §1 を飛ばして §2 へ。

## §2. ADR 執筆

### 2.1 番号とテンプレート

```bash
docdag new "<ADR タイトル>" --dry-run --format json
# → {
#     "schema_version": 1,
#     "id": "000991",
#     "path": "docs/ADR/000991.md",
#     "exists": false,
#     "rewrites": []
#   }
```

`--dry-run` は次の空き番号を計算するだけで何も書かない。返ってきた `id` / `path` をそのまま使う
（`docdag.yaml` の `filename: "{id}.md"` により、パスは常に `docs/ADR/NNNNNN.md`）。
ディレクトリを `ls` して数えない — 欠番や採番衝突を踏む。

ファイル本体は `docs/ADR/template.md` を Read で開き、そのセクション見出しをそのまま使って
Write する（勝手に増減しない）。**`docdag new` に実際にファイルを書かせない**：docdag 内蔵の
テンプレートは Go template プレースホルダ（`{{ .Title }}`）前提で、Alt の `template.md` とは
形式が違う。だから `docdag.yaml` に `template:` は設定していない。

### 2.2 Frontmatter

| フィールド | 値の決め方 |
|---|---|
| `title` | 動詞始まりの行動指向の一文。ADR 番号は含めない |
| `date` | `YYYY-MM-DD`（当日） |
| `status` | 原則 `accepted`。新 ADR 自身を `superseded` にしない（置換される側の status はグラフ投影）。**取り下げ（後継 ADR を書かずに撤回）は `withdrawn`** — `superseded` にすると誰も置換していないので `superseded_orphan` エラーが永久に残る（このリポジトリでは error） |
| `tags` | §2.4 の許可タグから最大 5 個 |
| `affected_services` | サービス名と変更概要を 1 行/件。バッククォートや `: ` を含む項目はシングルクォートで囲む（厳密 YAML） |
| `aliases` | `ADR-NNN` と `ADR-000NNN` の 2 形式を必ず両方入れる（Obsidian のリンク解決用） |
| `supersedes` | 本 ADR が既存 ADR の決定を**完全置換**する場合のみ、旧 ADR 番号（6 桁）を列挙。置き換えないならキーごと省略する（空の `supersedes: -` stub は `empty_edge` エラー）。新 ADR 側にだけ書き、逆辺は DocDag が算出する |
| `depends-on` | 本 ADR の決定が既存 ADR の決定を**前提にする・拡張する・部分的に修正する**場合に、旧 ADR 番号（6 桁）を列挙。本文で「〜を前提に」と書いた ADR は必ずここにも列挙する（本文と frontmatter の食い違いは書き手の責任で防ぐ）。単なる言及・背景・対比・却下した代替案は `[[000NNN]]` wikilink のみでよい。省略可、空の `depends-on: -` stub は `empty_edge` エラー |

### 2.3 本文ルール

- **日本語で書く。** サービス名 / コマンド / ライブラリ名 / ファイルパスは英語のまま
- **`## Status` → `## Context` → `## Decision` → `## Consequences` の 4 見出しは `docdag.yaml` の
  `sections:` で強制されている**（`missing_section` / `section_order` エラー）。改名・翻訳・
  順序変更・省略はしない。Consequences 配下の Pros / Cons/Tradeoffs、Related ADRs 等その他の
  見出しは任意で、`template.md` の並びを尊重する
- **Context**: なぜこの決定が必要だったかを定量/定性の根拠とともに。障害や計測結果は数値を残す
- **Decision**: 採用案に加え、**検討した代替案と却下理由**を書く。後から読む人に最も価値があるのはここ
- **Consequences**: Pros と Cons/Tradeoffs を分けて列挙。未解決の負債は Cons に書く
- コードブロックは判断の根拠に必要な最小限に。ロジックの羅列は GitHub の diff で読める
- **Related ADRs は wikilink `[[000NNN]] タイトル` 形式**で列挙する。Obsidian のグラフビューと
  バックリンクはこの形式でしか機能しないため、`ADR-000NNN (タイトル)` 形式は使わない

### 2.4 許可タグ

```
architecture, clean-architecture, connect-rpc, performance, security,
database, migration, pgbouncer, frontend, backend, api, rss, search,
caching, authentication, docker, networking, ci-cd, testing, refactoring,
bugfix, monitoring, logging, ai, rag, recap, nats, queue, 3d-graphics
```

この外のタグを増やしたくなったら、ADR ではなく `docs/CLAUDE.md` を先に更新する。

### 2.5 情報衛生

Alt は OSS として公開されている。以下を含めない:
- 本番 IP / 本番ドメイン / 秘匿ポート
- 資格情報・API キー・シークレット類
- プライベートリポジトリの内部識別子（デプロイ workflow 名 / job 名 / 変数名 / ゲート条件等、`docs/CLAUDE.md` §ルール「このリポジトリは public」参照）
- 社内・個人的なサーバー名
- ハードウェア構成・GPU モデル名・VRAM 容量（`docs/CLAUDE.md` §ルール「このリポジトリは public」参照）
- 個人名・組織名（公開コントリビューターを除く）

`localhost:XXXX` と compose サービス名は OK。
振る舞い（何が起きるか、運用者が何をすべきか、`docs/CLAUDE.md` §ルール「このリポジトリは public」参照）で記述する。

### 2.6 書き込みと検証

Write ツールで `docs/ADR/NNNNNN.md` を作る（heredoc や `cat > ...` は使わない）。

書いたら必ず検証する（`supersedes` を書いたときだけではない）:

```bash
docdag validate --touching docs/ADR/NNNNNN.md
```

**validate → fix → repeat ループ:**
`docdag validate --touching docs/ADR/NNNNNN.md` を実行し、findings があれば修正して再検証を繰り返す。
`--touching` はコーパス全体を検査したうえで、そのファイルと、そこから典型 edge 1 ホップで
つながる文書に関する findings だけを表示する。終了コードはコーパス全体で判定されるので、
絞り込んでも壊れたリポジトリが緑になることはない。検出されるのは循環（`supersedes` と
`depends-on` の和集合をまたぐ循環も含む）・dangling 参照・`empty_edge`（`supersedes:` /
`depends-on:` と書いて中身が空）・status ドリフト・`missing_section` / `section_order`（4 見出し
の欠落・順序違い）・`unmanaged_file`（`docs/ADR/` 直下の野良 Markdown）・`superseded_orphan`
（誰も置き換えていない文書の `status: superseded`）・本文の壊れた `[[000NNN]]` リンク。この
リポジトリでは上記すべて warning ではなく error。非ゼロ終了なら frontmatter や見出しを直す。

DocDag プラグインの `PostToolUse` フックが入っていれば、`docs/ADR/` 配下への Write のたびに
同じチェックが自動で走る。fail-closed 設計なので、docdag が想定外の終了コードを返すと
フック自体が exit 2 で失敗し docdag の stderr をそのまま転送する（`jq` / `docdag` が見つからな
い場合だけ、enforcement が OFF である旨の非ブロッキングな notice を stderr に出して exit 0）。
手で回す `docdag validate --touching` はその保険で、フックが無い・OFF のときも自分で確認する。

置き換え対象の旧 ADR の `status` は同じ commit で `superseded` に揃える（status 投影の例外）。

### 2.7 commit

ADR とコードは同じ commit にまとめ、英語 1 行メッセージで `git commit` する。
`Co-Authored-By` は付けない。`git push` はしない — push はユーザの明示指示があったときだけ、
ユーザ自身が行う。

## §3. 完了報告

- 書いた ADR のパス（`docs/ADR/NNNNNN.md`）とタイトル
- 緑だったテスト（どのサービスで何を回したか）
- `docdag validate --touching docs/ADR/NNNNNN.md` の結果
- 次に目を向けておく指標や運用フォロー（あれば 1 行）

## §4. デプロイを求められた場合

「ADR 書いて」はデプロイの許可ではない。`./scripts/deploy.sh` や `c2quay` を独断で実行しない。
ユーザが明示的にデプロイを指示した場合のみ `docs/runbooks/deploy.md` に従う。
DB マイグレーションが絡む場合は必ず `migrate → deploy` の順 — 逆にするとアプリが新スキーマを
期待したまま旧スキーマで起動し、healthcheck が通らない。

## 参照

- `docs/ADR/template.md` — セクションと frontmatter のソース。§2.1 で必ず Read する
- `docs/runbooks/deploy.md` ([[deploy]]) — §4 でデプロイを指示されたときだけ読む
- `docs/runbooks/pact-broker-ops.md` ([[pact-broker-ops]]) — Broker 運用が ADR の対象になったとき
- `docs/CLAUDE.md` — vault 全体の編集ルール
- `docdag.yaml` — ADR DAG 検証設定（セクション順序、不変条件、edge ルール）
