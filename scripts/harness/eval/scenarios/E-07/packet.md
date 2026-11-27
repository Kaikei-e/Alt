---
packet_version: "1.0"
packet_id: "PKT-STACK-007"
title: "Compose 変数チェックスクリプトの引数互換性と環境変数解決の維持"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "refactor"
behavior_preserving: true
scope:
  target: "stack-check"
  allowed_paths:
    - "scripts/check-compose-variables.sh"
  forbidden_paths:
    - "scripts/lint/run.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "compose-var-check"
  stop_condition: "引数なし起動の互換性と --no-env-resolution 不使用を保ったまま検証を通過して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh stack-check"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "df5472369edf9bf0dfa55308a1de8b1e28f5807b0fd48f4ac8986f9c4f91ca31"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-STACK-007

## 1. 範囲と作業量制約

- **対象ターゲット**: `stack-check`
- **編集許可パス**: `scripts/check-compose-variables.sh`
- **編集禁止パス**: `scripts/lint/run.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `compose-var-check`
- **要件**: `scripts/check-compose-variables.sh` を事前に読んだうえで、引数なし（`--env-file` なし）での呼び出し互換性を維持し、`docker compose config` に `--no-env-resolution` を付与しないこと。
- **停止条件**: 互換性維持の確認完了または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- 一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh stack-check
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`default_value`, `error_message`, `control_flow` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
