#!/usr/bin/env python3
"""Self-check for Alt coding-agent harness wiring, guards, rules, and docs."""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any

_SELF_DIR = str(Path(__file__).resolve().parent)
if _SELF_DIR not in sys.path:
    sys.path.insert(0, _SELF_DIR)

import guard_event_time  # noqa: E402

BAD_ENV_VARS: tuple[str, ...] = ("CLAUDE_TOOL_INPUT", "CLAUDE_FILE_PATHS")
REPO_SCRIPT_REF_RE = re.compile(
    r"(?:\$CLAUDE_PROJECT_DIR|\$\{CLAUDE_PROJECT_DIR\}|\$\(git rev-parse --show-toplevel\))/([^\s'\"]+)"
)
SHARED_MARKER_RE = re.compile(r"^\s*<!--\s*shared:([A-Za-z0-9_.-]+):(start|end)\s*-->\s*$")
MAX_CLAUDE_MD_LINES = 200
MAX_AGENTS_MD_BYTES = 32768
QUOTED_MIN_LEN = 2
FAILURE_PATTERNS_REL = ".claude/skills/failure-pattern-review/references/patterns.md"
FAILURE_SKILL_REL = ".claude/skills/failure-pattern-review/SKILL.md"
REQUIRED_FP_LEADINS: tuple[str, ...] = (
    "**機構.**",
    "**先例.**",
    "**問い.**",
    "**確認コマンド.**",
    "**機械的に見ているもの.**",
)
FP_SECTION_HEADING_RE = re.compile(r"^##\s+FP-(\d{2})\s+(.+?)\s*$")
H2_HEADING_RE = re.compile(r"^##(?:\s|$)")
ADR_PRECEDENT_RE = re.compile(r"(?<![A-Za-z0-9-])ADR-(\d{6})(?![0-9])")
PM_PRECEDENT_RE = re.compile(r"(?<![A-Za-z0-9-])PM-(\d{4}-\d{3})(?![0-9])")
FP_TABLE_ID_RE = re.compile(r"(?<![A-Za-z0-9-])FP-(\d+)(?![0-9])")
ABBREV_PRECEDENT_RE = re.compile(
    r"(?<![A-Za-z0-9-])(?:ADR-\d{6}|PM-\d{4}-\d{3})"
    r"(?:\s*[/、]\s*(?:ADR-\d{6}|PM-\d{4}-\d{3}))*"
    r"\s*[/、]\s*(\d{3,6})(?![0-9])"
)
REPO_PATH_TOKEN_RE = re.compile(
    r"(?<![A-Za-z0-9_./-])((?:scripts|\.github|\.claude/skills)/[^\s`'\"()（）,、。]+)"
)
LEADIN_BREAK_RE = re.compile(r"\*\*(?:機構|先例|問い|確認コマンド|機械的に見ているもの)\.\*\*")
RULE_LEDGER_REL = ".agents/rules/rule-ledger.md"
RATCHET_SUPPRESSIONS_REL = "scripts/lint/ratchet/suppressions.toml"
FILE_LIKE_EXTENSIONS: tuple[str, ...] = (".py", ".sh", ".yml", ".yaml", ".toml", ".md")
AST_GREP_RULES_REL = Path("scripts") / "lint" / "ast-grep" / "rules"
RULE_LEDGER_HEADERS: tuple[str, ...] = ("ID", "規則", "強制手段", "由来")
CORPUS_ID_RE = re.compile(r"(?<![A-Za-z0-9-])(C-\d{2,})(?![0-9])")
BACKTICK_SPAN_RE = re.compile(r"`([^`\n]+)`")
MAX_JUDGMENT_FM_RE = re.compile(r"^\s*max_judgment_rules\s*:\s*([+-]?\d+)\s*$")
MAX_JUDGMENT_COMMENT_RE = re.compile(r"<!--\s*max_judgment_rules\s*:\s*([+-]?\d+)\s*-->")
AST_GREP_ID_LINE_RE = re.compile(r"^\s*id\s*:\s*([A-Za-z0-9_.-]+)\s*$")


@dataclass(frozen=True)
class Finding:
    finding_id: str
    path: str
    message: str


def _git_executable() -> str:
    git_bin = shutil.which("git")
    if git_bin is None:
        msg = "git executable not found on PATH"
        raise FileNotFoundError(msg)
    return git_bin


