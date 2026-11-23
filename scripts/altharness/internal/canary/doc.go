// Package canary is the Go port of scripts/lint/canary_audit.py and
// scripts/lint/entrypoint_canary.py.
//
// RunAudit verifies isolated scanner canaries, rejects zero-file scans, and
// checks scripts/lint/canary_ledger.toml against ./scripts/lint/run.sh --list.
// RunEntrypoint stages one target per language family, injects a violation,
// and runs the staged scripts/lint/run.sh. A zero-file scan is a failure.
package canary
