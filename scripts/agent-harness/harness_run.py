#!/usr/bin/env python3
"""Durable run, decision log (LOOP-03), subagent capture (EXEC-03), and parallelism (LOOP-06).

Subcommands:
  init --policy <fix-existing|block-new> --goal <str> --completion <str> [--force]
  log --phase <str> --decision <str> --why <str> --evidence <pointer> --result <result>
  activate-packet <packet-path>
  deactivate-packet
  record-handoff <handoff-path> --result <adopted|reverted|mismatch|cancelled> [--why <str>]
  subagent-stop
  status [--format text|json] [--base-parallelism N]
"""

from __future__ import annotations

import argparse
import importlib
import json
import os
import re
import shutil
import sys
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

_task_packet = importlib.import_module("task_packet")

EXIT_OK = 0
EXIT_ERROR = 1
EXIT_USAGE = 2

TSV_COLUMNS: tuple[str, ...] = ("ts", "phase", "decision", "why", "evidence", "result")
TSV_HEADER = "\t".join(TSV_COLUMNS)
VALID_POLICIES = ("fix-existing", "block-new")
TERMINAL_RESULTS = ("adopted", "reverted", "mismatch", "cancelled")
VALID_RESULTS = (*TERMINAL_RESULTS, "info")

RUN_SUBDIRS: tuple[str, ...] = ("packets", "handoffs", "verify", "evidence")
ACTIVE_PACKET_FILENAME = "active-packet.json"
DECISIONS_FILENAME = "decisions.tsv"
SUBAGENTS_FILENAME = "subagents.jsonl"

PARALLELISM_BASE_DEFAULT = 2
PARALLELISM_WINDOW = 10
PARALLELISM_MAX = 8
PARALLELISM_MIN = 1
PARALLELISM_STEP = 2
SCALE_UP_MAX_REVERTED = 1
SCALE_DOWN_MIN_REVERTED = 3
SCALE_DOWN_MIN_MISMATCH = 2
MAX_ASSISTANT_MSG_CHARS = 4000
DETACHED_SHA_LEN = 12

PLACEHOLDER_EVIDENCE = frozenset(
    {
        "",
        "tbd",
        "n/a",
        "na",
        "-",
        "--",
        "—",
        "todo",
        "none",
        "null",
        "done",
        "ok",
        "なし",
        "無し",
        "未記入",
        "未定",
        "xxx",
        "...",
    }
)

POINTER_RE = re.compile(
    r"^(?:"
    r"#\d+"
    r"|(?:ADR-\d{6}|PM-\d{4}-\d{3}|C-\d{2,})(?:\b|$)"
    r"|commit:[0-9a-fA-F]{4,40}"
    r"|cmd:\S.*"
    r"|https?://\S+"
    r"|file://\S+"
    r"|[^\s]+/[^\s]+"
    r"|[^\s]+\.(?:py|go|rs|ts|tsx|js|jsx|svelte|sh|md|json|jsonl|tsv|toml|yaml|yml|txt|log|sql|proto)"
    r")$",
    re.IGNORECASE,
)


def _utc_now_iso() -> str:
    return datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def sanitize_branch_name(branch: str) -> str:
    """Sanitize a git branch name into a single safe directory component."""
    cleaned = re.sub(r"[^A-Za-z0-9._-]+", "-", branch.strip())
    cleaned = re.sub(r"\.{2,}", "-", cleaned)
    cleaned = re.sub(r"-{2,}", "-", cleaned)
    cleaned = cleaned.strip(".-")
    return cleaned or "default"


def resolve_repo_root(
    override: str | Path | None = None,
    *,
    start_dir: str | Path | None = None,
) -> Path:
    """Resolve repository root without spawning subprocesses."""
    if override is not None:
        return Path(override).resolve()
    env_root = os.environ.get("ALT_HARNESS_REPO_ROOT", "").strip()
    if env_root:
        return Path(env_root).resolve()

    probe_dir = Path(start_dir).resolve() if start_dir is not None else Path.cwd().resolve()
    for candidate in (probe_dir, *probe_dir.parents):
        if (candidate / ".git").exists():
            return candidate

    claude_dir = os.environ.get("CLAUDE_PROJECT_DIR", "").strip()
    if claude_dir:
        p = Path(claude_dir).resolve()
        if p.is_dir():
            return p

    return Path(__file__).resolve().parents[2]