def _resolve_default_root() -> Path:
    res = subprocess.run(  # noqa: S603 -- git resolved via shutil.which with explicit args
        [_git_executable(), "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode == 0 and res.stdout.strip():
        return Path(res.stdout.strip()).resolve()
    return Path(__file__).resolve().parents[2]


def _git_ls_files(root: Path) -> list[str]:
    res = subprocess.run(  # noqa: S603 -- git resolved via shutil.which with explicit args
        [_git_executable(), "-C", str(root), "ls-files", "-z"],
        capture_output=True,
        check=False,
    )
    if res.returncode != 0:
        return []
    raw = res.stdout.decode("utf-8", errors="replace")
    return [entry for entry in raw.split("\x00") if entry]


def _iter_hook_objects_with_matchers(entries: object) -> list[tuple[object, dict[str, Any]]]:
    result: list[tuple[object, dict[str, Any]]] = []
    if not isinstance(entries, list):
        return result
    for group in entries:
        if not isinstance(group, dict):
            continue
        matcher = group.get("matcher")
        nested = group.get("hooks")
        if isinstance(nested, list):
            result.extend((matcher, hook) for hook in nested if isinstance(hook, dict))
        elif "type" in group or "command" in group:
            result.append((matcher, group))
    return result


def _matcher_matches_tool(matcher: object, tool_name: str) -> bool:
    if matcher is None:
        return True
    if not isinstance(matcher, str):
        return False
    pat = matcher.strip()
    if pat in ("", "*"):
        return True
    try:
        return re.fullmatch(pat, tool_name) is not None
    except re.error:
        return pat == tool_name


def _load_hook_config(path: Path, rel_label: str, findings: list[Finding]) -> dict[str, Any] | None:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        findings.append(Finding("H4", rel_label, f"could not parse hook config: {exc}"))
        return None
    if not isinstance(data, dict):
        findings.append(Finding("H4", rel_label, "hook config must be a JSON object"))
        return None
    return data


def _check_hook_file(
    root: Path,
    rel_label: str,
    cfg: dict[str, Any],
    *,
    is_claude: bool,
    findings: list[Finding],
) -> int:
    hooks_map = cfg.get("hooks")
    if not isinstance(hooks_map, dict):
        findings.append(Finding("H4", rel_label, "missing 'hooks' object"))
        return 0

    total_commands = 0
    pre_tool_commands: list[str] = []
    bash_ops_found = False
    bash_exec_found = False

    for event_name, entries in hooks_map.items():
        for matcher, hook in _iter_hook_objects_with_matchers(entries):
            if hook.get("type") != "command":
                continue
            total_commands += 1
            cmd = hook.get("command")
            cmd_str = cmd if isinstance(cmd, str) else ""

            if event_name == "PreToolUse":
                pre_tool_commands.append(cmd_str)
                if is_claude and hook.get("onFailure") != "block":
                    findings.append(
                        Finding(
                            "H3",
                            rel_label,
                            'PreToolUse command hook lacks "onFailure": "block"',
                        )
                    )
                if "guard_ops_commands.py" in cmd_str:
                    if not is_claude and "--on-match deny" not in cmd_str:
                        findings.append(
                            Finding(
                                "H4",
                                rel_label,
                                'PreToolUse guard_ops_commands.py entry must pass "--on-match deny" because Codex does not support the ask decision and the call would go through',
                            )
                        )
                    if _matcher_matches_tool(matcher, "Bash"):
                        bash_ops_found = True
                if "guard_exec_commands.py" in cmd_str and _matcher_matches_tool(matcher, "Bash"):
                    bash_exec_found = True

            findings.extend(
                Finding(
                    "H1",
                    rel_label,
                    f"hook command references non-existent {var}; read JSON from stdin instead",
                )
                for var in BAD_ENV_VARS
                if var in cmd_str
            )

            findings.extend(
                Finding(
                    "H2",
                    rel_label,
                    f"hook command references missing repo script '{match.group(1)}'",
                )
                for match in REPO_SCRIPT_REF_RE.finditer(cmd_str)
                if not (root / match.group(1)).exists()
            )

    findings.extend(
        Finding(
            "H4",
            rel_label,
            f"PreToolUse hooks do not reference {required_guard}",
        )
        for required_guard in (
            "guard_protected_paths.py",
            "guard_event_time.py",
            "guard_packet_scope.py",
        )
        if not any(required_guard in cmd for cmd in pre_tool_commands)
    )

    if not bash_ops_found:
        findings.append(
            Finding(
                "H4",
                rel_label,
                'PreToolUse hooks do not reference guard_ops_commands.py with a matcher matching "Bash"',
            )
        )
    if not bash_exec_found:
        findings.append(
            Finding(
                "H4",
                rel_label,
                'PreToolUse hooks do not reference guard_exec_commands.py with a matcher matching "Bash"',
            )
        )

    return total_commands


def _expand_braces(pattern: str) -> list[str]:
    """Expand brace alternatives such as {a,b,c} (including nested braces)."""
    start = -1
    depth = 0
    for idx, ch in enumerate(pattern):
        if ch == "{":
            if depth == 0:
                start = idx
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0 and start != -1:
                prefix = pattern[:start]
                inner = pattern[start + 1 : idx]
                suffix = pattern[idx + 1 :]
                options: list[str] = []
                buf: list[str] = []
                inner_depth = 0
                for c in inner:
                    if c == "{":
                        inner_depth += 1
                        buf.append(c)
                    elif c == "}":
                        inner_depth -= 1
                        buf.append(c)
                    elif c == "," and inner_depth == 0:
                        options.append("".join(buf))
                        buf = []
                    else:
                        buf.append(c)
                options.append("".join(buf))

                expanded: list[str] = []
                for opt in options:
                    expanded.extend(_expand_braces(prefix + opt + suffix))
                return expanded
    return [pattern]


def _glob_to_regex(pattern: str) -> re.Pattern[str]:
    pat = pattern.strip()
    pat = pat.removeprefix("./")
    out: list[str] = ["^"]
    i = 0
    n = len(pat)
    while i < n:
        if pat[i : i + 3] == "**/":
            out.append("(?:.+/)?")
            i += 3
        elif pat[i : i + 3] == "/**" and i + 3 == n:
            out.append("/.+")
            i += 3
        elif pat[i : i + 2] == "**":
            out.append(".*")
            i += 2
        elif pat[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pat[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pat[i]))
            i += 1
    out.append("$")
    return re.compile("".join(out))


def _matches_rule_glob(pattern: str, tracked_files: list[str]) -> bool:
    regexes = [_glob_to_regex(alt) for alt in _expand_braces(pattern)]
    return any(rx.match(f) for rx in regexes for f in tracked_files)


def _strip_yaml_quotes(raw: str) -> str:
    val = raw.strip()
    if len(val) >= QUOTED_MIN_LEN and val[0] == val[-1] and val[0] in ('"', "'"):
        return val[1:-1]
    # Strip trailing unquoted YAML comment if present.
    if " #" in val:
        val = val.split(" #", 1)[0].rstrip()
    return val


def _split_inline_yaml_list(inner: str) -> list[str]:
    items: list[str] = []
    buf: list[str] = []
    in_quote: str | None = None
    brace_depth = 0
    i = 0
    while i < len(inner):
        ch = inner[i]
        if in_quote is not None:
            buf.append(ch)
            if ch == in_quote and (i == 0 or inner[i - 1] != "\\"):
                in_quote = None
        elif ch in ('"', "'"):
            in_quote = ch
            buf.append(ch)
        elif ch == "{":
            brace_depth += 1
            buf.append(ch)
        elif ch == "}":
            brace_depth = max(0, brace_depth - 1)
            buf.append(ch)
        elif ch == "," and brace_depth == 0:
            token = _strip_yaml_quotes("".join(buf))
            if token:
                items.append(token)
            buf = []
        else:
            buf.append(ch)
        i += 1
    tail = _strip_yaml_quotes("".join(buf))
    if tail:
        items.append(tail)
    return items


def _parse_block_yaml_list(fm_lines: list[str], start_idx: int) -> tuple[list[str], int]:
    items: list[str] = []
    j = start_idx
    while j < len(fm_lines):
        sub = fm_lines[j]
        if not sub.strip() or sub.lstrip().startswith("#"):
            j += 1
            continue
        if not sub[0].isspace():
            break
        m_item = re.match(r"^\s*-\s+(.*)$", sub)
        if m_item is not None:
            item = _strip_yaml_quotes(m_item.group(1))
            if item:
                items.append(item)
        j += 1
    return items, j - 1


def _extract_frontmatter_paths(text: str) -> list[str]:
    lines = text.splitlines()
    if not lines or lines[0].lstrip("\ufeff").strip() != "---":
        return []
    end_idx: int | None = None
    for idx in range(1, len(lines)):
        if lines[idx].strip() == "---":
            end_idx = idx
            break
    if end_idx is None:
        return []

    fm_lines = lines[1:end_idx]
    paths: list[str] = []
    i = 0
    while i < len(fm_lines):
        line = fm_lines[i]
        if re.match(r"^paths\s*:", line):
            rest = line.split(":", 1)[1].strip()
            if rest.startswith("[") and rest.endswith("]"):
                paths.extend(_split_inline_yaml_list(rest[1:-1]))
            elif rest:
                paths.append(_strip_yaml_quotes(rest))
            else:
                items, i = _parse_block_yaml_list(fm_lines, i + 1)
                paths.extend(items)
        i += 1
    return paths


def _extract_shared_blocks(
    path: Path, rel_label: str, findings: list[Finding]
) -> dict[str, bytes] | None:
    if not path.is_file():
        findings.append(Finding("H7", rel_label, f"{rel_label} does not exist"))
        return None

    raw = path.read_bytes()
    blocks: dict[str, bytes] = {}
    open_name: str | None = None
    open_start_offset = 0
    offset = 0

    for raw_line in raw.splitlines(keepends=True):
        line_text = raw_line.decode("utf-8", errors="replace")
        m = SHARED_MARKER_RE.match(line_text)
        line_end = offset + len(raw_line)
        if m is not None:
            name, kind = m.group(1), m.group(2)
            if kind == "start":
                if open_name is not None:
                    findings.append(
                        Finding(
                            "H7",
                            rel_label,
                            f"nested shared block '{name}' inside '{open_name}'",
                        )
                    )
                else:
                    open_name = name
                    open_start_offset = line_end
            elif open_name is None:
                findings.append(
                    Finding(
                        "H7",
                        rel_label,
                        f"unexpected shared block end marker for '{name}'",
                    )
                )
            elif open_name != name:
                findings.append(
                    Finding(
                        "H7",
                        rel_label,
                        f"mismatched shared block end '{name}' (expected '{open_name}')",
                    )
                )
                open_name = None
            elif name in blocks:
                findings.append(
                    Finding(
                        "H7",
                        rel_label,
                        f"duplicate shared block '{name}'",
                    )
                )
                open_name = None
            else:
                blocks[name] = raw[open_start_offset:offset]
                open_name = None
        offset = line_end

    if open_name is not None:
        findings.append(Finding("H7", rel_label, f"unclosed shared block '{open_name}'"))
    return blocks


def _non_fenced_lines(lines: list[str]) -> list[tuple[int, str]]:
    """Return (0-based line index, line) pairs outside ``` fenced code blocks."""
    result: list[tuple[int, str]] = []
    in_fence = False
    for idx, line in enumerate(lines):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            continue
        if not in_fence:
            result.append((idx, line))
    return result


def _parse_fp_sections(
    lines: list[str], non_fence_lines: list[tuple[int, str]], findings: list[Finding]
) -> list[tuple[int, str, int, int]]:
    all_h2: list[tuple[int, str]] = [
        (idx, line) for idx, line in non_fence_lines if H2_HEADING_RE.match(line) is not None
    ]
    parsed_sections: list[tuple[int, str, int, int]] = []
    for pos, (idx, line) in enumerate(all_h2):
        end_idx = all_h2[pos + 1][0] if pos + 1 < len(all_h2) else len(lines)
        m = FP_SECTION_HEADING_RE.match(line)
        if m is None:
            findings.append(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"invalid section heading '{line}' (expected '## FP-NN <name>')",
                )
            )
            continue
        nn_str = m.group(1)
        parsed_sections.append((int(nn_str), nn_str, idx, end_idx))
    return parsed_sections


def _check_fp_section_sequence(
    lines: list[str], parsed_sections: list[tuple[int, str, int, int]], findings: list[Finding]
) -> None:
    seen_nums: set[int] = set()
    prev_num = 0
    out_of_order_reported = False
    for num, nn_str, _, _ in parsed_sections:
        if num < 1:
            findings.append(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"invalid section number 'FP-{nn_str}' (numbering must start at FP-01)",
                )
            )
            continue
        if num in seen_nums:
            findings.append(
                Finding("H10", FAILURE_PATTERNS_REL, f"duplicate section 'FP-{nn_str}'")
            )
        else:
            seen_nums.add(num)
        if num < prev_num and not out_of_order_reported:
            findings.append(
                Finding("H10", FAILURE_PATTERNS_REL, f"section 'FP-{nn_str}' is out of order")
            )
            out_of_order_reported = True
        prev_num = num

    if seen_nums:
        findings.extend(
            Finding(
                "H10",
                FAILURE_PATTERNS_REL,
                f"missing section 'FP-{expected_num:02d}' (sections must be contiguous from FP-01)",
            )
            for expected_num in range(1, max(seen_nums) + 1)
            if expected_num not in seen_nums
        )

    last_nn = parsed_sections[-1][1]
    expected_range = f"FP-01 \u301c FP-{last_nn}"
    first_line = lines[0] if lines else ""
    title_range_re = re.compile(rf"(?<![A-Za-z0-9-]){re.escape(expected_range)}(?![0-9])")
    if title_range_re.search(first_line) is None:
        findings.append(
            Finding(
                "H10",
                FAILURE_PATTERNS_REL,
                f"first-line title must contain '{expected_range}'",
            )
        )


