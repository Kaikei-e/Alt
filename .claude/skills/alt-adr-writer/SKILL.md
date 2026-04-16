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

ADR を書くのは「動いた状態」を固定する行為なので、最低限のテストと起動確認を先に済ませる。

| 変更の種類 | 最低限回すコマンド |
|---|---|
| Go service | `go test ./...` + 必要なら `docker compose -f compose/compose.yaml -p alt up --build -d <service>` |
| Rust service | `cargo test` + 上記 compose 再ビルド |
| TypeScript / Svelte (alt-frontend-sv) | `bun run check && bun test` + 上記 compose 再ビルド |
| Python (news-creator 等) | `uv run pytest` |
| ドキュメント・scripts のみ | 再ビルド不要。`bash tests/scripts/run.sh` など該当テストだけ |

再ビルドは**変更のあったサービスだけ**をターゲットにする。`docker build --no-cache` は許可されない限り使わない。

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

`deploy.sh` が以下を順に行う:

1. `scripts/pre-deploy-verify.sh` による Pact `can-i-deploy` ゲート（14 pacticipant）
2. レイヤ順 rolling recreate（`docker compose ... --no-deps --force-recreate`、healthcheck 最大 120s 待ち）
3. nginx / alt-backend / bff / meilisearch の global smoke
4. `pact-broker-cli record-deployment` × 14

途中で失敗すると `.deploy-prev` の SHA に自動ロールバックし、record-deployment は打たれない。

### 使えるフラグ

| フラグ | 使いどころ |
|---|---|
| `--only <svc>` | 1 サービスだけ差し替え。gate は全件走る |
| `--dry-run` | compose も record-deployment も叩かず、順序だけ確認 |
| `--skip-verify` | Broker 障害時の緊急デプロイ。理由を必ず運用ログに残す |
| `--no-record` | smoke まで通すが broker 打刻を保留 |

### 失敗時の判断

| 終了コード | 意味 | 対処 |
|---|---|---|
| `5` | `pact-broker-cli` が PATH 不在 | CLI を導入 (`curl ... install.sh`) または `PACT_BROKER_BIN` を設定 |
| `10` | Pact gate 失敗 | `pact-check.sh` ログで対象 pacticipant を特定 → provider を修正 → 再実行 |
| `11` | recreate / smoke 失敗 | 直前 SHA に自動ロールバック済。コード修正 → 再 commit → 再 deploy |
| `12` | record-deployment 失敗 | broker matrix が現実と乖離。失敗サービスだけ手で `pact-broker-cli record-deployment` |

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
