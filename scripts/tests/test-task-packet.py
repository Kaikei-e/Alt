#!/usr/bin/env python3
"""TDD unit and CLI tests for scripts/agent-harness/task_packet.py and its schema/templates.

Run:
    python3 scripts/tests/test-task-packet.py
"""

from __future__ import annotations

import copy
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "agent-harness" / "task_packet.py"
SCHEMA_PATH = ROOT / "scripts" / "agent-harness" / "task_packet.schema.json"
PACKET_TMPL_PATH = ROOT / "scripts" / "agent-harness" / "templates" / "task-packet.md.tmpl"
HANDOFF_TMPL_PATH = ROOT / "scripts" / "agent-harness" / "templates" / "task-handoff.md.tmpl"
SPEC_DOC_PATH = ROOT / "docs" / "best_practices" / "agent-task-packet-standard.md"
RULE_DOC_PATH = ROOT / ".agents" / "rules" / "task-packet.md"
HOST_FINAL_CHECK_PATH = ROOT / "scripts" / "verify" / "host-final-check.sh"
CORPUS_DIR = ROOT / "scripts" / "harness" / "corpus"

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2
MAX_FILES_DEFAULT = 10
MAX_FUNCTIONS_DEFAULT = 3
SHA256_HEX_LEN = 64


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        RESULTS.failed += 1


def _run_proc(
    argv: list[str],
    *,
    cwd: pathlib.Path | None = None,
    check_rc: bool = False,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 -- fixed argv test helper invoking repo CLI or git/bash
        argv,
        cwd=str(cwd) if cwd is not None else None,
        capture_output=True,
        text=True,
        check=check_rc,
    )


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return _run_proc([sys.executable, str(SCRIPT_PATH), *args])


def _cmd_sha(cmd: str) -> str:
    return hashlib.sha256(cmd.strip().encode("utf-8")).hexdigest()


def _set_verify_cmd(pkt: dict[str, Any], cmd: str) -> None:
    pkt["execution_policy"]["verification_command"] = cmd
    measured = pkt["execution_policy"].get("measured")
    if isinstance(measured, dict):
        measured["command_sha256"] = _cmd_sha(cmd)


def base_packet() -> dict[str, Any]:
    verify_cmd = "./scripts/lint/run.sh scripts"
    return {
        "packet_version": "1.0",
        "packet_id": "SPEC-04-SAMPLE",
        "scope": {
            "target": "scripts",
            "allowed_paths": [
                "scripts/agent-harness/**",
                "scripts/lint/{harness,other}.sh",
            ],
            "forbidden_paths": ["scripts/agent-harness/harness_selfcheck.py"],
            "max_files": MAX_FILES_DEFAULT,
            "max_functions": MAX_FUNCTIONS_DEFAULT,
            "allowed_rules": ["S101", "PTH123"],
            "stop_condition": "対象作業量の上限到達または全ルール解消時点で handoff を作成し終了する",
        },
        "execution_policy": {
            "foreground_only": True,
            "forbid_repo_wide_test_build": True,
            "forbid_bulk_autofix": True,
            "verification_command": verify_cmd,
            "max_command_budget_s": 180,
            "measured": {
                "duration_s": 1.25,
                "exit_code": 0,
                "at_commit": "0123456",
                "command_sha256": _cmd_sha(verify_cmd),
            },
        },
        "handoff_requirements": {
            "require_modified_files_and_lines": True,
            "require_runtime_behavior_impact": True,
            "require_change_rationale": True,
        },
    }


def base_handoff() -> dict[str, Any]:
    return {
        "packet_id": "SPEC-04-SAMPLE",
        "status": "completed",
        "modified_files": [
            {
                "path": "scripts/agent-harness/example.py",
                "added_lines": 12,
                "deleted_lines": 4,
            }
        ],
        "modified_functions_count": 2,
        "runtime_behavior_impact": {
            "has_change": False,
            "checked_dimensions": [
                "timeout",
                "error_message",
                "permissions",
                "type_conversion",
                "default_value",
                "control_flow",
            ],
            "summary": "変更なし（静的検査の型注釈と pathlib 置換のみで実行時挙動は同一）",
        },
        "rationale": "対象ルール S101/PTH123 解消のため最小限の静的リファクタを実施。仕様変更なし。",
        "verification": {
            "command": "./scripts/lint/run.sh scripts",
            "exit_code": 0,
        },
    }


def write_json(tmp: pathlib.Path, filename: str, payload: dict[str, Any]) -> pathlib.Path:
    path = tmp / filename
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
    return path


def valid_packet_markdown(extra_body: str = "") -> str:
    return (
        "---\n"
        'packet_version: "1.0"\n'
        'packet_id: "MD-PKT-01"\n'
        "scope:\n"
        '  target: "scripts"\n'
        "  allowed_paths:\n"
        '    - "scripts/agent-harness/**"\n'
        "  max_files: 5\n"
        "  max_functions: 3\n"
        "  allowed_rules:\n"
        '    - "G112"\n'
        '    - "errorlint"\n'
        '  stop_condition: "作業量上限に到達したら handoff を出力して終了する"\n'
        "execution_policy:\n"
        "  foreground_only: true\n"
        "  forbid_repo_wide_test_build: true\n"
        "  forbid_bulk_autofix: true\n"
        '  verification_command: "./scripts/lint/run.sh scripts"\n'
        "  max_command_budget_s: 180\n"
        "  measured:\n"
        "    duration_s: 1.5\n"
        "    exit_code: 0\n"
        '    at_commit: "0123456"\n'
        f'    command_sha256: "{_cmd_sha("./scripts/lint/run.sh scripts")}"\n'
        "handoff_requirements:\n"
        "  require_modified_files_and_lines: true\n"
        "  require_runtime_behavior_impact: true\n"
        "  require_change_rationale: true\n"
        "---\n\n"
        "# タスクパケット MD-PKT-01\n\n"
        "## 1. 範囲と作業量制約 (Bounded Scope)\n"
        "- 対象: `scripts/agent-harness/**`（最大 5 ファイル / 3 関数）\n\n"
        "## 2. 実行・検証手順\n"
        "- 前景実行のみ許可（背景 `&` 禁止）。\n"
        "- `golangci-lint --fix`、`cargo clippy --fix`、`ruff check --unsafe-fixes`、"
        "`biome check --write --unsafe` などの一括自動修正は禁止。\n"
        "- 検証コマンド:\n"
        "```bash\n"
        "python3 scripts/tests/test-task-packet.py && ./scripts/lint/run.sh scripts 2>&1\n"
        "```\n"
        f"{extra_body}\n"
        "## 3. Hand-off 記述必須項目\n"
        "1. 修正ファイルと行数\n"
        "2. 実行時挙動の変更可能性の有無\n"
        "3. 変更理由\n"
    )


def valid_handoff_markdown(packet_id: str = "MD-PKT-01") -> str:
    return (
        "---\n"
        f'packet_id: "{packet_id}"\n'
        'status: "completed"\n'
        "modified_files:\n"
        '  - path: "scripts/agent-harness/task_packet.py"\n'
        "    added_lines: 20\n"
        "    deleted_lines: 5\n"
        "modified_functions_count: 2\n"
        "runtime_behavior_impact:\n"
        "  has_change: false\n"
        "  checked_dimensions:\n"
        '    - "timeout"\n'
        '    - "error_message"\n'
        '    - "permissions"\n'
        '    - "type_conversion"\n'
        '    - "default_value"\n'
        '    - "control_flow"\n'
        '  summary: "変更なし（境界チェックと型注釈の確認済み）"\n'
        'rationale: "変更なし（対象 lint ルール準拠のみ）"\n'
        "verification:\n"
        '  command: "./scripts/lint/run.sh scripts"\n'
        "  exit_code: 0\n"
        "---\n\n"
        f"# Hand-off 報告 {packet_id}\n\n"
        "## 1. 修正ファイルと行数\n"
        "- `scripts/agent-harness/task_packet.py` (+20 / -5)\n\n"
        "## 2. 実行時挙動の変更可能性の有無\n"
        "- なし: timeout / error_message / permissions / type_conversion / default_value / "
        "control_flow を確認し変更なし。\n\n"
        "## 3. 変更理由\n"
        "- 変更なし（対象ルール準拠のため最小差分のみ適用）\n"
    )


def test_schema_structure() -> None:
    sys.stdout.write("\n[1] JSON Schema structure\n")
    check("task_packet.schema.json exists", SCHEMA_PATH.is_file(), str(SCHEMA_PATH))
    if not SCHEMA_PATH.is_file():
        return
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    check(
        "schema uses Draft 2020-12",
        schema.get("$schema") == "https://json-schema.org/draft/2020-12/schema",
        str(schema.get("$schema")),
    )
    defs = schema.get("$defs", {})
    check("schema defines TaskPacket in $defs", "TaskPacket" in defs)
    check("schema defines TaskHandoff in $defs", "TaskHandoff" in defs)
    if "TaskPacket" in defs and "TaskHandoff" in defs:
        check(
            "TaskPacket forbids additionalProperties",
            defs["TaskPacket"].get("additionalProperties") is False,
        )
        check(
            "TaskHandoff forbids additionalProperties",
            defs["TaskHandoff"].get("additionalProperties") is False,
        )
        scope_def = defs.get("Scope", {})
        check(
            "Scope forbids additionalProperties (blocks time-based fields)",
            scope_def.get("additionalProperties") is False,
        )