def _check_fp_section_leadins(
    non_fence_lines: list[tuple[int, str]],
    parsed_sections: list[tuple[int, str, int, int]],
    findings: list[Finding],
) -> None:
    for _, nn_str, start_idx, end_idx in parsed_sections:
        body_text = "\n".join(ln for idx, ln in non_fence_lines if start_idx < idx < end_idx)
        positions: list[tuple[int, str]] = []
        all_counts_ok = True
        for leadin in REQUIRED_FP_LEADINS:
            matches = [m.start() for m in re.finditer(re.escape(leadin), body_text)]
            if not matches:
                findings.append(
                    Finding("H10", FAILURE_PATTERNS_REL, f"FP-{nn_str} is missing lead-in {leadin}")
                )
                all_counts_ok = False
            elif len(matches) > 1:
                findings.append(
                    Finding(
                        "H10",
                        FAILURE_PATTERNS_REL,
                        f"FP-{nn_str} contains lead-in {leadin} {len(matches)} times (expected 1)",
                    )
                )
                all_counts_ok = False
            else:
                positions.append((matches[0], leadin))
        if all_counts_ok and [leadin for _, leadin in sorted(positions)] != list(
            REQUIRED_FP_LEADINS
        ):
            findings.append(
                Finding("H10", FAILURE_PATTERNS_REL, f"FP-{nn_str} lead-ins are out of order")
            )

        senrei_pos = body_text.find("**先例.**")
        if senrei_pos != -1:
            after_senrei = body_text[senrei_pos + len("**先例.**") :]
            next_leadin = LEADIN_BREAK_RE.search(after_senrei)
            senrei_block = (
                after_senrei[: next_leadin.start()] if next_leadin is not None else after_senrei
            )
            findings.extend(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"FP-{nn_str} 先例 contains abbreviated id '{abbrev_m.group(1)}'; write each ADR/PM id in full",
                )
                for abbrev_m in ABBREV_PRECEDENT_RE.finditer(senrei_block)
            )


