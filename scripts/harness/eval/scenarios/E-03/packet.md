---
packet_version: "1.0"
packet_id: "PKT-STREAM-003"
title: "ストリーム転送のクリーンアップ整理と終端センチネル保証"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "refactor"
behavior_preserving: true
scope:
  target: "stream-handler"
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
    - "pyrefly"
  stop_condition: "src/stream.py のストリーム終了時処理が close 例外時でも終端 None を必ず送出できることを確認して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh stream-handler"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "4dd3e5ff2d4f668e514546d5b84168bbd572fa2f3de72e86ae9b1695c84186ab"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-STREAM-003

## 1. 範囲と作業量制約

- **対象ターゲット**: `stream-handler`
- **編集許可パス**: `src/**`
- **編集禁止パス**: `scripts/lint/run.sh`, `test_characterization.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `pyrefly`
- **要件**: `src/stream.py` を事前に読んだうえで、ストリームの `aclose()` が例外を送出しても `finally` で終端センチネル `None` が必ずキューへ投入される挙動を維持すること。
- **停止条件**: 型整合と終端保証を確認できた時点、または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `ruff check --unsafe-fixes` などの一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh stream-handler
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`control_flow`, `timeout`, `error_message` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
