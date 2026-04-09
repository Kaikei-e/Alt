---
name: alt-adr-writer
description: |
  Altプロジェクト専用のADR（Architecture Decision Record）作成スキル。
  実装完了後にコンテナの再ビルド・起動・動作確認を行い、その内容を最新のADRファイルに日本語でまとめる。
  以下のようなフレーズが出たときは必ずこのスキルを使うこと:
  - 「ADRを書いて」「ADRにまとめて」「ADRに記録して」
  - 「コンテナ再ビルドしてADR書いて」
  - 「実装まとめをADRに」「docs/ADR」への言及
  - 「〜の実装が終わったのでドキュメントに」
  コンテナ操作なしでADRだけ書く場合にも使うこと。
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
---

# Alt ADR Writer スキル

Altプロジェクトで実装が完了した際に、関連コンテナの再ビルド・起動・確認を行い、
最新のADRファイルに実装内容を日本語でまとめるスキル。

---

## 前提

- Altリポジトリのルートで作業していること（`compose/compose.yaml` が存在する場所）
- `docs/ADR/` ディレクトリにADRファイルが連番で存在すること（例: `0032-*.md`）
- `docs/ADR/template.md` が参照テンプレートとして存在すること

---

## ステップ 1: ADRテンプレートと最新ADRを確認する

```bash
# テンプレートを読む
cat docs/ADR/template.md

# 最新のADR番号とファイルを確認
ls docs/ADR/ | sort | tail -5

# 対象ADRの現在の内容を読む（番号は会話から判断、不明なら最新を使う）
cat docs/ADR/.md
```

- ADR番号は会話中で明示されていればそれを使う。なければ `ls` で最新を特定する。
- テンプレートのセクション構成・書き方を把握してから執筆に入る。

---

## ステップ 2: 関連コンテナの再ビルドと起動

実装内容から影響を受けるサービス名を特定し、ピンポイントで再ビルドする。

```bash
# 対象サービスを再ビルドして起動（サービス名は会話から判断）
docker compose -f compose/compose.yaml up --build -d

# 全サービスが対象の場合
docker compose -f compose/compose.yaml up --build -d
```

### ビルド失敗時
- エラーログを表示してユーザーに報告し、ADR執筆の前に修正を促す
- `docker compose -f compose/compose.yaml logs <service>` で詳細を確認

---

## ステップ 3: 起動確認

```bash
# コンテナの状態確認
docker compose -f compose/compose.yaml ps

# ヘルスチェック（数秒待ってから）
sleep 5 && docker compose -f compose/compose.yaml ps

# 必要に応じてログ確認
docker compose -f compose/compose.yaml logs --tail=50
```

**確認ポイント:**
- 対象サービスの `State` が `Up` または `running` になっていること
- `Exit` や `Restarting` が出ていないこと
- ヘルスチェックがある場合は `healthy` になっていること

コンテナが正常に起動したことを確認してからADR執筆に進む。
異常があればユーザーに報告して止まる。

---

## ステップ 4: ADRを日本語で執筆する

### 執筆ルール

1. **テンプレートの構成に従う** — `template.md` のセクション見出しをそのまま使う
2. **日本語で書く** — コード・コマンド・固有名詞（サービス名、ライブラリ名等）は英語のまま
3. **OSSとして公開されているため、機微な情報を一切含めない:**
   - IPアドレス・ドメイン・ポート番号の具体値（`localhost:8080` 程度はOK）
   - 認証情報・APIキー・シークレット類
   - 社内・個人的なサーバー名、インフラ固有の構成
   - 個人名・組織名（Altコントリビューターとして公開されている情報は除く）
4. **実装の意図・背景・トレードオフを重視する** — コードの羅列ではなく「なぜそう決めたか」を書く
5. **既存の記載を尊重する** — Contextセクションなど既に書かれている部分は保持・拡充する
6. **Related ADRsはwikilink形式で書く** — `[[000NNN]]` 形式を使い、説明テキストはリンクの外に書く
   - 正: `- [[000139]] Dead Letter Queue パターン導入`
   - 誤: `- ADR-000139 (Dead Letter Queue パターン導入)`
   - 理由: Obsidianのグラフビュー・バックリンクがwikilink形式でのみ機能するため

### テンプレート典型構成（template.mdの実際の内容に従うこと）

テンプレートに含まれていることが多いセクション（実際のtemplate.mdを優先）:
- **タイトル / ADR番号**
- **ステータス** (Proposed / Accepted / Deprecated / Superseded)
- **コンテキスト** — なぜこの決定が必要になったか
- **決定事項** — 何を決めたか
- **実装の概要** — どう実装したか（アーキテクチャ、主要コンポーネント）
- **影響・トレードオフ** — メリット・デメリット・今後の課題
- **関連するADR** — 参照・影響するADR番号

### 執筆後の確認チェックリスト

- [ ] テンプレートの全セクションが埋まっているか
- [ ] 機微な情報が含まれていないか
- [ ] Clean Architecture / Altの設計原則と矛盾していないか
- [ ] 日本語として自然に読めるか

### frontmatter 記入ルール

| フィールド | 型 | 説明 |
|---|---|---|
| title | text | ADR番号を除いたタイトル（動詞始まりの行動指向） |
| date | date | 作成日 YYYY-MM-DD |
| status | text | proposed / accepted / deprecated / superseded |
| tags | list | 分類タグ（下記の許可リストから選択） |
| affected_services | list | 影響サービス名（例: alt-backend, pre-processor） |
| aliases | list | Obsidianリンク用の別名。`ADR-NNN` と `ADR-000NNN` の2形式を必ず含める |

### 使用可能なタグ

architecture, clean-architecture, connect-rpc, performance, security,
database, migration, pgbouncer, frontend, backend, api, rss, search,
caching, authentication, docker, networking, ci-cd, testing, refactoring,
bugfix, monitoring, logging, ai, rag, recap, nats, queue, 3d-graphics

---

## ステップ 5: ファイルに書き込む

```bash
# 既存ファイルを上書き（バックアップ不要、Gitで管理されているため）
cat > docs/ADR/.md << 'EOF'

EOF

# 確認
cat docs/ADR/.md
```

---

## ステップ 6: 完了報告

以下をユーザーに報告する:
1. 再ビルドしたサービス名と起動状態
2. 書き込んだADRファイルのパス
3. ADRの主要セクションのサマリー（3〜5行）
4. `git diff docs/ADR/` で変更差分を表示（オプション）

---

## 注意事項

- コンテナ再ビルドが不要な場合（「ADRだけ書いて」など）はステップ2〜3をスキップしてよい
- ADR番号が会話中に明示されている場合はそれを使い、`ls`で確認しない
- 複数サービスにまたがる実装の場合、各サービスの役割を明示してADRに記載する
- Altのマイクロサービス構成（Go / TypeScript / Rust / Python）を意識した記述を心がける