def test_valid_packets(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[2] Valid TaskPacket inputs\n")
    pkt_json = write_json(tmp, "valid_packet.json", base_packet())
    res = run_cli("validate-packet", str(pkt_json))
    check("valid JSON packet passes", res.returncode == EXIT_OK, res.stderr or res.stdout)

    res_json = run_cli("validate-packet", str(pkt_json), "--format", "json")
    parsed = json.loads(res_json.stdout) if res_json.stdout.strip() else {}
    check(
        "validate-packet --format json returns valid=true",
        res_json.returncode == EXIT_OK and parsed.get("valid") is True,
        res_json.stdout or res_json.stderr,
    )

    multi = base_packet()
    multi["scope"]["target"] = ["scripts", "harness"]
    _set_verify_cmd(
        multi, "./scripts/lint/run.sh scripts && LINT_BASE=HEAD~1 ./scripts/lint/run.sh harness"
    )
    multi_path = write_json(tmp, "valid_multi_target.json", multi)
    res_multi = run_cli("validate-packet", str(multi_path))
    check(
        "valid packet with multi-target list passes",
        res_multi.returncode == EXIT_OK,
        res_multi.stderr or res_multi.stdout,
    )

    md_path = tmp / "valid_packet.md"
    md_path.write_text(valid_packet_markdown(), encoding="utf-8")
    res_md = run_cli("validate-packet", str(md_path))
    check(
        "valid Markdown packet with YAML frontmatter and prohibition prose passes",
        res_md.returncode == EXIT_OK,
        res_md.stderr or res_md.stdout,
    )

    json_block_md = tmp / "valid_packet_json_block.md"
    json_block_md.write_text(
        "# タスクパケット\n\n"
        "```json\n"
        f"{json.dumps(base_packet(), ensure_ascii=False, indent=2)}\n"
        "```\n\n"
        "## 1. 範囲と作業量制約\n- 対象: scripts\n\n"
        "## 2. 実行・検証手順\n- `./scripts/lint/run.sh scripts` を前景実行する。\n\n"
        "## 3. Hand-off 記述必須項目\n- 必須3項目を記述する。\n",
        encoding="utf-8",
    )
    res_jblk = run_cli("validate-packet", str(json_block_md))
    check(
        "valid Markdown packet with JSON code block passes",
        res_jblk.returncode == EXIT_OK,
        res_jblk.stderr or res_jblk.stdout,
    )


def _assert_packet_rejected(
    tmp: pathlib.Path,
    name: str,
    mutated: dict[str, Any],
    expected_substr: str = "",
) -> None:
    slug = "".join(ch if ch.isalnum() else "_" for ch in name)[:48]
    path = write_json(tmp, f"bad_pkt_{slug}.json", mutated)
    res = run_cli("validate-packet", str(path))
    combined = (res.stdout + "\n" + res.stderr).lower()
    ok = res.returncode == EXIT_INVALID and (
        not expected_substr or expected_substr.lower() in combined
    )
    check(name, ok, f"rc={res.returncode} out={res.stdout!r} err={res.stderr!r}")


def test_invalid_packets_schema(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[3a] Invalid TaskPacket schema & scope checks\n")

    p_timeout = copy.deepcopy(base_packet())
    p_timeout["scope"]["timeout_minutes"] = 30
    _assert_packet_rejected(
        tmp,
        "rejects time-based field timeout_minutes in scope",
        p_timeout,
        "timeout_minutes",
    )

    p_ver = copy.deepcopy(base_packet())
    p_ver["packet_version"] = "2.0"
    _assert_packet_rejected(tmp, "rejects unsupported packet_version", p_ver, "packet_version")

    p_id = copy.deepcopy(base_packet())
    p_id["packet_id"] = "-bad id!"
    _assert_packet_rejected(tmp, "rejects invalid packet_id pattern", p_id, "packet_id")

    p_max_files = copy.deepcopy(base_packet())
    p_max_files["scope"]["max_files"] = 26
    _assert_packet_rejected(tmp, "rejects max_files > 25", p_max_files, "max_files")

    p_bool_files = copy.deepcopy(base_packet())
    p_bool_files["scope"]["max_files"] = True
    _assert_packet_rejected(tmp, "rejects boolean max_files", p_bool_files, "max_files")

    p_max_funcs = copy.deepcopy(base_packet())
    p_max_funcs["scope"]["max_functions"] = 21
    _assert_packet_rejected(tmp, "rejects max_functions > 20", p_max_funcs, "max_functions")

    p_traversal = copy.deepcopy(base_packet())
    p_traversal["scope"]["allowed_paths"] = ["scripts/../etc/passwd"]
    _assert_packet_rejected(tmp, "rejects .. traversal in allowed_paths", p_traversal)

    p_abs = copy.deepcopy(base_packet())
    p_abs["scope"]["allowed_paths"] = ["/etc/passwd"]
    _assert_packet_rejected(tmp, "rejects absolute path in allowed_paths", p_abs)

    p_star = copy.deepcopy(base_packet())
    p_star["scope"]["allowed_rules"] = ["*"]
    _assert_packet_rejected(tmp, "rejects wildcard '*' in allowed_rules", p_star)

    p_all = copy.deepcopy(base_packet())
    p_all["scope"]["allowed_rules"] = ["ALL"]
    _assert_packet_rejected(tmp, "rejects 'ALL' in allowed_rules", p_all)

    p_stop = copy.deepcopy(base_packet())
    p_stop["scope"]["stop_condition"] = "TBD"
    _assert_packet_rejected(tmp, "rejects placeholder stop_condition", p_stop)


def test_invalid_packets_policy(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[3b] Invalid TaskPacket execution & handoff policy checks\n")

    p_fg = copy.deepcopy(base_packet())
    p_fg["execution_policy"]["foreground_only"] = False
    _assert_packet_rejected(tmp, "rejects foreground_only=false", p_fg)

    p_fix = copy.deepcopy(base_packet())
    p_fix["execution_policy"]["forbid_bulk_autofix"] = False
    _assert_packet_rejected(tmp, "rejects forbid_bulk_autofix=false", p_fix)

    p_verify = copy.deepcopy(base_packet())
    p_verify["execution_policy"]["verification_command"] = "pytest tests/"
    _assert_packet_rejected(
        tmp,
        "rejects verification_command without ./scripts/lint/run.sh <target>",
        p_verify,
        "run.sh",
    )

    p_verify_fix = copy.deepcopy(base_packet())
    p_verify_fix["execution_policy"]["verification_command"] = (
        "golangci-lint run --fix ./... && ./scripts/lint/run.sh scripts"
    )
    _assert_packet_rejected(
        tmp,
        "rejects verification_command with golangci-lint --fix",
        p_verify_fix,
    )

    p_verify_bg = copy.deepcopy(base_packet())
    p_verify_bg["execution_policy"]["verification_command"] = "./scripts/lint/run.sh scripts &"
    _assert_packet_rejected(
        tmp,
        "rejects verification_command with background '&'",
        p_verify_bg,
    )

    p_verify_all = copy.deepcopy(base_packet())
    p_verify_all["execution_policy"]["verification_command"] = (
        "./scripts/lint/run.sh scripts && ./scripts/lint/run.sh all"
    )
    _assert_packet_rejected(
        tmp,
        "rejects repo-wide ./scripts/lint/run.sh all when forbid_repo_wide_test_build=true",
        p_verify_all,
    )

    p_ho_req = copy.deepcopy(base_packet())
    p_ho_req["handoff_requirements"]["require_change_rationale"] = False
    _assert_packet_rejected(tmp, "rejects disabled handoff requirement flag", p_ho_req)


def test_invalid_packet_markdown_body(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[4] Invalid TaskPacket Markdown headings & forbidden instructions\n")

    missing_h = tmp / "missing_heading.md"
    missing_h.write_text(
        valid_packet_markdown().replace("## 2. 実行・検証手順", "## 2. メモ"),
        encoding="utf-8",
    )
    res_mh = run_cli("validate-packet", str(missing_h))
    check(
        "rejects Markdown packet missing required heading",
        res_mh.returncode == EXIT_INVALID,
        res_mh.stderr or res_mh.stdout,
    )

    prose_fix = tmp / "prose_autofix.md"
    prose_fix.write_text(
        valid_packet_markdown(
            extra_body="- まず `golangci-lint run --fix ./...` を実行せよ。\n",
        ),
        encoding="utf-8",
    )
    res_pf = run_cli("validate-packet", str(prose_fix))
    check(
        "rejects Markdown packet affirmatively instructing golangci-lint --fix in prose",
        res_pf.returncode == EXIT_INVALID,
        res_pf.stderr or res_pf.stdout,
    )

    block_clippy = tmp / "block_clippy_fix.md"
    block_clippy.write_text(
        valid_packet_markdown(
            extra_body="```bash\ncargo clippy --fix --allow-dirty\n```\n",
        ),
        encoding="utf-8",
    )
    res_bc = run_cli("validate-packet", str(block_clippy))
    check(
        "rejects Markdown packet instructing cargo clippy --fix in shell block",
        res_bc.returncode == EXIT_INVALID,
        res_bc.stderr or res_bc.stdout,
    )

    block_ruff = tmp / "block_ruff_unsafe.md"
    block_ruff.write_text(
        valid_packet_markdown(
            extra_body="```sh\nruff check --unsafe-fixes .\n```\n",
        ),
        encoding="utf-8",
    )
    res_br = run_cli("validate-packet", str(block_ruff))
    check(
        "rejects Markdown packet instructing ruff check --unsafe-fixes in shell block",
        res_br.returncode == EXIT_INVALID,
        res_br.stderr or res_br.stdout,
    )

    block_biome = tmp / "block_biome_unsafe.md"
    block_biome.write_text(
        valid_packet_markdown(
            extra_body="```bash\nbiome check --write --unsafe .\n```\n",
        ),
        encoding="utf-8",
    )
    res_bb = run_cli("validate-packet", str(block_biome))
    check(
        "rejects Markdown packet instructing biome check --write --unsafe in shell block",
        res_bb.returncode == EXIT_INVALID,
        res_bb.stderr or res_bb.stdout,
    )

    block_bg = tmp / "block_background.md"
    block_bg.write_text(
        valid_packet_markdown(
            extra_body="```bash\n./scripts/lint/run.sh scripts &\n```\n",
        ),
        encoding="utf-8",
    )
    res_bg = run_cli("validate-packet", str(block_bg))
    check(
        "rejects Markdown packet instructing background '&' in shell block",
        res_bg.returncode == EXIT_INVALID,
        res_bg.stderr or res_bg.stdout,
    )

    inline_bg = tmp / "inline_background.md"
    inline_bg.write_text(
        valid_packet_markdown(
            extra_body="- `./scripts/lint/run.sh scripts &` でバックグラウンド実行する。\n",
        ),
        encoding="utf-8",
    )
    res_ibg = run_cli("validate-packet", str(inline_bg))
    check(
        "rejects Markdown packet instructing background '&' in prose",
        res_ibg.returncode == EXIT_INVALID,
        res_ibg.stderr or res_ibg.stdout,
    )


def _assert_handoff_rejected(
    tmp: pathlib.Path,
    name: str,
    handoff: dict[str, Any],
    packet_path: pathlib.Path | None = None,
    expected_substr: str = "",
) -> None:
    slug = "".join(ch if ch.isalnum() else "_" for ch in name)[:48]
    path = write_json(tmp, f"bad_ho_{slug}.json", handoff)
    args = ["validate-handoff", str(path)]
    if packet_path is not None:
        args.extend(["--packet", str(packet_path)])
    res = run_cli(*args)
    combined = (res.stdout + "\n" + res.stderr).lower()
    ok = res.returncode == EXIT_INVALID and (
        not expected_substr or expected_substr.lower() in combined
    )
    check(name, ok, f"rc={res.returncode} out={res.stdout!r} err={res.stderr!r}")


def test_handoff_standalone(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[5a] TaskHandoff standalone & Markdown validation\n")

    pkt_path = write_json(tmp, "cross_packet.json", base_packet())
    ho_path = write_json(tmp, "cross_handoff.json", base_handoff())

    res = run_cli("validate-handoff", str(ho_path), "--packet", str(pkt_path))
    check(
        "valid JSON handoff passes cross-validation against packet",
        res.returncode == EXIT_OK,
        res.stderr or res.stdout,
    )

    ho_brace = copy.deepcopy(base_handoff())
    ho_brace["modified_files"] = [
        {"path": "scripts/lint/harness.sh", "added_lines": 5, "deleted_lines": 0},
    ]
    ho_brace_path = write_json(tmp, "cross_handoff_brace.json", ho_brace)
    res_brace = run_cli("validate-handoff", str(ho_brace_path), "--packet", str(pkt_path))
    check(
        "handoff matching brace glob allowed_paths passes",
        res_brace.returncode == EXIT_OK,
        res_brace.stderr or res_brace.stdout,
    )

    ho_partial = copy.deepcopy(base_handoff())
    ho_partial["status"] = "partial_bounded_stop"
    ho_partial["verification"]["exit_code"] = 1
    ho_partial_path = write_json(tmp, "valid_partial_handoff.json", ho_partial)
    res_partial = run_cli("validate-handoff", str(ho_partial_path), "--packet", str(pkt_path))
    check(
        "partial_bounded_stop handoff with non-zero exit_code passes",
        res_partial.returncode == EXIT_OK,
        res_partial.stderr or res_partial.stdout,
    )

    pkt_md = tmp / "cross_packet.md"
    pkt_md.write_text(valid_packet_markdown(), encoding="utf-8")
    ho_md = tmp / "cross_handoff.md"
    ho_md.write_text(valid_handoff_markdown("MD-PKT-01"), encoding="utf-8")
    res_md = run_cli("validate-handoff", str(ho_md), "--packet", str(pkt_md))
    check(
        "valid Markdown handoff passes cross-validation against Markdown packet",
        res_md.returncode == EXIT_OK,
        res_md.stderr or res_md.stdout,
    )

    bad_ho_md = tmp / "missing_heading_handoff.md"
    bad_ho_md.write_text(
        valid_handoff_markdown("MD-PKT-01").replace("## 3. 変更理由", "## 3. 補足"),
        encoding="utf-8",
    )
    res_bmd = run_cli("validate-handoff", str(bad_ho_md))
    check(
        "rejects Markdown handoff missing required heading",
        res_bmd.returncode == EXIT_INVALID,
        res_bmd.stderr or res_bmd.stdout,
    )


def test_handoff_rejections_and_cross_validation(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[5b] TaskHandoff rejection & cross-validation checks\n")
    pkt_path = write_json(tmp, "cross_packet_rej.json", base_packet())

    for bad_token in ("", "TBD", "N/A", "-"):
        ho_r = copy.deepcopy(base_handoff())
        ho_r["rationale"] = bad_token
        _assert_handoff_rejected(tmp, f"rejects rationale={bad_token!r}", ho_r)

    ho_empty_sum = copy.deepcopy(base_handoff())
    ho_empty_sum["runtime_behavior_impact"]["summary"] = ""
    _assert_handoff_rejected(
        tmp,
        "rejects empty runtime_behavior_impact.summary even when has_change=false",
        ho_empty_sum,
    )

    ho_silent = copy.deepcopy(base_handoff())
    ho_silent["status"] = "completed"
    ho_silent["verification"]["exit_code"] = 1
    _assert_handoff_rejected(
        tmp,
        "rejects completed status with non-zero verification.exit_code",
        ho_silent,
    )

    ho_mismatch = copy.deepcopy(base_handoff())
    ho_mismatch["packet_id"] = "OTHER-PACKET-99"
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects packet_id mismatch",
        ho_mismatch,
        pkt_path,
        "packet_id",
    )

    ho_too_many_files = copy.deepcopy(base_handoff())
    ho_too_many_files["modified_files"] = [
        {
            "path": f"scripts/agent-harness/f{idx}.py",
            "added_lines": 1,
            "deleted_lines": 0,
        }
        for idx in range(MAX_FILES_DEFAULT + 1)
    ]
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects modified_files exceeding max_files",
        ho_too_many_files,
        pkt_path,
        "max_files",
    )

    ho_too_many_funcs = copy.deepcopy(base_handoff())
    ho_too_many_funcs["modified_functions_count"] = MAX_FUNCTIONS_DEFAULT + 1
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects modified_functions_count exceeding max_functions",
        ho_too_many_funcs,
        pkt_path,
        "max_functions",
    )

    ho_outside = copy.deepcopy(base_handoff())
    ho_outside["modified_files"] = [
        {"path": "scripts/lint/run.sh", "added_lines": 1, "deleted_lines": 0},
    ]
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects modified file outside allowed_paths",
        ho_outside,
        pkt_path,
        "allowed_paths",
    )

    ho_forbidden = copy.deepcopy(base_handoff())
    ho_forbidden["modified_files"] = [
        {
            "path": "scripts/agent-harness/harness_selfcheck.py",
            "added_lines": 1,
            "deleted_lines": 0,
        },
    ]
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects modified file matching forbidden_paths",
        ho_forbidden,
        pkt_path,
        "forbidden_paths",
    )

    pkt_quoted_ok = copy.deepcopy(base_packet())
    _set_verify_cmd(pkt_quoted_ok, './scripts/lint/run.sh "scripts"')
    pkt_quoted_ok_path = write_json(tmp, "pkt_quoted_ok.json", pkt_quoted_ok)
    res_qok = run_cli("validate-packet", str(pkt_quoted_ok_path))
    check(
        "quoted target in ./scripts/lint/run.sh passes packet validation",
        res_qok.returncode == EXIT_OK,
        res_qok.stderr or res_qok.stdout,
    )

    for wide_cmd in (
        './scripts/lint/run.sh "all" && ./scripts/lint/run.sh scripts',
        "make lint && ./scripts/lint/run.sh scripts",
    ):
        pkt_wide = copy.deepcopy(base_packet())
        pkt_wide["execution_policy"]["verification_command"] = wide_cmd
        pkt_wide_path = write_json(
            tmp,
            f"pkt_wide_{abs(hash(wide_cmd))}.json",
            pkt_wide,
        )
        res_wide = run_cli("validate-packet", str(pkt_wide_path))
        check(
            f"forbid_repo_wide_test_build rejects {wide_cmd!r}",
            res_wide.returncode == EXIT_INVALID
            and "FORBIDDEN_REPO_WIDE_COMMAND" in res_wide.stderr,
            res_wide.stderr or res_wide.stdout,
        )

    ho_bad_verify = copy.deepcopy(base_handoff())
    ho_bad_verify["verification"]["command"] = "golangci-lint run ./..."
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects completed handoff bypassing ./scripts/lint/run.sh <target>",
        ho_bad_verify,
        pkt_path,
        "MISSING_VERIFICATION_TARGET",
    )

    for unnorm_path in (
        "scripts//agent-harness/harness_selfcheck.py",
        "scripts/./agent-harness/harness_selfcheck.py",
    ):
        ho_unnorm = copy.deepcopy(base_handoff())
        ho_unnorm["modified_files"] = [
            {"path": unnorm_path, "added_lines": 1, "deleted_lines": 0},
        ]
        _assert_handoff_rejected(
            tmp,
            f"rejects unnormalized modified_files path {unnorm_path!r}",
            ho_unnorm,
            pkt_path,
        )