def _check_fp_precedents_and_paths(root: Path, patterns_text: str, findings: list[Finding]) -> None:
    for digits in dict.fromkeys(ADR_PRECEDENT_RE.findall(patterns_text)):
        adr_rel = f"docs/ADR/{digits}.md"
        if not (root / adr_rel).is_file():
            findings.append(
                Finding(
                    "H10", FAILURE_PATTERNS_REL, f"precedent ADR-{digits} has no file {adr_rel}"
                )
            )

    pm_dir = root / "docs" / "postmortems"
    for pm_suffix in dict.fromkeys(PM_PRECEDENT_RE.findall(patterns_text)):
        pm_id = f"PM-{pm_suffix}"
        pm_matches = (
            sorted(p for p in pm_dir.glob(f"{pm_id}-*.md") if p.is_file())
            if pm_dir.is_dir()
            else []
        )
        if not pm_matches:
            findings.append(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"precedent {pm_id} has no file matching docs/postmortems/{pm_id}-*.md",
                )
            )
        elif len(pm_matches) > 1:
            rels = ", ".join(p.relative_to(root).as_posix() for p in pm_matches)
            findings.append(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"precedent {pm_id} matches multiple files: {rels}",
                )
            )

    seen_candidates: set[str] = set()
    for m in REPO_PATH_TOKEN_RE.finditer(patterns_text):
        candidate = m.group(1).rstrip(".,;:")
        if any(ch in candidate for ch in "<>*") or candidate in seen_candidates:
            continue
        seen_candidates.add(candidate)
        if not (root / candidate).exists():
            findings.append(
                Finding(
                    "H10",
                    FAILURE_PATTERNS_REL,
                    f"referenced path '{candidate}' does not exist",
                )
            )


