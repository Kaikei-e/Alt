---
title: エージェント向けタスクパケット標準仕様 (Task Packet Standard)
date: 2026-11-21
tags:
  - alt
---

# エージェント向けタスクパケット標準仕様 (Task Packet Standard)

Alt リポジトリにおいてコーディングエージェントへ作業を委譲する際の**機械検証可能な契約仕様（TaskPacket / TaskHandoff）**と、**GUARD-01（危険な一括 `--fix` の原則禁止）**の正規定義。

- JSON Schema (Draft 2020-12): `scripts/agent-harness/task_packet.schema.json`
- バリデータ / テンプレート CLI: `./scripts/altharness/alt-harness packet` (`scripts/altharness/internal/packet/`)
- 標準テンプレート:
  - `scripts/agent-harness/templates/task-packet.md.tmpl`
  - `scripts/agent-harness/templates/task-handoff.md.tmpl`
- 実行時エージェントルール: `.agents/rules/task-packet.md`
- ユニットテスト: `scripts/altharness/internal/packet/packet_test.go`

## 1. 設計原則（Alt の内部不変条件）

1. **時間ではなく作業量で制約する (Work-Bounded, Not Time-Bounded)**:
   - エージェントの作業境界は `allowed_paths`・`max_files`・`max_functions`・`allowed_rules` の作業量で定義する。
   - `timeout_minutes` や `duration` 等の時間制約フィールドは JSON Schema の `additionalProperties: false` により機械的に拒否する。
2. **決定論的な局所検証ゲート**:
   - 各パケットは `./scripts/lint/run.sh <target>` を含む検証コマンドを明示し、ホスト側で実施する全体テスト・全体ビルド（`./scripts/lint/run.sh all` 等）を原則禁止する。
3. **前景実行のみ許可 (Foreground-Only)**:
   - 背景実行（`&`・`nohup`）は非決定的な競合と終了状態の欠落を生むため全面禁止する。
4. **GUARD-01: ロジック改変を伴う一括自動修正 (`--fix`) の原則禁止**:
   - リンタの一括自動修正はエラー握り潰し・型変換変更・制御フロー改変を無音で混入させるため禁止し、対象ルールを明示して 1 件ずつ検証付きで修正する。
5. **監査可能な Hand-off 必須 3 項目**:
   - 完了または上限停止時には「修正ファイルと行数」「実行時挙動の変更可能性の有無」「変更理由」を必ず構造化して残す。変更が無い場合も空欄・`TBD`・`N/A`・`-` 単独を禁止し、「変更なし」を明示する。

## 2. JSON Schema 要約 (`task_packet.schema.json`)

スキーマは JSON Schema Draft 2020-12 (`$defs` 構造) で `TaskPacket` と `TaskHandoff` を定義し、全オブジェクトに `additionalProperties: false` を設定している。Go 標準ライブラリのみで検証可能。

### 2.1 `TaskPacket` フィールド定義

