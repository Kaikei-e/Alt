---
name: alt-adr-writer
description: Alt の Architecture Decision Record を日本語で `docs/ADR/NNNNNN.md` に書き起こす。6 桁の番号採番、frontmatter（title/date/status/tags/affected_services/aliases/supersedes）、Context・Decision・Consequences の書き分け、`[[000NNN]]` wikilink、OSS 公開向けの情報衛生を扱う。ユーザが「ADR書いて」「ADRにまとめて」「ADRに記録して」「実装が終わったのでドキュメントに」と言ったとき、または設計判断を伴う変更が一段落したときに使う。障害の事後分析には postmortem-writer を使う（ADR = 決定の記録、postmortem = 障害の記録）。
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: "[決定の対象] [--only-docs]"
---

# Alt ADR Writer

**§1 実装確認 → §2 ADR 執筆** の順に実行する。このスキルはデプロイをしない（§4）。

## §1. 実装確認

ADR は「動いた状態」を固定する記録なので、先に最低限のテストを green にする。
コンテナの再ビルド・再起動はしない。

| 変更の種類 | 回すコマンド |
|---|---|
| Go service | `go test ./...`（alt-backend / harvester / datahub は `alt-backend/app` で一括） |
| Rust service | `cargo test` |
| TypeScript / Svelte (alt-frontend-sv) | `bun run check && bun test` |
| Python (news-creator 等) | `uv run pytest` |
| ドキュメント・scripts のみ | 該当テストだけ（例: `bash tests/scripts/run.sh`） |

テストが落ちていたら ADR は書かず、原因を報告して止まる。ADR は動いた実装の決定記録であり、
憶測を書く場所ではない。

`--only-docs`、または「ADR だけ書いて」「docs だけ」と言われた場合は §1 を飛ばして §2 へ。

## §2. ADR 執筆

### 2.1 番号とテンプレート

```bash
ls docs/ADR/ | sort | tail -1     # 最新番号を確認
```

最新 +1 の 6 桁ゼロ埋め（`000750` → `000751`）をファイル名にする。`docs/ADR/template.md` を Read で
開き、そのセクション見出しをそのまま使う（勝手に増減しない）。

### 2.2 Frontmatter

| フィールド | 値の決め方 |
|---|---|
| `title` | 動詞始まりの行動指向の一文。ADR 番号は含めない |
| `date` | `YYYY-MM-DD`（当日） |
| `status` | 原則 `accepted`。新 ADR 自身を `superseded` にしない（置換される側の status はグラフ投影） |
| `tags` | §2.4 の許可タグから最大 5 個 |
| `affected_services` | サービス名と変更概要を 1 行/件。バッククォートや `: ` を含む項目はシングルクォートで囲む（厳密 YAML） |
| `aliases` | `ADR-NNN` と `ADR-000NNN` の 2 形式を必ず両方入れる（Obsidian のリンク解決用） |
| `supersedes` | 本 ADR が既存 ADR を**完全置換**する場合のみ、旧 ADR 番号（6 桁）を列挙。置き換えないならキーごと省略する（空の `supersedes: -` stub は dangling 判定を汚す）。新 ADR 側にだけ書き、逆辺は DocDag が算出する |

### 2.3 本文ルール

- **日本語で書く。** サービス名 / コマンド / ライブラリ名 / ファイルパスは英語のまま
- **セクション順は `template.md` を尊重する**（Context / Decision / Consequences (Pros, Cons/Tradeoffs) /
  Related ADRs が基本）
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

Alt は OSS として公開されている。本番 IP / 本番ドメイン / 秘匿ポート、資格情報・API キー・
シークレット、社内・個人的なサーバー名、個人名・組織名（公開コントリビューターを除く）は
含めない。`localhost:XXXX` と compose サービス名は OK。

### 2.6 書き込みと検証

Write ツールで `docs/ADR/NNNNNN.md` を作る（heredoc や `cat > ...` は使わない）。

`supersedes` を書いた場合は `docdag validate` を実行し、循環・dangling・空 stub・status ドリフトが
無いことを確認する（非ゼロ終了なら frontmatter を直す）。置き換え対象の旧 ADR の `status` は
同じ commit で `superseded` に揃える（status 投影の例外）。

### 2.7 commit

ADR とコードは同じ commit にまとめ、英語 1 行メッセージで `git commit` する。
`Co-Authored-By` は付けない。`git push` はしない — push はユーザの明示指示があったときだけ、
ユーザ自身が行う。

## §3. 完了報告

- 書いた ADR のパス（`docs/ADR/NNNNNN.md`）とタイトル
- 緑だったテスト（どのサービスで何を回したか）
- `docdag validate` の結果（`supersedes` を書いた場合）
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
- `docs/CLAUDE.md` — vault 全体の編集ルール。許可タグを増やすときに更新する