def _read_git_head_text(repo_root: Path) -> str | None:
    dot_git = repo_root / ".git"
    if dot_git.is_dir():
        head_file = dot_git / "HEAD"
    elif dot_git.is_file():
        try:
            first_line = dot_git.read_text(encoding="utf-8", errors="replace").splitlines()[0]
        except (OSError, IndexError):
            return None
        if not first_line.startswith("gitdir:"):
            return None
        raw_gitdir = first_line.split(":", 1)[1].strip()
        git_dir = Path(raw_gitdir)
        if not git_dir.is_absolute():
            git_dir = (repo_root / git_dir).resolve()
        head_file = git_dir / "HEAD"
    else:
        return None

    try:
        return head_file.read_text(encoding="utf-8", errors="replace").strip()
    except OSError:
        return None


def resolve_git_branch(repo_root: Path, override: str | None = None) -> str:
    """Resolve current branch name directly from .git/HEAD without subprocesses."""
    if override is not None and override.strip():
        return override.strip()
    env_branch = os.environ.get("ALT_HARNESS_BRANCH", "").strip()
    if env_branch:
        return env_branch

    head_text = _read_git_head_text(repo_root)
    if not head_text:
        return "default"
    if head_text.startswith("ref: refs/heads/"):
        return head_text.removeprefix("ref: refs/heads/").strip() or "default"
    if head_text.startswith("ref: "):
        return head_text.removeprefix("ref: ").strip() or "default"
    return f"detached-{head_text[:DETACHED_SHA_LEN]}"


def run_dir_for_branch(repo_root: Path, branch: str | None = None) -> Path:
    """Return <repo_root>/.harness/runs/<sanitized_branch> with containment verification."""
    runs_base = (repo_root / ".harness" / "runs").resolve()
    raw_branch = resolve_git_branch(repo_root, override=branch)
    safe_branch = sanitize_branch_name(raw_branch)
    target = (runs_base / safe_branch).resolve()
    if not target.is_relative_to(runs_base):
        msg = f"resolved run directory escapes .harness/runs: {target}"
        raise ValueError(msg)
    return target


def active_packet_path(repo_root: Path, branch: str | None = None) -> Path:
    env_pkt = os.environ.get("ALT_ACTIVE_PACKET", "").strip()
    if env_pkt:
        return Path(env_pkt)
    return run_dir_for_branch(repo_root, branch=branch) / ACTIVE_PACKET_FILENAME


