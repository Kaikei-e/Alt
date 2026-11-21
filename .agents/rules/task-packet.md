# Agent Task Packet & GUARD-01 Rules

- **GUARD-01 (No Dangerous Bulk Autofix)**: Never run logic-mutating bulk autofix commands (`golangci-lint --fix`, `cargo clippy --fix`, `cargo fix`, `ruff check --unsafe-fixes`, `biome check --write --unsafe`). Fix targeted lint findings explicitly and re-verify via `./scripts/lint/run.sh <target>`.
- **Foreground Execution Only**: Never run verification or build commands in the background (`&`, `nohup`). Execute every command synchronously in the foreground and observe its exit status.
- **Work-Bounded Scope (Never Time-Bounded)**: Bound agent work by `allowed_paths`, `max_files` (recommended ≤ 10, hard cap 25), `max_functions` (recommended ≤ 3, hard cap 20), and explicit `allowed_rules` (wildcards `*` and `ALL` are forbidden). Do not run repo-wide builds or `./scripts/lint/run.sh all` when `forbid_repo_wide_test_build: true`. Stop and emit a handoff when bounded work completes or hits its cap.
- **Mandatory Three-Item Hand-off**: Every handoff must explicitly report:
  1. Modified files with added/deleted line counts and modified function count.
  2. Potential runtime behavior impact (`has_change`, checked dimensions such as `timeout`, `error_message`, `permissions`, `type_conversion`, `default_value`, `control_flow`, and an explicit confirmation summary even when unchanged).
  3. Change rationale (write `"変更なし"` explicitly when unchanged; never leave blank or use placeholders like `TBD`, `N/A`, or `-`) plus deterministic verification command and exit code.
- **Validation Gate**: Validate packets and handoffs with `python3 scripts/agent-harness/task_packet.py validate-packet <path>`, `validate-handoff <handoff> --packet <packet>`, and `check-templates`. Canonical spec: `docs/best_practices/agent-task-packet-standard.md`.
