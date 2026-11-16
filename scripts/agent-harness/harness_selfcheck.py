#!/usr/bin/env python3
"""Self-check for Alt coding-agent harness wiring, guards, rules, and docs."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
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
SHARED_MARKER_RE = re.compile(
    r"^\s*<!--\s*shared:([A-Za-z0-9_.-]+):(start|end)\s*-->\s*$"
)
MAX_CLAUDE_MD_LINES = 200
MAX_AGENTS_MD_BYTES = 32768


@dataclass(frozen=True)
class Finding:
    finding_id: str
    path: str
    message: str


def _resolve_default_root() -> Path:
    res = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode == 0 and res.stdout.strip():
        return Path(res.stdout.strip()).resolve()
    return Path(__file__).resolve().parents[2]


def _git_ls_files(root: Path) -> list[str]:
    res = subprocess.run(
        ["git", "-C", str(root), "ls-files", "-z"],
        capture_output=True,
        check=False,
    )
    if res.returncode != 0:
        return []
    raw = res.stdout.decode("utf-8", errors="replace")
    return [entry for entry in raw.split("\x00") if entry]


def _iter_hook_objects(entries: Any) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    if not isinstance(entries, list):
        return result
    for group in entries:
        if not isinstance(group, dict):
            continue
        nested = group.get("hooks")
        if isinstance(nested, list):
            for hook in nested:
                if isinstance(hook, dict):
                    result.append(hook)
        elif "type" in group or "command" in group:
            result.append(group)
    return result


def _load_hook_config(
    path: Path, rel_label: str, findings: list[Finding]
) -> dict[str, Any] | None:
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

    for event_name, entries in hooks_map.items():
        for hook in _iter_hook_objects(entries):
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

            for var in BAD_ENV_VARS:
                if var in cmd_str:
                    findings.append(
                        Finding(
                            "H1",
                            rel_label,
                            f"hook command references non-existent {var}; read JSON from stdin instead",
                        )
                    )

            for match in REPO_SCRIPT_REF_RE.finditer(cmd_str):
                rel_script = match.group(1)
                if not (root / rel_script).exists():
                    findings.append(
                        Finding(
                            "H2",
                            rel_label,
                            f"hook command references missing repo script '{rel_script}'",
                        )
                    )

    for required_guard in ("guard_protected_paths.py", "guard_event_time.py"):
        if not any(required_guard in cmd for cmd in pre_tool_commands):
            findings.append(
                Finding(
                    "H4",
                    rel_label,
                    f"PreToolUse hooks do not reference {required_guard}",
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
    if len(val) >= 2 and val[0] == val[-1] and val[0] in ('"', "'"):
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
        else:
            if ch in ('"', "'"):
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
                j = i + 1
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
                            paths.append(item)
                    j += 1
                i = j - 1
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
            else:
                if open_name is None:
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
        findings.append(
            Finding("H7", rel_label, f"unclosed shared block '{open_name}'")
        )
    return blocks


def run_selfcheck(root: Path) -> tuple[list[Finding], dict[str, Any]]:
    findings: list[Finding] = []
    tracked_files = _git_ls_files(root)

    # H1-H4: .codex/hooks.json (tracked) & .claude/settings.json (when present)
    codex_path = root / ".codex" / "hooks.json"
    codex_hooks_count = 0
    if codex_path.is_file():
        cfg = _load_hook_config(codex_path, ".codex/hooks.json", findings)
        if cfg is not None:
            codex_hooks_count = _check_hook_file(
                root, ".codex/hooks.json", cfg, is_claude=False, findings=findings
            )
    else:
        findings.append(
            Finding("H4", ".codex/hooks.json", ".codex/hooks.json is missing")
        )

    claude_path = root / ".claude" / "settings.json"
    claude_present = claude_path.is_file()
    claude_hooks_count: int | str = "absent"
    if claude_present:
        cfg = _load_hook_config(claude_path, ".claude/settings.json", findings)
        if cfg is not None:
            claude_hooks_count = _check_hook_file(
                root, ".claude/settings.json", cfg, is_claude=True, findings=findings
            )

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
            for pat in _extract_frontmatter_paths(text):
                if not _matches_rule_glob(pat, tracked_files):
                    findings.append(
                        Finding(
                            "H6",
                            rel_rule,
                            f"paths entry '{pat}' matches no tracked file",
                        )
                    )

    # H7: shared blocks between CLAUDE.md and AGENTS.md
    claude_blocks = _extract_shared_blocks(root / "CLAUDE.md", "CLAUDE.md", findings)
    agents_blocks = _extract_shared_blocks(root / "AGENTS.md", "AGENTS.md", findings)
    shared_block_count = 0
    if claude_blocks is not None and agents_blocks is not None:
        all_names = sorted(set(claude_blocks) | set(agents_blocks))
        if not all_names:
            findings.append(
                Finding(
                    "H7",
                    "CLAUDE.md",
                    "no shared blocks (<!-- shared:NAME:start/end -->) found in CLAUDE.md and AGENTS.md",
                )
            )
        else:
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

    # H8: root CLAUDE.md <= 200 lines, AGENTS.md <= 32768 bytes
    claude_md = root / "CLAUDE.md"
    if claude_md.is_file():
        line_count = len(
            claude_md.read_text(encoding="utf-8", errors="replace").splitlines()
        )
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

    # H9: .agents/skills entries must resolve to a directory containing SKILL.md
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
            print(
                f"agent_harness_selfcheck_FAILED: {len(findings)} finding(s): {ids} "
                "run python3 scripts/agent-harness/harness_selfcheck.py"
            )
        else:
            print(
                "agent_harness_selfcheck: OK "
                f"(codex_hooks={meta['codex_hooks']}, "
                f"claude_hooks={meta['claude_hooks']}, "
                f"guarded_globs={meta['guarded_globs']}, "
                f"shared_blocks={meta['shared_blocks']})"
            )
        return 0

    if not meta["claude_present"]:
        print(
            "NOTE: Claude hooks were not checked because .claude/settings.json is absent."
        )

    if findings:
        for finding in findings:
            print(f"{finding.finding_id} {finding.path}: {finding.message}")
        return 1

    print(
        "agent_harness_selfcheck: OK "
        f"(codex_hooks={meta['codex_hooks']}, "
        f"claude_hooks={meta['claude_hooks']}, "
        f"guarded_globs={meta['guarded_globs']}, "
        f"shared_blocks={meta['shared_blocks']})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