def _read_active_packet_json(pkt_path: Path) -> tuple[dict[str, Any] | None, str | None]:
    try:
        data = json.loads(pkt_path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        return None, f"malformed active packet JSON ({pkt_path}): {exc}"
    if not isinstance(data, dict):
        return None, f"active packet JSON must be an object ({pkt_path})"
    return (data if data.get("active") is True else None), None


def load_active_packet(
    repo_root: Path,
    branch: str | None = None,
) -> tuple[dict[str, Any] | None, str | None]:
    """Load active-packet.json if present and active=true.

    Returns (packet_dict_or_None, error_message_or_None).
    Malformatted or symlink-escaping active-packet.json returns an error (fail-closed).
    """
    env_pkt = os.environ.get("ALT_ACTIVE_PACKET", "").strip()
    pkt_path = Path(env_pkt) if env_pkt else active_packet_path(repo_root, branch=branch)

    if not pkt_path.exists() and not pkt_path.is_symlink():
        return None, None
    if pkt_path.is_symlink():
        return None, f"active packet file must not be a symlink: {pkt_path}"

    if not env_pkt:
        try:
            if not pkt_path.resolve().is_relative_to(repo_root.resolve()):
                return None, f"active packet file escapes repository root: {pkt_path}"
        except OSError as exc:
            return None, f"could not resolve active packet path: {exc}"

    return _read_active_packet_json(pkt_path)


def validate_tsv_field(name: str, value: str) -> str:
    """Validate a single TSV column contains no tabs/newlines and is non-empty."""
    if not isinstance(value, str):
        msg = f"field '{name}' must be a string"
        raise TypeError(msg)
    if "\t" in value or "\n" in value or "\r" in value:
        msg = f"field '{name}' must not contain tabs or newlines"
        raise ValueError(msg)
    cleaned = value.strip()
    if not cleaned:
        msg = f"field '{name}' must not be empty"
        raise ValueError(msg)
    return cleaned


def is_valid_evidence_pointer(evidence: str) -> bool:
    """Return True if evidence looks like a concrete pointer rather than free prose."""
    cleaned = evidence.strip()
    if cleaned.lower() in PLACEHOLDER_EVIDENCE:
        return False
    return POINTER_RE.match(cleaned) is not None


@dataclass(frozen=True)
class DecisionEntry:
    phase: str
    decision: str
    why: str
    evidence: str
    result: str


def _ensure_decisions_file(run_dir: Path) -> Path:
    run_dir.mkdir(parents=True, exist_ok=True)
    decisions_path = run_dir / DECISIONS_FILENAME
    if not decisions_path.exists():
        decisions_path.write_text(TSV_HEADER + "\n", encoding="utf-8")
    return decisions_path


def append_decision_row(run_dir: Path, entry: DecisionEntry) -> Path:
    """Validate and append one decision row to decisions.tsv."""
    c_phase = validate_tsv_field("phase", entry.phase)
    c_decision = validate_tsv_field("decision", entry.decision)
    c_why = validate_tsv_field("why", entry.why)
    c_evidence = validate_tsv_field("evidence", entry.evidence)
    if not is_valid_evidence_pointer(c_evidence):
        msg = (
            f"evidence '{c_evidence}' must be a concrete pointer "
            "(file path, commit:<sha>, cmd:<command>, #<seq>, ADR/PM/C id, or URL)"
        )
        raise ValueError(msg)
    if entry.result not in VALID_RESULTS:
        msg = f"invalid result '{entry.result}'; expected one of {', '.join(VALID_RESULTS)}"
        raise ValueError(msg)

    decisions_path = _ensure_decisions_file(run_dir)
    row = "\t".join((_utc_now_iso(), c_phase, c_decision, c_why, c_evidence, entry.result))
    with decisions_path.open("a", encoding="utf-8") as handle:
        handle.write(row + "\n")
    return decisions_path


def read_decision_rows(run_dir: Path) -> list[dict[str, str]]:
    """Parse decisions.tsv rows into dicts."""
    decisions_path = run_dir / DECISIONS_FILENAME
    if not decisions_path.is_file():
        return []
    lines = decisions_path.read_text(encoding="utf-8", errors="replace").splitlines()
    if not lines:
        return []
    rows: list[dict[str, str]] = []
    start_idx = 1 if lines[0].strip() == TSV_HEADER else 0
    for line in lines[start_idx:]:
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) < len(TSV_COLUMNS):
            parts.extend([""] * (len(TSV_COLUMNS) - len(parts)))
        rows.append(dict(zip(TSV_COLUMNS, parts[: len(TSV_COLUMNS)], strict=True)))
    return rows


def compute_recommended_parallelism(
    outcomes: list[str],
    *,
    base: int = PARALLELISM_BASE_DEFAULT,
) -> dict[str, Any]:
    """Compute LOOP-06 recommended parallelism from terminal packet outcomes.

    Rules:
      - Start from `base` (default 2, clamped to [1, 8]).
      - Inspect the last 10 terminal outcomes (`adopted`, `reverted`, `mismatch`, `cancelled`).
      - If `reverted >= 3` or `mismatch >= 2`: `-2`, floored at `1`.
      - Else if `adopted > 0` and `reverted <= 1` and `mismatch == 0`: `+2`, capped at `8`.
      - Otherwise hold steady (`delta = 0`).
    """
    clamped_base = max(PARALLELISM_MIN, min(PARALLELISM_MAX, base))
    terminal = [r for r in outcomes if r in TERMINAL_RESULTS]
    window = terminal[-PARALLELISM_WINDOW:]
    counts = {kind: sum(1 for item in window if item == kind) for kind in TERMINAL_RESULTS}

    if not window:
        delta = 0
        recommended = clamped_base
        reason = "no terminal packet outcomes recorded yet; holding base parallelism"
    elif (
        counts["reverted"] >= SCALE_DOWN_MIN_REVERTED
        or counts["mismatch"] >= SCALE_DOWN_MIN_MISMATCH
    ):
        delta = -PARALLELISM_STEP
        recommended = max(PARALLELISM_MIN, clamped_base + delta)
        reason = (
            f"last {len(window)} outcomes had reverted={counts['reverted']}, "
            f"mismatch={counts['mismatch']}; scaling down (-2, floor {PARALLELISM_MIN})"
        )
    elif (
        len(window) >= PARALLELISM_WINDOW
        and counts["adopted"] > 0
        and counts["reverted"] <= SCALE_UP_MAX_REVERTED
        and counts["mismatch"] == 0
    ):
        delta = PARALLELISM_STEP
        recommended = min(PARALLELISM_MAX, clamped_base + delta)
        reason = (
            f"last {len(window)} outcomes had adopted={counts['adopted']}, "
            f"reverted={counts['reverted']}, mismatch=0; scaling up (+2, cap {PARALLELISM_MAX})"
        )
    else:
        delta = 0
        recommended = clamped_base
        reason = (
            f"last {len(window)} outcomes in steady/warm-up band "
            f"(adopted={counts['adopted']}, reverted={counts['reverted']}, "
            f"mismatch={counts['mismatch']}, required_window={PARALLELISM_WINDOW}); "
            "holding parallelism"
        )

    return {
        "base": clamped_base,
        "delta": delta,
        "recommended": recommended,
        "window_size": len(window),
        "counts": counts,
        "reason": reason,
    }


