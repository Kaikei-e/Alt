---
name: alt-adr-writer
description: Alt の Architecture Decision Record を日本語で `docs/ADR/NNNNNN.md` に起票・確定する（2 段階ライフサイクル: 実装前ドラフト `status: proposed` → 実装・テスト green 後の確定昇格 `status: accepted`）。6 桁の番号採番、frontmatter（title/date/status/tags/affected_services/aliases/supersedes/depends-on）、Context・Decision・Consequences の書き分け、`[[000NNN]]` wikilink、OSS 公開向けの情報衛生を扱う。本番コード編集の着手前（ADR 先行起票・`adr-preflight` ゲート通過時）、または実装・テスト green 後に `proposed` を `accepted` へ昇格させるとき、あるいはユーザが「ADR書いて」「ADRにまとめて」「ADRを起票して」「ADRを確定して」「実装が終わったのでドキュメントに」と言ったときに使う。障害の事後分析には postmortem-writer を使う（ADR = 決定の記録、postmortem = 障害の記録）。
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: "[決定の対象] [--draft | --finalize <NNNNNN> | --only-docs]"
---

# Alt ADR Writer

Alt の ADR は **2 段階ライフサイクル（Phase 1 実装前ドラフト `proposed` → Phase 2 テスト green 後の確定昇格 `accepted`）** で運用する（[[001032]]）。本番コード編集前の既定モードは **Phase 1（`--draft`）** である。このスキルはデプロイをしない（§5）。

### ワークフロー

#### Phase 1: `--draft`（実装前起票 — 本番コード編集前の既定）

- [ ] 1. 現行契約確認: 変更スコープの binding ADR を `docdag context <id> --depth 2 --budget 1500` で確認する。既存 binding ADR と矛盾・置換する場合は **ここで作業を止め、ユーザ承認を得る**
- [ ] 2. 採番: `docdag new "<ADR タイトル>" --dry-run --format json` で空き番号とパスを取得
- [ ] 3. ドラフト執筆: `docs/ADR/template.md` の見出し順に従い、`status: proposed` / `## Status\n\nProposed` で `docs/ADR/NNNNNN.md` を作成。**`depends-on` または `supersedes` に記載する各 ID は、題名や他者（人間・エージェント）の要約だけで選ばず、辺を追加する前に必ず 1 本ずつ `docdag context <id>`（必要に応じて `--section Consequences` 等）で `Decision` を読むこと**。読んだ結果、依存先の決定と新 ADR の間に緊張関係があれば `## Context` または `## Decision` に明記する（既存 ADR 置換予定時は本文に `[[000NNN]]` を明記し、frontmatter の `supersedes:` はまだ省略する — §2.2 参照）
- [ ] 4. 検証: `docdag validate --touching docs/ADR/NNNNNN.md`（findings が 0 になるまで修正して再検証）
- [ ] 5. 実装へ進む: 検証済みの `docs/ADR/NNNNNN.md` が作業ツリーに存在すれば `alt-harness preflight` ゲートは自動通過する

#### Phase 2: `--finalize`（実装・テスト green 後の確定昇格）

- [ ] 1. 実装確認（Prove It Works）: 対象サービスのテスト・リントをすべて green にする（§1。落ちていたら昇格しない）
- [ ] 2. 本文・結果の更新: 実装で確定した事実、計測値、Consequences（Pros / Cons）をドラフトに反映
- [ ] 3. 確定昇格: frontmatter を `status: proposed` → `status: accepted`、本文 `## Status` を `Proposed` → `Accepted` へ更新。承認済みの置換対象があれば、**このタイミングで** frontmatter に `supersedes: ["000NNN"]` を追加し、旧 ADR `docs/ADR/000NNN.md` の frontmatter `status` を `superseded` に同時更新する
- [ ] 4. 検証: `docdag validate --touching docs/ADR/NNNNNN.md` および `docdag validate --immutable-since origin/main` を実行（findings 0 を確認）
- [ ] 5. コミット: コードと確定済み ADR（および旧 ADR の `status: superseded` 更新）を同一コミットに記録（英語 1 行メッセージ、push はしない）
- [ ] 6. 完了報告: パス、タイトル、テスト結果、validate 結果を報告（§4）

> `--only-docs`、または「ADR だけ書いて」「docs だけ」と言われた場合は実装確認をスキップして Phase 1 の手順で作成し、既定で確定済み決定なら `status: accepted`（提案段階なら `status: proposed`）で保存・検証する。

## §1. 実装確認（Phase 2 `--finalize` 必須 / `--draft`・`--only-docs` はスキップ）

`status: accepted` の ADR は「動いた状態」を固定する現行契約（binding ADR）なので、確定昇格前に必ずテストを green にする（**Prove It Works**）。コンテナの再ビルド・再起動はしない。

| 変更の種類 | 回すコマンド |
|---|---|
| Go service | `cd alt-backend/app && go test ./...`（alt-backend / harvester / notifier / datahub 一括）または各サービスディレクトリで `go test ./...` |
| Rust service | `cargo test` |
| TypeScript / Svelte (alt-frontend-sv) | `cd alt-frontend-sv && bun run check && bun run test` |
| Python (news-creator) | `cd news-creator/app && uv run pytest` |
| Python (tag-generator) | `cd tag-generator/app && uv run pytest` |
| ドキュメント・scripts のみ | 該当テストだけ（例: `bash tests/scripts/run.sh` や `cd scripts/altharness && go test ./...`） |

