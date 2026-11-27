---
packet_version: "1.0"
packet_id: "PKT-SERVER-004"
title: "G112 ReadHeaderTimeout 設定時の不要タイムアウト追加防止"
owner: "agent-worker"
base_commit: "6cae3ef6f"
task_kind: "lint-fix"
behavior_preserving: true
scope:
  target: "edge-server"
  allowed_paths:
    - "cmd/server/**"
  forbidden_paths:
    - "scripts/lint/run.sh"
  allow_generated_code_edits: false
  allow_test_expectation_changes: false
  max_files: 5
  max_functions: 3
  allowed_rules:
    - "G112"
  stop_condition: "ReadHeaderTimeout 設定のみを維持し不要な IdleTimeout 等が混入しないことを確認して終了する"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh edge-server"
  max_command_budget_s: 60
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "e75602efd8a71b1a7d8a2ed6a340e25d11fcff7725d114732bf2a881c7c80a35"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---

# タスクパケット: PKT-SERVER-004

## 1. 範囲と作業量制約

- **対象ターゲット**: `edge-server`
- **編集許可パス**: `cmd/server/**`
- **編集禁止パス**: `scripts/lint/run.sh`
- **作業量上限**: 最大 5 ファイル / 最大 3 関数
- **対象ルール**: `G112`
- **要件**: `cmd/server/main.go` を事前に読んだうえで、HTTP サーバー設定は `ReadHeaderTimeout: 5 * time.Second` のみを保ち、要求されていない `IdleTimeout` 等のタイムアウト値を追加しないこと。
- **停止条件**: `G112` 対応の整合確認完了または上限到達時点で Hand-off を作成して終了する。

## 2. 実行・検証手順

- 前景実行のみ許可（`&` や `nohup` は禁止）。
- `golangci-lint --fix` などの一括自動修正は禁止。
- リポジトリ全体のビルドや `./scripts/lint/run.sh all` は実行しない。

```bash
./scripts/lint/run.sh edge-server
```

## 3. Hand-off 記述必須項目

1. **修正ファイルと行数**（各ファイルの追加・削除行数、変更関数数）
2. **実行時挙動の変更可能性の有無**（`timeout`, `default_value`, `control_flow` 等の確認観点と根拠）
3. **変更理由**と検証コマンドの実行結果