def _cmd_init(args: argparse.Namespace, repo_root: Path) -> int:
    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    decisions_path = run_dir / DECISIONS_FILENAME
    if decisions_path.exists() and not args.force:
        sys.stderr.write(
            f"error: run already initialized at {decisions_path} (pass --force to reinitialize)\n"
        )
        return EXIT_ERROR

    try:
        goal = validate_tsv_field("goal", args.goal)
        completion = validate_tsv_field("completion", args.completion)
    except ValueError as exc:
        sys.stderr.write(f"error: {exc}\n")
        return EXIT_USAGE

    run_dir.mkdir(parents=True, exist_ok=True)
    for sub in RUN_SUBDIRS:
        (run_dir / sub).mkdir(parents=True, exist_ok=True)

    decisions_path.write_text(TSV_HEADER + "\n", encoding="utf-8")
    rel_decisions = decisions_path.relative_to(repo_root.resolve()).as_posix()
    append_decision_row(
        run_dir,
        DecisionEntry(
            phase="init",
            decision=f"policy={args.policy}; goal={goal}; completion={completion}",
            why="LOOP-03 run initialization: record policy and completion criteria",
            evidence=rel_decisions,
            result="info",
        ),
    )
    sys.stdout.write(f"initialized harness run at {run_dir}\n")
    return EXIT_OK


def _cmd_log(args: argparse.Namespace, repo_root: Path) -> int:
    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    try:
        decisions_path = append_decision_row(
            run_dir,
            DecisionEntry(
                phase=args.phase,
                decision=args.decision,
                why=args.why,
                evidence=args.evidence,
                result=args.result,
            ),
        )
    except (TypeError, ValueError) as exc:
        sys.stderr.write(f"error: {exc}\n")
        return EXIT_USAGE
    sys.stdout.write(f"logged decision to {decisions_path}\n")
    return EXIT_OK


def _extract_packet_id(data: dict[str, Any]) -> str:
    raw_id = data.get("packet_id")
    if not isinstance(raw_id, str) or not _task_packet.PACKET_ID_RE.match(raw_id):
        msg = f"invalid or missing packet_id: {raw_id!r}"
        raise ValueError(msg)
    return raw_id


def _load_and_validate_packet(src_path: Path) -> tuple[dict[str, Any] | None, str | None, int]:
    if not src_path.is_file():
        sys.stderr.write(f"error: packet file not found: {src_path}\n")
        return None, None, EXIT_USAGE

    try:
        parsed = _task_packet.load_document(src_path)
        schema = _task_packet.load_schema()
        issues = _task_packet.validate_packet_doc(parsed, schema)
    except (OSError, ValueError) as exc:
        sys.stderr.write(f"error: failed to parse packet {src_path}: {exc}\n")
        return None, None, EXIT_USAGE

    if issues:
        sys.stderr.write(
            f"error: packet {src_path} failed validation ({len(issues)} issue(s)):\n"
            + "\n".join(f"  [{issue.code}] {issue.path}: {issue.message}" for issue in issues)
            + "\n"
        )
        return None, None, EXIT_ERROR

    try:
        packet_id = _extract_packet_id(parsed.data)
    except ValueError as exc:
        sys.stderr.write(f"error: failed to extract packet_id from {src_path}: {exc}\n")
        return None, None, EXIT_USAGE
    return parsed.data, packet_id, EXIT_OK