テストが落ちていたら `accepted` へ昇格せず、原因を報告して止まる。未検証の計画を `accepted` にすると `docdag query --binding` の現行契約集合が汚染される。

## §2. ADR 執筆ルール

### 2.1 番号とテンプレート

```bash
docdag new "<ADR タイトル>" --dry-run --format json
# → { "schema_version": 1, "id": "000991", "path": "docs/ADR/000991.md", "exists": false, "rewrites": [] }
```

`--dry-run` は次の空き番号を計算するだけで何も書かない。返ってきた `id` / `path` をそのまま使う（ディレクトリを `ls` して数えない）。
ファイル本体は `docs/ADR/template.md` を Read で開き、そのセクション見出しをそのまま使って Write する（`docdag new` に実際にファイルを書かせない — 内蔵の Go template プレースホルダは Alt の `template.md` と形式が違う）。

### 2.2 Frontmatter・許可タグ・`supersedes` の `status_drift` 規則

frontmatter を組む前に [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) を Read し、各フィールドの規則と 29 個の許可タグ一覧に従う。要点:
- `status`: Phase 1（実装前ドラフト）は **`proposed`**（本文 `## Status` は `Proposed`）、Phase 2（テスト green 後の確定昇格）で **`accepted`**（本文は `Accepted`）。後継なしの撤回は `withdrawn`
- `aliases`: `ADR-NNN` と `ADR-000NNN` の両方必須
- `supersedes` / `depends-on`: 該当しないときはキーごと省略（空 stub は `empty_edge` エラー）。**辺を張る前の必須確認（[[001035]]）**: `depends-on` または `supersedes` に記載するすべての ID は、辺を追加する前に必ず 1 本ずつ `docdag context <id>`（必要に応じて `docdag context <id> --section Consequences` 等）で `Decision` 本文を直接読むこと。ADR の題名や他者の要約だけを根拠に辺を追加してはならない。依存先の決定と新 ADR の内容に緊張関係・制約が見つかった場合は、その関係を `## Context` または `## Decision` に明記する

**重要（`docdag.yaml` の `status_drift` 規則と既存 binding ADR の置換手順）:**
`docdag.yaml` の `status_drift` 規則は、inbound `supersedes:` エッジを持つ ADR の `status` が `superseded` でない限り**常にエラー**となる（新 ADR が `status: proposed` であっても例外ではない）。したがって既存の binding ADR（例: `000NNN`）を置換する場合は必ず次の 3 段階を守る:

1. **着手前に停止してユーザ承認を得る**: 既存の binding ADR と矛盾する変更を独断で始めない。
2. **Phase 1（`status: proposed` ドラフト段階）**: 置換対象を本文で `[[000NNN]]` として明記し、**frontmatter から `supersedes:` を省略する**（ドラフト段階で旧 binding ADR を `superseded` に変えない）。
3. **Phase 2（`status: accepted` 昇格段階）**: テスト green 確認後、新 ADR の frontmatter に `supersedes: ["000NNN"]` を追加し、**同一コミットで**旧 ADR `docs/ADR/000NNN.md` の frontmatter を `status: superseded` に更新する（本文でも `[[000NNN]]` を参照すること — `alt-harness adr-gate` 要件）。

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

## §3. 書き込み・検証・commit

Write / Edit ツールで `docs/ADR/NNNNNN.md` を作成・更新し（heredoc や `cat >` 禁止）、必ず検証する:

```bash
# Phase 1（ドラフト作成後）および Phase 2（確定昇格後）
docdag validate --touching docs/ADR/NNNNNN.md

# Phase 2（確定昇格後・コミット前の追加検証: 既確定 ADR の不変条件確認）
docdag validate --immutable-since origin/main
```

エラーが出た場合は [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) §3 の DocDag エラー分類を参照し、findings が 0 になるまで修正と再検証を繰り返す。

Phase 2 完了時、ADR とコード（および承認済み置換対象の旧 ADR `status: superseded` 更新）は同じ commit にまとめ、英語 1 行メッセージで `git commit` する。`Co-Authored-By` は付けない。`git push` はユーザの明示指示があるまで行わない。

## §4. 完了報告

- 書いた／昇格した ADR のパス（`docs/ADR/NNNNNN.md`）、タイトル、現在の `status`（`proposed` / `accepted`）
- 緑だったテスト（Phase 2 の場合: どのサービスで何を回したか）
- `docdag validate --touching docs/ADR/NNNNNN.md`（および Phase 2 では `--immutable-since origin/main`）の結果
- 次に目を向けておく指標や運用フォロー（あれば 1 行）

## §5. デプロイを求められた場合

「ADR 書いて」はデプロイの許可ではない。`./scripts/deploy.sh` や `c2quay` を独断で実行しない。
ユーザが明示的にデプロイを指示した場合のみ `docs/runbooks/deploy.md` に従う（DB マイグレーションが絡む場合は必ず `migrate → deploy` の順）。

## 参照

- [references/frontmatter-and-validation.md](references/frontmatter-and-validation.md) — frontmatter 仕様・許可タグ・`status_drift` 規則・DocDag エラー一覧
- `docs/ADR/template.md` — セクションと frontmatter のソース。§2.1 で必ず Read する
- `docs/runbooks/deploy.md` ([[deploy]]) — §5 でデプロイを指示されたときだけ読む
- `docs/runbooks/pact-broker-ops.md` ([[pact-broker-ops]]) — Broker 運用が ADR の対象になったとき
- `docs/CLAUDE.md` / `docdag.yaml` — vault 全体の編集ルールと DAG 検証設定
