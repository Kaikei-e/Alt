---
name: alt-adr-writer
description: Writes an Architecture Decision Record for the Alt project in Japanese after a completed implementation, then runs the Pact-gated manual deploy (./scripts/deploy.sh production). Trigger when the user says "ADR書いて" / "ADRにまとめて" / "ADRに記録して" / "実装が終わったのでドキュメントに" / "コンテナ再ビルドしてADR書いて" / "docs/ADR" 関連のまとめ依頼, or after finishing code changes that clearly warrant a decision record. Skip the deploy step only when the user explicitly says "ADRだけ書いて"; skip the build step only for documentation-only changes.
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
---

# Alt ADR Writer

このスキルは 3 つのフェーズを順に実行する:

1. **実装確認** (§1) — 動作と green を担保してから書く
2. **ADR 執筆** (§2) — `docs/ADR/NNNNNN.md` を日本語で追加する
3. **Pact ゲート付きデプロイ** (§3) — `./scripts/deploy.sh production`

各フェーズはユーザ依頼の範囲に応じてスキップ条件がある (§5 参照)。

---

## §1. 実装確認

ADR を書くのは「動いた状態」を固定する行為なので、最低限のテストで動作確認を先に済ませる。コンテナの再ビルド・再起動は行わない（本番反映は §3 の `scripts/deploy.sh` 側に集約する）。

| 変更の種類 | 最低限回すコマンド |
|---|---|
| Go service | `go test ./...` |
| Rust service | `cargo test` |
| TypeScript / Svelte (alt-frontend-sv) | `bun run check && bun test` |
| Python (news-creator 等) | `uv run pytest` |
| ドキュメント・scripts のみ | 該当テストだけ（例: `bash tests/scripts/run.sh`） |

テストが落ちていたら ADR は書かず、ユーザに原因を報告して止まる。ADR は「動いた実装の決定記録」であり、憶測を書く場所ではない。

---

## §2. ADR 執筆

### 2.1 番号とテンプレート

```bash
ls docs/ADR/ | sort | tail -1     # 最新番号を確認
```

最新 +1 の 6 桁ゼロ埋め（例: `000750` → `000751`）をファイル名にする。`docs/ADR/template.md` を Read で開き、そのセクション見出しをそのまま使う（勝手に増減しない）。

### 2.2 Frontmatter

| フィールド | 値の決め方 |
|---|---|
| `title` | 動詞始まりの行動指向の一文。ADR 番号は含めない |
| `date` | `YYYY-MM-DD`（当日） |
| `status` | 原則 `accepted`。過去 ADR を無効化する場合のみ `superseded` |
| `tags` | §2.4 の許可タグから最大 5 個 |
| `affected_services` | サービス名と変更概要を 1 行/件で列挙 |
| `aliases` | `ADR-NNN` と `ADR-000NNN` の 2 形式を必ず両方入れる（Obsidian リンク解決用） |

### 2.3 本文ルール

- **日本語で書く**。サービス名 / コマンド / ライブラリ名 / ファイルパスは英語のまま。
- **セクション順は `template.md` を尊重**する。Context / Decision / Consequences (Pros, Cons/Tradeoffs) / Related ADRs の順が基本。
- **Context** は「なぜこの決定が必要だったか」を定量/定性の根拠とともに書く。障害や計測結果があれば数値を残す。
- **Decision** は採用した選択肢に加え、**検討した代替案と却下理由**を書く。これが後から読む人への最大の贈り物になる。
- **Consequences** は Pros と Cons/Tradeoffs を分けて列挙する。未解決の負債は Cons に書く。
- コードブロックは判断の根拠に必要な最小限にする。ロジックの羅列は GitHub の diff で読めるので省く。
- **Related ADRs は wikilink `[[000NNN]] タイトル` 形式**で列挙する。Obsidian のグラフビュー / バックリンクがこの形式でのみ機能するため、`ADR-000NNN (タイトル)` 形式は使わない。

### 2.4 許可タグ

```
architecture, clean-architecture, connect-rpc, performance, security,
database, migration, pgbouncer, frontend, backend, api, rss, search,
caching, authentication, docker, networking, ci-cd, testing, refactoring,
bugfix, monitoring, logging, ai, rag, recap, nats, queue, 3d-graphics
```

この外のタグを増やしたくなったら ADR ではなく `docs/CLAUDE.md` を先に更新する。

### 2.5 情報衛生

Alt は OSS として公開されている。以下を含めない:

- 本番 IP / 本番ドメイン / 秘匿ポート
- 資格情報・API キー・シークレット類
- 社内・個人的なサーバー名
- 個人名・組織名（公開コントリビューターとして記録されているものを除く）

