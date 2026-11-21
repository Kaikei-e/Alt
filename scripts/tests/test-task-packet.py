#!/usr/bin/env python3
"""TDD unit and CLI tests for scripts/agent-harness/task_packet.py and its schema/templates.

Run:
    python3 scripts/tests/test-task-packet.py
"""

from __future__ import annotations

import copy
import json
import pathlib
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

    sys.stdout.write(
        f"\nSummary: {RESULTS.passed} passed, {RESULTS.failed} failed\n",
    )
    return EXIT_OK if RESULTS.failed == 0 else EXIT_INVALID


if __name__ == "__main__":
    sys.exit(main())
