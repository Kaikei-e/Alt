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
# → { "schema_version": 1, "id": "000991", "path": "docs/ADR/000991.md", "exists": false, "rewrites": [] }
```

`--dry-run` は次の空き番号を計算するだけで何も書かない。返ってきた `id` / `path` をそのまま使う（ディレクトリを `ls` して数えない）。
ファイル本体は `docs/ADR/template.md` を Read で開き、そのセクション見出しをそのまま使って Write する（`docdag new` に実際にファイルを書かせない — 内蔵の Go template プレースホルダは Alt の `template.md` と形式が違う）。

### 2.2 Frontmatter と許可タグ

Step 3 で frontmatter を組む前に [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) を Read し、各フィールドの規則（`title`, `date`, `status`, `tags`, `affected_services`, `aliases`, `supersedes`, `depends-on`）と 29 個の許可タグ一覧に従う。要点:
- `status`: 原則 `accepted`（後継なしの撤回は `withdrawn`）
- `aliases`: `ADR-NNN` と `ADR-000NNN` の両方必須
- `supersedes` / `depends-on`: 該当しないときはキーごと省略（空 stub は `empty_edge` エラー）

### 2.3 本文ルール

- **日本語で書く。** サービス名 / コマンド / ライブラリ名 / ファイルパスは英語のまま
- **`## Status` → `## Context` → `## Decision` → `## Consequences` の 4 見出しは `docdag.yaml` の `sections:` で強制されている**（改名・翻訳・順序変更・省略禁止）
- **Context**: なぜこの決定が必要だったかを定量/定性の根拠とともに。障害や計測結果は数値を残す
- **Decision**: 採用案に加え、**検討した代替案と却下理由**を書く
- **Consequences**: Pros と Cons/Tradeoffs を分けて列挙。未解決の負債は Cons に書く
- **Related ADRs は wikilink `[[000NNN]] タイトル` 形式**で列挙する（`ADR-000NNN (タイトル)` 形式は不可）

### 2.4 情報衛生

Alt は OSS として公開されている。以下を含めない:
- 本番 IP / 本番ドメイン / 秘匿ポート / 資格情報・API キー・シークレット類
- プライベートリポジトリの内部識別子（デプロイ workflow 名 / job 名 / 変数名 / ゲート条件等、`docs/CLAUDE.md` 参照）
- 社内・個人的なサーバー名 / ハードウェア構成・GPU モデル名・VRAM 容量 / 個人名・組織名（公開コントリビューターを除く）

`localhost:XXXX` と compose サービス名は OK。振る舞い（何が起きるか、運用者が何をすべきか）で記述する。

### 2.5 書き込み・検証・commit

Write ツールで `docs/ADR/NNNNNN.md` を作成し（heredoc や `cat >` 禁止）、必ず検証する:

```bash
docdag validate --touching docs/ADR/NNNNNN.md
```

エラーが出た場合は [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) §3 の DocDag エラー分類を参照し、findings が 0 になるまで修正と再検証を繰り返す。置換対象の旧 ADR がある場合は同じ commit で `status: superseded` に更新する。

ADR とコードは同じ commit にまとめ、英語 1 行メッセージで `git commit` する。`Co-Authored-By` は付けない。`git push` はユーザの明示指示があるまで行わない。

## §3. 完了報告

- 書いた ADR のパス（`docs/ADR/NNNNNN.md`）とタイトル
- 緑だったテスト（どのサービスで何を回したか）
- `docdag validate --touching docs/ADR/NNNNNN.md` の結果
- 次に目を向けておく指標や運用フォロー（あれば 1 行）

## §4. デプロイを求められた場合

「ADR 書いて」はデプロイの許可ではない。`./scripts/deploy.sh` や `c2quay` を独断で実行しない。
ユーザが明示的にデプロイを指示した場合のみ `docs/runbooks/deploy.md` に従う（DB マイグレーションが絡む場合は必ず `migrate → deploy` の順）。

## 参照

- [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) — Step 3/4 で読む frontmatter 仕様・許可タグ・DocDag エラー一覧
- `docs/ADR/template.md` — セクションと frontmatter のソース。§2.1 で必ず Read する
- `docs/runbooks/deploy.md` ([[deploy]]) — §4 でデプロイを指示されたときだけ読む
- `docs/runbooks/pact-broker-ops.md` ([[pact-broker-ops]]) — Broker 運用が ADR の対象になったとき
- `docs/CLAUDE.md` / `docdag.yaml` — vault 全体の編集ルールと DAG 検証設定