`localhost:XXXX` と compose サービス名は OK。

### 2.6 書き込み

Write ツールで `docs/ADR/NNNNNN.md` を作る。heredoc や `cat > ...` は使わない。書き込み後に Read で自分の出力を読み返し、見出し / frontmatter / wikilink 形式を確認する。

---

## §3. Pact ゲート付きデプロイ

ADR を書いたら**コードと ADR を同じ commit にまとめて**、`scripts/deploy.sh` を手で叩く。CI 自動発火はしない（方針: [[000740]] / [[deploy]]）。

```bash
git add -A
git commit -m "<英語の 1 行メッセージ>"     # Co-Authored-By は付けない
./scripts/deploy.sh production
```

`deploy.sh` は [c2quay](https://github.com/Kaikei-e/c2quay) の薄いラッパで、以下を順に叩く:

1. `scripts/pact-check.sh --broker` — Pact file を Broker に publish
2. `c2quay deploy --env production --config c2quay.yml` — can-i-deploy → サービス反映 → smoke → record-deployment を c2quay が内部で実行する
3. `scripts/record-remote-pacticipant.sh production` — 別ホストの tts-speaker 用

途中で失敗すれば `set -e` で即停止する。自動ロールバックは無い。復旧は `git revert` → 再 commit → `./scripts/deploy.sh production` を再実行。内部手順の詳細や緊急時の手当てはスキルでは扱わず `docs/runbooks/deploy.md` に委譲する。

### 使えるフラグとサブコマンド

| やりたいこと | コマンド |
|---|---|
| 現状確認 (副作用なし) | `c2quay verify --env production --config c2quay.yml` |
| デプロイ計画の確認 | `c2quay deploy --env production --dry-run --config c2quay.yml` |
| 1 サービスだけ再 recreate | `c2quay deploy --env production --service <svc> --config c2quay.yml` |
| broker matrix の現状 | `c2quay status --env production --config c2quay.yml` |

`--skip-verify` / `--no-record` は廃止済み。Broker が不稼働なら先に復旧してから deploy を再実行する（復旧手順は `docs/runbooks/pact-broker-ops.md`）。

### 失敗時の判断

| 段 | 兆候 | 対処 |
|---|---|---|
| pact-check | 出力に `contract regression` | provider/consumer テストを修正 → 再 commit → 再 deploy |
| c2quay: can-i-deploy | `blocked by` のログ | 対象 pacticipant の provider 側を修正 → 再 deploy |
| c2quay: サービス反映 | `container … not healthy` | healthcheck 修正・依存関係見直し → 再 deploy |
| c2quay: smoke | `smoke FAIL: <url>` | 該当サービスのログを確認 |
| c2quay: record-deployment | `record-deployment failed` | broker matrix が乖離。`c2quay status` と `pact-broker-cli` で確認し、手で record-deployment を再実行 |
| record-remote-pacticipant | `tts-speaker record-deployment failed` | 別 GPU ホスト・Broker 到達性を確認後、`scripts/record-remote-pacticipant.sh production` を再実行 |

### DB マイグレーションが絡む場合

必ず `migrate → deploy` の順。逆にするとアプリが新スキーマを期待するまま旧スキーマで起動し、healthcheck が通らず自動ロールバックで戻される:

```bash
cd migrations-atlas && atlas migrate hash && atlas migrate apply --env production
cd ~/alt && ./scripts/deploy.sh production
```

---

## §4. 完了報告

ユーザに以下を伝える:

- 書いた ADR のパス（`docs/ADR/NNNNNN.md`）とタイトル
- 緑だったテスト / 再ビルド / healthcheck
- `deploy.sh` の終了コード + `.deploy-current` に記録された SHA
- 次に目を向けておく指標や運用フォロー（あれば 1 行）

---

## §5. スキップ条件

| ユーザ発話 | §1 | §2 | §3 |
|---|---|---|---|
| 「ADR だけ書いて」「docs だけ」 | skip | run | skip |
| 「実装まとめて ADR 書いて」「ADR 書いてデプロイして」 | run | run | run |
| ドキュメント / scripts のみの変更 | skip build (test は run) | run | run |

迷ったら `§1 → §2 → §3` の全実行を既定とする。Alt の運用は「ADR を書く = デプロイ準備完了」という前提で組まれている。

---

## 参照

- `docs/ADR/template.md` — セクションと frontmatter のソース
- `docs/runbooks/deploy.md` ([[deploy]]) — デプロイ手順の完全版
- `docs/runbooks/pact-broker-ops.md` ([[pact-broker-ops]]) — Broker 運用
- `docs/CLAUDE.md` — vault 全体の編集ルール
