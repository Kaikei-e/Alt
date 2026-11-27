---
packet_version: "1.0"
packet_id: "PKT-API-005"
title: "Echo v5 移行時の BodyLimit 1M (1,000,000 bytes) 境界値維持"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "migration"
behavior_preserving: true
scope:
  target: "api-bootstrap"
  allowed_paths:
    - "internal/bootstrap/**"
  forbidden_paths:
    - "scripts/lint/run.sh"
    - "test_characterization.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "echo-v5-body-limit"
  stop_condition: "BodyLimit が 1,000,000 bytes 境界を保持することを確認して Hand-off を作成し終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh api-bootstrap"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "d112b8d49e87dc18a8ba5388773094d6909796c17a8b48542ddf3c54b7809b7c"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-API-005

## 1. 範囲と作業量制約

- **対象ターゲット**: `api-bootstrap`
- **編集許可パス**: `internal/bootstrap/**`
- **編集禁止パス**: `scripts/lint/run.sh`, `test_characterization.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `echo-v5-body-limit`
- **要件**: `internal/bootstrap/server.go` を事前に読んだうえで、リクエストボディ上限が 10 進 1M（1,000,000 bytes）の境界を保ち、`1 << 20`（1,048,576 bytes）へドリフトしないことを維持する。
- **停止条件**: 境界値の整合確認完了または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `golangci-lint --fix` などの一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh api-bootstrap
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`default_value`, `type_conversion`, `control_flow` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