def _cmd_activate_packet(args: argparse.Namespace, repo_root: Path) -> int:
    src_path = Path(args.packet_path)
    data, packet_id, rc = _load_and_validate_packet(src_path)
    if rc != EXIT_OK or data is None or packet_id is None:
        return rc

    scope = data.get("scope")
    if not isinstance(scope, dict):
        sys.stderr.write("error: packet missing 'scope' mapping\n")
        return EXIT_USAGE

    allowed_paths = scope.get("allowed_paths")
    if (
        not isinstance(allowed_paths, list)
        or not allowed_paths
        or not all(isinstance(p, str) and p.strip() for p in allowed_paths)
    ):
        sys.stderr.write("error: packet scope.allowed_paths must be a non-empty list of strings\n")
        return EXIT_USAGE

    raw_forbidden = scope.get("forbidden_paths", [])
    if not isinstance(raw_forbidden, list) or not all(isinstance(p, str) for p in raw_forbidden):
        sys.stderr.write("error: packet scope.forbidden_paths must be a list of strings\n")
        return EXIT_USAGE

    exec_policy = data.get("execution_policy")
    exec_dict = exec_policy if isinstance(exec_policy, dict) else {}

    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    packets_dir = run_dir / "packets"
    packets_dir.mkdir(parents=True, exist_ok=True)

    dest_path = packets_dir / f"{packet_id}.md"
    shutil.copyfile(src_path, dest_path)

    rel_packet = dest_path.relative_to(repo_root.resolve()).as_posix()
    active_state: dict[str, Any] = {
        "active": True,
        "packet_id": packet_id,
        "packet_file": rel_packet,
        "activated_at": _utc_now_iso(),
        "allowed_paths": list(allowed_paths),
        "forbidden_paths": list(raw_forbidden),
        "allow_generated_code_edits": bool(scope.get("allow_generated_code_edits", False)),
        "forbid_repo_wide_test_build": bool(exec_dict.get("forbid_repo_wide_test_build", True)),
        "foreground_only": bool(exec_dict.get("foreground_only", True)),
    }
    state_path = run_dir / ACTIVE_PACKET_FILENAME
    state_path.write_text(json.dumps(active_state, indent=2) + "\n", encoding="utf-8")

    append_decision_row(
        run_dir,
        DecisionEntry(
            phase="dispatch",
            decision=f"activate packet {packet_id}",
            why=f"dispatch bounded task packet (allowed_paths={len(allowed_paths)})",
            evidence=rel_packet,
            result="info",
        ),
    )
    sys.stdout.write(f"activated packet {packet_id} -> {state_path}\n")
    return EXIT_OK


