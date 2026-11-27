---
packet_version: "1.0"
packet_id: "PKT-QUEUE-002"
title: "優先度セマフォ acquire の分割リファクタと戻り値タプル保持"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "refactor"
behavior_preserving: true
scope:
  target: "worker-queue"
  allowed_paths:
    - "src/**"
  forbidden_paths:
    - "scripts/lint/run.sh"
    - "test_characterization.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "C901"
    - "PLR0912"
  stop_condition: "src/queue.py の分割後も通常獲得戻り値とキャンセル伝播を維持できることを確認して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh worker-queue"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "721bd84ae54f2fe29abc76cb8db3480511abdc2553e6b6d3f2ee6c4f05fc0389"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-QUEUE-002

## 1. 範囲と作業量制約

- **対象ターゲット**: `worker-queue`
- **編集許可パス**: `src/**`
- **編集禁止パス**: `scripts/lint/run.sh`, `test_characterization.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `C901`, `PLR0912`
- **要件**: `src/queue.py` を事前に読んだうえで、制御フロー分割時も正常系 `(wait_time, sid)` の返却と `CancelledError` 時のキャンセル再送出を維持すること。
- **停止条件**: 分割作業の完了と検証通過、または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `ruff check --unsafe-fixes` などの一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh worker-queue
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`control_flow`, `default_value`, `error_message` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