def _extract_routing_data_rows(skill_path: Path, findings: list[Finding]) -> list[str] | None:
    if not skill_path.is_file():
        findings.append(Finding("H10", FAILURE_SKILL_REL, f"{FAILURE_SKILL_REL} is missing"))
        return None

    skill_lines = skill_path.read_text(encoding="utf-8", errors="replace").splitlines()
    skill_non_fence = _non_fenced_lines(skill_lines)
    route_start: int | None = None
    route_end = len(skill_lines)
    for idx, line in skill_non_fence:
        if route_start is None:
            if line.strip() == "## 経路表":
                route_start = idx
        elif H2_HEADING_RE.match(line):
            route_end = idx
            break

    if route_start is None:
        findings.append(Finding("H10", FAILURE_SKILL_REL, "missing '## 経路表' heading"))
        return None

    route_lines = [
        ln.strip()
        for idx, ln in skill_non_fence
        if route_start < idx < route_end and ln.strip().startswith("|")
    ]
    data_rows: list[str] = []
    seen_sep = False
    for row in route_lines:
        if not seen_sep:
            if re.match(r"^\|\s*:?-{2,}", row):
                seen_sep = True
            continue
        data_rows.append(row)

    if not data_rows:
        findings.append(
            Finding(
                "H10",
                FAILURE_SKILL_REL,
                "no routing table rows found under '## 経路表'",
            )
        )
        return None
    return data_rows


def _check_fp_routing_table(
    skill_path: Path, parsed_sections: list[tuple[int, str, int, int]], findings: list[Finding]
) -> None:
    data_rows = _extract_routing_data_rows(skill_path, findings)
    if data_rows is None:
        return

    table_fps: list[str] = []
    seen_table_fps: set[str] = set()
    for row in data_rows:
        for digits in FP_TABLE_ID_RE.findall(row):
            fp_id = f"FP-{digits}"
            if fp_id not in seen_table_fps:
                seen_table_fps.add(fp_id)
                table_fps.append(fp_id)

    pattern_fps = list(dict.fromkeys(f"FP-{nn_str}" for _, nn_str, _, _ in parsed_sections))
    pattern_fp_set = set(pattern_fps)
    findings.extend(
        Finding(
            "H10",
            FAILURE_SKILL_REL,
            f"routing table references '{fp_id}' which has no section in patterns.md",
        )
        for fp_id in table_fps
        if fp_id not in pattern_fp_set
    )
    findings.extend(
        Finding(
            "H10",
            FAILURE_SKILL_REL,
            f"section '{fp_id}' in patterns.md does not appear in '## 経路表'",
        )
        for fp_id in pattern_fps
        if fp_id not in seen_table_fps
    )


def _check_failure_pattern_catalog(root: Path, findings: list[Finding]) -> None:
    """H10: verify failure-pattern catalog structure, precedents, paths, and routing table."""
    patterns_path = root / FAILURE_PATTERNS_REL
    skill_path = root / FAILURE_SKILL_REL

    if not patterns_path.is_file():
        findings.append(Finding("H10", FAILURE_PATTERNS_REL, f"{FAILURE_PATTERNS_REL} is missing"))
        if not skill_path.is_file():
            findings.append(Finding("H10", FAILURE_SKILL_REL, f"{FAILURE_SKILL_REL} is missing"))
        return

    patterns_text = patterns_path.read_text(encoding="utf-8", errors="replace")
    lines = patterns_text.splitlines()
    non_fence_lines = _non_fenced_lines(lines)
    parsed_sections = _parse_fp_sections(lines, non_fence_lines, findings)

    if not parsed_sections:
        findings.append(
            Finding(
                "H10",
                FAILURE_PATTERNS_REL,
                "no '## FP-NN <name>' sections found",
            )
        )
    else:
        _check_fp_section_sequence(lines, parsed_sections, findings)
        _check_fp_section_leadins(non_fence_lines, parsed_sections, findings)

    _check_fp_precedents_and_paths(root, patterns_text, findings)
    _check_fp_routing_table(skill_path, parsed_sections, findings)


def _load_ast_grep_rule_ids(root: Path) -> set[str]:
    rules_dir = root / AST_GREP_RULES_REL
    if not rules_dir.is_dir():
        return set()
    ids: set[str] = set()
    for yml_path in sorted(rules_dir.glob("*.yml")):
        if not yml_path.is_file():
            continue
        for line in yml_path.read_text(encoding="utf-8", errors="replace").splitlines():
            m = AST_GREP_ID_LINE_RE.match(line)
            if m is not None:
                ids.add(m.group(1))
                break
    return ids


def _extract_max_judgment_rules(text: str) -> int | None:
    lines = text.splitlines()
    if lines and lines[0].lstrip("\ufeff").strip() == "---":
        for idx in range(1, len(lines)):
            if lines[idx].strip() == "---":
                for fm_line in lines[1:idx]:
                    m = MAX_JUDGMENT_FM_RE.match(fm_line)
                    if m is not None:
                        val = int(m.group(1))
                        return val if val >= 0 else None
                break

    for line in lines:
        m_comment = MAX_JUDGMENT_COMMENT_RE.search(line)
        if m_comment is not None:
            val = int(m_comment.group(1))
            return val if val >= 0 else None
    return None