def test_templates_and_cli_utilities(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[6] Bundled templates, render-template, check-templates & docs\n")

    check("task-packet.md.tmpl exists", PACKET_TMPL_PATH.is_file(), str(PACKET_TMPL_PATH))
    check("task-handoff.md.tmpl exists", HANDOFF_TMPL_PATH.is_file(), str(HANDOFF_TMPL_PATH))

    res_chk = run_cli("check-templates")
    check("check-templates passes", res_chk.returncode == EXIT_OK, res_chk.stderr or res_chk.stdout)

    res_rp = run_cli("render-template", "--kind", "packet", "--packet-id", "RENDER-42")
    check(
        "render-template --kind packet exits 0 and strips pre-baked measured block",
        res_rp.returncode == EXIT_OK
        and "RENDER-42" in res_rp.stdout
        and "measured:" not in res_rp.stdout,
        res_rp.stderr,
    )
    rendered_pkt = tmp / "rendered_packet.md"
    rendered_pkt.write_text(res_rp.stdout, encoding="utf-8")
    res_vp_unmeas = run_cli("validate-packet", str(rendered_pkt))
    check(
        "rendered packet requires measure before validate-packet passes (MF-04)",
        res_vp_unmeas.returncode == EXIT_INVALID
        and "UNMEASURED_VERIFICATION_COMMAND" in res_vp_unmeas.stderr,
        res_vp_unmeas.stderr or res_vp_unmeas.stdout,
    )

    tmpl_repo = tmp / "tmpl_measure_repo"
    _init_temp_git_repo(tmpl_repo)
    _write_stub_run_sh(tmpl_repo, exit_code=0)
    _run_git(tmpl_repo, "add", ".")
    _run_git(tmpl_repo, "commit", "-q", "-m", "init")
    res_m_rp = run_cli("measure", str(rendered_pkt), "--repo-root", str(tmpl_repo))
    res_vp = run_cli("validate-packet", str(rendered_pkt))
    check(
        "rendered packet validates cleanly after measure",
        res_m_rp.returncode == EXIT_OK and res_vp.returncode == EXIT_OK,
        res_m_rp.stderr or res_vp.stderr,
    )

    res_rh = run_cli("render-template", "--kind", "handoff", "--packet-id", "RENDER-42")
    check(
        "render-template --kind handoff exits 0",
        res_rh.returncode == EXIT_OK and "RENDER-42" in res_rh.stdout,
        res_rh.stderr,
    )
    rendered_ho = tmp / "rendered_handoff.md"
    rendered_ho.write_text(res_rh.stdout, encoding="utf-8")
    res_vh = run_cli("validate-handoff", str(rendered_ho), "--packet", str(rendered_pkt))
    check(
        "rendered handoff template cross-validates cleanly against measured rendered packet",
        res_vh.returncode == EXIT_OK,
        res_vh.stderr or res_vh.stdout,
    )

    res_rp_json = run_cli(
        "render-template",
        "--kind",
        "packet",
        "--packet-id",
        "RENDER-JSON-1",
        "--format",
        "json",
    )
    rendered_json = tmp / "rendered_packet.json"
    rendered_json.write_text(res_rp_json.stdout, encoding="utf-8")
    res_m_rj = run_cli("measure", str(rendered_json), "--repo-root", str(tmpl_repo))
    res_vpj = run_cli("validate-packet", str(rendered_json))
    check(
        "render-template --format json omits measured and validates cleanly after measure",
        res_rp_json.returncode == EXIT_OK
        and '"measured"' not in res_rp_json.stdout
        and res_m_rj.returncode == EXIT_OK
        and res_vpj.returncode == EXIT_OK,
        res_rp_json.stderr or res_m_rj.stderr or res_vpj.stderr,
    )

    res_missing = run_cli("validate-packet", str(tmp / "does-not-exist.json"))
    check(
        "missing input file exits 2 with not found message",
        res_missing.returncode == EXIT_USAGE and "not found" in res_missing.stderr.lower(),
        res_missing.stderr,
    )

    res_bad_id = run_cli("render-template", "--kind", "packet", "--packet-id", "bad id!")
    check(
        "invalid --packet-id exits 2 with invalid message",
        res_bad_id.returncode == EXIT_USAGE and "invalid --packet-id" in res_bad_id.stderr.lower(),
        res_bad_id.stderr,
    )

    check("spec doc exists", SPEC_DOC_PATH.is_file(), str(SPEC_DOC_PATH))
    if SPEC_DOC_PATH.is_file():
        spec_text = SPEC_DOC_PATH.read_text(encoding="utf-8")
        check(
            "spec doc has Obsidian Vault frontmatter (title, date: 2026-11-21, tags: alt)",
            spec_text.startswith("---\n")
            and "title:" in spec_text.split("---", 2)[1]
            and "date: 2026-11-21" in spec_text.split("---", 2)[1]
            and "alt" in spec_text.split("---", 2)[1],
        )

    check("agent rule doc exists", RULE_DOC_PATH.is_file(), str(RULE_DOC_PATH))
    if RULE_DOC_PATH.is_file():
        rule_text = RULE_DOC_PATH.read_text(encoding="utf-8")
        check(
            "agent rule doc covers GUARD-01, foreground execution, and 3-item handoff",
            "GUARD-01" in rule_text
            and "--fix" in rule_text
            and "run.sh" in rule_text
            and "変更なし" in rule_text,
        )


def _run_git(repo: pathlib.Path, *git_args: str) -> str:
    git_bin = shutil.which("git")
    assert git_bin is not None
    res = subprocess.run(  # noqa: S603 -- test helper invoking resolved git binary in temp repo
        [git_bin, "-C", str(repo), *git_args],
        check=True,
        capture_output=True,
        text=True,
    )
    return res.stdout.strip()


def _make_c01_test_repo(tmp: pathlib.Path) -> pathlib.Path:
    c01_test_repo = tmp / "c01_test_repo"
    (c01_test_repo / "pkg").mkdir(parents=True, exist_ok=True)
    (c01_test_repo / "pkg" / "example_test.go").write_text(
        'package pkg\nimport "testing"\nfunc TestExampleReturnPath(t *testing.T) {}\n',
        encoding="utf-8",
    )
    (c01_test_repo / "tests").mkdir(parents=True, exist_ok=True)
    (c01_test_repo / "tests" / "test_example.py").write_text(
        "def test_example_fn_returns_expected():\n    assert True\n",
        encoding="utf-8",
    )
    return c01_test_repo


def test_check01_candidate_enforcement(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[7] CHECK-01 handoff behavior candidate enforcement\n")
    pkt_path = write_json(tmp, "c01_packet.json", base_packet())

    candidates = [
        {
            "id": "cand-001",
            "file": "scripts/agent-harness/example.py",
            "function": "example_fn",
            "kind": "python.control_flow_return_change",
            "before": "if=1,return=1",
            "after": "if=1,return=0",
        }
    ]
    report_path = tmp / "screener_report.json"
    report_path.write_text(json.dumps(candidates, indent=2), encoding="utf-8")
    empty_report = tmp / "screener_empty.json"
    empty_report.write_text("[]\n", encoding="utf-8")

    # 1. Unanswered candidate => UNANSWERED_BEHAVIOR_CANDIDATE
    ho_unanswered = write_json(tmp, "c01_ho_unanswered.json", base_handoff())
    res_un = run_cli(
        "validate-handoff",
        str(ho_unanswered),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(report_path),
    )
    check(
        "rejects unanswered screener candidate with UNANSWERED_BEHAVIOR_CANDIDATE",
        res_un.returncode == EXIT_INVALID and "UNANSWERED_BEHAVIOR_CANDIDATE" in res_un.stderr,
        f"rc={res_un.returncode} err={res_un.stderr!r}",
    )

    # 2. disposition=intended without test_name => MISSING_CANDIDATE_TEST_NAME
    ho_missing_test = copy.deepcopy(base_handoff())
    ho_missing_test["runtime_behavior_impact"]["has_change"] = True
    ho_missing_test["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "candidate_id": "cand-001",
            "file": "scripts/agent-harness/example.py",
            "function": "example_fn",
            "kind": "python.control_flow_return_change",
            "disposition": "intended",
            "test_name": "TBD",
            "note": "intentional change",
        }
    ]
    ho_mt_path = write_json(tmp, "c01_ho_missing_test.json", ho_missing_test)
    res_mt = run_cli(
        "validate-handoff",
        str(ho_mt_path),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(report_path),
    )
    check(
        "rejects intended candidate with placeholder test_name (MISSING_CANDIDATE_TEST_NAME)",
        res_mt.returncode == EXIT_INVALID and "MISSING_CANDIDATE_TEST_NAME" in res_mt.stderr,
        f"rc={res_mt.returncode} err={res_mt.stderr!r}",
    )

    c01_test_repo = _make_c01_test_repo(tmp)

    # 3. disposition=intended with concrete test_name matching by candidate_id => PASS
    ho_intended_ok = copy.deepcopy(base_handoff())
    ho_intended_ok["runtime_behavior_impact"]["has_change"] = True
    ho_intended_ok["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "candidate_id": "cand-001",
            "disposition": "intended",
            "test_name": "TestExampleReturnPath",
            "note": "verified by unit test",
        }
    ]
    ho_ok_path = write_json(tmp, "c01_ho_intended_ok.json", ho_intended_ok)
    res_ok = run_cli(
        "validate-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(report_path),
        "--repo-root",
        str(c01_test_repo),
    )
    check(
        "accepts intended candidate with concrete test_name matched by candidate_id",
        res_ok.returncode == EXIT_OK,
        res_ok.stderr or res_ok.stdout,
    )

    # 4. disposition=intended matched by (file, function, kind) => PASS
    ho_tuple_ok = copy.deepcopy(base_handoff())
    ho_tuple_ok["runtime_behavior_impact"]["has_change"] = True
    ho_tuple_ok["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "file": "scripts/agent-harness/example.py",
            "function": "example_fn",
            "kind": "python.control_flow_return_change",
            "disposition": "intended",
            "test_name": "test_example_fn_returns_expected",
        }
    ]
    ho_tuple_path = write_json(tmp, "c01_ho_tuple_ok.json", ho_tuple_ok)
    res_tuple = run_cli(
        "validate-handoff",
        str(ho_tuple_path),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(report_path),
        "--repo-root",
        str(c01_test_repo),
    )
    check(
        "accepts intended candidate matched by (file, function, kind)",
        res_tuple.returncode == EXIT_OK,
        res_tuple.stderr or res_tuple.stdout,
    )

    # 5. disposition=unintended_reverted while STILL present => UNREVERTED_UNINTENDED_CANDIDATE
    ho_unrev = copy.deepcopy(base_handoff())
    ho_unrev["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "candidate_id": "cand-001",
            "disposition": "unintended_reverted",
            "note": "claimed reverted",
        }
    ]
    ho_unrev_path = write_json(tmp, "c01_ho_unrev.json", ho_unrev)
    res_unrev = run_cli(
        "validate-handoff",
        str(ho_unrev_path),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(report_path),
    )
    check(
        "rejects unintended_reverted when candidate still active (UNREVERTED_UNINTENDED_CANDIDATE)",
        res_unrev.returncode == EXIT_INVALID
        and "UNREVERTED_UNINTENDED_CANDIDATE" in res_unrev.stderr,
        f"rc={res_unrev.returncode} err={res_unrev.stderr!r}",
    )

    # 6. disposition=unintended_reverted when screener report is empty (really reverted) => PASS
    res_rev_ok = run_cli(
        "validate-handoff",
        str(ho_unrev_path),
        "--packet",
        str(pkt_path),
        "--screener-report",
        str(empty_report),
    )
    check(
        "accepts unintended_reverted once candidate is absent from active screener report",
        res_rev_ok.returncode == EXIT_OK,
        res_rev_ok.stderr or res_rev_ok.stdout,
    )

    # 7. --base-commit auto-runs behavior_screener.py on a temp git repo
    git_repo = tmp / "c01_git_repo"
    git_repo.mkdir()
    _run_git(git_repo, "init", "-q")
    _run_git(git_repo, "config", "user.email", "test@example.com")
    _run_git(git_repo, "config", "user.name", "Test")
    target_py = git_repo / "scripts" / "agent-harness" / "example.py"
    target_py.parent.mkdir(parents=True, exist_ok=True)
    target_py.write_text(
        "def run(flag):\n    if flag:\n        return 1\n    return 0\n",
        encoding="utf-8",
    )
    _run_git(git_repo, "add", ".")
    _run_git(git_repo, "commit", "-q", "-m", "base")
    target_py.write_text("def run(flag):\n    return 0\n", encoding="utf-8")

    res_auto = run_cli(
        "validate-handoff",
        str(ho_unanswered),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(git_repo),
        "--base-commit",
        "HEAD",
    )
    check(
        "validate-handoff --base-commit auto-runs screener and flags unanswered candidate",
        res_auto.returncode == EXIT_INVALID and "UNANSWERED_BEHAVIOR_CANDIDATE" in res_auto.stderr,
        f"rc={res_auto.returncode} err={res_auto.stderr!r}",
    )


def test_check02_policy_field(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[8] CHECK-02 ExecutionPolicy.policy validation\n")

    # Template includes policy: "fix-existing"
    tmpl_text = PACKET_TMPL_PATH.read_text(encoding="utf-8")
    check(
        'task-packet.md.tmpl declares policy: "fix-existing"',
        'policy: "fix-existing"' in tmpl_text,
    )

    p_block = copy.deepcopy(base_packet())
    p_block["execution_policy"]["policy"] = "block-new"
    _set_verify_cmd(p_block, "LINT_POLICY=block-new ./scripts/lint/run.sh scripts")
    p_block_path = write_json(tmp, "c02_block_ok.json", p_block)
    res_ok = run_cli("validate-packet", str(p_block_path))
    check(
        "accepts policy='block-new' with matching LINT_POLICY=block-new",
        res_ok.returncode == EXIT_OK,
        res_ok.stderr or res_ok.stdout,
    )

    p_bad_enum = copy.deepcopy(base_packet())
    p_bad_enum["execution_policy"]["policy"] = "allow-all"
    _assert_packet_rejected(
        tmp,
        "rejects invalid execution_policy.policy value",
        p_bad_enum,
        "policy",
    )

    p_mismatch_env = copy.deepcopy(base_packet())
    p_mismatch_env["execution_policy"]["policy"] = "block-new"
    p_mismatch_env["execution_policy"]["verification_command"] = (
        "LINT_POLICY=fix-existing ./scripts/lint/run.sh scripts"
    )
    _assert_packet_rejected(
        tmp,
        "rejects policy='block-new' with LINT_POLICY=fix-existing (POLICY_COMMAND_MISMATCH)",
        p_mismatch_env,
        "POLICY_COMMAND_MISMATCH",
    )

    p_mismatch_flag = copy.deepcopy(base_packet())
    p_mismatch_flag["execution_policy"]["policy"] = "block-new"
    p_mismatch_flag["execution_policy"]["verification_command"] = (
        "./scripts/lint/run.sh --policy fix-existing scripts"
    )
    _assert_packet_rejected(
        tmp,
        "rejects policy='block-new' with --policy fix-existing (POLICY_COMMAND_MISMATCH)",
        p_mismatch_flag,
        "POLICY_COMMAND_MISMATCH",
    )

    p_mismatch_rev = copy.deepcopy(base_packet())
    p_mismatch_rev["execution_policy"]["policy"] = "fix-existing"
    p_mismatch_rev["execution_policy"]["verification_command"] = (
        "LINT_POLICY=block-new ./scripts/lint/run.sh scripts"
    )
    _assert_packet_rejected(
        tmp,
        "rejects policy='fix-existing' with LINT_POLICY=block-new (POLICY_COMMAND_MISMATCH)",
        p_mismatch_rev,
        "POLICY_COMMAND_MISMATCH",
    )


def test_check05_generated_code_protection(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[9] CHECK-05 dynamic DO NOT EDIT generated code protection\n")
    repo = tmp / "c05_repo"
    repo.mkdir()
    _run_git(repo, "init", "-q")
    _run_git(repo, "config", "user.email", "test@example.com")
    _run_git(repo, "config", "user.name", "Test")

    gen_go = repo / "pkg" / "api.pb.go"
    gen_go.parent.mkdir(parents=True, exist_ok=True)
    gen_go.write_text(
        "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage pkg\n",
        encoding="utf-8",
    )
    normal_go = repo / "pkg" / "handler.go"
    normal_go.write_text("package pkg\nfunc Handle() {}\n", encoding="utf-8")
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "init")

    # list-generated-forbidden-paths finds tracked generated files
    res_list = run_cli(
        "list-generated-forbidden-paths",
        "--repo-root",
        str(repo),
        "--format",
        "json",
    )
    listed = json.loads(res_list.stdout) if res_list.returncode == EXIT_OK else []
    check(
        "list-generated-forbidden-paths returns only generated tracked files",
        res_list.returncode == EXIT_OK and listed == ["pkg/api.pb.go"],
        f"rc={res_list.returncode} out={res_list.stdout!r} err={res_list.stderr!r}",
    )

    pkt = copy.deepcopy(base_packet())
    pkt["scope"]["allowed_paths"] = ["pkg/**"]
    pkt_default_path = write_json(tmp, "c05_pkt_default.json", pkt)

    ho_gen = copy.deepcopy(base_handoff())
    ho_gen["modified_files"] = [{"path": "pkg/api.pb.go", "added_lines": 2, "deleted_lines": 1}]
    ho_gen_path = write_json(tmp, "c05_ho_gen.json", ho_gen)

    res_block = run_cli(
        "validate-handoff",
        str(ho_gen_path),
        "--packet",
        str(pkt_default_path),
        "--repo-root",
        str(repo),
    )
    check(
        "rejects handoff modifying DO NOT EDIT file with GENERATED_CODE_MODIFIED",
        res_block.returncode == EXIT_INVALID and "GENERATED_CODE_MODIFIED" in res_block.stderr,
        f"rc={res_block.returncode} err={res_block.stderr!r}",
    )

    pkt_allow = copy.deepcopy(pkt)
    pkt_allow["scope"]["allow_generated_code_edits"] = True
    pkt_allow_path = write_json(tmp, "c05_pkt_allow.json", pkt_allow)
    res_allow = run_cli(
        "validate-handoff",
        str(ho_gen_path),
        "--packet",
        str(pkt_allow_path),
        "--repo-root",
        str(repo),
    )
    check(
        "allows generated code edit when scope.allow_generated_code_edits=true",
        res_allow.returncode == EXIT_OK,
        res_allow.stderr or res_allow.stdout,
    )

    ho_normal = copy.deepcopy(base_handoff())
    ho_normal["modified_files"] = [{"path": "pkg/handler.go", "added_lines": 3, "deleted_lines": 0}]
    ho_normal_path = write_json(tmp, "c05_ho_normal.json", ho_normal)
    res_normal = run_cli(
        "validate-handoff",
        str(ho_normal_path),
        "--packet",
        str(pkt_default_path),
        "--repo-root",
        str(repo),
    )
    check(
        "allows normal non-generated file modification under --repo-root",
        res_normal.returncode == EXIT_OK,
        res_normal.stderr or res_normal.stdout,
    )


def test_wave4_hardening(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[10] Wave 4 hardening: placeholders, contradiction, policy, exclusions, CWE-88\n"
    )
    pkt_path = write_json(tmp, "w4_packet.json", base_packet())

    # 1. Expanded PLACEHOLDER_STRINGS: none, null, なし, 無し
    for bad_token in ("none", "None", "null", "なし", "無し"):
        ho_r = copy.deepcopy(base_handoff())
        ho_r["rationale"] = bad_token
        _assert_handoff_rejected(tmp, f"rejects placeholder rationale={bad_token!r}", ho_r)

        ho_t = copy.deepcopy(base_handoff())
        ho_t["runtime_behavior_impact"]["has_change"] = True
        ho_t["runtime_behavior_impact"]["candidate_responses"] = [
            {
                "candidate_id": "cand-001",
                "disposition": "intended",
                "test_name": bad_token,
            }
        ]
        _assert_handoff_rejected(
            tmp,
            f"rejects placeholder candidate test_name={bad_token!r}",
            ho_t,
            pkt_path,
            "MISSING_CANDIDATE_TEST_NAME",
        )

    # 2. Contradictory has_change=false with disposition='intended' -> CONTRADICTORY_BEHAVIOR_IMPACT
    ho_contra = copy.deepcopy(base_handoff())
    ho_contra["runtime_behavior_impact"]["has_change"] = False
    ho_contra["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "candidate_id": "cand-001",
            "disposition": "intended",
            "test_name": "TestIntendedBehavior",
            "note": "actual behavior changed",
        }
    ]
    _assert_handoff_rejected(
        tmp,
        "rejects has_change=false when candidate_responses has disposition=intended",
        ho_contra,
        pkt_path,
        "CONTRADICTORY_BEHAVIOR_IMPACT",
    )

    # 3. policy='block-new' requires explicit --policy block-new or LINT_POLICY=block-new
    p_block_omit = copy.deepcopy(base_packet())
    p_block_omit["execution_policy"]["policy"] = "block-new"
    p_block_omit["execution_policy"]["verification_command"] = "./scripts/lint/run.sh scripts"
    _assert_packet_rejected(
        tmp,
        "rejects policy='block-new' when verification_command omits explicit block-new flag",
        p_block_omit,
        "POLICY_COMMAND_MISMATCH",
    )

    p_block_flag = copy.deepcopy(base_packet())
    p_block_flag["execution_policy"]["policy"] = "block-new"
    _set_verify_cmd(p_block_flag, "./scripts/lint/run.sh --policy block-new scripts")
    p_block_flag_path = write_json(tmp, "w4_block_flag_ok.json", p_block_flag)
    res_bf = run_cli("validate-packet", str(p_block_flag_path))
    check(
        "accepts policy='block-new' with --policy block-new in verification_command",
        res_bf.returncode == EXIT_OK,
        res_bf.stderr or res_bf.stdout,
    )

    ho_block_omit = copy.deepcopy(base_handoff())
    ho_block_omit["verification"]["command"] = "./scripts/lint/run.sh scripts"
    _assert_handoff_rejected(
        tmp,
        "cross-validation rejects handoff verification omitting block-new for policy='block-new'",
        ho_block_omit,
        p_block_flag_path,
        "POLICY_COMMAND_MISMATCH",
    )


