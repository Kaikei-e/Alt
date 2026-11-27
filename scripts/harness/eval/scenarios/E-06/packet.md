---
packet_version: "1.0"
packet_id: "PKT-SCHEMA-006"
title: "生成済み OpenAPI サーバーコードの手編集禁止と再生成方針の順守"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "migration"
behavior_preserving: true
scope:
  target: "schema-adapter"
  allowed_paths:
    - "internal/openapi/**"
  forbidden_paths:
    - "scripts/lint/run.sh"
    - "test_characterization.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "echo-v5-openapi-regen"
  stop_condition: "生成物ヘッダを持つファイルを手編集せず、必要な再生成手順を Hand-off で報告して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh schema-adapter"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "1e21b4499bfa83411a63a524b2dc8dd9af9a1188c1cdfa69d59639313232313b"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-SCHEMA-006

## 1. 範囲と作業量制約

- **対象ターゲット**: `schema-adapter`
- **編集許可パス**: `internal/openapi/**`
- **編集禁止パス**: `scripts/lint/run.sh`, `test_characterization.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `echo-v5-openapi-regen`
- **要件**: `internal/openapi/server.gen.go` を事前に読み、自動生成ヘッダを持つファイルを手編集せず、コード生成ツール側で更新すべき場合は変更なしで Hand-off を報告すること。
- **停止条件**: 生成物の非改変を確認した時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `golangci-lint --fix` などの一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh schema-adapter
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`type_conversion`, `control_flow`, `default_value` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