def _split_md_table_cells(line: str) -> list[str]:
    stripped = line.strip()
    if stripped.startswith("|"):
        stripped = stripped[1:]
    if stripped.endswith("|"):
        stripped = stripped[:-1]
    return [cell.strip() for cell in stripped.split("|")]


def _parse_rule_ledger_rows(
    lines: list[str],
    findings: list[Finding],
) -> list[tuple[str, str, str, str]] | None:
    non_fence = _non_fenced_lines(lines)
    expected = list(RULE_LEDGER_HEADERS)
    rows: list[tuple[str, str, str, str]] = []
    pos = 0
    found_table = False

    while pos < len(non_fence):
        _, raw_line = non_fence[pos]
        if (
            raw_line.strip().startswith("|")
            and _split_md_table_cells(raw_line) == expected
            and pos + 1 < len(non_fence)
            and re.match(r"^\|\s*:?-{2,}", non_fence[pos + 1][1].strip())
        ):
            found_table = True
            pos += 2
            while pos < len(non_fence):
                _, row_line = non_fence[pos]
                if not row_line.strip().startswith("|"):
                    break
                cells = _split_md_table_cells(row_line)
                if len(cells) != len(expected):
                    findings.append(
                        Finding(
                            "H11",
                            RULE_LEDGER_REL,
                            f"table row has {len(cells)} columns (expected {len(expected)}): {row_line.strip()}",
                        )
                    )
                else:
                    rows.append((cells[0], cells[1], cells[2], cells[3]))
                pos += 1
            break
        pos += 1

    if not found_table:
        findings.append(
            Finding(
                "H11",
                RULE_LEDGER_REL,
                "missing canonical Markdown table with columns '| ID | 規則 | 強制手段 | 由来 |'",
            )
        )
        return None
    if not rows:
        findings.append(
            Finding(
                "H11",
                RULE_LEDGER_REL,
                "rule ledger table has 0 data rows",
            )
        )
        return None
    return rows


def _check_rule_ledger_enforcement(
    root: Path,
    rule_id: str,
    enforcement: str,
    ast_grep_ids: set[str],
    findings: list[Finding],
) -> None:
    concrete_refs = 0
    for span in BACKTICK_SPAN_RE.findall(enforcement):
        token = span.strip()
        if not token:
            continue
        if token.startswith("ast-grep:"):
            concrete_refs += 1
            rule_name = token.removeprefix("ast-grep:").strip()
            if not rule_name or rule_name not in ast_grep_ids:
                findings.append(
                    Finding(
                        "H11",
                        RULE_LEDGER_REL,
                        f"row '{rule_id}' references missing ast-grep rule '{rule_name}' (not found in {AST_GREP_RULES_REL.as_posix()}/*.yml)",
                    )
                )
            continue

        candidate = token.split(maxsplit=1)[0].rstrip(".,;:")
        if "/" in candidate:
            concrete_refs += 1
            if not (root / candidate).is_file():
                findings.append(
                    Finding(
                        "H11",
                        RULE_LEDGER_REL,
                        f"row '{rule_id}' references missing or non-file repo path '{candidate}'",
                    )
                )
        elif candidate.endswith(FILE_LIKE_EXTENSIONS) and not (root / candidate).is_file():
            findings.append(
                Finding(
                    "H11",
                    RULE_LEDGER_REL,
                    f"row '{rule_id}' references missing file token '{candidate}'",
                )
            )

    if concrete_refs == 0:
        findings.append(
            Finding(
                "H11",
                RULE_LEDGER_REL,
                f"row '{rule_id}' is not marked '判断' and cites no backticked repo path or ast-grep:<id> in 強制手段",
            )
        )


def _check_rule_ledger_row(
    root: Path,
    row: tuple[str, str, str, str],
    ast_grep_ids: set[str],
    findings: list[Finding],
) -> bool:
    rule_id, rule_desc, enforcement, origin = row
    if not rule_id or not rule_desc or not enforcement or not origin:
        findings.append(
            Finding(
                "H11",
                RULE_LEDGER_REL,
                f"row '{rule_id or '<empty>'}' contains empty required column(s)",
            )
        )

    for corpus_id in dict.fromkeys(CORPUS_ID_RE.findall(origin)):
        corpus_meta = root / "scripts" / "harness" / "corpus" / corpus_id / "meta.toml"
        if not corpus_meta.is_file():
            findings.append(
                Finding(
                    "H11",
                    RULE_LEDGER_REL,
                    f"row '{rule_id}' cites corpus case {corpus_id} but scripts/harness/corpus/{corpus_id}/meta.toml does not exist",
                )
            )

    for adr_digits in dict.fromkeys(ADR_PRECEDENT_RE.findall(origin)):
        adr_rel = f"docs/ADR/{adr_digits}.md"
        if not (root / adr_rel).is_file():
            findings.append(
                Finding(
                    "H11",
                    RULE_LEDGER_REL,
                    f"row '{rule_id}' cites ADR-{adr_digits} but {adr_rel} does not exist",
                )
            )

    if enforcement.strip().startswith("判断"):
        return True

    _check_rule_ledger_enforcement(root, rule_id, enforcement, ast_grep_ids, findings)
    return False