| フィールド | 型 / 制約 | 必須 | 説明 |
|---|---|:---:|---|
| `packet_version` | `const: "1.0"` | 必須 | スキーマバージョン |
| `packet_id` | `string` (`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`) | 必須 | パケット一意識別子 |
| `title` / `owner` | 非空文字列 | 任意 | 人間向けタイトル・担当識別子 |
| `base_commit` | `^[0-9a-f]{7,40}$` | 任意 | `verify-handoff` / `verify-sequence` の基準コミット SHA |
| `task_kind` | `"lint-fix"` / `"refactor"` / `"migration"` / `"bugfix"` / `"feature"` | 任意 | タスク種別（既定 `"lint-fix"`）。`lint-fix` では不要コメント追加とテスト期待値改変を追加禁止 |
| `behavior_preserving` | `boolean` | 任意 | 観測可能な実行時挙動を不変に保つか（`verify-sequence` 既定 `true`） |
| `scope.target` | ターゲット文字列 または 非空一意配列 | 必須 | `./scripts/lint/run.sh <target>` の検証対象 |
| `scope.allowed_paths` | 非空一意配列（`..` / 絶対パス / `\` 禁止） | 必須 | 編集許可パス / グロブ |
| `scope.forbidden_paths` | 一意配列（`..` / 絶対パス / `\` 禁止） | 任意 | 明示的に編集禁止とするパス / グロブ |
| `scope.allow_test_expectation_changes` | `boolean`（既定 `false`） | 任意 | `lint-fix` タスクでテスト期待値変更・テスト削除を明示許可するオプトイン |
| `scope.max_files` | 整数 `1..25`（推奨デフォルト `10`） | 必須 | 変更ファイル数の上限 |
| `scope.max_functions` | 整数 `1..20`（推奨デフォルト `3`） | 必須 | 変更関数数の上限 |
| `scope.allowed_rules` | 非空一意配列（`*` / `ALL` 禁止） | 必須 | 修正対象のルール ID / カテゴリ |
| `scope.stop_condition` | 非空文字列（プレースホルダ禁止） | 必須 | 上限到達時の停止・Hand-off 条件 |
| `execution_policy.foreground_only` | `const: true` | 必須 | 前景実行のみ許可 |
| `execution_policy.forbid_repo_wide_test_build` | `boolean`（デフォルト `true`） | 必須 | リポジトリ全体ビルド・全体テストの禁止 |
| `execution_policy.forbid_bulk_autofix` | `const: true` | 必須 | GUARD-01 一括自動修正の禁止 |
| `execution_policy.verification_command` | 非空文字列 | 必須 | 全 `scope.target` の `./scripts/lint/run.sh <target>` を含む検証コマンド |
| `execution_policy.max_command_budget_s` | 整数 `1..600`（既定 `180`） | 任意 | LOOP-02: ホスト検証コマンドの許容実行時間上限（秒） |
| `execution_policy.measured` | `{ duration_s>=0, exit_code, at_commit, command_sha256? }` | 必須 | LOOP-02: 委譲前にホストで実測した検証コマンド記録（未計測・上限超過・コマンド乖離は拒否） |
| `handoff_requirements.*` | 3 フラグすべて `const: true` | 必須 | 必須 3 項目の強制 |

### 2.2 `TaskHandoff` フィールド定義

| フィールド | 型 / 制約 | 必須 | 説明 |
|---|---|:---:|---|
| `packet_version` | `const: "1.0"` | 任意 | スキーマバージョン |
| `packet_id` | `PacketId` | 必須 | 対応する `TaskPacket.packet_id` |
| `commit_sha` | `^[0-9a-f]{7,40}$` | 任意 | 成果コミット SHA |
| `status` | `"completed"` / `"partial_bounded_stop"` / `"blocked"` | 必須 | 終了ステータス（`completed` 時は `verification.exit_code == 0` 必須） |
| `modified_files` | `{ path, added_lines>=0, deleted_lines>=0 }[]` | 必須 | 重複パス・`..`・グロブ不可。クロス検証で `max_files`・`allowed_paths`・`forbidden_paths` を検査 |
| `modified_functions_count` | 整数 `>= 0` | 必須 | クロス検証で `max_functions` 上限を検査 |
| `runtime_behavior_impact.has_change` | `boolean` | 必須 | 実行時挙動変更の可能性有無 |
| `runtime_behavior_impact.checked_dimensions` | 非空一意配列 | 必須 | 確認観点リスト（標準 6 観点: `timeout`, `error_message`, `permissions`, `type_conversion`, `default_value`, `control_flow`） |
| `runtime_behavior_impact.summary` | 非空文字列（プレースホルダ禁止） | 必須 | 変更影響の根拠。`has_change: false` でも「変更なし」と根拠を明記 |
| `rationale` | 非空文字列（`TBD` / `N/A` / `-` 禁止） | 必須 | 変更理由。変更が無い場合も「変更なし」を明示 |
| `verification` | `{ command: string, exit_code: int, summary?: string }` | 必須 | 実行した検証コマンドと終了コード |

## 3. GUARD-01: 危険な自動修正 (`--fix`) の原則禁止

| 禁止コマンド例 | 主なリスク | 安全な代替手順 |
|---|---|---|
| `golangci-lint run --fix` | エラー処理・型変換・defer の意味論改変 | 指摘箇所を読み手動修正し `./scripts/lint/run.sh <go-dir>` で検証 |
| `cargo clippy --fix` / `cargo fix` | 所有権・borrow・パターンマッチの意図しない書き換え | 警告ごとに意図を確認して修正し `./scripts/lint/run.sh <rust-dir>` で検証 |
| `ruff check --unsafe-fixes` | 例外捕捉・比較式・ミュータブル既定値の挙動変化 | 違反単位で修正し `./scripts/lint/run.sh <python-dir>` で検証 |
| `biome check --write --unsafe` | TypeScript / Svelte の型・評価順・副作用の変化 | 対象ルールのみ手動修正し `./scripts/lint/run.sh <fe-dir>` で検証 |

純粋なフォーマット整形（`gofmt`, `ruff format`, `cargo fmt`）は構文木と意味論を保持するため対象外とし、ロジック改変を伴うリンタ一括 `--fix` / `--unsafe` のみを GUARD-01 で禁止する。

## 4. Markdown 形式と必須見出し

`./scripts/altharness/alt-harness packet` は純粋な JSON ファイルに加え、**YAML フロントマター** または **フェンス付き ```` ```json ```` コードブロック** を持つ Markdown ファイルを検証できる。