def _deactivate_state_file(run_dir: Path) -> bool:
    state_path = run_dir / ACTIVE_PACKET_FILENAME
    if not state_path.is_file():
        return False
    try:
        data = json.loads(state_path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        data = {}
    if not isinstance(data, dict):
        data = {}
    data["active"] = False
    data["deactivated_at"] = _utc_now_iso()
    state_path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    return True


def _cmd_deactivate_packet(args: argparse.Namespace, repo_root: Path) -> int:
    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    changed = _deactivate_state_file(run_dir)
    if changed:
        sys.stdout.write(f"deactivated active packet in {run_dir}\n")
    else:
        sys.stdout.write(f"no active packet to deactivate in {run_dir}\n")
    return EXIT_OK


def _cmd_record_handoff(args: argparse.Namespace, repo_root: Path) -> int:
    src_path = Path(args.handoff_path)
    if not src_path.is_file():
        sys.stderr.write(f"error: handoff file not found: {src_path}\n")
        return EXIT_USAGE

    try:
        parsed = _task_packet.load_document(src_path)
        packet_id = _extract_packet_id(parsed.data)
    except (OSError, ValueError) as exc:
        sys.stderr.write(f"error: failed to parse handoff {src_path}: {exc}\n")
        return EXIT_USAGE

    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    handoffs_dir = run_dir / "handoffs"
    handoffs_dir.mkdir(parents=True, exist_ok=True)

    ext = src_path.suffix if src_path.suffix.lower() in (".md", ".json") else ".md"
    dest_path = handoffs_dir / f"{packet_id}{ext}"
    shutil.copyfile(src_path, dest_path)

    _deactivate_state_file(run_dir)

    rel_handoff = dest_path.relative_to(repo_root.resolve()).as_posix()
    why_text = args.why or f"recorded handoff outcome for {packet_id}"
    try:
        append_decision_row(
            run_dir,
            DecisionEntry(
                phase="handoff",
                decision=f"handoff {packet_id}: {args.result}",
                why=why_text,
                evidence=rel_handoff,
                result=args.result,
            ),
        )
    except (TypeError, ValueError) as exc:
        sys.stderr.write(f"error: {exc}\n")
        return EXIT_USAGE

    sys.stdout.write(f"recorded handoff {packet_id} ({args.result}) -> {dest_path}\n")
    return EXIT_OK


def _cmd_subagent_stop(args: argparse.Namespace, repo_root: Path) -> int:
    raw = sys.stdin.read()
    if not raw.strip():
        return EXIT_OK
    try:
        payload = json.loads(raw)
    except ValueError:
        return EXIT_OK
    if not isinstance(payload, dict):
        return EXIT_OK

    last_msg = payload.get("last_assistant_message")
    if isinstance(last_msg, str) and len(last_msg) > MAX_ASSISTANT_MSG_CHARS:
        last_msg = last_msg[:MAX_ASSISTANT_MSG_CHARS] + "...(truncated)"

    record: dict[str, Any] = {
        "ts": _utc_now_iso(),
        "hook_event_name": payload.get("hook_event_name", "SubagentStop"),
        "session_id": payload.get("session_id", ""),
        "agent_id": payload.get("agent_id", ""),
        "subagent_type": payload.get("subagent_type") or payload.get("agent_type") or "",
        "agent_transcript_path": payload.get("agent_transcript_path", ""),
        "cwd": payload.get("cwd", ""),
        "last_assistant_message": last_msg if isinstance(last_msg, str) else "",
    }
    run_dir = run_dir_for_branch(repo_root, branch=args.branch)
    run_dir.mkdir(parents=True, exist_ok=True)
    subagents_path = run_dir / SUBAGENTS_FILENAME
    with subagents_path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(record, ensure_ascii=False) + "\n")
    return EXIT_OK


def _summarize_run_dir(run_dir: Path, *, base_parallelism: int) -> dict[str, Any]:
    rows = read_decision_rows(run_dir)
    outcomes = [row.get("result", "") for row in rows]
    parallelism = compute_recommended_parallelism(outcomes, base=base_parallelism)

    active_file = run_dir / ACTIVE_PACKET_FILENAME
    active_packet: dict[str, Any] | None = None
    if active_file.is_file():
        try:
            loaded = json.loads(active_file.read_text(encoding="utf-8"))
            if isinstance(loaded, dict):
                active_packet = loaded
        except (OSError, ValueError):
            active_packet = {"active": False, "error": "malformed active-packet.json"}

    subagents_file = run_dir / SUBAGENTS_FILENAME
    subagents_count = 0
    if subagents_file.is_file():
        subagents_count = sum(
            1
            for line in subagents_file.read_text(encoding="utf-8", errors="replace").splitlines()
            if line.strip()
        )

    def _list_names(sub: str) -> list[str]:
        d = run_dir / sub
        if not d.is_dir():
            return []
        return sorted(p.name for p in d.iterdir() if p.is_file())

    return {
        "branch": run_dir.name,
        "path": str(run_dir),
        "decisions_count": len(rows),
        "active_packet": active_packet,
        "packets": _list_names("packets"),
        "handoffs": _list_names("handoffs"),
        "verify": _list_names("verify"),
        "evidence": _list_names("evidence"),
        "subagents_count": subagents_count,
        "parallelism": parallelism,
    }