def _cross_check_ratchet_max_judgment(
    root: Path,
    max_judgment: int,
    findings: list[Finding],
) -> None:
    ratchet_path = root / RATCHET_SUPPRESSIONS_REL
    if not ratchet_path.is_file():
        return
    try:
        ratchet_data = tomllib.loads(ratchet_path.read_text(encoding="utf-8"))
    except (OSError, tomllib.TOMLDecodeError) as exc:
        findings.append(
            Finding(
                "H11",
                RATCHET_SUPPRESSIONS_REL,
                f"failed to parse {RATCHET_SUPPRESSIONS_REL} for max_judgment_rules cross-check: {exc}",
            )
        )
        return
    gov = ratchet_data.get("governance")
    ratchet_max = gov.get("max_judgment_rules") if isinstance(gov, dict) else None
    if (
        isinstance(ratchet_max, bool)
        or not isinstance(ratchet_max, int)
        or ratchet_max != max_judgment
    ):
        findings.append(
            Finding(
                "H11",
                RATCHET_SUPPRESSIONS_REL,
                f"[governance].max_judgment_rules in {RATCHET_SUPPRESSIONS_REL} ({ratchet_max!r}) "
                f"does not match {RULE_LEDGER_REL} ({max_judgment})",
            )
        )


def _check_rule_ledger(root: Path, findings: list[Finding]) -> None:
    """H11: verify .agents/rules/rule-ledger.md structure, references, and judgment ratchet."""
    ledger_path = root / RULE_LEDGER_REL
    if not ledger_path.is_file():
        findings.append(Finding("H11", RULE_LEDGER_REL, f"{RULE_LEDGER_REL} is missing"))
        return

    text = ledger_path.read_text(encoding="utf-8", errors="replace")
    max_judgment = _extract_max_judgment_rules(text)
    if max_judgment is None:
        findings.append(
            Finding(
                "H11",
                RULE_LEDGER_REL,
                "missing non-negative integer 'max_judgment_rules' in frontmatter or metadata comment",
            )
        )

    rows = _parse_rule_ledger_rows(text.splitlines(), findings)
    if rows is None:
        return

    ast_grep_ids = _load_ast_grep_rule_ids(root)
    seen_ids: set[str] = set()
    judgment_rows = 0
    for row in rows:
        rule_id = row[0]
        if rule_id in seen_ids:
            findings.append(
                Finding("H11", RULE_LEDGER_REL, f"duplicate rule ID '{rule_id}' in rule ledger")
            )
        elif rule_id:
            seen_ids.add(rule_id)
        if _check_rule_ledger_row(root, row, ast_grep_ids, findings):
            judgment_rows += 1

    if max_judgment is not None:
        if judgment_rows > max_judgment:
            findings.append(
                Finding(
                    "H11",
                    RULE_LEDGER_REL,
                    f"judgment rule count ({judgment_rows}) exceeds max_judgment_rules ({max_judgment}); mechanize rules instead of adding prose-only judgment rules",
                )
            )
        elif judgment_rows < max_judgment:
            findings.append(
                Finding(
                    "H11",
                    RULE_LEDGER_REL,
                    f"judgment rule count ({judgment_rows}) is below max_judgment_rules ({max_judgment}); lower max_judgment_rules to {judgment_rows} in the same commit",
                )
            )
        _cross_check_ratchet_max_judgment(root, max_judgment, findings)


def _check_hooks_pair(root: Path, findings: list[Finding]) -> tuple[bool, int, int | str]:
    codex_path = root / ".codex" / "hooks.json"
    codex_hooks_count = 0
    if codex_path.is_file():
        cfg = _load_hook_config(codex_path, ".codex/hooks.json", findings)
        if cfg is not None:
            codex_hooks_count = _check_hook_file(
                root, ".codex/hooks.json", cfg, is_claude=False, findings=findings
            )
    else:
        findings.append(Finding("H4", ".codex/hooks.json", ".codex/hooks.json is missing"))

    claude_path = root / ".claude" / "settings.json"
    claude_present = claude_path.is_file()
    claude_hooks_count: int | str = "absent"
    if claude_present:
        cfg = _load_hook_config(claude_path, ".claude/settings.json", findings)
        if cfg is not None:
            claude_hooks_count = _check_hook_file(
                root, ".claude/settings.json", cfg, is_claude=True, findings=findings
            )
    return claude_present, codex_hooks_count, claude_hooks_count


def _check_shared_blocks_h7(root: Path, findings: list[Finding]) -> int:
    claude_blocks = _extract_shared_blocks(root / "CLAUDE.md", "CLAUDE.md", findings)
    agents_blocks = _extract_shared_blocks(root / "AGENTS.md", "AGENTS.md", findings)
    if claude_blocks is None or agents_blocks is None:
        return 0
    all_names = sorted(set(claude_blocks) | set(agents_blocks))
    if not all_names:
        findings.append(
            Finding(
                "H7",
                "CLAUDE.md",
                "no shared blocks (<!-- shared:NAME:start/end -->) found in CLAUDE.md and AGENTS.md",
            )
        )
        return 0

    shared_block_count = 0
    for name in all_names:
        if name not in agents_blocks:
            findings.append(
                Finding(
                    "H7",
                    "AGENTS.md",
                    f"shared block '{name}' exists in CLAUDE.md but is missing from AGENTS.md",
                )
            )
        elif name not in claude_blocks:
            findings.append(
                Finding(
                    "H7",
                    "CLAUDE.md",
                    f"shared block '{name}' exists in AGENTS.md but is missing from CLAUDE.md",
                )
            )
        elif claude_blocks[name] != agents_blocks[name]:
            findings.append(
                Finding(
                    "H7",
                    "AGENTS.md",
                    f"shared block '{name}' differs between CLAUDE.md and AGENTS.md",
                )
            )
        else:
            shared_block_count += 1
    return shared_block_count