- **TaskPacket Markdown 必須見出し（順序厳守）**:
  1. `## 1. 範囲と作業量制約`
  2. `## 2. 実行・検証手順`
  3. `## 3. Hand-off 記述必須項目`
- **TaskHandoff Markdown 必須見出し（順序厳守）**:
  1. `## 1. 修正ファイルと行数`
  2. `## 2. 実行時挙動の変更可能性の有無`
  3. `## 3. 変更理由`
- **本文ポリシー検査**: TaskPacket 本文やシェルコードブロック内に GUARD-01 禁止コマンドの肯定指示や背景実行 `&` / `nohup` の指示が含まれている場合は検証エラーとなる（禁止を明記する注意書きは許容する）。

## 5. CLI 検証コマンドと CI 配線

```bash
# 標準テンプレートの出力（packet / handoff）
./scripts/altharness/alt-harness packet render-template --kind packet --packet-id TASK-001 --target scripts
./scripts/altharness/alt-harness packet render-template --kind handoff --packet-id TASK-001

# LOOP-02: パケットの検証コマンドをホスト実測して execution_policy.measured へ記録
./scripts/altharness/alt-harness packet measure path/to/task-packet.md

# パケット単体検証（JSON / Markdown 両対応、--format json 可。未計測・予算超過・コマンド乖離を拒否）
./scripts/altharness/alt-harness packet validate-packet path/to/task-packet.md

# Hand-off 単体検証およびパケットとのクロス検証（max_files / max_functions / allowed_paths / packet_id）
./scripts/altharness/alt-harness packet validate-handoff path/to/handoff.md --packet path/to/task-packet.md

# LOOP-01: Git 実差分・AST 変更関数数・検証コマンド再実行・CHECK-01・lint-fix 厳格検査による実地検証
./scripts/altharness/alt-harness packet verify-handoff path/to/handoff.md --packet path/to/task-packet.md --base-commit <sha>

# VERIFY-01: テスト先行コミット順検証（behavior-preserving 既定、--bugfix で RED→GREEN 検証）
./scripts/altharness/alt-harness packet verify-sequence --base-commit <sha> --test-command "<cmd>"

# ホスト最終統合ゲート（worktree hygiene + 対象 lint + repo lint + 任意テスト + verify-handoff + git status）
./scripts/verify/host-final-check.sh --target scripts --handoff path/to/handoff.md --packet path/to/task-packet.md --base-commit <sha>

# スキーマと同梱テンプレートの自己検証
./scripts/altharness/alt-harness packet check-templates
```

CI では `./scripts/lint/run.sh harness`（`./scripts/altharness/alt-harness gate harness`）が `./scripts/altharness/alt-harness packet check-templates` と `go -C scripts/altharness test -race ./internal/packet` を自動実行し、スキーマ・テンプレート・バリデータの整合性を継続検証する。