def _cmd_status(args: argparse.Namespace, repo_root: Path) -> int:
    current_dir = run_dir_for_branch(repo_root, branch=args.branch)
    runs_base = repo_root / ".harness" / "runs"
    run_dirs: list[Path] = []
    if runs_base.is_dir():
        run_dirs = sorted(p for p in runs_base.iterdir() if p.is_dir())

    summaries = [_summarize_run_dir(d, base_parallelism=args.base_parallelism) for d in run_dirs]
    current_summary = _summarize_run_dir(current_dir, base_parallelism=args.base_parallelism)

    payload = {
        "repo_root": str(repo_root),
        "current_branch": resolve_git_branch(repo_root, override=args.branch),
        "current_run": current_summary,
        "runs": summaries,
    }

    if args.format == "json":
        sys.stdout.write(json.dumps(payload, indent=2, ensure_ascii=False) + "\n")
        return EXIT_OK

    par = current_summary["parallelism"]
    pkt = current_summary["active_packet"]
    pkt_label = (
        f"{pkt.get('packet_id')} (active)"
        if isinstance(pkt, dict) and pkt.get("active") is True
        else "none"
    )
    sys.stdout.write(
        f"Harness Run Status (branch={current_summary['branch']})\n"
        f"  active_packet : {pkt_label}\n"
        f"  decisions     : {current_summary['decisions_count']}\n"
        f"  packets       : {len(current_summary['packets'])}\n"
        f"  handoffs      : {len(current_summary['handoffs'])}\n"
        f"  subagents     : {current_summary['subagents_count']}\n"
        f"  parallelism   : {par['recommended']} (base={par['base']}, delta={par['delta']:+d}; {par['reason']})\n"
        f"  total_runs    : {len(summaries)}\n"
    )
    return EXIT_OK


def _build_parser() -> argparse.ArgumentParser:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--repo-root", type=Path, default=None, help="Repository root override")
    common.add_argument("--branch", default=None, help="Branch name override")

    parser = argparse.ArgumentParser(
        description="Manage durable .harness/runs/<branch>/ logs, packets, handoffs, and LOOP-06 parallelism."
    )
    sub = parser.add_subparsers(dest="subcommand", required=True)

    p_init = sub.add_parser("init", parents=[common], help="Initialize branch run directory")
    p_init.add_argument("--policy", required=True, choices=VALID_POLICIES)
    p_init.add_argument("--goal", required=True)
    p_init.add_argument("--completion", required=True)
    p_init.add_argument("--force", action="store_true")

    p_log = sub.add_parser("log", parents=[common], help="Append a decision row to decisions.tsv")
    p_log.add_argument("--phase", required=True)
    p_log.add_argument("--decision", required=True)
    p_log.add_argument("--why", required=True)
    p_log.add_argument("--evidence", required=True)
    p_log.add_argument("--result", required=True, choices=VALID_RESULTS)

    p_act = sub.add_parser(
        "activate-packet", parents=[common], help="Activate a TaskPacket for runtime scope guards"
    )
    p_act.add_argument("packet_path")

    sub.add_parser("deactivate-packet", parents=[common], help="Deactivate active-packet.json")

    p_ho = sub.add_parser(
        "record-handoff",
        parents=[common],
        help="Record a TaskHandoff outcome and deactivate active packet",
    )
    p_ho.add_argument("handoff_path")
    p_ho.add_argument("--result", required=True, choices=TERMINAL_RESULTS)
    p_ho.add_argument("--why", default="")

    sub.add_parser(
        "subagent-stop",
        parents=[common],
        help="SubagentStop stdin hook capturing subagent transcript pointer",
    )

    p_status = sub.add_parser(
        "status", parents=[common], help="Show run status and LOOP-06 recommended parallelism"
    )
    p_status.add_argument("--format", choices=("text", "json"), default="text")
    p_status.add_argument("--base-parallelism", type=int, default=PARALLELISM_BASE_DEFAULT)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = _build_parser()
    args = parser.parse_args(argv)
    repo_root = resolve_repo_root(args.repo_root)

    handlers = {
        "init": _cmd_init,
        "log": _cmd_log,
        "activate-packet": _cmd_activate_packet,
        "deactivate-packet": _cmd_deactivate_packet,
        "record-handoff": _cmd_record_handoff,
        "subagent-stop": _cmd_subagent_stop,
        "status": _cmd_status,
    }
    return handlers[args.subcommand](args, repo_root)


if __name__ == "__main__":
    sys.exit(main())