def test_wave4_exclusions_and_security(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[11] Wave 4 exclusions, CWE-88 --base-commit, & fail-closed screener\n")
    pkt_path = write_json(tmp, "w4_sec_packet.json", base_packet())
    repo = tmp / "w4_gen_repo"
    repo.mkdir()
    _run_git(repo, "init", "-q")
    _run_git(repo, "config", "user.email", "test@example.com")
    _run_git(repo, "config", "user.name", "Test")
    (repo / "pkg").mkdir()
    (repo / "pkg" / "real.pb.go").write_text(
        "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage pkg\n",
        encoding="utf-8",
    )
    (repo / "stopped-using-k8s").mkdir()
    (repo / "stopped-using-k8s" / "old.pb.go").write_text(
        "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage old\n",
        encoding="utf-8",
    )
    (repo / "scripts" / "harness" / "corpus" / "C-16").mkdir(parents=True)
    (repo / "scripts" / "harness" / "corpus" / "C-16" / "before.go").write_text(
        "// Code generated by oapi-codegen. DO NOT EDIT.\npackage c16\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "init")
    res_gen = run_cli(
        "list-generated-forbidden-paths",
        "--repo-root",
        str(repo),
        "--format",
        "json",
    )
    listed = json.loads(res_gen.stdout) if res_gen.returncode == EXIT_OK else []
    check(
        "list-generated-forbidden-paths excludes stopped-using-k8s/ and scripts/harness/corpus/",
        res_gen.returncode == EXIT_OK and listed == ["pkg/real.pb.go"],
        f"rc={res_gen.returncode} listed={listed}",
    )

    ho_ok_path = write_json(tmp, "w4_ho_ok.json", base_handoff())
    res_bad_base = run_cli(
        "validate-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--base-commit=--output=/tmp/evil",
    )
    check(
        "CWE-88: rejects leading '-' on --base-commit with exit 2",
        res_bad_base.returncode == EXIT_USAGE,
        f"rc={res_bad_base.returncode} err={res_bad_base.stderr!r}",
    )

    bad_py = repo / "scripts" / "agent-harness" / "example.py"
    bad_py.parent.mkdir(parents=True, exist_ok=True)
    bad_py.write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "add example.py")
    bad_py.write_text("def f(:\n    broken\n", encoding="utf-8")
    res_broken = run_cli(
        "validate-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--base-commit",
        "HEAD",
    )
    check(
        "validate-handoff --base-commit fails closed when screener hits syntax error",
        res_broken.returncode != EXIT_OK,
        f"rc={res_broken.returncode} out={res_broken.stdout!r} err={res_broken.stderr!r}",
    )


def test_wave2_test_existence_and_header_deletion(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[12] Wave 2 hardening: intended test_name existence & DO NOT EDIT header-deletion\n"
    )
    repo = tmp / "w2_repo"
    repo.mkdir()
    _run_git(repo, "init", "-q")
    _run_git(repo, "config", "user.email", "test@example.com")
    _run_git(repo, "config", "user.name", "Test")

    # Populate Go, Python, Rust, and TS test files
    (repo / "pkg").mkdir(parents=True)
    (repo / "pkg" / "service_test.go").write_text(
        'package pkg\nimport "testing"\n'
        "func TestSuite(t *testing.T) {\n"
        '    t.Run("SubCase", func(t *testing.T) {})\n'
        "}\n"
        "func setupServer() {}\n"
        "type MySuite struct{}\n"
        "func (s *MySuite) TestSuiteMethod() {}\n",
        encoding="utf-8",
    )
    (repo / "tests").mkdir(parents=True)
    (repo / "tests" / "test_x.py").write_text(
        "def _helper():\n    return 1\n"
        "def test_top():\n    assert True\n"
        "class TestCls:\n    async def test_bar(self):\n        assert True\n",
        encoding="utf-8",
    )
    (repo / "src").mkdir(parents=True)
    (repo / "src" / "lib.rs").write_text(
        "pub fn helper_not_a_test() -> bool { true }\n"
        "#[cfg(test)]\nmod tests {\n"
        "    #[tokio::test]\n    async fn test_async_rust() {}\n"
        "    #[rstest]\n    fn parses_valid_url() {}\n"
        "}\n",
        encoding="utf-8",
    )
    (repo / "web").mkdir(parents=True)
    (repo / "web" / "widget.test.ts").write_text(
        'describe("Widget", () => {\n  it("should render widget", () => {});\n});\n',
        encoding="utf-8",
    )

    # Generated files (normal + lowercase header)
    (repo / "api").mkdir(parents=True)
    gen_file = repo / "api" / "server.gen.go"
    gen_file.write_text(
        "// Code generated by oapi-codegen. DO NOT EDIT.\npackage api\n",
        encoding="utf-8",
    )
    lower_gen = repo / "api" / "lower.gen.go"
    lower_gen.write_text(
        "// code generated by custom-gen. do not edit.\npackage api\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "init")

    pkt = copy.deepcopy(base_packet())
    pkt["scope"]["allowed_paths"] = ["scripts/agent-harness/**", "api/**"]
    pkt_path = write_json(tmp, "w2_pkt.json", pkt)

    def validate_intended(slug: str, test_name: str) -> subprocess.CompletedProcess[str]:
        ho = copy.deepcopy(base_handoff())
        ho["runtime_behavior_impact"]["has_change"] = True
        ho["runtime_behavior_impact"]["candidate_responses"] = [
            {
                "candidate_id": "cand-w2",
                "disposition": "intended",
                "test_name": test_name,
                "note": "verified",
            }
        ]
        ho_path = write_json(tmp, f"w2_ho_{slug}.json", ho)
        return run_cli(
            "validate-handoff",
            str(ho_path),
            "--packet",
            str(pkt_path),
            "--repo-root",
            str(repo),
        )

    # Fake non-existent test => NONEXISTENT_CANDIDATE_TEST
    res_fake = validate_intended("fake", "TestCompletelyFakeNonExistent_99999")
    check(
        "rejects fake test_name with NONEXISTENT_CANDIDATE_TEST",
        res_fake.returncode == EXIT_INVALID and "NONEXISTENT_CANDIDATE_TEST" in res_fake.stderr,
        f"rc={res_fake.returncode} err={res_fake.stderr!r}",
    )

    # Non-test Rust production function must NOT satisfy test_name
    res_prod_rs = validate_intended("rust_prod", "helper_not_a_test")
    check(
        "rejects non-test Rust production function name with NONEXISTENT_CANDIDATE_TEST",
        res_prod_rs.returncode == EXIT_INVALID
        and "NONEXISTENT_CANDIDATE_TEST" in res_prod_rs.stderr,
        f"rc={res_prod_rs.returncode} err={res_prod_rs.stderr!r}",
    )

    # Go existing parent + fake subtest => NONEXISTENT_CANDIDATE_TEST
    res_bad_sub = validate_intended("go_bad_sub", "TestSuite/FakeSubCase")
    check(
        "rejects Go TestSuite/FakeSubCase when subtest does not exist",
        res_bad_sub.returncode == EXIT_INVALID
        and "NONEXISTENT_CANDIDATE_TEST" in res_bad_sub.stderr,
        f"rc={res_bad_sub.returncode} err={res_bad_sub.stderr!r}",
    )

    # CS-R2-2: Non-test helpers inside *_test.go / test_*.py must NOT satisfy test_name
    for slug, bad_helper in (("go_helper", "setupServer"), ("py_helper", "_helper")):
        res_bad_helper = validate_intended(slug, bad_helper)
        check(
            f"CS-R2-2: rejects non-test helper {bad_helper!r} inside test file",
            res_bad_helper.returncode == EXIT_INVALID
            and "NONEXISTENT_CANDIDATE_TEST" in res_bad_helper.stderr,
            f"rc={res_bad_helper.returncode} err={res_bad_helper.stderr!r}",
        )

    # Real tests across Go, Python, Rust, TS => PASS
    for label, tname in (
        ("go_func", "TestSuite"),
        ("go_subtest", "TestSuite/SubCase"),
        ("go_suite_method", "TestSuiteMethod"),
        ("py_top", "test_top"),
        ("py_nodeid", "tests/test_x.py::TestCls::test_bar"),
        ("py_param", "test_bar[case-1]"),
        ("rust_tokio", "pkg::tests::test_async_rust"),
        ("rust_rstest", "parses_valid_url"),
        ("ts_it", "should render widget"),
        ("ts_describe", "Widget"),
    ):
        res_real = validate_intended(label, tname)
        check(
            f"accepts existing test_name {tname!r} ({label})",
            res_real.returncode == EXIT_OK,
            f"rc={res_real.returncode} err={res_real.stderr!r}",
        )

    # MF-2: Deleting the DO NOT EDIT header on disk MUST STILL trigger GENERATED_CODE_MODIFIED
    gen_file.write_text("package api\nfunc HandEdited() {}\n", encoding="utf-8")
    ho_del_hdr = copy.deepcopy(base_handoff())
    ho_del_hdr["modified_files"] = [
        {"path": "api/server.gen.go", "added_lines": 1, "deleted_lines": 1}
    ]
    ho_del_hdr_path = write_json(tmp, "w2_ho_del_hdr.json", ho_del_hdr)
    res_del_hdr = run_cli(
        "validate-handoff",
        str(ho_del_hdr_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
    )
    check(
        "MF-2: catches generated file even after worker deletes DO NOT EDIT header on disk",
        res_del_hdr.returncode == EXIT_INVALID and "GENERATED_CODE_MODIFIED" in res_del_hdr.stderr,
        f"rc={res_del_hdr.returncode} err={res_del_hdr.stderr!r}",
    )

    # MF-2: Case-insensitive DO NOT EDIT header detection
    ho_lower = copy.deepcopy(base_handoff())
    ho_lower["modified_files"] = [
        {"path": "api/lower.gen.go", "added_lines": 1, "deleted_lines": 0}
    ]
    ho_lower_path = write_json(tmp, "w2_ho_lower_gen.json", ho_lower)
    res_lower = run_cli(
        "validate-handoff",
        str(ho_lower_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
    )
    check(
        "MF-2: catches case-insensitive 'code generated ... do not edit' header",
        res_lower.returncode == EXIT_INVALID and "GENERATED_CODE_MODIFIED" in res_lower.stderr,
        f"rc={res_lower.returncode} err={res_lower.stderr!r}",
    )


def _init_temp_git_repo(repo: pathlib.Path) -> None:
    repo.mkdir(parents=True, exist_ok=True)
    _run_git(repo, "init", "-q")
    _run_git(repo, "config", "user.email", "test@example.com")
    _run_git(repo, "config", "user.name", "Test")


def _write_stub_run_sh(repo: pathlib.Path, exit_code: int = 0, sleep_s: float = 0.0) -> None:
    run_sh = repo / "scripts" / "lint" / "run.sh"
    run_sh.parent.mkdir(parents=True, exist_ok=True)
    sleep_line = f"sleep {sleep_s}\n" if sleep_s > 0 else ""
    run_sh.write_text(
        f"#!/usr/bin/env bash\nset -euo pipefail\n{sleep_line}exit {exit_code}\n",
        encoding="utf-8",
    )
    run_sh.chmod(0o755)


def test_loop02_validation(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[13a] LOOP-02 validate-packet & render-template checks\n")

    # 1. Missing measured => UNMEASURED_VERIFICATION_COMMAND
    p_unmeasured = copy.deepcopy(base_packet())
    del p_unmeasured["execution_policy"]["measured"]
    _assert_packet_rejected(
        tmp,
        "LOOP-02 rejects packet missing execution_policy.measured",
        p_unmeasured,
        "UNMEASURED_VERIFICATION_COMMAND",
    )

    # 2. Over-budget duration_s => VERIFICATION_BUDGET_EXCEEDED
    p_over = copy.deepcopy(base_packet())
    p_over["execution_policy"]["max_command_budget_s"] = 30
    p_over["execution_policy"]["measured"]["duration_s"] = 30.5
    _assert_packet_rejected(
        tmp,
        "LOOP-02 rejects measured.duration_s > max_command_budget_s",
        p_over,
        "VERIFICATION_BUDGET_EXCEEDED",
    )

    # 3. Stale command_sha256 => STALE_MEASURED_COMMAND
    p_stale = copy.deepcopy(base_packet())
    p_stale["execution_policy"]["measured"]["command_sha256"] = "0" * SHA256_HEX_LEN
    _assert_packet_rejected(
        tmp,
        "LOOP-02 rejects mismatched measured.command_sha256",
        p_stale,
        "STALE_MEASURED_COMMAND",
    )

    # 4. Matching command_sha256 + non-zero measured exit_code (fix-existing baseline) => PASS
    p_match = copy.deepcopy(base_packet())
    cmd = p_match["execution_policy"]["verification_command"]
    p_match["execution_policy"]["measured"]["command_sha256"] = hashlib.sha256(
        cmd.encode("utf-8")
    ).hexdigest()
    p_match["execution_policy"]["measured"]["exit_code"] = 1
    p_match["base_commit"] = "abcdef1"
    p_match["task_kind"] = "lint-fix"
    p_match["behavior_preserving"] = True
    p_match["scope"]["allow_test_expectation_changes"] = False
    p_match_path = write_json(tmp, "loop02_match.json", p_match)
    res_match = run_cli("validate-packet", str(p_match_path))
    check(
        "LOOP-02 accepts packet with matching command_sha256, non-zero exit_code, and new fields",
        res_match.returncode == EXIT_OK,
        res_match.stderr or res_match.stdout,
    )

    # 5. render-template --target omits measured (fails validate-packet until measured)
    res_rt = run_cli(
        "render-template",
        "--kind",
        "packet",
        "--packet-id",
        "LOOP02-TARGET",
        "--target",
        "alt-backend/app",
    )
    rt_path = tmp / "loop02_rendered_target.md"
    rt_path.write_text(res_rt.stdout, encoding="utf-8")
    res_rt_val = run_cli("validate-packet", str(rt_path))
    check(
        "render-template --target omits measured and fails validate-packet until measure runs",
        res_rt.returncode == EXIT_OK
        and "./scripts/lint/run.sh alt-backend/app" in res_rt.stdout
        and res_rt_val.returncode == EXIT_INVALID
        and "UNMEASURED_VERIFICATION_COMMAND" in res_rt_val.stderr,
        res_rt_val.stderr or res_rt.stderr,
    )


def test_loop02_measure_cli(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[13b] LOOP-02 measure subcommand (JSON, Markdown, budget)\n")
    repo = tmp / "loop02_repo"
    _init_temp_git_repo(repo)
    _write_stub_run_sh(repo, exit_code=0)
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "init")
    head_sha = _run_git(repo, "rev-parse", "HEAD")

    p_raw = copy.deepcopy(base_packet())
    del p_raw["execution_policy"]["measured"]
    p_raw_json = write_json(tmp, "loop02_to_measure.json", p_raw)
    res_m_json = run_cli("measure", str(p_raw_json), "--repo-root", str(repo))
    measured_data = json.loads(p_raw_json.read_text(encoding="utf-8"))
    m_obj = measured_data["execution_policy"].get("measured", {})
    check(
        "measure records duration_s, exit_code, at_commit, command_sha256 into JSON packet",
        res_m_json.returncode == EXIT_OK
        and m_obj.get("exit_code") == 0
        and m_obj.get("at_commit") == head_sha
        and isinstance(m_obj.get("duration_s"), (int, float))
        and m_obj.get("command_sha256")
        == hashlib.sha256(b"./scripts/lint/run.sh scripts").hexdigest(),
        f"rc={res_m_json.returncode} m_obj={m_obj} err={res_m_json.stderr!r}",
    )
    res_val_after = run_cli("validate-packet", str(p_raw_json))
    check(
        "JSON packet passes validate-packet after measure",
        res_val_after.returncode == EXIT_OK,
        res_val_after.stderr or res_val_after.stdout,
    )

    md_unmeasured = valid_packet_markdown().replace(
        "  measured:\n"
        "    duration_s: 1.5\n"
        "    exit_code: 0\n"
        '    at_commit: "0123456"\n'
        f'    command_sha256: "{_cmd_sha("./scripts/lint/run.sh scripts")}"\n',
        "",
    )
    md_path = tmp / "loop02_to_measure.md"
    md_path.write_text(md_unmeasured, encoding="utf-8")
    res_m_md = run_cli("measure", str(md_path), "--repo-root", str(repo))
    res_val_md = run_cli("validate-packet", str(md_path))
    check(
        "measure updates Markdown YAML frontmatter and passes validate-packet",
        res_m_md.returncode == EXIT_OK and res_val_md.returncode == EXIT_OK,
        f"m_rc={res_m_md.returncode} v_rc={res_val_md.returncode} err={res_m_md.stderr or res_val_md.stderr!r}",
    )

    slow_repo = tmp / "loop02_slow_repo"
    _init_temp_git_repo(slow_repo)
    _write_stub_run_sh(slow_repo, exit_code=0, sleep_s=1.2)
    _run_git(slow_repo, "add", ".")
    _run_git(slow_repo, "commit", "-q", "-m", "init")
    p_slow = copy.deepcopy(base_packet())
    p_slow["execution_policy"]["max_command_budget_s"] = 1
    del p_slow["execution_policy"]["measured"]
    p_slow_path = write_json(tmp, "loop02_slow.json", p_slow)
    res_slow = run_cli("measure", str(p_slow_path), "--repo-root", str(slow_repo))
    check(
        "measure exits 1 with VERIFICATION_BUDGET_EXCEEDED when command exceeds budget",
        res_slow.returncode == EXIT_INVALID and "VERIFICATION_BUDGET_EXCEEDED" in res_slow.stderr,
        f"rc={res_slow.returncode} err={res_slow.stderr!r}",
    )


def _setup_loop01_repo(tmp: pathlib.Path, name: str) -> tuple[pathlib.Path, str]:
    repo = tmp / name
    _init_temp_git_repo(repo)
    _write_stub_run_sh(repo, exit_code=0)
    mod = repo / "pkg" / "worker.py"
    mod.parent.mkdir(parents=True, exist_ok=True)
    mod.write_text(
        "def alpha(x: int) -> int:\n    return x + 1\n\ndef beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )
    test_f = repo / "tests" / "test_worker.py"
    test_f.parent.mkdir(parents=True, exist_ok=True)
    test_f.write_text(
        "from pkg.worker import alpha\n\ndef test_alpha():\n    assert alpha(1) == 2\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "base")
    base_sha = _run_git(repo, "rev-parse", "HEAD")
    return repo, base_sha


def _make_loop01_packet(base_sha: str | None) -> dict[str, Any]:
    pkt = copy.deepcopy(base_packet())
    pkt["packet_id"] = "LOOP01-TEST"
    pkt["task_kind"] = "lint-fix"
    if base_sha is not None:
        pkt["base_commit"] = base_sha
    pkt["scope"]["allowed_paths"] = ["pkg/**", "tests/**"]
    pkt["scope"]["forbidden_paths"] = ["pkg/secret.py"]
    pkt["scope"]["max_files"] = 3
    pkt["scope"]["max_functions"] = 2
    return pkt


def _make_loop01_handoff(
    files: list[dict[str, Any]],
    func_count: int,
    exit_code: int = 0,
) -> dict[str, Any]:
    ho = copy.deepcopy(base_handoff())
    ho["packet_id"] = "LOOP01-TEST"
    ho["modified_files"] = files
    ho["modified_functions_count"] = func_count
    ho["verification"]["exit_code"] = exit_code
    return ho


def test_loop01_verify_handoff_core(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[14a] LOOP-01 verify-handoff ground-truth file/AST/rerun checks\n")
    repo, base_sha = _setup_loop01_repo(tmp, "loop01_core")
    runs_dir = tmp / "loop01_runs"

    # Modify 1 function cleanly in pkg/worker.py
    (repo / "pkg" / "worker.py").write_text(
        "def alpha(x: int) -> int:\n    result = x + 1\n    return result\n\n"
        "def beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )

    pkt_path = write_json(tmp, "l01_pkt.json", _make_loop01_packet(base_sha))
    ho_ok_path = write_json(
        tmp,
        "l01_ho_ok.json",
        _make_loop01_handoff([{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1}], 1),
    )

    # 1. Clean verify-handoff passes & writes evidence JSON
    res_ok = run_cli(
        "verify-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--runs-dir",
        str(runs_dir),
    )
    evidence_files = list(runs_dir.glob("*/verify/LOOP01-TEST.json"))
    ev_valid = False
    if evidence_files:
        ev = json.loads(evidence_files[0].read_text(encoding="utf-8"))
        ev_valid = (
            ev.get("packet_id") == "LOOP01-TEST"
            and ev.get("exit_code") == 0
            and ev.get("base_commit") == base_sha
            and isinstance(ev.get("output_sha256"), str)
            and len(ev["output_sha256"]) == SHA256_HEX_LEN
        )
    check(
        "verify-handoff passes on truthful handoff and persists host rerun evidence JSON",
        res_ok.returncode == EXIT_OK and ev_valid,
        f"rc={res_ok.returncode} err={res_ok.stderr!r} ev_files={evidence_files}",
    )

    # 2. Missing base_commit (neither CLI nor packet) fails
    pkt_no_base = write_json(tmp, "l01_pkt_nobase.json", _make_loop01_packet(None))
    res_nb = run_cli(
        "verify-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_no_base),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff fails when neither --base-commit nor packet.base_commit is provided",
        res_nb.returncode == EXIT_INVALID and "MISSING_BASE_COMMIT" in res_nb.stderr,
        f"rc={res_nb.returncode} err={res_nb.stderr!r}",
    )

    # 3. Phantom modified file => PHANTOM_MODIFIED_FILE
    ho_phantom = write_json(
        tmp,
        "l01_ho_phantom.json",
        _make_loop01_handoff(
            [
                {"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1},
                {"path": "pkg/ghost.py", "added_lines": 1, "deleted_lines": 0},
            ],
            1,
        ),
    )
    res_ph = run_cli(
        "verify-handoff",
        str(ho_phantom),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff rejects phantom declared file with PHANTOM_MODIFIED_FILE",
        res_ph.returncode == EXIT_INVALID and "PHANTOM_MODIFIED_FILE" in res_ph.stderr,
        f"rc={res_ph.returncode} err={res_ph.stderr!r}",
    )

    # 4. Undeclared untracked file => UNDECLARED_MODIFIED_FILE + PATH_OUTSIDE_ALLOWED_PATHS
    untracked = repo / "outside.txt"
    untracked.write_text("sneaky\n", encoding="utf-8")
    res_un = run_cli(
        "verify-handoff",
        str(ho_ok_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    untracked.unlink()
    check(
        "verify-handoff rejects undeclared untracked file (UNDECLARED_MODIFIED_FILE + PATH_OUTSIDE_ALLOWED_PATHS)",
        res_un.returncode == EXIT_INVALID
        and "UNDECLARED_MODIFIED_FILE" in res_un.stderr
        and "PATH_OUTSIDE_ALLOWED_PATHS" in res_un.stderr,
        f"rc={res_un.returncode} err={res_un.stderr!r}",
    )

    # 5. AST modified functions mismatch => MODIFIED_FUNCTIONS_MISMATCH
    ho_bad_fn = write_json(
        tmp,
        "l01_ho_bad_fn.json",
        _make_loop01_handoff([{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1}], 2),
    )
    res_fn = run_cli(
        "verify-handoff",
        str(ho_bad_fn),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff rejects false modified_functions_count with MODIFIED_FUNCTIONS_MISMATCH",
        res_fn.returncode == EXIT_INVALID and "MODIFIED_FUNCTIONS_MISMATCH" in res_fn.stderr,
        f"rc={res_fn.returncode} err={res_fn.stderr!r}",
    )

    # 6. Host rerun exit code mismatch & VERIFICATION_RERUN_FAILED
    _write_stub_run_sh(repo, exit_code=1)
    # Restore run.sh tracked state? No, put stub failing via separate repo so git diff stays clean!
    fail_repo, fail_base = _setup_loop01_repo(tmp, "loop01_fail_rerun")
    _write_stub_run_sh(fail_repo, exit_code=1)
    _run_git(fail_repo, "add", ".")
    _run_git(fail_repo, "commit", "-q", "-m", "failing lint")
    fail_base = _run_git(fail_repo, "rev-parse", "HEAD")
    (fail_repo / "pkg" / "worker.py").write_text(
        "def alpha(x: int) -> int:\n    result = x + 1\n    return result\n\n"
        "def beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )
    fail_pkt = write_json(tmp, "l01_fail_pkt.json", _make_loop01_packet(fail_base))
    res_rerun_fail = run_cli(
        "verify-handoff",
        str(ho_ok_path),
        "--packet",
        str(fail_pkt),
        "--repo-root",
        str(fail_repo),
        "--runs-dir",
        str(runs_dir),
    )
    check(
        "verify-handoff catches lied exit_code on host rerun (VERIFICATION_EXIT_CODE_MISMATCH + VERIFICATION_RERUN_FAILED)",
        res_rerun_fail.returncode == EXIT_INVALID
        and "VERIFICATION_EXIT_CODE_MISMATCH" in res_rerun_fail.stderr
        and "VERIFICATION_RERUN_FAILED" in res_rerun_fail.stderr,
        f"rc={res_rerun_fail.returncode} err={res_rerun_fail.stderr!r}",
    )


def test_loop01_verify_handoff_lint_fix_extras(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[14b] LOOP-01 verify-handoff lint-fix comment & test-expectation guards\n")
    repo, base_sha = _setup_loop01_repo(tmp, "loop01_extras")
    pkt_path = write_json(tmp, "l01_ext_pkt.json", _make_loop01_packet(base_sha))

    # 1. Unrequested plain comment added => UNREQUESTED_COMMENT_ADDED
    (repo / "pkg" / "worker.py").write_text(
        "# Added commentary nobody asked for\n"
        "def alpha(x: int) -> int:\n    return x + 1\n\n"
        "def beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )
    ho_comment = write_json(
        tmp,
        "l01_ho_comment.json",
        _make_loop01_handoff([{"path": "pkg/worker.py", "added_lines": 1, "deleted_lines": 0}], 0),
    )
    res_cmt = run_cli(
        "verify-handoff",
        str(ho_comment),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "lint-fix verify-handoff rejects unrequested comment with UNREQUESTED_COMMENT_ADDED",
        res_cmt.returncode == EXIT_INVALID and "UNREQUESTED_COMMENT_ADDED" in res_cmt.stderr,
        f"rc={res_cmt.returncode} err={res_cmt.stderr!r}",
    )

    # 2. Valid suppression comment is allowed (avoid raw directive token in test source)
    suppression_line = "#" + " noqa: S101 -- ADR-000001 expires: 2027-01-01\n"
    (repo / "pkg" / "worker.py").write_text(
        suppression_line
        + "def alpha(x: int) -> int:\n    return x + 1\n\n"
        + "def beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )
    res_supp = run_cli(
        "verify-handoff",
        str(ho_comment),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "lint-fix verify-handoff allows suppression/governance comments (ADR- / expires:)",
        res_supp.returncode == EXIT_OK,
        f"rc={res_supp.returncode} err={res_supp.stderr!r}",
    )

    # 3. Weakening test expectation in lint-fix => FORBIDDEN_TEST_EXPECTATION_CHANGE
    _run_git(repo, "checkout", "--", "pkg/worker.py")
    (repo / "tests" / "test_worker.py").write_text(
        "from pkg.worker import alpha\n\ndef test_alpha():\n    assert alpha(1) == 999\n",
        encoding="utf-8",
    )
    ho_test_chg = _make_loop01_handoff(
        [{"path": "tests/test_worker.py", "added_lines": 1, "deleted_lines": 1}],
        1,
    )
    ho_test_chg["runtime_behavior_impact"]["has_change"] = True
    ho_test_chg["runtime_behavior_impact"]["candidate_responses"] = [
        {
            "file": "tests/test_worker.py",
            "function": "<test>",
            "kind": "test.expectation_change",
            "disposition": "intended",
            "test_name": "test_alpha",
        }
    ]
    ho_tc_path = write_json(tmp, "l01_ho_tc.json", ho_test_chg)
    res_tc = run_cli(
        "verify-handoff",
        str(ho_tc_path),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "lint-fix verify-handoff rejects test expectation change with FORBIDDEN_TEST_EXPECTATION_CHANGE",
        res_tc.returncode == EXIT_INVALID and "FORBIDDEN_TEST_EXPECTATION_CHANGE" in res_tc.stderr,
        f"rc={res_tc.returncode} err={res_tc.stderr!r}",
    )

    # 4. Opt-in allow_test_expectation_changes=true permits it (with candidate response)
    pkt_allow_tc = _make_loop01_packet(base_sha)
    pkt_allow_tc["scope"]["allow_test_expectation_changes"] = True
    pkt_allow_tc_path = write_json(tmp, "l01_pkt_allow_tc.json", pkt_allow_tc)
    res_tc_ok = run_cli(
        "verify-handoff",
        str(ho_tc_path),
        "--packet",
        str(pkt_allow_tc_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "lint-fix verify-handoff permits test expectation change when allow_test_expectation_changes=true",
        res_tc_ok.returncode == EXIT_OK,
        f"rc={res_tc_ok.returncode} err={res_tc_ok.stderr!r}",
    )


def test_verify01_sequence_rejections(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[15a] VERIFY-01 verify-sequence structural rejections\n")

    repo_few = tmp / "v01_few"
    _init_temp_git_repo(repo_few)
    (repo_few / "app.py").write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo_few, "add", ".")
    _run_git(repo_few, "commit", "-q", "-m", "base")
    base_few = _run_git(repo_few, "rev-parse", "HEAD")
    (repo_few / "test_app.py").write_text("from app import f\nassert f() == 1\n", encoding="utf-8")
    _run_git(repo_few, "add", ".")
    _run_git(repo_few, "commit", "-q", "-m", "only one commit")
    res_few = run_cli(
        "verify-sequence",
        "--base-commit",
        base_few,
        "--test-command",
        "python3 test_app.py",
        "--repo-root",
        str(repo_few),
    )
    check(
        "verify-sequence rejects <2 commits with SEQUENCE_TOO_FEW_COMMITS",
        res_few.returncode == EXIT_INVALID and "SEQUENCE_TOO_FEW_COMMITS" in res_few.stderr,
        f"rc={res_few.returncode} err={res_few.stderr!r}",
    )

    repo_mixed = tmp / "v01_mixed"
    _init_temp_git_repo(repo_mixed)
    (repo_mixed / "app.py").write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo_mixed, "add", ".")
    _run_git(repo_mixed, "commit", "-q", "-m", "base")
    base_mixed = _run_git(repo_mixed, "rev-parse", "HEAD")
    (repo_mixed / "app.py").write_text("def f():\n    return 1 + 0\n", encoding="utf-8")
    (repo_mixed / "test_app.py").write_text(
        "from app import f\nassert f() == 1\n", encoding="utf-8"
    )
    _run_git(repo_mixed, "add", ".")
    _run_git(repo_mixed, "commit", "-q", "-m", "mixed c1")
    (repo_mixed / "app.py").write_text("def f():\n    val = 1\n    return val\n", encoding="utf-8")
    _run_git(repo_mixed, "add", ".")
    _run_git(repo_mixed, "commit", "-q", "-m", "impl c2")
    res_mixed = run_cli(
        "verify-sequence",
        "--base-commit",
        base_mixed,
        "--test-command",
        "python3 test_app.py",
        "--repo-root",
        str(repo_mixed),
    )
    check(
        "verify-sequence rejects non-test-only commit 1 with SEQUENCE_FIRST_COMMIT_NOT_TEST_ONLY",
        res_mixed.returncode == EXIT_INVALID
        and "SEQUENCE_FIRST_COMMIT_NOT_TEST_ONLY" in res_mixed.stderr,
        f"rc={res_mixed.returncode} err={res_mixed.stderr!r}",
    )

    repo_mut = tmp / "v01_mut"
    _init_temp_git_repo(repo_mut)
    (repo_mut / "app.py").write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo_mut, "add", ".")
    _run_git(repo_mut, "commit", "-q", "-m", "base")
    base_mut = _run_git(repo_mut, "rev-parse", "HEAD")
    (repo_mut / "test_app.py").write_text("from app import f\nassert f() == 1\n", encoding="utf-8")
    _run_git(repo_mut, "add", ".")
    _run_git(repo_mut, "commit", "-q", "-m", "c1 test")
    (repo_mut / "app.py").write_text("def f():\n    return 2\n", encoding="utf-8")
    (repo_mut / "test_app.py").write_text("from app import f\nassert f() == 2\n", encoding="utf-8")
    _run_git(repo_mut, "add", ".")
    _run_git(repo_mut, "commit", "-q", "-m", "c2 mutates test")
    res_mut = run_cli(
        "verify-sequence",
        "--base-commit",
        base_mut,
        "--test-command",
        "python3 test_app.py",
        "--repo-root",
        str(repo_mut),
    )
    check(
        "verify-sequence rejects test mutation in impl commit with SEQUENCE_TEST_MUTATED_IN_IMPL",
        res_mut.returncode == EXIT_INVALID and "SEQUENCE_TEST_MUTATED_IN_IMPL" in res_mut.stderr,
        f"rc={res_mut.returncode} err={res_mut.stderr!r}",
    )


def test_verify01_sequence_modes(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[15b] VERIFY-01 behavior-preserving vs --bugfix & worktree cleanup\n")

    repo_bf = tmp / "v01_basefail"
    _init_temp_git_repo(repo_bf)
    (repo_bf / "app.py").write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo_bf, "add", ".")
    _run_git(repo_bf, "commit", "-q", "-m", "base")
    base_bf = _run_git(repo_bf, "rev-parse", "HEAD")
    (repo_bf / "test_app.py").write_text("from app import f\nassert f() == 2\n", encoding="utf-8")
    _run_git(repo_bf, "add", ".")
    _run_git(repo_bf, "commit", "-q", "-m", "c1 red test")
    (repo_bf / "app.py").write_text("def f():\n    return 2\n", encoding="utf-8")
    _run_git(repo_bf, "add", ".")
    _run_git(repo_bf, "commit", "-q", "-m", "c2 makes test green")
    res_bf = run_cli(
        "verify-sequence",
        "--base-commit",
        base_bf,
        "--test-command",
        "python3 test_app.py",
        "--repo-root",
        str(repo_bf),
    )
    check(
        "verify-sequence rejects failing baseline test with SEQUENCE_BASELINE_TEST_FAILED",
        res_bf.returncode == EXIT_INVALID and "SEQUENCE_BASELINE_TEST_FAILED" in res_bf.stderr,
        f"rc={res_bf.returncode} err={res_bf.stderr!r}",
    )

    res_bugfix_ok = run_cli(
        "verify-sequence",
        "--base-commit",
        base_bf,
        "--test-command",
        "python3 test_app.py",
        "--bugfix",
        "--repo-root",
        str(repo_bf),
    )
    check(
        "verify-sequence --bugfix passes when c1 is RED on base and GREEN on HEAD",
        res_bugfix_ok.returncode == EXIT_OK,
        res_bugfix_ok.stderr or res_bugfix_ok.stdout,
    )

    repo_ok = tmp / "v01_green_ok"
    _init_temp_git_repo(repo_ok)
    (repo_ok / "app.py").write_text("def f():\n    return 1\n", encoding="utf-8")
    _run_git(repo_ok, "add", ".")
    _run_git(repo_ok, "commit", "-q", "-m", "base")
    base_ok = _run_git(repo_ok, "rev-parse", "HEAD")
    (repo_ok / "test_app.py").write_text("from app import f\nassert f() == 1\n", encoding="utf-8")
    _run_git(repo_ok, "add", ".")
    _run_git(repo_ok, "commit", "-q", "-m", "c1 characterization test")
    (repo_ok / "app.py").write_text("def f():\n    x = 1\n    return x\n", encoding="utf-8")
    _run_git(repo_ok, "add", ".")
    _run_git(repo_ok, "commit", "-q", "-m", "c2 behavior-preserving refactor")

    res_not_red = run_cli(
        "verify-sequence",
        "--base-commit",
        base_ok,
        "--test-command",
        "python3 test_app.py",
        "--bugfix",
        "--repo-root",
        str(repo_ok),
    )
    check(
        "verify-sequence --bugfix rejects already-passing baseline with SEQUENCE_BUGFIX_NOT_RED",
        res_not_red.returncode == EXIT_INVALID and "SEQUENCE_BUGFIX_NOT_RED" in res_not_red.stderr,
        f"rc={res_not_red.returncode} err={res_not_red.stderr!r}",
    )

    res_bp_ok = run_cli(
        "verify-sequence",
        "--base-commit",
        base_ok,
        "--test-command",
        "python3 test_app.py",
        "--repo-root",
        str(repo_ok),
    )
    wt_list = _run_git(repo_ok, "worktree", "list", "--porcelain")
    check(
        "verify-sequence passes clean behavior-preserving sequence without leaking worktrees",
        res_bp_ok.returncode == EXIT_OK and wt_list.count("worktree ") == 1,
        f"rc={res_bp_ok.returncode} err={res_bp_ok.stderr!r} wt={wt_list!r}",
    )


def _test_verify01_c06(tmp: pathlib.Path) -> None:
    repo_c06 = tmp / "v01_c06"
    _init_temp_git_repo(repo_c06)
    (repo_c06 / "semaphore.py").write_text(
        (CORPUS_DIR / "C-06" / "before.py").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    _run_git(repo_c06, "add", ".")
    _run_git(repo_c06, "commit", "-q", "-m", "base C-06")
    base_c06 = _run_git(repo_c06, "rev-parse", "HEAD")

    (repo_c06 / "tests").mkdir()
    (repo_c06 / "tests" / "test_semaphore.py").write_text(
        "import asyncio\n"
        "from semaphore import PrioritySemaphore\n\n"
        "async def main() -> None:\n"
        "    sem = PrioritySemaphore(lambda res, t0, hp: (1.5, 42))\n"
        "    fut: asyncio.Future[str] = asyncio.get_running_loop().create_future()\n"
        "    fut.set_result('ok')\n"
        "    out = await sem.acquire(fut, 0.0, True)\n"
        "    assert out == (1.5, 42), f'expected (1.5, 42), got {out!r}'\n\n"
        "asyncio.run(main())\n",
        encoding="utf-8",
    )
    _run_git(repo_c06, "add", ".")
    _run_git(repo_c06, "commit", "-q", "-m", "c1 characterization test for C-06")

    (repo_c06 / "semaphore.py").write_text(
        (CORPUS_DIR / "C-06" / "after.py").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    _run_git(repo_c06, "add", ".")
    _run_git(repo_c06, "commit", "-q", "-m", "c2 C-06 refactor drops else return")

    res_c06 = run_cli(
        "verify-sequence",
        "--base-commit",
        base_c06,
        "--test-command",
        "PYTHONPATH=. python3 tests/test_semaphore.py",
        "--repo-root",
        str(repo_c06),
    )
    check(
        "VERIFY-01 catches C-06 (Python else: return drop) with SEQUENCE_FINAL_TEST_FAILED",
        res_c06.returncode == EXIT_INVALID and "SEQUENCE_FINAL_TEST_FAILED" in res_c06.stderr,
        f"rc={res_c06.returncode} err={res_c06.stderr!r}",
    )


def _test_verify01_c15(tmp: pathlib.Path) -> None:
    repo_c15 = tmp / "v01_c15"
    _init_temp_git_repo(repo_c15)
    (repo_c15 / "go.mod").write_text(
        "module example.com/c15\n\n"
        "go 1.21\n\n"
        "require github.com/labstack/echo/v5 v5.0.0\n\n"
        "replace github.com/labstack/echo/v5 => ./stub/echo/v5\n",
        encoding="utf-8",
    )
    stub_echo = repo_c15 / "stub" / "echo" / "v5"
    (stub_echo / "middleware").mkdir(parents=True)
    (stub_echo / "go.mod").write_text(
        "module github.com/labstack/echo/v5\n\ngo 1.21\n", encoding="utf-8"
    )
    (stub_echo / "echo.go").write_text(
        "package echo\n\n"
        "type HandlerFunc func() error\n"
        "type MiddlewareFunc func(HandlerFunc) HandlerFunc\n"
        "type Group struct{ LastLimit int64 }\n"
        "type Echo struct{ Groups []*Group }\n"
        "func New() *Echo { return &Echo{} }\n"
        "func (e *Echo) Group(_ string) *Group {\n"
        "    g := &Group{}\n"
        "    e.Groups = append(e.Groups, g)\n"
        "    return g\n"
        "}\n"
        "var RecordedLimit int64\n"
        "func (g *Group) Use(_ MiddlewareFunc) { g.LastLimit = RecordedLimit }\n"
        "func (g *Group) POST(_ string, _ HandlerFunc) {}\n",
        encoding="utf-8",
    )
    (stub_echo / "middleware" / "body_limit.go").write_text(
        'package middleware\n\nimport "github.com/labstack/echo/v5"\n\n'
        "func BodyLimit(limit int64) echo.MiddlewareFunc {\n"
        "    echo.RecordedLimit = limit\n"
        "    return func(next echo.HandlerFunc) echo.HandlerFunc { return next }\n"
        "}\n",
        encoding="utf-8",
    )
    (repo_c15 / "bootstrap").mkdir()
    (repo_c15 / "bootstrap" / "server.go").write_text(
        (CORPUS_DIR / "C-15" / "before.go").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    _run_git(repo_c15, "add", ".")
    _run_git(repo_c15, "commit", "-q", "-m", "base C-15")
    base_c15 = _run_git(repo_c15, "rev-parse", "HEAD")

    (repo_c15 / "bootstrap" / "server_test.go").write_text(
        'package bootstrap\n\nimport (\n    "testing"\n    "github.com/labstack/echo/v5"\n)\n\n'
        "func TestSummarizeBodyLimit(t *testing.T) {\n"
        "    e := echo.New()\n"
        "    registerSummarizeGroup(e, func() error { return nil })\n"
        "    if got := e.Groups[0].LastLimit; got != 1_000_000 {\n"
        '        t.Fatalf("expected 1_000_000 bytes, got %d", got)\n'
        "    }\n"
        "}\n",
        encoding="utf-8",
    )
    _run_git(repo_c15, "add", ".")
    _run_git(repo_c15, "commit", "-q", "-m", "c1 characterization test for C-15")

    (repo_c15 / "bootstrap" / "server.go").write_text(
        (CORPUS_DIR / "C-15" / "after.go").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    _run_git(repo_c15, "add", ".")
    _run_git(repo_c15, "commit", "-q", "-m", "c2 C-15 shifts limit to 1 << 20")

    res_c15 = run_cli(
        "verify-sequence",
        "--base-commit",
        base_c15,
        "--test-command",
        "go test ./bootstrap",
        "--repo-root",
        str(repo_c15),
    )
    check(
        "VERIFY-01 catches C-15 (Go BodyLimit 1_000_000 vs 1 << 20) with SEQUENCE_FINAL_TEST_FAILED",
        res_c15.returncode == EXIT_INVALID and "SEQUENCE_FINAL_TEST_FAILED" in res_c15.stderr,
        f"rc={res_c15.returncode} err={res_c15.stderr!r}",
    )


def _test_verify01_c18(tmp: pathlib.Path) -> None:
    repo_c18 = tmp / "v01_c18"
    _init_temp_git_repo(repo_c18)
    chk_sh = repo_c18 / "scripts" / "check-compose-variables.sh"
    chk_sh.parent.mkdir(parents=True)
    chk_sh.write_text(
        (CORPUS_DIR / "C-18" / "before.sh").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    chk_sh.chmod(0o755)
    _run_git(repo_c18, "add", ".")
    _run_git(repo_c18, "commit", "-q", "-m", "base C-18")
    base_c18 = _run_git(repo_c18, "rev-parse", "HEAD")

    t_c18 = repo_c18 / "tests" / "test_check_compose_variables.sh"
    t_c18.parent.mkdir(parents=True)
    t_c18.write_text(
        "#!/usr/bin/env bash\nset -euo pipefail\n"
        'tmp_dir="$(mktemp -d)"\n'
        "trap 'rm -rf \"$tmp_dir\"' EXIT\n"
        "cat > \"$tmp_dir/docker\" <<'EOF'\n"
        "#!/usr/bin/env bash\n"
        'printf "%s\\n" "$*" > "$(dirname "$0")/args.txt"\n'
        "exit 0\n"
        "EOF\n"
        'chmod +x "$tmp_dir/docker"\n'
        'DOCKER_BIN="$tmp_dir/docker" bash scripts/check-compose-variables.sh\n'
        'if grep -q -- "--no-env-resolution" "$tmp_dir/args.txt"; then\n'
        '  echo "unexpected --no-env-resolution" >&2\n'
        "  exit 1\n"
        "fi\n",
        encoding="utf-8",
    )
    t_c18.chmod(0o755)
    _run_git(repo_c18, "add", ".")
    _run_git(repo_c18, "commit", "-q", "-m", "c1 characterization test for C-18")

    chk_sh.write_text(
        (CORPUS_DIR / "C-18" / "after.sh").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    chk_sh.chmod(0o755)
    _run_git(repo_c18, "add", ".")
    _run_git(repo_c18, "commit", "-q", "-m", "c2 C-18 requires --env-file and skips env resolution")

    res_c18 = run_cli(
        "verify-sequence",
        "--base-commit",
        base_c18,
        "--test-command",
        "bash tests/test_check_compose_variables.sh",
        "--repo-root",
        str(repo_c18),
    )
    check(
        "VERIFY-01 catches C-18 (shell check-compose-variables.sh) with SEQUENCE_FINAL_TEST_FAILED",
        res_c18.returncode == EXIT_INVALID and "SEQUENCE_FINAL_TEST_FAILED" in res_c18.stderr,
        f"rc={res_c18.returncode} err={res_c18.stderr!r}",
    )


def test_verify01_corpus_reproductions(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[15c] VERIFY-01 corpus reproductions (C-06, C-15, C-18)\n")
    _test_verify01_c06(tmp)
    _test_verify01_c15(tmp)
    _test_verify01_c18(tmp)


def test_host_final_check_script(tmp: pathlib.Path) -> None:
    sys.stdout.write("\n[16] scripts/verify/host-final-check.sh integration\n")
    check(
        "scripts/verify/host-final-check.sh exists and is executable",
        HOST_FINAL_CHECK_PATH.is_file() and os.access(HOST_FINAL_CHECK_PATH, os.X_OK),
        str(HOST_FINAL_CHECK_PATH),
    )
    if not HOST_FINAL_CHECK_PATH.is_file():
        return

    bash_bin = shutil.which("bash")
    assert bash_bin is not None

    repo, base_sha = _setup_loop01_repo(tmp, "hfc_repo")
    # Copy hygiene checker into temp repo so host-final-check.sh can invoke it
    hygiene_dst = repo / "scripts" / "lint" / "check_worktree_hygiene.py"
    hygiene_dst.write_text(
        (ROOT / "scripts" / "lint" / "check_worktree_hygiene.py").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "add hygiene")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    (repo / "pkg" / "worker.py").write_text(
        "def alpha(x: int) -> int:\n    result = x + 1\n    return result\n\n"
        "def beta(y: int) -> int:\n    return y * 2\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "clean fix")

    pkt_path = write_json(tmp, "hfc_pkt.json", _make_loop01_packet(base_sha))
    ho_path = write_json(
        tmp,
        "hfc_ho.json",
        _make_loop01_handoff([{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1}], 1),
    )
    runs_dir = tmp / "hfc_runs"

    res_pass = _run_proc(
        [
            bash_bin,
            str(HOST_FINAL_CHECK_PATH),
            "--repo-root",
            str(repo),
            "--target",
            "scripts",
            "--skip-repo-lint",
            "--test-cmd",
            "python3 -c 'import sys; sys.exit(0)'",
            "--handoff",
            str(ho_path),
            "--packet",
            str(pkt_path),
            "--base-commit",
            base_sha,
            "--runs-dir",
            str(runs_dir),
        ]
    )
    check(
        "host-final-check.sh passes on clean repo with lint, test-cmd, and verify-handoff",
        res_pass.returncode == EXIT_OK,
        f"rc={res_pass.returncode} out={res_pass.stdout!r} err={res_pass.stderr!r}",
    )

    # Dirty worktree fails host-final-check.sh
    (repo / "dirty.tmp").write_text("leftover\n", encoding="utf-8")
    res_dirty = _run_proc(
        [
            bash_bin,
            str(HOST_FINAL_CHECK_PATH),
            "--repo-root",
            str(repo),
            "--target",
            "scripts",
            "--skip-repo-lint",
        ]
    )
    (repo / "dirty.tmp").unlink()
    check(
        "host-final-check.sh fails when git status --porcelain is non-empty",
        res_dirty.returncode != EXIT_OK,
        f"rc={res_dirty.returncode} out={res_dirty.stdout!r} err={res_dirty.stderr!r}",
    )


def test_wave7_mf01_mf02_mf03(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[17a] Wave 7 hardening: MF-01 (git mv/staged/gen), MF-02 (AST), MF-03 (comments)\n"
    )

    # MF-01: git mv forbidden.txt -> allowed/moved.txt + staged-only + stripped+declared DO NOT EDIT
    repo = tmp / "w7_mf01"
    _init_temp_git_repo(repo)
    _write_stub_run_sh(repo, exit_code=0)
    (repo / "forbidden.txt").write_text("secret\n", encoding="utf-8")
    (repo / "a.txt").write_text("orig\n", encoding="utf-8")
    (repo / "gen.pb.go").write_text(
        "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage p\nfunc X() {}\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "base")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    (repo / "allowed").mkdir()
    _run_git(repo, "mv", "forbidden.txt", "allowed/moved.txt")
    pkt_mv = _make_loop01_packet(base_sha)
    pkt_mv["scope"]["allowed_paths"] = ["allowed/**"]
    pkt_mv["scope"]["forbidden_paths"] = ["forbidden.txt"]
    pkt_mv_path = write_json(tmp, "w7_pkt_mv.json", pkt_mv)
    ho_mv_path = write_json(
        tmp,
        "w7_ho_mv.json",
        _make_loop01_handoff(
            [{"path": "allowed/moved.txt", "added_lines": 1, "deleted_lines": 0}], 0
        ),
    )
    res_mv = run_cli(
        "verify-handoff",
        str(ho_mv_path),
        "--packet",
        str(pkt_mv_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "MF-01 catches git mv from forbidden_paths with PATH_IN_FORBIDDEN_PATHS",
        res_mv.returncode == EXIT_INVALID and "PATH_IN_FORBIDDEN_PATHS" in res_mv.stderr,
        f"rc={res_mv.returncode} err={res_mv.stderr!r}",
    )

    _run_git(repo, "reset", "--hard", "-q", base_sha)
    (repo / "a.txt").write_text("STAGED_ONLY\n", encoding="utf-8")
    _run_git(repo, "add", "a.txt")
    (repo / "a.txt").write_text("orig\n", encoding="utf-8")
    pkt_stg = _make_loop01_packet(base_sha)
    pkt_stg["scope"]["allowed_paths"] = ["**"]
    pkt_stg_path = write_json(tmp, "w7_pkt_stg.json", pkt_stg)
    ho_empty = write_json(tmp, "w7_ho_empty.json", _make_loop01_handoff([], 0))
    res_stg = run_cli(
        "verify-handoff",
        str(ho_empty),
        "--packet",
        str(pkt_stg_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "MF-01 catches staged-only edit even when worktree matches base_commit",
        res_stg.returncode == EXIT_INVALID and "UNDECLARED_MODIFIED_FILE" in res_stg.stderr,
        f"rc={res_stg.returncode} err={res_stg.stderr!r}",
    )

    _run_git(repo, "reset", "--hard", "-q", base_sha)
    (repo / "gen.pb.go").write_text("package p\nfunc X() {}\n", encoding="utf-8")
    _run_git(repo, "add", "gen.pb.go")
    _run_git(repo, "commit", "-q", "-m", "strip gen header")
    ho_gen = write_json(
        tmp,
        "w7_ho_gen.json",
        _make_loop01_handoff([{"path": "gen.pb.go", "added_lines": 1, "deleted_lines": 1}], 0),
    )
    res_gen = run_cli(
        "verify-handoff",
        str(ho_gen),
        "--packet",
        str(pkt_stg_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "MF-01 catches committed stripped DO NOT EDIT header even when declared",
        res_gen.returncode == EXIT_INVALID and "GENERATED_CODE_MODIFIED" in res_gen.stderr,
        f"rc={res_gen.returncode} err={res_gen.stderr!r}",
    )


def test_wave7_mf02_mf03_ast_and_comments(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[17b] Wave 7 hardening: MF-02 AST Go/Rust/TS/Svelte & MF-03 lexer comments\n"
    )
    repo = tmp / "w7_mf02"
    _init_temp_git_repo(repo)
    _write_stub_run_sh(repo, exit_code=0)
    (repo / "svc.go").write_text(
        'package p\ntype Srv struct{}\nfunc Alpha() string { return "{" }\n'
        "func (s *Srv) Foo() int { return 1 }\nfunc F1() int { return 1 }\n",
        encoding="utf-8",
    )
    (repo / "lib.rs").write_text(
        'fn alpha() -> &\'static str { "{" }\nimpl A { fn new() -> Self { A } }\n'
        "pub fn calc() -> i32 { 42 }\n",
        encoding="utf-8",
    )
    (repo / "app.py").write_text("def f() -> str:\n    return 'ok'\n", encoding="utf-8")
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "base")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    # MF-02 + MF-03: Go brace-string + receiver rename (2 funcs), Rust #[must_use] (0 comment FP, 1 fn)
    (repo / "svc.go").write_text(
        'package p\ntype Srv struct{}\nfunc Alpha() string { return "{" }\n'
        "func (srv *Srv) Foo() int { return 1 }\nfunc F1() int { return 2 }\n",
        encoding="utf-8",
    )
    (repo / "lib.rs").write_text(
        'fn alpha() -> &\'static str { "{" }\nimpl Pre { fn new() -> Self { Pre } }\n'
        "impl A { fn new() -> Self { A } }\n#[must_use]\npub fn calc() -> i32 { 42 }\n",
        encoding="utf-8",
    )
    (repo / "app.py").write_text(
        'def f() -> str:\n    return """a\n# heading in multiline string\n"""\n',
        encoding="utf-8",
    )
    pkt = _make_loop01_packet(base_sha)
    pkt["scope"]["allowed_paths"] = ["**"]
    pkt["scope"]["max_functions"] = 10
    pkt_path = write_json(tmp, "w7_mf02_pkt.json", pkt)
    ho_ok = write_json(
        tmp,
        "w7_mf02_ho_ok.json",
        _make_loop01_handoff(
            [
                {"path": "svc.go", "added_lines": 2, "deleted_lines": 2},
                {"path": "lib.rs", "added_lines": 2, "deleted_lines": 0},
                {"path": "app.py", "added_lines": 2, "deleted_lines": 1},
            ],
            4,
        ),
    )
    res_ok = run_cli(
        "verify-handoff",
        str(ho_ok),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "MF-02/MF-03 accurately counts Go/Rust/Py functions and allows Rust #[must_use] & multiline '#'",
        res_ok.returncode == EXIT_OK,
        f"rc={res_ok.returncode} err={res_ok.stderr!r}",
    )

    # MF-03: trailing inline excuse comment with bare ADR-000001 (no suppression) MUST be rejected
    (repo / "app.py").write_text(
        "def f() -> str:\n    return 'ok'  # sneaky excuse citing ADR-000001\n",
        encoding="utf-8",
    )
    res_inline = run_cli(
        "verify-handoff",
        str(ho_ok),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "MF-03 catches trailing inline excuse comment even when citing bare ADR-000001",
        res_inline.returncode == EXIT_INVALID and "UNREQUESTED_COMMENT_ADDED" in res_inline.stderr,
        f"rc={res_inline.returncode} err={res_inline.stderr!r}",
    )


def test_wave7_mf04_fs01_fs02(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[17c] Wave 7 hardening: MF-04 (sha/zero/flow), FS-01 (gate/CWE-22), FS-02 (branch)\n"
    )

    # MF-04: missing command_sha256 & all-zero at_commit rejected
    p_no_sha = copy.deepcopy(base_packet())
    del p_no_sha["execution_policy"]["measured"]["command_sha256"]
    _assert_packet_rejected(
        tmp, "MF-04 rejects measured without command_sha256", p_no_sha, "command_sha256"
    )

    p_zero_sha = copy.deepcopy(base_packet())
    p_zero_sha["execution_policy"]["measured"]["at_commit"] = "0000000"
    _assert_packet_rejected(
        tmp,
        "MF-04 rejects all-zero measured.at_commit",
        p_zero_sha,
        "PLACEHOLDER_MEASURED_COMMIT",
    )

    # MF-04: measure handles YAML flow execution_policy and fenced JSON with \d
    repo = tmp / "w7_mf04_repo"
    _init_temp_git_repo(repo)
    _run_git(repo, "checkout", "-q", "-b", "feat/demo")
    _write_stub_run_sh(repo, exit_code=0)
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "init")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    flow_md = tmp / "w7_flow.md"
    flow_md.write_text(
        "---\n"
        'packet_version: "1.0"\n'
        'packet_id: "FLOW-001"\n'
        'scope: { target: "scripts", allowed_paths: ["scripts/**"], max_files: 5, '
        'max_functions: 3, allowed_rules: ["S101"], stop_condition: "done" }\n'
        'execution_policy: { policy: "fix-existing", foreground_only: true, '
        "forbid_repo_wide_test_build: true, forbid_bulk_autofix: true, "
        'verification_command: "./scripts/lint/run.sh scripts" }\n'
        "handoff_requirements: { require_modified_files_and_lines: true, "
        "require_runtime_behavior_impact: true, require_change_rationale: true }\n"
        "---\n\n## 1. 範囲と作業量制約\n\n## 2. 実行・検証手順\n\n## 3. Hand-off 記述必須項目\n",
        encoding="utf-8",
    )
    res_flow_m = run_cli("measure", str(flow_md), "--repo-root", str(repo))
    res_flow_v = run_cli("validate-packet", str(flow_md))
    check(
        "MF-04 measure expands flow-mapping execution_policy cleanly",
        res_flow_m.returncode == EXIT_OK and res_flow_v.returncode == EXIT_OK,
        res_flow_m.stderr or res_flow_v.stderr,
    )

    fenced_md = tmp / "w7_fenced_d.md"
    p_bs = copy.deepcopy(base_packet())
    del p_bs["execution_policy"]["measured"]
    p_bs["scope"]["stop_condition"] = r"Stop when \d+ errors reach 0"
    fenced_md.write_text(
        f"# P\n\n```json\n{json.dumps(p_bs, indent=2)}\n```\n\n"
        "## 1. 範囲と作業量制約\n\n## 2. 実行・検証手順\n\n## 3. Hand-off 記述必須項目\n",
        encoding="utf-8",
    )
    res_bs_m = run_cli("measure", str(fenced_md), "--repo-root", str(repo))
    res_bs_v = run_cli("validate-packet", str(fenced_md))
    check(
        "MF-04 measure preserves \\d backslashes in Markdown fenced JSON",
        res_bs_m.returncode == EXIT_OK and res_bs_v.returncode == EXIT_OK,
        res_bs_m.stderr or res_bs_v.stderr,
    )

    # FS-01: invalid packet with traversal packet_id does NOT execute or write outside runs_dir
    runs_dir = tmp / "w7_runs"
    evil_pkt = _make_loop01_packet(base_sha)
    evil_pkt["packet_id"] = "../../escaped_outside"
    evil_pkt_path = write_json(tmp, "w7_evil_pkt.json", evil_pkt)
    evil_ho = _make_loop01_handoff([], 0)
    evil_ho["packet_id"] = "../../escaped_outside"
    evil_ho_path = write_json(tmp, "w7_evil_ho.json", evil_ho)
    res_evil = run_cli(
        "verify-handoff",
        str(evil_ho_path),
        "--packet",
        str(evil_pkt_path),
        "--repo-root",
        str(repo),
        "--runs-dir",
        str(runs_dir),
    )
    check(
        "FS-01 blocks packet_id path traversal and skips rerun on invalid packet",
        res_evil.returncode == EXIT_INVALID
        and not (tmp / "escaped_outside.json").exists()
        and not list(runs_dir.rglob("*.json")),
        f"rc={res_evil.returncode} err={res_evil.stderr!r}",
    )

    # FS-02: branch feat/demo maps to runs_dir/feat-demo/verify/<id>.json (matching harness_run.py)
    good_pkt = write_json(tmp, "w7_fs02_pkt.json", _make_loop01_packet(base_sha))
    good_ho = write_json(tmp, "w7_fs02_ho.json", _make_loop01_handoff([], 0))
    res_fs02 = run_cli(
        "verify-handoff",
        str(good_ho),
        "--packet",
        str(good_pkt),
        "--repo-root",
        str(repo),
        "--runs-dir",
        str(runs_dir),
    )
    expected_ev = runs_dir / "feat-demo" / "verify" / "LOOP01-TEST.json"
    check(
        "FS-02 verify-handoff writes evidence under harness_run branch dir 'feat-demo'",
        res_fs02.returncode == EXIT_OK and expected_ev.is_file(),
        f"rc={res_fs02.returncode} expected={expected_ev} err={res_fs02.stderr!r}",
    )


def test_wave7_mf05_fs03_fs04_hfc(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[17d] Wave 7 hardening: MF-05/FS-03/FS-04 (verify-sequence) & C-02/CS-01 (host-final-check)\n"
    )

    # MF-05 + FS-03: dirty worktree, c1 deletion, --bugfix c2 mutation, background '&' test_command
    repo = tmp / "w7_seq"
    _init_temp_git_repo(repo)
    (repo / "calc.py").write_text("def add(a, b):\n    return a - b\n", encoding="utf-8")
    (repo / "tests").mkdir()
    (repo / "src").mkdir()
    (repo / "src" / "prod.txt").write_text("ORIGINAL_PROD\n", encoding="utf-8")
    (repo / "tests" / "test_alias.py").symlink_to("../src/prod.txt")
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "base")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    # FS-04: replacing tracked symlink tests/test_alias.py in c1 must NOT overwrite src/prod.txt
    (repo / "tests" / "test_alias.py").unlink()
    (repo / "tests" / "test_alias.py").write_text("assert True\n", encoding="utf-8")
    _run_git(repo, "add", "tests/test_alias.py")
    _run_git(repo, "commit", "-q", "-m", "c1 replace symlink with real test")
    (repo / "calc.py").write_text("def add(a, b):\n    return a + b\n", encoding="utf-8")
    _run_git(repo, "add", "calc.py")
    _run_git(repo, "commit", "-q", "-m", "c2 impl")

    res_fs04 = run_cli(
        "verify-sequence",
        "--base-commit",
        base_sha,
        "--test-command",
        "grep -q ORIGINAL_PROD src/prod.txt",
        "--repo-root",
        str(repo),
    )
    check(
        "FS-04 unlinks symlink in overlay worktree instead of overwriting src/prod.txt",
        res_fs04.returncode == EXIT_OK,
        f"rc={res_fs04.returncode} err={res_fs04.stderr!r}",
    )

    # FS-03: background '&' in --test-command rejected
    res_fs03 = run_cli(
        "verify-sequence",
        "--base-commit",
        base_sha,
        "--test-command",
        "false &",
        "--repo-root",
        str(repo),
    )
    check(
        "FS-03 rejects background '&' in verify-sequence --test-command",
        res_fs03.returncode == EXIT_INVALID and "FORBIDDEN_BACKGROUND_EXECUTION" in res_fs03.stderr,
        f"rc={res_fs03.returncode} err={res_fs03.stderr!r}",
    )

    # MF-05: dirty repo_root fails with SEQUENCE_DIRTY_WORKTREE
    (repo / "calc.py").write_text("def add(a, b):\n    return 999\n", encoding="utf-8")
    res_dirty = run_cli(
        "verify-sequence",
        "--base-commit",
        base_sha,
        "--test-command",
        "grep -q ORIGINAL_PROD src/prod.txt",
        "--repo-root",
        str(repo),
    )
    _run_git(repo, "checkout", "--", "calc.py")
    check(
        "MF-05 fails closed with SEQUENCE_DIRTY_WORKTREE on dirty repo_root",
        res_dirty.returncode == EXIT_INVALID and "SEQUENCE_DIRTY_WORKTREE" in res_dirty.stderr,
        f"rc={res_dirty.returncode} err={res_dirty.stderr!r}",
    )

    # CS-01 + C-02: host-final-check.sh rejects --target --list / all / bg test-cmd & forwards --policy block-new
    bash_bin = shutil.which("bash")
    assert bash_bin is not None
    res_flag_t = _run_proc([bash_bin, str(HOST_FINAL_CHECK_PATH), "--target", "--list"])
    res_all_t = _run_proc([bash_bin, str(HOST_FINAL_CHECK_PATH), "--target", "all"])
    res_bg_cmd = _run_proc(
        [
            bash_bin,
            str(HOST_FINAL_CHECK_PATH),
            "--repo-root",
            str(repo),
            "--target",
            "scripts",
            "--skip-repo-lint",
            "--test-cmd",
            "false &",
        ]
    )
    check(
        "CS-01 host-final-check.sh rejects --target --list, --target all, and background --test-cmd",
        res_flag_t.returncode == EXIT_USAGE
        and res_all_t.returncode == EXIT_USAGE
        and res_bg_cmd.returncode == EXIT_USAGE,
        f"flag={res_flag_t.returncode} all={res_all_t.returncode} bg={res_bg_cmd.returncode}",
    )

    # C-02: verify --policy block-new forwarding from --packet
    hfc_repo = tmp / "w7_hfc_policy"
    _init_temp_git_repo(hfc_repo)
    log_file = tmp / "hfc_run_sh_args.txt"
    run_sh = hfc_repo / "scripts" / "lint" / "run.sh"
    run_sh.parent.mkdir(parents=True, exist_ok=True)
    run_sh.write_text(
        f'#!/usr/bin/env bash\nprintf "%s\\n" "$*" > "{log_file}"\nexit 0\n',
        encoding="utf-8",
    )
    run_sh.chmod(0o755)
    _run_git(hfc_repo, "add", ".")
    _run_git(hfc_repo, "commit", "-q", "-m", "init")
    p_bn = copy.deepcopy(base_packet())
    p_bn["execution_policy"]["policy"] = "block-new"
    _set_verify_cmd(p_bn, "./scripts/lint/run.sh --policy block-new scripts")
    p_bn_path = write_json(tmp, "w7_hfc_bn.json", p_bn)
    res_bn = _run_proc(
        [
            bash_bin,
            str(HOST_FINAL_CHECK_PATH),
            "--repo-root",
            str(hfc_repo),
            "--target",
            "scripts",
            "--packet",
            str(p_bn_path),
            "--skip-repo-lint",
        ]
    )
    recorded_args = log_file.read_text(encoding="utf-8").strip() if log_file.is_file() else ""
    check(
        "C-02 host-final-check.sh forwards --policy block-new from --packet to run.sh",
        res_bn.returncode == EXIT_OK and recorded_args == "--policy block-new scripts",
        f"rc={res_bn.returncode} args={recorded_args!r} err={res_bn.stderr!r}",
    )


def test_wave8_packet_verify_hardening(tmp: pathlib.Path) -> None:
    sys.stdout.write(
        "\n[18] Wave 8 hardening: Python AST ruff-invariance, syntax fail-closed, nested decorators, thin wrappers & base_commit template\n"
    )

    # 1. task_packet.py and host-final-check.sh are thin wrappers with no dead Python duplicate
    max_wrapper_lines = 40
    tp_lines = SCRIPT_PATH.read_text(encoding="utf-8").splitlines()
    hfc_text = HOST_FINAL_CHECK_PATH.read_text(encoding="utf-8")
    check(
        "task_packet.py is a thin os.execv wrapper (< 40 lines)",
        len(tp_lines) < max_wrapper_lines and "os.execv(" in "\n".join(tp_lines),
        f"lines={len(tp_lines)}",
    )
    check(
        "host-final-check.sh delegates to alt-harness host-final without importing task_packet",
        "import task_packet" not in hfc_text
        and "alt-harness" in hfc_text
        and "host-final" in hfc_text,
        hfc_text,
    )

    # 2. task-packet.md.tmpl includes base_commit
    tmpl_text = PACKET_TMPL_PATH.read_text(encoding="utf-8")
    check(
        "task-packet.md.tmpl declares base_commit in YAML frontmatter",
        'base_commit: "' in tmpl_text,
        "missing base_commit in task-packet.md.tmpl",
    )

    # 3. verify-handoff Python AST ruff-format invariance, nested decorator edit, and broken syntax fail-closed
    repo = tmp / "w8_py_ast"
    _init_temp_git_repo(repo)
    _write_stub_run_sh(repo, exit_code=0)
    (repo / "mod.py").write_text(
        "def greet(name: str) -> str:\n"
        "    return 'hello ' + name\n\n"
        "def outer(x: int) -> int:\n"
        "    @deco('v1')\n"
        "    def inner(y: int) -> int:\n"
        "        return y + 1\n"
        "    return inner(x)\n",
        encoding="utf-8",
    )
    _run_git(repo, "add", ".")
    _run_git(repo, "commit", "-q", "-m", "base")
    base_sha = _run_git(repo, "rev-parse", "HEAD")

    pkt = _make_loop01_packet(base_sha)
    pkt["scope"]["allowed_paths"] = ["**"]
    pkt["scope"]["max_functions"] = 5
    pkt_path = write_json(tmp, "w8_pkt.json", pkt)

    # 3a. Format-only quote/comma/wrap edit -> 0 modified functions
    (repo / "mod.py").write_text(
        "def greet(\n"
        "    name: str,\n"
        ") -> str:\n"
        "    return (\n"
        '        "hello "\n'
        "        + name\n"
        "    )\n\n"
        "def outer(x: int) -> int:\n"
        '    @deco("v1")\n'
        "    def inner(y: int,) -> int:\n"
        "        return y + 1\n"
        "    return inner(x)\n",
        encoding="utf-8",
    )
    ho_zero = write_json(
        tmp,
        "w8_ho_zero.json",
        _make_loop01_handoff([{"path": "mod.py", "added_lines": 7, "deleted_lines": 2}], 0),
    )
    res_fmt = run_cli(
        "verify-handoff",
        str(ho_zero),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff counts 0 modified functions for ruff-format-only Python edit",
        res_fmt.returncode == EXIT_OK,
        f"rc={res_fmt.returncode} err={res_fmt.stderr!r}",
    )

    # 3b. Nested decorated inner function edit -> 1 modified function (outer not double-counted)
    (repo / "mod.py").write_text(
        "def greet(name: str) -> str:\n"
        "    return 'hello ' + name\n\n"
        "def outer(x: int) -> int:\n"
        "    @deco('v2')\n"
        "    def inner(y: int) -> int:\n"
        "        return y + 2\n"
        "    return inner(x)\n",
        encoding="utf-8",
    )
    ho_one = write_json(
        tmp,
        "w8_ho_one.json",
        _make_loop01_handoff([{"path": "mod.py", "added_lines": 2, "deleted_lines": 2}], 1),
    )
    res_nested = run_cli(
        "verify-handoff",
        str(ho_one),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff counts 1 modified function when editing a nested decorated Python function",
        res_nested.returncode == EXIT_OK,
        f"rc={res_nested.returncode} err={res_nested.stderr!r}",
    )

    # 3c. Broken Python syntax -> fail closed with VERIFY_HANDOFF_ERROR
    (repo / "mod.py").write_text("def broken(:\n    return 1\n", encoding="utf-8")
    res_broken = run_cli(
        "verify-handoff",
        str(ho_zero),
        "--packet",
        str(pkt_path),
        "--repo-root",
        str(repo),
        "--no-rerun",
    )
    check(
        "verify-handoff fails closed on broken Python syntax",
        res_broken.returncode == EXIT_INVALID and "VERIFY_HANDOFF_ERROR" in res_broken.stderr,
        f"rc={res_broken.returncode} err={res_broken.stderr!r}",
    )


def main() -> int:
    sys.stdout.write("Task Packet specification & validator tests\n")
    test_schema_structure()
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp = pathlib.Path(tmpdir)
        test_valid_packets(tmp)
        test_invalid_packets_schema(tmp)
        test_invalid_packets_policy(tmp)
        test_invalid_packet_markdown_body(tmp)
        test_handoff_standalone(tmp)
        test_handoff_rejections_and_cross_validation(tmp)
        test_templates_and_cli_utilities(tmp)
        test_check01_candidate_enforcement(tmp)
        test_check02_policy_field(tmp)
        test_check05_generated_code_protection(tmp)
        test_wave4_hardening(tmp)
        test_wave4_exclusions_and_security(tmp)
        test_wave2_test_existence_and_header_deletion(tmp)
        test_loop02_validation(tmp)
        test_loop02_measure_cli(tmp)
        test_loop01_verify_handoff_core(tmp)
        test_loop01_verify_handoff_lint_fix_extras(tmp)
        test_verify01_sequence_rejections(tmp)
        test_verify01_sequence_modes(tmp)
        test_verify01_corpus_reproductions(tmp)
        test_host_final_check_script(tmp)
        test_wave7_mf01_mf02_mf03(tmp)
        test_wave7_mf02_mf03_ast_and_comments(tmp)
        test_wave7_mf04_fs01_fs02(tmp)
        test_wave7_mf05_fs03_fs04_hfc(tmp)
        test_wave8_packet_verify_hardening(tmp)

    sys.stdout.write(
        f"\nSummary: {RESULTS.passed} passed, {RESULTS.failed} failed\n",
    )
    return EXIT_OK if RESULTS.failed == 0 else EXIT_INVALID


if __name__ == "__main__":
    sys.exit(main())
