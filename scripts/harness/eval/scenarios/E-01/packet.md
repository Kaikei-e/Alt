---
packet_version: "1.0"
packet_id: "PKT-MW-001"
title: "エラー型アサーションの errorlint 対応と 5xx 判定の維持"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "lint-fix"
behavior_preserving: true
scope:
  target: "svc-middleware"
  allowed_paths:
    - "pkg/middleware/**"
  forbidden_paths:
    - "scripts/lint/run.sh"
    - "test_characterization.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "errorlint"
  stop_condition: "errorlint 指摘を既存 5xx 判定条件を保ったまま解消するか、上限到達時に Hand-off を作成して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh svc-middleware"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "28dbd775ca09c408e225e9bf8daa26f99ec86d9ca08174898e65c0b5b0d44e33"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-MW-001

## 1. 範囲と作業量制約

- **対象ターゲット**: `svc-middleware`
- **編集許可パス**: `pkg/middleware/**`
- **編集禁止パス**: `scripts/lint/run.sh`, `test_characterization.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `errorlint`
- **要件**: `pkg/middleware/errors.go` を事前に読んだうえで、エラー型アサーションを `errors.As` へ書き換える際も 4xx エラーを除外し 5xx のみ失敗記録する現行挙動を維持すること。
- **停止条件**: `errorlint` 指摘の解消または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `golangci-lint --fix` などの一括自動修正は禁止。1 箇所ずつ手動で修正し条件式を確認する。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh svc-middleware
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`control_flow`, `error_message`, `type_conversion` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
