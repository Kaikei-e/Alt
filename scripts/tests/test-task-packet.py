#!/usr/bin/env python3
"""TDD unit and CLI tests for scripts/agent-harness/task_packet.py and its schema/templates.

Run:
    python3 scripts/tests/test-task-packet.py
"""

from __future__ import annotations

import copy
import json
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

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2
MAX_FILES_DEFAULT = 10
MAX_FUNCTIONS_DEFAULT = 3


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


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 -- fixed argv invoking repo CLI with sys.executable
        [sys.executable, str(SCRIPT_PATH), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def base_packet() -> dict[str, Any]:
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
            "verification_command": "./scripts/lint/run.sh scripts",
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
    multi["execution_policy"]["verification_command"] = (
        "./scripts/lint/run.sh scripts && LINT_BASE=HEAD~1 ./scripts/lint/run.sh harness"
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
    pkt_quoted_ok["execution_policy"]["verification_command"] = './scripts/lint/run.sh "scripts"'
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
        "render-template --kind packet exits 0",
        res_rp.returncode == EXIT_OK and "RENDER-42" in res_rp.stdout,
        res_rp.stderr,
    )
    rendered_pkt = tmp / "rendered_packet.md"
    rendered_pkt.write_text(res_rp.stdout, encoding="utf-8")
    res_vp = run_cli("validate-packet", str(rendered_pkt))
    check(
        "rendered packet template validates cleanly",
        res_vp.returncode == EXIT_OK,
        res_vp.stderr or res_vp.stdout,
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
        "rendered handoff template cross-validates cleanly against rendered packet",
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
    res_vpj = run_cli("validate-packet", str(rendered_json))
    check(
        "render-template --format json produces valid TaskPacket JSON",
        res_rp_json.returncode == EXIT_OK and res_vpj.returncode == EXIT_OK,
        res_rp_json.stderr or res_vpj.stderr,
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


def _run_git(repo: pathlib.Path, *git_args: str) -> None:
    git_bin = shutil.which("git")
    assert git_bin is not None
    subprocess.run(  # noqa: S603 -- test helper invoking resolved git binary in temp repo
        [git_bin, "-C", str(repo), *git_args],
        check=True,
        capture_output=True,
        text=True,
    )


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
    p_block["execution_policy"]["verification_command"] = (
        "LINT_POLICY=block-new ./scripts/lint/run.sh scripts"
    )
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
    p_block_flag["execution_policy"]["verification_command"] = (
        "./scripts/lint/run.sh --policy block-new scripts"
    )
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

    sys.stdout.write(
        f"\nSummary: {RESULTS.passed} passed, {RESULTS.failed} failed\n",
    )
    return EXIT_OK if RESULTS.failed == 0 else EXIT_INVALID


if __name__ == "__main__":
    sys.exit(main())