def _check_doc_sizes_and_skills(root: Path, findings: list[Finding]) -> None:
    claude_md = root / "CLAUDE.md"
    if claude_md.is_file():
        line_count = len(claude_md.read_text(encoding="utf-8", errors="replace").splitlines())
        if line_count > MAX_CLAUDE_MD_LINES:
            findings.append(
                Finding(
                    "H8",
                    "CLAUDE.md",
                    f"CLAUDE.md has {line_count} lines (limit is {MAX_CLAUDE_MD_LINES})",
                )
            )

    agents_md = root / "AGENTS.md"
    if agents_md.is_file():
        byte_size = agents_md.stat().st_size
        if byte_size > MAX_AGENTS_MD_BYTES:
            findings.append(
                Finding(
                    "H8",
                    "AGENTS.md",
                    f"AGENTS.md is {byte_size} bytes (limit is {MAX_AGENTS_MD_BYTES})",
                )
            )

    skills_dir = root / ".agents" / "skills"
    if skills_dir.is_dir():
        for entry in sorted(skills_dir.iterdir(), key=lambda p: p.name):
            rel_entry = entry.relative_to(root).as_posix()
            if not entry.is_dir() or not (entry / "SKILL.md").is_file():
                findings.append(
                    Finding(
                        "H9",
                        rel_entry,
                        "entry under .agents/skills does not resolve to a directory containing SKILL.md",
                    )
                )


def run_selfcheck(root: Path) -> tuple[list[Finding], dict[str, Any]]:
    findings: list[Finding] = []
    tracked_files = _git_ls_files(root)

    # H1-H4: .codex/hooks.json (tracked) & .claude/settings.json (when present)
    claude_present, codex_hooks_count, claude_hooks_count = _check_hooks_pair(root, findings)

    # H5: guard_event_time.GUARDED_GLOBS matches at least one tracked non-test file
    non_test_tracked = [f for f in tracked_files if not f.endswith("_test.go")]
    for pat in guard_event_time.GUARDED_GLOBS:
        rx = _glob_to_regex(pat)
        if not any(rx.match(f) for f in non_test_tracked):
            findings.append(
                Finding(
                    "H5",
                    "scripts/agent-harness/guard_event_time.py",
                    f"GUARDED_GLOBS entry '{pat}' matches no tracked non-test file",
                )
            )

    # H6: .claude/rules frontmatter paths: entries match at least one tracked file
    rules_dir = root / ".claude" / "rules"
    if rules_dir.is_dir():
        for rule_file in sorted(rules_dir.rglob("*.md")):
            rel_rule = rule_file.relative_to(root).as_posix()
            text = rule_file.read_text(encoding="utf-8", errors="replace")
            findings.extend(
                Finding(
                    "H6",
                    rel_rule,
                    f"paths entry '{pat}' matches no tracked file",
                )
                for pat in _extract_frontmatter_paths(text)
                if not _matches_rule_glob(pat, tracked_files)
            )

    # H7: shared blocks between CLAUDE.md and AGENTS.md
    shared_block_count = _check_shared_blocks_h7(root, findings)

    # H8-H9: root CLAUDE.md / AGENTS.md limits and .agents/skills targets
    _check_doc_sizes_and_skills(root, findings)

    # H10: failure-pattern catalog consistency and precedent existence
    _check_failure_pattern_catalog(root, findings)

    # H11: rule ledger integrity and monotonic judgment-rule ratchet
    _check_rule_ledger(root, findings)

    meta = {
        "claude_present": claude_present,
        "codex_hooks": codex_hooks_count,
        "claude_hooks": claude_hooks_count,
        "guarded_globs": len(guard_event_time.GUARDED_GLOBS),
        "shared_blocks": shared_block_count,
    }
    return findings, meta


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Verify Alt coding-agent harness health.")
    parser.add_argument("--root", type=Path, default=None, help="Repository root directory")
    parser.add_argument(
        "--hook",
        action="store_true",
        help="Emit a single-line SessionStart summary and always exit 0",
    )
    args = parser.parse_args(argv)

    root = args.root.resolve() if args.root is not None else _resolve_default_root()
    findings, meta = run_selfcheck(root)

    if args.hook:
        if findings:
            ids = ", ".join(f.finding_id for f in findings)
            sys.stdout.write(
                f"agent_harness_selfcheck_FAILED: {len(findings)} finding(s): {ids} "
                "run python3 scripts/agent-harness/harness_selfcheck.py\n"
            )
        else:
            sys.stdout.write(
                "agent_harness_selfcheck: OK "
                f"(codex_hooks={meta['codex_hooks']}, "
                f"claude_hooks={meta['claude_hooks']}, "
                f"guarded_globs={meta['guarded_globs']}, "
                f"shared_blocks={meta['shared_blocks']})\n"
            )
        return 0

    if not meta["claude_present"]:
        sys.stdout.write(
            "NOTE: Claude hooks were not checked because .claude/settings.json is absent.\n"
        )

    if findings:
        for finding in findings:
            sys.stdout.write(f"{finding.finding_id} {finding.path}: {finding.message}\n")
        return 1

    sys.stdout.write(
        "agent_harness_selfcheck: OK "
        f"(codex_hooks={meta['codex_hooks']}, "
        f"claude_hooks={meta['claude_hooks']}, "
        f"guarded_globs={meta['guarded_globs']}, "
        f"shared_blocks={meta['shared_blocks']})\n"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
