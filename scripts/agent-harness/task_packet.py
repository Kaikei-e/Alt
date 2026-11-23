#!/usr/bin/env python3
"""Deterministic parser, validator, and template CLI for Alt agent Task Packets and Handoffs.

Subcommands:
  validate-packet <path> [--format text|json]
  validate-handoff <handoff-path> [--packet <packet-path>] [--format text|json]
  measure <packet-path> [--repo-root <dir>] [--format text|json]
  verify-handoff <handoff-path> --packet <packet-path> [--base-commit <rev>] [--repo-root <dir>] [--runs-dir <dir>] [--no-rerun] [--format text|json]
  verify-sequence --base-commit <rev> --test-command <cmd> [--packet <packet-path>] [--behavior-preserving | --bugfix] [--repo-root <dir>] [--format text|json]
  render-template --kind packet|handoff [--packet-id ID] [--target TARGET] [--format markdown|json]
  check-templates

Exit codes:
  0  Valid / clean
  1  Validation or self-check failure
  2  CLI usage or unreadable input file
"""

from __future__ import annotations

import argparse
import ast
import hashlib
import importlib.util
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import tokenize
import unittest.mock
from collections import Counter
from dataclasses import asdict, dataclass
from pathlib import Path, PurePosixPath
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from types import ModuleType

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2

SCHEMA_DRAFT_2020_12 = "https://json-schema.org/draft/2020-12/schema"
DEFAULT_PACKET_ID = "EXAMPLE-TASK-001"
DEFAULT_TARGET = "scripts"
QUOTED_MIN_LEN = 2
INT_PATTERN = re.compile(r"^-?[0-9]+$")
FLOAT_PATTERN = re.compile(
    r"^-?(?:[0-9]+\.[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$|^-?[0-9]+[eE][+-]?[0-9]+$"
)
PACKET_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$")
TARGET_RE = re.compile(r"^(?!.*\.\.)(?!/)[A-Za-z0-9._/-]+$")
DEFAULT_COMMAND_BUDGET_S = 180
MEASURE_TIMEOUT_SLACK_S = 30
TIMEOUT_EXIT_CODE = 124
SEQUENCE_MIN_COMMITS = 2
HARNESS_STATE_PREFIX = ".harness/"
DEFAULT_TEMPLATE_VERIFY_CMD = f"./scripts/lint/run.sh {DEFAULT_TARGET}"
DEFAULT_TEMPLATE_VERIFY_SHA = hashlib.sha256(
    DEFAULT_TEMPLATE_VERIFY_CMD.encode("utf-8")
).hexdigest()
SEQUENCE_TEST_FILE_RE = re.compile(
    r"(?:^|/)(?:"
    r"(?:tests?|__tests__|e2e|testdata)/.+"
    r"|[^/]+_test\.go"
    r"|test_[^/]+\.py|test-[^/]+\.py|[^/]+_test\.py"
    r"|[^/]*test[^/]*\.sh"
    r"|[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)"
    r"|[^/]+_test\.[cm]?[jt]sx?"
    r"|tests\.rs|[^/]+_tests?\.rs"
    r")$",
    re.IGNORECASE,
)
ALLOWED_LINT_FIX_COMMENT_RE = re.compile(
    r"(?:#\s*(?:ruff:\s*)?noqa\b|(?://|#|/\*)\s*#?nosec\b|//\s*nolint\b|"
    r"#!?\[\s*(?:allow|expect)\b|biome-ignore\b|deno-lint-ignore\b|"
    r"\bexpires\s*:|^#!)",
    re.IGNORECASE,
)
GO_FUNC_DECL_RE = re.compile(
    r"\bfunc\s+(?:\(([^)]*)\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*(?:\[[^\]]*\])?\s*\("
)
TS_FUNC_DECL_RE = re.compile(
    r"\b(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\("
)
TS_ARROW_DECL_RE = re.compile(
    r"\b(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=\n]+)?=\s*"
    r"(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=\n]+)?=>"
)
TS_CLASS_DECL_RE = re.compile(
    r"\b(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)[^{]*\{"
)
TS_METHOD_DECL_RE = re.compile(
    r"(?:^|[{;,\n])\s*(?:(?:public|private|protected|static|readonly|override|async|get|set)\s+)*"
    r"([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>\n]*>)?\s*\([^)]*\)\s*(?::[^{;=\n]+)?\{"
)
TS_CONTROL_KEYWORDS = frozenset(
    {"if", "for", "while", "switch", "catch", "with", "return", "function", "class", "else", "try"}
)
SVELTE_SCRIPT_RE = re.compile(r"<script\b[^>]*>(.*?)</script>", re.IGNORECASE | re.DOTALL)
RUST_IMPL_DECL_RE = re.compile(r"\b(?:unsafe\s+)?impl(?:\s*<[^>]*>)?\s+([^{;]+?)\s*\{")
RUST_PRE_FN_RE = re.compile(
    r"(?:(?:pub(?:\s*\([^)]*\))?|default|const|async|unsafe|extern(?:\s+\"[^\"]*\")?)\s+)+$"
)
ALL_ZERO_SHA_RE = re.compile(r"^0+$")
GIT_EXEC_MODE = "100755"
GIT_SYMLINK_MODE = "120000"
MAX_BUDGET_SECONDS = 600
C_STYLE_COMMENT_EXTS = frozenset(
    {".go", ".rs", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".svelte"}
)
SHELL_COMMENT_EXTS = frozenset({".sh", ".bash", ".zsh"})
HARNESS_DIR = Path(__file__).resolve().parent
HARNESS_RUN_SCRIPT_PATH = HARNESS_DIR / "harness_run.py"
REPO_ROOT_DEFAULT = HARNESS_DIR.parents[1]
SCREENER_SCRIPT_PATH = REPO_ROOT_DEFAULT / "scripts" / "lint" / "behavior_screener.py"
SCHEMA_PATH = HARNESS_DIR / "task_packet.schema.json"
TEMPLATES_DIR = HARNESS_DIR / "templates"
PACKET_TEMPLATE_PATH = TEMPLATES_DIR / "task-packet.md.tmpl"
HANDOFF_TEMPLATE_PATH = TEMPLATES_DIR / "task-handoff.md.tmpl"

PLACEHOLDER_STRINGS = frozenset(
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
        "なし",
        "無し",
        "未記入",
        "未定",
        "xxx",
        "...",
    }
)
EXCLUDED_GENERATED_PREFIXES = ("stopped-using-k8s/", "scripts/harness/corpus/")
SUBPROCESS_TIMEOUT_SECONDS = 60

TIME_FIELD_RE = re.compile(
    r"(?:^|_)(?:timeout|minutes?|seconds?|hours?|duration|deadline|time_limit|time_budget|ttl)(?:$|_)",
    re.IGNORECASE,
)

FORBIDDEN_AUTOFIX_PATTERNS: tuple[tuple[str, re.Pattern[str]], ...] = (
    ("golangci-lint --fix", re.compile(r"\bgolangci-lint\b[^\n;|&]*--fix\b")),
    ("cargo clippy --fix", re.compile(r"\bcargo\s+clippy\b[^\n;|&]*--fix\b")),
    ("cargo fix", re.compile(r"\bcargo\s+fix\b")),
    ("ruff check --unsafe-fixes", re.compile(r"\bruff\s+check\b[^\n;|&]*--unsafe-fixes\b")),
    ("biome --unsafe", re.compile(r"\bbiome\s+(?:check|lint)\b[^\n;|&]*--unsafe\b")),
)

PROHIBITION_CUE_RE = re.compile(
    r"(?:禁止|不可|しない|使わない|実行しない|避ける|原則禁止|❌|"
    r"\b(?:ng|do\s+not|don't|never|forbid(?:den)?|prohibit(?:ed)?|must\s+not|disallow(?:ed)?)\b)",
    re.IGNORECASE,
)

SHELL_FENCE_LANGS = frozenset({"", "sh", "bash", "zsh", "shell", "console"})
SHELL_NG_COMMENT_RE = re.compile(r"(?:#\s*(?:NG|禁止|DO\s+NOT|FORBIDDEN)|❌)", re.IGNORECASE)
BG_PROSE_AFFIRMATIVE_RE = re.compile(
    r"(?:バックグラウンド(?:で|実行)|背景実行|run\s+in\s+(?:the\s+)?background)",
    re.IGNORECASE,
)
NOHUP_RE = re.compile(r"(?:^|[\s`('\"])nohup\s+")
INLINE_CODE_RE = re.compile(r"`([^`\n]+)`")
RUN_SH_CALL_RE = re.compile(
    r"""(?:^|[\s;&|()])(?:\./)?scripts/lint/run\.sh\s+(?:--policy(?:=|\s+)(?:"[^"\n]+"|'[^'\n]+'|[^\s;&|()'"]+)\s+)?(?:"([^"\n]+)"|'([^'\n]+)'|([^\s;&|()'"]+))""",
)
MAKE_LINT_RE = re.compile(r"(?:^|[\s;&|()])make\s+([^\n;&|)]*)")
MAKE_LINT_TARGET_RE = re.compile(r"\bTARGET=(?:\"([^\"\n]*)\"|'([^'\n]*)'|([^\s;&|()'\"#]+))")
CMD_LINT_POLICY_RE = re.compile(
    r"""(?:^|[\s;&|()])(?:LINT_POLICY=|--policy(?:=|\s+))(?:"([^"\n]+)"|'([^'\n]+)'|([^\s;&|()'"]+))""",
)
GENERATED_HEADER_SCAN_LINES = 40
GENERATED_HEADER_RE = re.compile(
    r"^\s*(?://+|#+|/\*|\*|--|<!--|;)\s*Code generated\b[^\n]*DO NOT EDIT\.?",
    re.IGNORECASE,
)
EXCLUDED_TEST_SCAN_DIRS = frozenset(
    {
        ".git",
        ".venv",
        "__pycache__",
        "build",
        "dist",
        "node_modules",
        "target",
        "vendor",
        "stopped-using-k8s",
    }
)
GO_TEST_FILE_RE = re.compile(r"(?:^|/)[^/]+_test\.go$")
PY_TEST_FILE_RE = re.compile(
    r"(?:^|/)(?:tests?/[^/].*\.py|test_[^/]+\.py|test-[^/]+\.py|[^/]+_test\.py)$"
)
RUST_TEST_PATH_RE = re.compile(r"(?:^|/)(?:tests/.*\.rs|tests\.rs|[^/]+_tests?\.rs)$")
RUST_TEST_ATTR_RE = re.compile(r"#\[\s*(?:(?:[\w:]+::)?test\b|rstest\b|proptest\b)")
TS_TEST_FILE_RE = re.compile(
    r"(?:^|/)(?:(?:__tests__|tests|e2e)/.*\.(?:[cm]?[jt]sx?|svelte)|"
    r"[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)|"
    r"[^/]+_test\.[cm]?[jt]sx?)$",
    re.IGNORECASE,
)
SOURCE_PATH_EXT_RE = re.compile(r"\.(?:go|py|rs|[cm]?[jt]sx?|svelte)$", re.IGNORECASE)
IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
GO_TEST_PREFIX_RE = re.compile(r"^(?:Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]*$")
PARAM_SUFFIX_RE = re.compile(r"\[[^\]]*\]$")
RUST_ATTR_LOOKBACK_LINES = 12
MIN_QUALIFIED_PARTS = 2

PACKET_REQUIRED_HEADINGS: tuple[tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"^##\s+1\.\s*範囲と作業量制約"), "## 1. 範囲と作業量制約"),
    (re.compile(r"^##\s+2\.\s*実行・検証手順"), "## 2. 実行・検証手順"),
    (re.compile(r"^##\s+3\.\s*Hand-?off\s*記述必須項目"), "## 3. Hand-off 記述必須項目"),
)

HANDOFF_REQUIRED_HEADINGS: tuple[tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"^#{2,3}\s+(?:1\.\s*)?.*修正ファイルと行数"), "## 1. 修正ファイルと行数"),
    (
        re.compile(r"^#{2,3}\s+(?:2\.\s*)?.*実行時挙動の変更可能性の有無"),
        "## 2. 実行時挙動の変更可能性の有無",
    ),
    (re.compile(r"^#{2,3}\s+(?:3\.\s*)?.*変更理由"), "## 3. 変更理由"),
)

STRUCTURED_PAYLOAD_KEYS = frozenset(
    {
        "packet_version",
        "packet_id",
        "scope",
        "execution_policy",
        "handoff_requirements",
        "status",
        "modified_files",
        "modified_functions_count",
        "runtime_behavior_impact",
        "rationale",
        "verification",
    }
)


class DocumentParseError(ValueError):
    """Raised when a TaskPacket or TaskHandoff document cannot be parsed."""


@dataclass(frozen=True)
class ValidationIssue:
    code: str
    path: str
    message: str


@dataclass(frozen=True)
class ParsedDocument:
    data: dict[str, Any]
    is_markdown: bool
    body_lines: list[str]


@dataclass(frozen=True)
class FencedBlock:
    lang: str
    start_line: int
    lines: list[tuple[int, str]]


# ---------------------------------------------------------------------------
# Stdlib YAML frontmatter subset parser
# ---------------------------------------------------------------------------


def _strip_yaml_comment(raw: str) -> str:
    in_quote: str | None = None
    escaped = False
    for idx, ch in enumerate(raw):
        if escaped:
            escaped = False
            continue
        if in_quote == '"' and ch == "\\":
            escaped = True
            continue
        if in_quote is not None:
            if ch == in_quote:
                in_quote = None
            continue
        if ch in ('"', "'"):
            in_quote = ch
        elif ch == "#" and (idx == 0 or raw[idx - 1].isspace()):
            return raw[:idx].rstrip()
    return raw.rstrip()


def _find_unquoted_colon(text: str) -> int:
    in_quote: str | None = None
    escaped = False
    depth = 0
    for idx, ch in enumerate(text):
        if escaped:
            escaped = False
            continue
        if in_quote == '"' and ch == "\\":
            escaped = True
            continue
        if in_quote is not None:
            if ch == in_quote:
                in_quote = None
            continue
        if ch in ('"', "'"):
            in_quote = ch
        elif ch in ("[", "{"):
            depth += 1
        elif ch in ("]", "}"):
            depth = max(0, depth - 1)
        elif ch == ":" and depth == 0 and (idx + 1 == len(text) or text[idx + 1].isspace()):
            return idx
    return -1


def _split_flow_items(inner: str) -> list[str]:
    items: list[str] = []
    buf: list[str] = []
    in_quote: str | None = None
    escaped = False
    depth = 0
    for ch in inner:
        if escaped:
            buf.append(ch)
            escaped = False
            continue
        if in_quote == '"' and ch == "\\":
            buf.append(ch)
            escaped = True
            continue
        if in_quote is not None:
            buf.append(ch)
            if ch == in_quote:
                in_quote = None
            continue
        if ch in ('"', "'"):
            in_quote = ch
            buf.append(ch)
        elif ch in ("[", "{"):
            depth += 1
            buf.append(ch)
        elif ch in ("]", "}"):
            depth = max(0, depth - 1)
            buf.append(ch)
        elif ch == "," and depth == 0:
            items.append("".join(buf).strip())
            buf = []
        else:
            buf.append(ch)
    tail = "".join(buf).strip()
    if tail:
        items.append(tail)
    return items


def _parse_yaml_flow_mapping(inner: str) -> dict[str, Any]:
    mapping: dict[str, Any] = {}
    if not inner.strip():
        return mapping
    for entry in _split_flow_items(inner):
        colon_idx = _find_unquoted_colon(entry)
        if colon_idx == -1:
            msg = f"invalid YAML flow mapping entry: {entry!r}"
            raise DocumentParseError(msg)
        raw_key = entry[:colon_idx].strip()
        raw_val = entry[colon_idx + 1 :].strip()
        key = str(_parse_yaml_scalar(raw_key))
        mapping[key] = _parse_yaml_scalar(raw_val)
    return mapping


def _parse_yaml_primitive(text: str) -> object:
    lower = text.lower()
    literals: dict[str, object] = {"true": True, "false": False, "null": None, "~": None}
    if lower in literals:
        return literals[lower]
    if INT_PATTERN.match(text):
        return int(text)
    if FLOAT_PATTERN.match(text):
        return float(text)
    return text


def _parse_yaml_scalar(raw: str) -> object:
    text = _strip_yaml_comment(raw).strip()
    if not text:
        return ""
    if len(text) >= QUOTED_MIN_LEN and text[0] == '"' and text[-1] == '"':
        try:
            return json.loads(text)
        except json.JSONDecodeError as exc:
            msg = f"invalid double-quoted YAML string {text!r}: {exc}"
            raise DocumentParseError(msg) from exc
    if len(text) >= QUOTED_MIN_LEN and text[0] == "'" and text[-1] == "'":
        return text[1:-1].replace("''", "'")
    if text.startswith("[") and text.endswith("]"):
        inner = text[1:-1].strip()
        return [_parse_yaml_scalar(item) for item in _split_flow_items(inner)] if inner else []
    if text.startswith("{") and text.endswith("}"):
        return _parse_yaml_flow_mapping(text[1:-1])
    return _parse_yaml_primitive(text)


def _prepare_yaml_lines(raw_lines: list[str]) -> list[tuple[int, str]]:
    prepared: list[tuple[int, str]] = []
    for raw in raw_lines:
        stripped = _strip_yaml_comment(raw)
        if not stripped.strip():
            continue
        indent = len(stripped) - len(stripped.lstrip(" "))
        if "\t" in stripped[:indent]:
            msg = f"tabs are not allowed for YAML indentation: {raw!r}"
            raise DocumentParseError(msg)
        prepared.append((indent, stripped.lstrip(" ")))
    return prepared


def _parse_yaml_sequence_item(
    lines: list[tuple[int, str]],
    idx: int,
    seq_indent: int,
    rest: str,
) -> tuple[Any, int]:
    if not rest:
        if idx + 1 < len(lines) and lines[idx + 1][0] > seq_indent:
            return _parse_yaml_node(lines, idx + 1, lines[idx + 1][0])
        return None, idx + 1

    colon_idx = _find_unquoted_colon(rest)
    if colon_idx == -1:
        return _parse_yaml_scalar(rest), idx + 1

    key = str(_parse_yaml_scalar(rest[:colon_idx].strip()))
    val_text = rest[colon_idx + 1 :].strip()
    item_map: dict[str, Any] = {}
    next_idx = idx + 1
    if val_text:
        item_map[key] = _parse_yaml_scalar(val_text)
    elif next_idx < len(lines) and lines[next_idx][0] > seq_indent + 2:
        val, next_idx = _parse_yaml_node(lines, next_idx, lines[next_idx][0])
        item_map[key] = val
    else:
        item_map[key] = None

    if next_idx < len(lines) and lines[next_idx][0] > seq_indent:
        cont_indent = lines[next_idx][0]
        if not lines[next_idx][1].startswith("-"):
            extra, next_idx = _parse_yaml_mapping(lines, next_idx, cont_indent)
            item_map.update(extra)
    return item_map, next_idx


def _parse_yaml_sequence(
    lines: list[tuple[int, str]],
    start: int,
    seq_indent: int,
) -> tuple[list[Any], int]:
    items: list[Any] = []
    idx = start
    while idx < len(lines):
        indent, content = lines[idx]
        if indent < seq_indent:
            break
        if indent > seq_indent:
            msg = f"unexpected YAML indentation inside sequence: {content!r}"
            raise DocumentParseError(msg)
        if content == "-":
            rest = ""
        elif content.startswith("- "):
            rest = content[2:].strip()
        else:
            break
        item, idx = _parse_yaml_sequence_item(lines, idx, seq_indent, rest)
        items.append(item)
    return items, idx


def _parse_yaml_mapping(
    lines: list[tuple[int, str]],
    start: int,
    map_indent: int,
) -> tuple[dict[str, Any], int]:
    mapping: dict[str, Any] = {}
    idx = start
    while idx < len(lines):
        indent, content = lines[idx]
        if indent < map_indent:
            break
        if indent > map_indent:
            msg = f"unexpected YAML indentation inside mapping: {content!r}"
            raise DocumentParseError(msg)
        if content == "-" or content.startswith("- "):
            break
        colon_idx = _find_unquoted_colon(content)
        if colon_idx == -1:
            msg = f"expected 'key: value' in YAML frontmatter, got: {content!r}"
            raise DocumentParseError(msg)
        key = str(_parse_yaml_scalar(content[:colon_idx].strip()))
        val_text = content[colon_idx + 1 :].strip()
        idx += 1
        if val_text:
            mapping[key] = _parse_yaml_scalar(val_text)
        elif idx < len(lines) and lines[idx][0] > map_indent:
            val, idx = _parse_yaml_node(lines, idx, lines[idx][0])
            mapping[key] = val
        elif idx < len(lines) and lines[idx][0] == map_indent and lines[idx][1].startswith("-"):
            val, idx = _parse_yaml_sequence(lines, idx, map_indent)
            mapping[key] = val
        else:
            mapping[key] = None
    return mapping, idx


def _parse_yaml_node(
    lines: list[tuple[int, str]],
    start: int,
    expected_indent: int,
) -> tuple[Any, int]:
    _, content = lines[start]
    if content == "-" or content.startswith("- "):
        return _parse_yaml_sequence(lines, start, expected_indent)
    return _parse_yaml_mapping(lines, start, expected_indent)


def parse_yaml_subset(raw_lines: list[str]) -> dict[str, Any]:
    prepared = _prepare_yaml_lines(raw_lines)
    if not prepared:
        return {}
    root, next_idx = _parse_yaml_node(prepared, 0, prepared[0][0])
    if next_idx != len(prepared):
        msg = f"unparsed trailing YAML line: {prepared[next_idx][1]!r}"
        raise DocumentParseError(msg)
    if not isinstance(root, dict):
        msg = "YAML frontmatter root must be a mapping"
        raise DocumentParseError(msg)
    return root


# ---------------------------------------------------------------------------
# Document loading (JSON & Markdown)
# ---------------------------------------------------------------------------


def _extract_frontmatter(lines: list[str]) -> tuple[list[str] | None, list[str]]:
    if not lines or lines[0].lstrip("\ufeff").strip() != "---":
        return None, lines
    for idx in range(1, len(lines)):
        if lines[idx].strip() in ("---", "..."):
            return lines[1:idx], lines[idx + 1 :]
    msg = "unterminated YAML frontmatter (missing closing '---')"
    raise DocumentParseError(msg)


def _split_markdown_blocks(
    lines: list[str],
) -> tuple[list[tuple[int, str]], list[FencedBlock]]:
    non_fenced: list[tuple[int, str]] = []
    fenced_blocks: list[FencedBlock] = []
    in_fence = False
    fence_lang = ""
    fence_start = 0
    fence_lines: list[tuple[int, str]] = []

    for line_no, line in enumerate(lines, start=1):
        stripped = line.lstrip()
        if stripped.startswith("```"):
            if not in_fence:
                in_fence = True
                fence_lang = stripped[3:].strip().split()[0] if stripped[3:].strip() else ""
                fence_start = line_no
                fence_lines = []
            else:
                fenced_blocks.append(FencedBlock(fence_lang, fence_start, fence_lines))
                in_fence = False
            continue
        if in_fence:
            fence_lines.append((line_no, line))
        else:
            non_fenced.append((line_no, line))
    return non_fenced, fenced_blocks


def _extract_json_block(fenced_blocks: list[FencedBlock]) -> dict[str, Any] | None:
    for block in fenced_blocks:
        if block.lang.lower() == "json":
            raw = "\n".join(line for _, line in block.lines)
            try:
                parsed = json.loads(raw)
            except json.JSONDecodeError as exc:
                msg = f"invalid JSON code block at line {block.start_line}: {exc}"
                raise DocumentParseError(msg) from exc
            if not isinstance(parsed, dict):
                msg = f"JSON code block at line {block.start_line} must be an object"
                raise DocumentParseError(msg)
            return parsed
    return None


def parse_document_text(text: str, *, is_json_hint: bool = False) -> ParsedDocument:
    stripped = text.lstrip("\ufeff").lstrip()
    if is_json_hint or stripped.startswith("{"):
        try:
            data = json.loads(text.lstrip("\ufeff"))
        except json.JSONDecodeError as exc:
            msg = f"invalid JSON document: {exc}"
            raise DocumentParseError(msg) from exc
        if not isinstance(data, dict):
            msg = "JSON document root must be an object"
            raise DocumentParseError(msg)
        return ParsedDocument(data=data, is_markdown=False, body_lines=[])

    lines = text.splitlines()
    fm_lines, body_lines = _extract_frontmatter(lines)
    fm_data = parse_yaml_subset(fm_lines) if fm_lines is not None else None
    if fm_data is not None and (set(fm_data) & STRUCTURED_PAYLOAD_KEYS):
        return ParsedDocument(data=fm_data, is_markdown=True, body_lines=body_lines)

    _, fenced_blocks = _split_markdown_blocks(body_lines)
    json_data = _extract_json_block(fenced_blocks)
    if json_data is not None:
        return ParsedDocument(data=json_data, is_markdown=True, body_lines=body_lines)

    msg = (
        "Markdown document must contain structured TaskPacket/TaskHandoff YAML frontmatter "
        "or a fenced ```json code block"
    )
    raise DocumentParseError(msg)


def load_document(path: Path) -> ParsedDocument:
    text = path.read_text(encoding="utf-8")
    return parse_document_text(text, is_json_hint=path.suffix.lower() == ".json")


# ---------------------------------------------------------------------------
# Stdlib JSON Schema Draft 2020-12 subset validator
# ---------------------------------------------------------------------------


def load_schema(schema_path: Path = SCHEMA_PATH) -> dict[str, Any]:
    raw = json.loads(schema_path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict):
        msg = f"schema root must be an object: {schema_path}"
        raise DocumentParseError(msg)
    return raw


def _resolve_ref(root_schema: dict[str, Any], ref: str) -> dict[str, Any]:
    prefix = "#/$defs/"
    if not ref.startswith(prefix):
        msg = f"unsupported JSON Schema $ref: {ref}"
        raise DocumentParseError(msg)
    name = ref[len(prefix) :]
    defs = root_schema.get("$defs", {})
    if not isinstance(defs, dict) or name not in defs or not isinstance(defs[name], dict):
        msg = f"unresolvable JSON Schema $ref: {ref}"
        raise DocumentParseError(msg)
    return defs[name]


def _matches_schema_type(value: object, expected: str) -> bool:
    if expected == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if expected == "number":
        return isinstance(value, (int, float)) and not isinstance(value, bool)
    if expected == "null":
        return value is None
    type_map: dict[str, type[object]] = {
        "object": dict,
        "array": list,
        "string": str,
        "boolean": bool,
    }
    py_type = type_map.get(expected)
    return py_type is not None and isinstance(value, py_type)


def _strict_json_equal(left: object, right: object) -> bool:
    if isinstance(left, bool) != isinstance(right, bool):
        return False
    return bool(left == right)


def _validate_string_keywords(
    value: str,
    node: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    min_len = node.get("minLength")
    if isinstance(min_len, int) and len(value) < min_len:
        issues.append(
            ValidationIssue(
                "SCHEMA_MIN_LENGTH",
                json_path,
                f"string length {len(value)} is shorter than minimum {min_len}",
            )
        )
    pattern = node.get("pattern")
    if isinstance(pattern, str) and re.search(pattern, value) is None:
        issues.append(
            ValidationIssue(
                "SCHEMA_PATTERN",
                json_path,
                f"value {value!r} does not match required pattern {pattern!r}",
            )
        )


def _validate_number_keywords(
    value: int | float,
    node: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    minimum = node.get("minimum")
    if isinstance(minimum, (int, float)) and not isinstance(minimum, bool) and value < minimum:
        issues.append(
            ValidationIssue(
                "SCHEMA_MINIMUM",
                json_path,
                f"value {value} is less than minimum {minimum}",
            )
        )
    maximum = node.get("maximum")
    if isinstance(maximum, (int, float)) and not isinstance(maximum, bool) and value > maximum:
        issues.append(
            ValidationIssue(
                "SCHEMA_MAXIMUM",
                json_path,
                f"value {value} exceeds maximum {maximum}",
            )
        )


def _validate_array_keywords(
    value: list[Any],
    node: dict[str, Any],
    root_schema: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    min_items = node.get("minItems")
    if isinstance(min_items, int) and len(value) < min_items:
        issues.append(
            ValidationIssue(
                "SCHEMA_MIN_ITEMS",
                json_path,
                f"array has {len(value)} item(s); minimum is {min_items}",
            )
        )
    if node.get("uniqueItems") is True:
        seen: list[str] = []
        for idx, item in enumerate(value):
            encoded = json.dumps(item, sort_keys=True, ensure_ascii=False)
            if encoded in seen:
                issues.append(
                    ValidationIssue(
                        "SCHEMA_UNIQUE_ITEMS",
                        f"{json_path}[{idx}]",
                        f"duplicate item {item!r} is not allowed",
                    )
                )
            else:
                seen.append(encoded)
    item_schema = node.get("items")
    if isinstance(item_schema, dict):
        for idx, item in enumerate(value):
            _validate_schema_node(item, item_schema, root_schema, f"{json_path}[{idx}]", issues)


def _validate_object_keywords(
    value: dict[str, Any],
    node: dict[str, Any],
    root_schema: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    required = node.get("required", [])
    if isinstance(required, list):
        issues.extend(
            ValidationIssue(
                "SCHEMA_REQUIRED",
                f"{json_path}.{req_key}",
                f"missing required property '{req_key}'",
            )
            for req_key in required
            if req_key not in value
        )
    props = node.get("properties", {})
    allowed_keys = set(props) if isinstance(props, dict) else set()
    if node.get("additionalProperties") is False:
        for key in sorted(value):
            if key not in allowed_keys:
                child_path = f"{json_path}.{key}"
                if TIME_FIELD_RE.search(key):
                    issues.append(
                        ValidationIssue(
                            "FORBIDDEN_TIME_CONSTRAINT",
                            child_path,
                            f"time-based constraint '{key}' is forbidden; "
                            "constrain tasks by work volume (max_files/max_functions), not time",
                        )
                    )
                else:
                    issues.append(
                        ValidationIssue(
                            "SCHEMA_ADDITIONAL_PROPERTY",
                            child_path,
                            f"unexpected property '{key}' (additionalProperties: false)",
                        )
                    )
    if isinstance(props, dict):
        for key, prop_schema in props.items():
            if key in value and isinstance(prop_schema, dict):
                _validate_schema_node(
                    value[key],
                    prop_schema,
                    root_schema,
                    f"{json_path}.{key}",
                    issues,
                )


def _validate_one_of(
    value: object,
    branches: list[Any],
    root_schema: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    valid_branches = 0
    branch_errors: list[list[ValidationIssue]] = []
    for branch in branches:
        sub_issues: list[ValidationIssue] = []
        _validate_schema_node(value, branch, root_schema, json_path, sub_issues)
        if not sub_issues:
            valid_branches += 1
        else:
            branch_errors.append(sub_issues)
    if valid_branches != 1:
        detail = branch_errors[0][0].message if branch_errors else "ambiguous oneOf match"
        issues.append(
            ValidationIssue(
                "SCHEMA_ONE_OF",
                json_path,
                f"value did not match exactly one allowed schema branch ({detail})",
            )
        )


def _validate_typed_keywords(
    value: object,
    node: dict[str, Any],
    root_schema: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    if isinstance(value, str):
        _validate_string_keywords(value, node, json_path, issues)
    elif isinstance(value, (int, float)) and not isinstance(value, bool):
        _validate_number_keywords(value, node, json_path, issues)
    elif isinstance(value, list):
        _validate_array_keywords(value, node, root_schema, json_path, issues)
    elif isinstance(value, dict):
        _validate_object_keywords(value, node, root_schema, json_path, issues)


def _validate_schema_node(
    value: object,
    node: dict[str, Any],
    root_schema: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    if "$ref" in node:
        target = _resolve_ref(root_schema, str(node["$ref"]))
        _validate_schema_node(value, target, root_schema, json_path, issues)
        return

    if "oneOf" in node and isinstance(node["oneOf"], list):
        _validate_one_of(value, node["oneOf"], root_schema, json_path, issues)
        return

    if "const" in node and not _strict_json_equal(value, node["const"]):
        issues.append(
            ValidationIssue(
                "SCHEMA_CONST",
                json_path,
                f"expected constant value {node['const']!r}, got {value!r}",
            )
        )
        return

    if (
        "enum" in node
        and isinstance(node["enum"], list)
        and not any(_strict_json_equal(value, option) for option in node["enum"])
    ):
        issues.append(
            ValidationIssue(
                "SCHEMA_ENUM",
                json_path,
                f"value {value!r} must be one of {node['enum']!r}",
            )
        )
        return

    schema_type = node.get("type")
    if isinstance(schema_type, str) and not _matches_schema_type(value, schema_type):
        issues.append(
            ValidationIssue(
                "SCHEMA_TYPE",
                json_path,
                f"expected type '{schema_type}', got {type(value).__name__} ({value!r})",
            )
        )
        return

    if "not" in node and isinstance(node["not"], dict):
        neg_issues: list[ValidationIssue] = []
        _validate_schema_node(value, node["not"], root_schema, json_path, neg_issues)
        if not neg_issues:
            issues.append(
                ValidationIssue(
                    "SCHEMA_NOT",
                    json_path,
                    f"value {value!r} matches a forbidden pattern",
                )
            )

    _validate_typed_keywords(value, node, root_schema, json_path, issues)


# ---------------------------------------------------------------------------
# Policy, path glob, and Markdown body checks
# ---------------------------------------------------------------------------


def _is_placeholder(text: str) -> bool:
    return text.strip().lower() in PLACEHOLDER_STRINGS


def _has_background_ampersand(command: str) -> bool:
    if NOHUP_RE.search(command):
        return True
    in_quote: str | None = None
    escaped = False
    n = len(command)
    idx = 0
    while idx < n:
        ch = command[idx]
        if escaped:
            escaped = False
            idx += 1
            continue
        if in_quote == '"' and ch == "\\":
            escaped = True
            idx += 1
            continue
        if in_quote is not None:
            if ch == in_quote:
                in_quote = None
            idx += 1
            continue
        if ch in ('"', "'"):
            in_quote = ch
            idx += 1
            continue
        if ch == "#" and (idx == 0 or command[idx - 1].isspace()):
            break
        if ch == "&":
            prev_ch = command[idx - 1] if idx > 0 else ""
            next_ch = command[idx + 1] if idx + 1 < n else ""
            if prev_ch in (">", "<"):
                idx += 1
                continue
            if next_ch in ("&", ">"):
                idx += 2
                continue
            return True
        idx += 1
    return False


def _match_forbidden_autofix(text: str) -> str | None:
    for label, regex in FORBIDDEN_AUTOFIX_PATTERNS:
        if regex.search(text) is not None:
            return label
    return None


def _normalize_target(target: str) -> str:
    return target.strip().removeprefix("./").rstrip("/")


def _extract_packet_targets(data: dict[str, Any]) -> list[str]:
    scope = data.get("scope")
    if not isinstance(scope, dict):
        return []
    raw_target = scope.get("target")
    if isinstance(raw_target, str):
        return [_normalize_target(raw_target)]
    if isinstance(raw_target, list):
        return [_normalize_target(item) for item in raw_target if isinstance(item, str)]
    return []


def _extract_invoked_lint_targets(command: str) -> list[str]:
    targets: list[str] = []
    for match in RUN_SH_CALL_RE.finditer(command):
        raw = match.group(1) or match.group(2) or match.group(3) or ""
        if raw:
            targets.append(_normalize_target(raw))
    for match in MAKE_LINT_RE.finditer(command):
        args_str = match.group(1)
        tokens = [tok for tok in args_str.split() if not tok.startswith("-") and "=" not in tok]
        if "lint" in tokens:
            t_match = MAKE_LINT_TARGET_RE.search(args_str)
            if t_match is None:
                targets.append("all")
            else:
                t_val = t_match.group(1) or t_match.group(2) or t_match.group(3) or "all"
                targets.append(_normalize_target(t_val))
    return targets


def _check_verification_command(
    command: str,
    *,
    json_path: str,
    expected_targets: list[str],
    forbid_repo_wide: bool,
    issues: list[ValidationIssue],
) -> None:
    bad_fix = _match_forbidden_autofix(command)
    if bad_fix is not None:
        issues.append(
            ValidationIssue(
                "FORBIDDEN_AUTOFIX_COMMAND",
                json_path,
                f"verification command contains forbidden GUARD-01 bulk autofix ({bad_fix})",
            )
        )
    if _has_background_ampersand(command):
        issues.append(
            ValidationIssue(
                "FORBIDDEN_BACKGROUND_EXECUTION",
                json_path,
                "verification command must run in foreground only (no '&' or 'nohup')",
            )
        )
    invoked = _extract_invoked_lint_targets(command)
    if forbid_repo_wide and "all" in invoked:
        issues.append(
            ValidationIssue(
                "FORBIDDEN_REPO_WIDE_COMMAND",
                json_path,
                "forbid_repo_wide_test_build=true forbids repo-wide lint ('./scripts/lint/run.sh all' or 'make lint')",
            )
        )
    if not expected_targets:
        return
    issues.extend(
        ValidationIssue(
            "MISSING_VERIFICATION_TARGET",
            json_path,
            f"verification command must invoke './scripts/lint/run.sh {target}'",
        )
        for target in expected_targets
        if target not in invoked
    )


def _check_markdown_headings(
    body_lines: list[str],
    required: tuple[tuple[re.Pattern[str], str], ...],
    issues: list[ValidationIssue],
) -> None:
    non_fenced, _ = _split_markdown_blocks(body_lines)
    matched_positions: list[tuple[int, str]] = []
    for regex, label in required:
        found = next(
            (line_no for line_no, line in non_fenced if regex.match(line.strip()) is not None),
            None,
        )
        if found is None:
            issues.append(
                ValidationIssue(
                    "MISSING_MARKDOWN_SECTION",
                    "markdown",
                    f"missing required heading '{label}'",
                )
            )
        else:
            matched_positions.append((found, label))

    if len(matched_positions) == len(required):
        line_nums = [pos for pos, _ in matched_positions]
        if line_nums != sorted(line_nums):
            issues.append(
                ValidationIssue(
                    "MARKDOWN_SECTION_ORDER",
                    "markdown",
                    "required Markdown headings appear out of order",
                )
            )


def _check_packet_markdown_body(body_lines: list[str], issues: list[ValidationIssue]) -> None:
    non_fenced, fenced_blocks = _split_markdown_blocks(body_lines)

    for block in fenced_blocks:
        if block.lang.lower() not in SHELL_FENCE_LANGS:
            continue
        for line_no, line in block.lines:
            stripped = line.strip()
            if not stripped or stripped.startswith("#") or SHELL_NG_COMMENT_RE.search(line):
                continue
            bad_fix = _match_forbidden_autofix(line)
            if bad_fix is not None:
                issues.append(
                    ValidationIssue(
                        "FORBIDDEN_AUTOFIX_COMMAND",
                        f"markdown:line:{line_no}",
                        f"shell block instructs forbidden GUARD-01 autofix ({bad_fix})",
                    )
                )
            if _has_background_ampersand(line):
                issues.append(
                    ValidationIssue(
                        "FORBIDDEN_BACKGROUND_EXECUTION",
                        f"markdown:line:{line_no}",
                        "shell block instructs forbidden background execution ('&' or 'nohup')",
                    )
                )

    for line_no, line in non_fenced:
        if PROHIBITION_CUE_RE.search(line) is not None:
            continue
        bad_fix = _match_forbidden_autofix(line)
        if bad_fix is not None:
            issues.append(
                ValidationIssue(
                    "FORBIDDEN_AUTOFIX_COMMAND",
                    f"markdown:line:{line_no}",
                    f"packet body affirmatively instructs forbidden autofix ({bad_fix})",
                )
            )
        inline_bg = any(
            len(span.strip()) > 1 and _has_background_ampersand(span)
            for span in INLINE_CODE_RE.findall(line)
        )
        if inline_bg or BG_PROSE_AFFIRMATIVE_RE.search(line) or NOHUP_RE.search(line):
            issues.append(
                ValidationIssue(
                    "FORBIDDEN_BACKGROUND_EXECUTION",
                    f"markdown:line:{line_no}",
                    "packet body affirmatively instructs background execution",
                )
            )


def _expand_braces(pattern: str) -> list[str]:
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
                for sub_ch in inner:
                    if sub_ch == "{":
                        inner_depth += 1
                        buf.append(sub_ch)
                    elif sub_ch == "}":
                        inner_depth -= 1
                        buf.append(sub_ch)
                    elif sub_ch == "," and inner_depth == 0:
                        options.append("".join(buf))
                        buf = []
                    else:
                        buf.append(sub_ch)
                options.append("".join(buf))
                expanded: list[str] = []
                for opt in options:
                    expanded.extend(_expand_braces(prefix + opt + suffix))
                return expanded
    return [pattern]


def _glob_to_regex(pattern: str) -> re.Pattern[str]:
    pat = pattern.strip().removeprefix("./")
    if pat.endswith("/"):
        pat = f"{pat}**"
    out: list[str] = ["^"]
    idx = 0
    n = len(pat)
    while idx < n:
        if pat[idx : idx + 3] == "**/":
            out.append("(?:.+/)?")
            idx += 3
        elif pat[idx : idx + 3] == "/**" and idx + 3 == n:
            out.append("/.+")
            idx += 3
        elif pat[idx : idx + 2] == "**":
            out.append(".*")
            idx += 2
        elif pat[idx] == "*":
            out.append("[^/]*")
            idx += 1
        elif pat[idx] == "?":
            out.append("[^/]")
            idx += 1
        else:
            out.append(re.escape(pat[idx]))
            idx += 1
    out.append("$")
    return re.compile("".join(out))


def _normalize_repo_rel_path(file_path: str) -> str:
    parts = [seg for seg in file_path.strip().split("/") if seg and seg != "."]
    return "/".join(parts)


def _matches_any_glob(file_path: str, patterns: list[str]) -> bool:
    norm = _normalize_repo_rel_path(file_path)
    for pattern in patterns:
        for alt in _expand_braces(pattern):
            if _glob_to_regex(_normalize_repo_rel_path(alt)).match(norm) is not None:
                return True
    return False


def _extract_command_policies(command: str) -> list[str]:
    policies: list[str] = []
    for match in CMD_LINT_POLICY_RE.finditer(command):
        val = (match.group(1) or match.group(2) or match.group(3) or "").strip()
        if val:
            policies.append(val)
    return policies


def _check_policy_command_match(
    command: str,
    *,
    declared_policy: str,
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    explicit_policies = _extract_command_policies(command)
    if declared_policy == "block-new" and not explicit_policies:
        issues.append(
            ValidationIssue(
                "POLICY_COMMAND_MISMATCH",
                json_path,
                "execution_policy.policy='block-new' requires verification command to explicitly "
                "include '--policy block-new' or 'LINT_POLICY=block-new'",
            )
        )
    issues.extend(
        ValidationIssue(
            "POLICY_COMMAND_MISMATCH",
            json_path,
            f"execution_policy.policy={declared_policy!r} conflicts with command policy {explicit!r}",
        )
        for explicit in explicit_policies
        if explicit != declared_policy
    )


def _has_generated_header_text(text: str) -> bool:
    for line in text.splitlines()[:GENERATED_HEADER_SCAN_LINES]:
        if GENERATED_HEADER_RE.search(line) is not None:
            return True
    return False


def _file_has_generated_header(path: Path) -> bool:
    try:
        raw = path.read_bytes()
    except OSError:
        return False
    if b"\x00" in raw[:4096]:
        return False
    text = raw[:8192].decode("utf-8", errors="replace")
    return _has_generated_header_text(text)


def _run_subprocess_bytes(
    argv: list[str],
    *,
    cwd: Path | None = None,
    timeout: int = SUBPROCESS_TIMEOUT_SECONDS,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(  # noqa: S603 -- centralized argv subprocess runner (screener, bash)
        argv,
        cwd=str(cwd) if cwd is not None else None,
        capture_output=True,
        timeout=timeout,
        check=False,
    )


def _run_git_bytes(repo_root: Path, git_bin: str, *args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(  # noqa: S603 -- runs resolved git binary inside explicit repo_root
        [git_bin, "-C", str(repo_root), *args],
        capture_output=True,
        timeout=SUBPROCESS_TIMEOUT_SECONDS,
        check=False,
    )


def _run_bash_command(
    command: str,
    *,
    cwd: Path,
    timeout: int,
) -> tuple[int, float, bytes, bytes]:
    bash_bin = shutil.which("bash")
    if bash_bin is None:
        msg = "bash executable not found in PATH"
        raise DocumentParseError(msg)
    t0 = time.monotonic()
    try:
        res = _run_subprocess_bytes([bash_bin, "-c", command], cwd=cwd, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        elapsed = round(time.monotonic() - t0, 3)
        out = exc.stdout if isinstance(exc.stdout, bytes) else b""
        err = exc.stderr if isinstance(exc.stderr, bytes) else b""
        return TIMEOUT_EXIT_CODE, elapsed, out, err
    elapsed = round(time.monotonic() - t0, 3)
    return res.returncode, elapsed, res.stdout, res.stderr


def _git_rev_has_generated_header(repo_root: Path, git_bin: str, rev: str, norm: str) -> bool:
    res = _run_git_bytes(repo_root, git_bin, "show", "--end-of-options", f"{rev}:{norm}")
    if res.returncode != 0 or b"\x00" in res.stdout[:4096]:
        return False
    text = res.stdout[:8192].decode("utf-8", errors="replace")
    return _has_generated_header_text(text)


def _repo_rel_is_generated_code(
    repo_root: Path,
    rel_path: str,
    *,
    base_commit: str | None = None,
) -> bool:
    norm = _normalize_repo_rel_path(rel_path)
    if not norm:
        return False
    repo_root_resolved = repo_root.resolve()
    disk_path = repo_root / norm
    if (
        disk_path.is_file()
        and not disk_path.is_symlink()
        and disk_path.resolve().is_relative_to(repo_root_resolved)
        and _file_has_generated_header(disk_path)
    ):
        return True
    git_bin = shutil.which("git")
    if git_bin is None or not (repo_root / ".git").exists():
        return False
    if _git_rev_has_generated_header(repo_root, git_bin, "HEAD", norm):
        return True
    if base_commit and base_commit != "HEAD" and not base_commit.startswith("-"):
        return _git_rev_has_generated_header(repo_root, git_bin, base_commit, norm)
    return False


def list_generated_forbidden_paths(repo_root: Path) -> list[str]:
    git_bin = shutil.which("git")
    candidates: list[str] = []
    if git_bin is not None and (repo_root / ".git").exists():
        res = _run_git_bytes(repo_root, git_bin, "ls-files", "-z")
        if res.returncode == 0:
            candidates = [
                entry.decode("utf-8", errors="surrogateescape")
                for entry in res.stdout.split(b"\x00")
                if entry
            ]
    if not candidates:
        candidates = [
            p.relative_to(repo_root).as_posix()
            for p in repo_root.rglob("*")
            if p.is_file() and ".git" not in p.parts
        ]

    repo_root_resolved = repo_root.resolve()
    generated: list[str] = []
    for rel in sorted(set(candidates)):
        if rel.startswith(EXCLUDED_GENERATED_PREFIXES):
            continue
        full = repo_root / rel
        if (
            full.is_file()
            and not full.is_symlink()
            and full.resolve().is_relative_to(repo_root_resolved)
            and _file_has_generated_header(full)
        ):
            generated.append(rel)
    return generated


def _run_behavior_screener_for_base(repo_root: Path, base_commit: str) -> list[dict[str, Any]]:
    if not base_commit or not base_commit.strip() or base_commit.startswith("-"):
        msg = f"invalid --base-commit revision: {base_commit!r}"
        raise DocumentParseError(msg)
    if not SCREENER_SCRIPT_PATH.is_file():
        msg = f"behavior screener script not found: {SCREENER_SCRIPT_PATH}"
        raise DocumentParseError(msg)
    try:
        res = _run_subprocess_bytes(
            [
                sys.executable,
                str(SCREENER_SCRIPT_PATH),
                "--repo-root",
                str(repo_root),
                "--base",
                base_commit,
            ],
            timeout=SUBPROCESS_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        msg = f"behavior_screener.py timed out after {SUBPROCESS_TIMEOUT_SECONDS}s"
        raise DocumentParseError(msg) from exc
    if res.returncode != 0:
        err_text = res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"behavior_screener.py failed (rc={res.returncode}): {err_text}"
        raise DocumentParseError(msg)
    out_text = res.stdout.decode("utf-8", errors="replace")
    return _parse_screener_candidates_text(out_text, source="--base-commit")


def _parse_screener_candidates_text(raw: str, *, source: str) -> list[dict[str, Any]]:
    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError as exc:
        msg = f"invalid screener report JSON ({source}): {exc}"
        raise DocumentParseError(msg) from exc
    if not isinstance(parsed, list) or not all(isinstance(item, dict) for item in parsed):
        msg = f"screener report ({source}) must be a JSON array of candidate objects"
        raise DocumentParseError(msg)
    return parsed


def _candidate_matches_response(
    candidate: dict[str, Any],
    response: dict[str, Any],
) -> bool:
    cand_id = candidate.get("id")
    resp_id = response.get("candidate_id")
    if isinstance(cand_id, str) and cand_id and isinstance(resp_id, str) and resp_id == cand_id:
        return True
    c_file = candidate.get("file")
    r_file = response.get("file")
    if not (isinstance(c_file, str) and isinstance(r_file, str)):
        return False
    return (
        _normalize_repo_rel_path(c_file) == _normalize_repo_rel_path(r_file)
        and candidate.get("function") == response.get("function")
        and candidate.get("kind") == response.get("kind")
    )


@dataclass(frozen=True)
class _TestNameQuery:
    raw: str
    path_hint: str | None
    leaf: str
    parent_symbol: str | None
    go_parent: str | None
    go_subtests: tuple[str, ...]


def _parse_test_name_query(test_name: str) -> _TestNameQuery:
    cleaned = test_name.strip().strip("`\"'").strip()
    path_hint: str | None = None
    symbol_part = cleaned

    if "::" in cleaned:
        parts = [p.strip() for p in cleaned.split("::") if p.strip()]
        if parts and SOURCE_PATH_EXT_RE.search(parts[0]) is not None:
            path_hint = _normalize_repo_rel_path(parts[0])
            parts = parts[1:]
        if parts:
            leaf = PARAM_SUFFIX_RE.sub("", parts[-1]).strip()
            parent = (
                PARAM_SUFFIX_RE.sub("", parts[-2]).strip()
                if len(parts) >= MIN_QUALIFIED_PARTS
                else None
            )
            return _TestNameQuery(
                raw=cleaned,
                path_hint=path_hint,
                leaf=leaf,
                parent_symbol=parent,
                go_parent=None,
                go_subtests=(),
            )

    if "/" in symbol_part:
        slash_parts = [p.strip() for p in symbol_part.split("/") if p.strip()]
        if slash_parts and GO_TEST_PREFIX_RE.match(slash_parts[0]) is not None:
            go_parent = slash_parts[0]
            go_subtests = tuple(slash_parts[1:])
            return _TestNameQuery(
                raw=cleaned,
                path_hint=None,
                leaf=go_subtests[-1] if go_subtests else go_parent,
                parent_symbol=go_parent,
                go_parent=go_parent,
                go_subtests=go_subtests,
            )

    if " > " in symbol_part:
        gt_parts = [p.strip() for p in symbol_part.split(" > ") if p.strip()]
        leaf = PARAM_SUFFIX_RE.sub("", gt_parts[-1]).strip()
        return _TestNameQuery(
            raw=cleaned,
            path_hint=None,
            leaf=leaf,
            parent_symbol=gt_parts[-2] if len(gt_parts) >= MIN_QUALIFIED_PARTS else None,
            go_parent=None,
            go_subtests=(),
        )

    if "." in symbol_part and " " not in symbol_part:
        dot_parts = [p.strip() for p in symbol_part.split(".") if p.strip()]
        if len(dot_parts) >= MIN_QUALIFIED_PARTS:
            leaf = PARAM_SUFFIX_RE.sub("", dot_parts[-1]).strip()
            return _TestNameQuery(
                raw=cleaned,
                path_hint=None,
                leaf=leaf,
                parent_symbol=dot_parts[-2],
                go_parent=None,
                go_subtests=(),
            )

    leaf = PARAM_SUFFIX_RE.sub("", symbol_part).strip()
    return _TestNameQuery(
        raw=cleaned,
        path_hint=None,
        leaf=leaf,
        parent_symbol=None,
        go_parent=None,
        go_subtests=(),
    )


def _classify_test_file_lang(rel_posix: str) -> str | None:
    if rel_posix.startswith(EXCLUDED_GENERATED_PREFIXES):
        return None
    if GO_TEST_FILE_RE.search(rel_posix) is not None:
        return "go"
    if PY_TEST_FILE_RE.search(rel_posix) is not None:
        return "python"
    if rel_posix.endswith(".rs"):
        return "rust"
    if TS_TEST_FILE_RE.search(rel_posix) is not None:
        return "typescript"
    return None


def _collect_repo_rel_paths(repo_root: Path) -> list[str]:
    git_bin = shutil.which("git")
    if git_bin is not None and (repo_root / ".git").exists():
        res = _run_git_bytes(
            repo_root,
            git_bin,
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
        )
        if res.returncode == 0:
            return [
                entry.decode("utf-8", errors="surrogateescape")
                for entry in res.stdout.split(b"\x00")
                if entry
            ]
    rel_paths: list[str] = []
    stack: list[Path] = [repo_root]
    while stack:
        cur = stack.pop()
        try:
            entries = list(cur.iterdir())
        except OSError:
            continue
        for child in entries:
            if child.is_symlink():
                continue
            if child.is_dir():
                if child.name not in EXCLUDED_TEST_SCAN_DIRS:
                    stack.append(child)
            elif child.is_file():
                rel_paths.append(child.relative_to(repo_root).as_posix())
    return rel_paths


def _list_repo_test_files(repo_root: Path) -> list[tuple[str, Path, str]]:
    repo_root_resolved = repo_root.resolve()
    result: list[tuple[str, Path, str]] = []
    for rel in sorted(set(_collect_repo_rel_paths(repo_root))):
        lang = _classify_test_file_lang(rel)
        if lang is None:
            continue
        full = repo_root / rel
        if (
            full.is_file()
            and not full.is_symlink()
            and full.resolve().is_relative_to(repo_root_resolved)
        ):
            result.append((rel, full, lang))
    return result


def _go_subtest_present(text: str, subtest: str) -> bool:
    escaped = re.escape(subtest).replace("_", r"[\s_/-]")
    pattern = re.compile(rf'(?:["`]{escaped}["`])')
    return pattern.search(text) is not None


def _go_test_func_present(text: str, name: str) -> bool:
    if GO_TEST_PREFIX_RE.match(name) is None:
        return False
    return re.search(rf"\bfunc\s+(?:\([^)]+\)\s+)?{re.escape(name)}\s*\(", text) is not None


def _matches_go_test(text: str, query: _TestNameQuery) -> bool:
    if query.go_parent is not None:
        if not _go_test_func_present(text, query.go_parent):
            return False
        return all(_go_subtest_present(text, sub) for sub in query.go_subtests)
    if IDENT_RE.match(query.leaf) is not None and _go_test_func_present(text, query.leaf):
        return True
    if query.leaf:
        run_pat = re.compile(rf'\bt\.Run\s*\(\s*["`]{re.escape(query.leaf)}["`]')
        return run_pat.search(text) is not None
    return False


def _matches_python_test(text: str, query: _TestNameQuery) -> bool:
    if IDENT_RE.match(query.leaf) is None or not query.leaf.startswith("test"):
        return False
    fn_pat = re.compile(rf"^\s*(?:async\s+)?def\s+{re.escape(query.leaf)}\s*\(", re.MULTILINE)
    if fn_pat.search(text) is None:
        return False
    if query.parent_symbol and IDENT_RE.match(query.parent_symbol) is not None:
        cls_pat = re.compile(rf"^\s*class\s+{re.escape(query.parent_symbol)}\b", re.MULTILINE)
        return cls_pat.search(text) is not None
    return True


def _matches_rust_test(rel_posix: str, text: str, query: _TestNameQuery) -> bool:
    if IDENT_RE.match(query.leaf) is None or query.leaf not in text:
        return False
    fn_pat = re.compile(rf"\b(?:async\s+)?fn\s+{re.escape(query.leaf)}\s*[<(]")
    lines = text.splitlines()
    is_dedicated_test_file = RUST_TEST_PATH_RE.search(rel_posix) is not None
    for match in fn_pat.finditer(text):
        line_idx = text.count("\n", 0, match.start())
        start_idx = max(0, line_idx - RUST_ATTR_LOOKBACK_LINES)
        preceding = "\n".join(lines[start_idx:line_idx])
        if RUST_TEST_ATTR_RE.search(preceding) is not None:
            return True
        if is_dedicated_test_file and query.leaf.startswith("test_"):
            return True
    return False


def _matches_ts_test(text: str, query: _TestNameQuery) -> bool:
    titles = {t for t in (query.leaf, query.raw) if t}
    for title in titles:
        pat = re.compile(
            rf"""(?:Deno\.)?(?:test|it|describe)(?:\.(?:only|each))?\s*\(\s*["'`]{re.escape(title)}["'`]"""
        )
        if pat.search(text) is not None:
            return True
    if IDENT_RE.match(query.leaf) is not None:
        deno_fn = re.compile(
            rf"\bDeno\.test\s*\(\s*(?:async\s+)?function\s+{re.escape(query.leaf)}\b"
        )
        return deno_fn.search(text) is not None
    return False


def _candidate_test_exists(
    repo_root: Path,
    test_name: str,
    test_files: list[tuple[str, Path, str]],
    text_cache: dict[Path, str],
) -> bool:
    query = _parse_test_name_query(test_name)
    if not query.leaf:
        return False

    candidates = test_files
    if query.path_hint is not None:
        hint = query.path_hint
        candidates = [
            item for item in test_files if item[0] == hint or item[0].endswith(f"/{hint}")
        ]

    for rel_posix, full_path, lang in candidates:
        text = text_cache.get(full_path)
        if text is None:
            try:
                text = full_path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                text = ""
            text_cache[full_path] = text
        if lang == "go" and _matches_go_test(text, query):
            return True
        if lang == "python" and _matches_python_test(text, query):
            return True
        if lang == "rust" and _matches_rust_test(rel_posix, text, query):
            return True
        if lang == "typescript" and _matches_ts_test(text, query):
            return True
    return False


def _validate_behavior_candidates(
    data: dict[str, Any],
    screener_candidates: list[dict[str, Any]] | None,
    issues: list[ValidationIssue],
    *,
    repo_root: Path | None = None,
) -> None:
    impact = data.get("runtime_behavior_impact")
    has_change = impact.get("has_change") if isinstance(impact, dict) else None
    raw_responses = impact.get("candidate_responses", []) if isinstance(impact, dict) else []
    responses: list[tuple[int, dict[str, Any]]] = [
        (idx, item) for idx, item in enumerate(raw_responses) if isinstance(item, dict)
    ]

    test_files_cache: list[tuple[str, Path, str]] | None = None
    text_cache: dict[Path, str] = {}

    for idx, resp in responses:
        if resp.get("disposition") == "intended":
            if has_change is False:
                issues.append(
                    ValidationIssue(
                        "CONTRADICTORY_BEHAVIOR_IMPACT",
                        f"$.runtime_behavior_impact.candidate_responses[{idx}].disposition",
                        "runtime_behavior_impact.has_change=false contradicts candidate response "
                        "with disposition='intended'",
                    )
                )
            test_name = resp.get("test_name")
            if not isinstance(test_name, str) or _is_placeholder(test_name):
                issues.append(
                    ValidationIssue(
                        "MISSING_CANDIDATE_TEST_NAME",
                        f"$.runtime_behavior_impact.candidate_responses[{idx}].test_name",
                        "candidate response with disposition='intended' requires a non-empty, "
                        "non-placeholder test_name",
                    )
                )
            elif repo_root is not None:
                if test_files_cache is None:
                    test_files_cache = _list_repo_test_files(repo_root)
                if not _candidate_test_exists(repo_root, test_name, test_files_cache, text_cache):
                    issues.append(
                        ValidationIssue(
                            "NONEXISTENT_CANDIDATE_TEST",
                            f"$.runtime_behavior_impact.candidate_responses[{idx}].test_name",
                            f"candidate response test_name {test_name!r} was not found in any "
                            f"test file under {repo_root}",
                        )
                    )

    if not screener_candidates:
        return

    for cand in screener_candidates:
        matched: tuple[int, dict[str, Any]] | None = next(
            ((idx, resp) for idx, resp in responses if _candidate_matches_response(cand, resp)),
            None,
        )
        cand_label = f"id={cand.get('id')!r} ({cand.get('file')}:{cand.get('function')} [{cand.get('kind')}])"
        if matched is None:
            issues.append(
                ValidationIssue(
                    "UNANSWERED_BEHAVIOR_CANDIDATE",
                    "$.runtime_behavior_impact.candidate_responses",
                    f"active CHECK-01 behavior candidate {cand_label} has no matching response",
                )
            )
            continue

        resp_idx, resp = matched
        if resp.get("disposition") == "unintended_reverted":
            issues.append(
                ValidationIssue(
                    "UNREVERTED_UNINTENDED_CANDIDATE",
                    f"$.runtime_behavior_impact.candidate_responses[{resp_idx}]",
                    f"candidate {cand_label} is marked 'unintended_reverted' but is still present "
                    "in the active screener report",
                )
            )


def _validate_generated_code_edits(
    handoff: dict[str, Any],
    *,
    repo_root: Path | None,
    allow_generated: bool,
    issues: list[ValidationIssue],
    base_commit: str | None = None,
) -> None:
    if repo_root is None or allow_generated:
        return
    modified_files = handoff.get("modified_files")
    if not isinstance(modified_files, list):
        return
    for idx, entry in enumerate(modified_files):
        if not isinstance(entry, dict):
            continue
        rel = entry.get("path")
        if isinstance(rel, str) and _repo_rel_is_generated_code(
            repo_root, rel, base_commit=base_commit
        ):
            issues.append(
                ValidationIssue(
                    "GENERATED_CODE_MODIFIED",
                    f"$.modified_files[{idx}].path",
                    f"modified file '{rel}' carries a generated-code DO NOT EDIT header; "
                    "set scope.allow_generated_code_edits=true only when explicitly regenerating",
                )
            )


# ---------------------------------------------------------------------------
# High-level validation functions
# ---------------------------------------------------------------------------


def _command_sha256(command: str) -> str:
    return hashlib.sha256(command.strip().encode("utf-8")).hexdigest()


def _resolve_command_budget(policy: dict[str, Any]) -> int:
    raw = policy.get("max_command_budget_s", DEFAULT_COMMAND_BUDGET_S)
    if isinstance(raw, int) and not isinstance(raw, bool) and 1 <= raw <= MAX_BUDGET_SECONDS:
        return raw
    return DEFAULT_COMMAND_BUDGET_S


def _validate_measured_execution(
    policy: dict[str, Any],
    verify_cmd: object,
    issues: list[ValidationIssue],
) -> None:
    if "measured" not in policy or policy.get("measured") is None:
        issues[:] = [
            item
            for item in issues
            if not (
                item.path == "$.execution_policy.measured"
                and item.code in ("SCHEMA_REQUIRED", "SCHEMA_TYPE")
            )
        ]
        issues.append(
            ValidationIssue(
                "UNMEASURED_VERIFICATION_COMMAND",
                "$.execution_policy.measured",
                "LOOP-02: execution_policy.measured is required; run "
                "'python3 scripts/agent-harness/task_packet.py measure <packet>' before dispatch",
            )
        )
        return

    measured = policy.get("measured")
    if not isinstance(measured, dict):
        return

    budget = _resolve_command_budget(policy)
    duration_s = measured.get("duration_s")
    if (
        isinstance(duration_s, (int, float))
        and not isinstance(duration_s, bool)
        and duration_s > budget
    ):
        issues.append(
            ValidationIssue(
                "VERIFICATION_BUDGET_EXCEEDED",
                "$.execution_policy.measured.duration_s",
                f"measured duration_s={duration_s} exceeds max_command_budget_s={budget}",
            )
        )

    at_commit = measured.get("at_commit")
    if isinstance(at_commit, str) and ALL_ZERO_SHA_RE.match(at_commit):
        issues.append(
            ValidationIssue(
                "PLACEHOLDER_MEASURED_COMMIT",
                "$.execution_policy.measured.at_commit",
                "measured.at_commit must be a real non-zero git commit SHA; run 'task_packet.py measure'",
            )
        )

    cmd_sha = measured.get("command_sha256")
    if isinstance(cmd_sha, str) and cmd_sha and isinstance(verify_cmd, str) and verify_cmd.strip():
        expected_sha = _command_sha256(verify_cmd)
        if cmd_sha != expected_sha:
            issues.append(
                ValidationIssue(
                    "STALE_MEASURED_COMMAND",
                    "$.execution_policy.measured.command_sha256",
                    f"measured command_sha256={cmd_sha} does not match verification_command "
                    f"({expected_sha}); re-run 'task_packet.py measure'",
                )
            )


def validate_packet_doc(doc: ParsedDocument, schema: dict[str, Any]) -> list[ValidationIssue]:
    issues: list[ValidationIssue] = []
    _validate_schema_node(
        doc.data,
        {"$ref": "#/$defs/TaskPacket"},
        schema,
        "$",
        issues,
    )

    scope = doc.data.get("scope")
    if isinstance(scope, dict):
        stop_cond = scope.get("stop_condition")
        if isinstance(stop_cond, str) and _is_placeholder(stop_cond):
            issues.append(
                ValidationIssue(
                    "PLACEHOLDER_VALUE",
                    "$.scope.stop_condition",
                    f"stop_condition must not be a placeholder ({stop_cond!r})",
                )
            )

    policy = doc.data.get("execution_policy")
    if isinstance(policy, dict):
        verify_cmd = policy.get("verification_command")
        forbid_wide = policy.get("forbid_repo_wide_test_build") is True
        declared_policy = policy.get("policy", "fix-existing")
        if isinstance(verify_cmd, str) and verify_cmd.strip():
            _check_verification_command(
                verify_cmd,
                json_path="$.execution_policy.verification_command",
                expected_targets=_extract_packet_targets(doc.data),
                forbid_repo_wide=forbid_wide,
                issues=issues,
            )
            if isinstance(declared_policy, str) and declared_policy in (
                "fix-existing",
                "block-new",
            ):
                _check_policy_command_match(
                    verify_cmd,
                    declared_policy=declared_policy,
                    json_path="$.execution_policy.verification_command",
                    issues=issues,
                )
        _validate_measured_execution(policy, verify_cmd, issues)

    if doc.is_markdown:
        _check_markdown_headings(doc.body_lines, PACKET_REQUIRED_HEADINGS, issues)
        _check_packet_markdown_body(doc.body_lines, issues)

    return issues


def _validate_handoff_impact(data: dict[str, Any], issues: list[ValidationIssue]) -> None:
    impact = data.get("runtime_behavior_impact")
    if not isinstance(impact, dict):
        return
    summary = impact.get("summary")
    if isinstance(summary, str) and _is_placeholder(summary):
        issues.append(
            ValidationIssue(
                "PLACEHOLDER_VALUE",
                "$.runtime_behavior_impact.summary",
                "runtime_behavior_impact.summary must not be empty or a placeholder",
            )
        )
    dims = impact.get("checked_dimensions")
    if isinstance(dims, list):
        issues.extend(
            ValidationIssue(
                "PLACEHOLDER_VALUE",
                f"$.runtime_behavior_impact.checked_dimensions[{idx}]",
                f"checked_dimensions entry must not be a placeholder ({dim!r})",
            )
            for idx, dim in enumerate(dims)
            if isinstance(dim, str) and _is_placeholder(dim)
        )


def _validate_handoff_verification_and_files(
    data: dict[str, Any],
    issues: list[ValidationIssue],
) -> None:
    verification = data.get("verification")
    if isinstance(verification, dict):
        cmd = verification.get("command")
        if isinstance(cmd, str) and cmd.strip():
            _check_verification_command(
                cmd,
                json_path="$.verification.command",
                expected_targets=[],
                forbid_repo_wide=False,
                issues=issues,
            )
        exit_code = verification.get("exit_code")
        if (
            data.get("status") == "completed"
            and isinstance(exit_code, int)
            and not isinstance(exit_code, bool)
            and exit_code != 0
        ):
            issues.append(
                ValidationIssue(
                    "COMPLETED_WITH_FAILED_VERIFICATION",
                    "$.verification.exit_code",
                    f"status='completed' requires verification.exit_code == 0 (got {exit_code})",
                )
            )

    modified = data.get("modified_files")
    if isinstance(modified, list):
        seen_paths: set[str] = set()
        for idx, entry in enumerate(modified):
            if not isinstance(entry, dict):
                continue
            rel = entry.get("path")
            if isinstance(rel, str):
                norm = _normalize_repo_rel_path(rel)
                if norm in seen_paths:
                    issues.append(
                        ValidationIssue(
                            "DUPLICATE_MODIFIED_FILE",
                            f"$.modified_files[{idx}].path",
                            f"duplicate modified file path '{norm}'",
                        )
                    )
                seen_paths.add(norm)


def validate_handoff_doc(
    doc: ParsedDocument,
    schema: dict[str, Any],
    packet_doc: ParsedDocument | None = None,
    *,
    screener_candidates: list[dict[str, Any]] | None = None,
    repo_root: Path | None = None,
) -> list[ValidationIssue]:
    issues: list[ValidationIssue] = []
    _validate_schema_node(
        doc.data,
        {"$ref": "#/$defs/TaskHandoff"},
        schema,
        "$",
        issues,
    )

    rationale = doc.data.get("rationale")
    if isinstance(rationale, str) and _is_placeholder(rationale):
        issues.append(
            ValidationIssue(
                "PLACEHOLDER_VALUE",
                "$.rationale",
                f"rationale must explain changes or state '変更なし' (got {rationale!r})",
            )
        )

    _validate_handoff_impact(doc.data, issues)
    _validate_handoff_verification_and_files(doc.data, issues)
    _validate_behavior_candidates(doc.data, screener_candidates, issues, repo_root=repo_root)

    allow_generated = (
        packet_doc is not None
        and isinstance(packet_doc.data.get("scope"), dict)
        and packet_doc.data["scope"].get("allow_generated_code_edits") is True
    )
    _validate_generated_code_edits(
        doc.data,
        repo_root=repo_root,
        allow_generated=allow_generated,
        issues=issues,
    )

    if doc.is_markdown:
        _check_markdown_headings(doc.body_lines, HANDOFF_REQUIRED_HEADINGS, issues)

    if packet_doc is not None:
        _cross_validate_handoff_against_packet(doc.data, packet_doc, schema, issues)

    return issues


def _cross_validate_modified_files(
    modified_files: object,
    scope: dict[str, Any],
    issues: list[ValidationIssue],
) -> None:
    if not isinstance(modified_files, list):
        return
    max_files = scope["max_files"]
    allowed_paths: list[str] = scope["allowed_paths"]
    forbidden_paths: list[str] = scope.get("forbidden_paths", [])
    if len(modified_files) > max_files:
        issues.append(
            ValidationIssue(
                "MAX_FILES_EXCEEDED",
                "$.modified_files",
                f"modified_files count {len(modified_files)} exceeds packet max_files={max_files}",
            )
        )
    for idx, entry in enumerate(modified_files):
        if not isinstance(entry, dict):
            continue
        rel = entry.get("path")
        if not isinstance(rel, str):
            continue
        if not _matches_any_glob(rel, allowed_paths):
            issues.append(
                ValidationIssue(
                    "PATH_OUTSIDE_ALLOWED_PATHS",
                    f"$.modified_files[{idx}].path",
                    f"modified file '{rel}' is outside packet scope.allowed_paths {allowed_paths!r}",
                )
            )
        if forbidden_paths and _matches_any_glob(rel, forbidden_paths):
            issues.append(
                ValidationIssue(
                    "PATH_IN_FORBIDDEN_PATHS",
                    f"$.modified_files[{idx}].path",
                    f"modified file '{rel}' matches packet scope.forbidden_paths {forbidden_paths!r}",
                )
            )


def _cross_validate_handoff_against_packet(
    handoff: dict[str, Any],
    packet_doc: ParsedDocument,
    schema: dict[str, Any],
    issues: list[ValidationIssue],
) -> None:
    packet_issues = validate_packet_doc(packet_doc, schema)
    if packet_issues:
        issues.extend(
            ValidationIssue(
                f"PACKET_{issue.code}",
                f"packet:{issue.path}",
                issue.message,
            )
            for issue in packet_issues
        )
        return

    packet = packet_doc.data
    expected_id = packet.get("packet_id")
    actual_id = handoff.get("packet_id")
    if expected_id != actual_id:
        issues.append(
            ValidationIssue(
                "PACKET_ID_MISMATCH",
                "$.packet_id",
                f"handoff packet_id {actual_id!r} does not match packet {expected_id!r}",
            )
        )

    scope = packet["scope"]
    _cross_validate_modified_files(handoff.get("modified_files", []), scope, issues)

    max_functions = scope["max_functions"]
    mod_funcs = handoff.get("modified_functions_count")
    if isinstance(mod_funcs, int) and not isinstance(mod_funcs, bool) and mod_funcs > max_functions:
        issues.append(
            ValidationIssue(
                "MAX_FUNCTIONS_EXCEEDED",
                "$.modified_functions_count",
                f"modified_functions_count {mod_funcs} exceeds packet max_functions={max_functions}",
            )
        )

    policy = packet.get("execution_policy", {})
    forbid_wide = isinstance(policy, dict) and policy.get("forbid_repo_wide_test_build") is True
    declared_policy = (
        policy.get("policy", "fix-existing") if isinstance(policy, dict) else "fix-existing"
    )
    verification = handoff.get("verification")
    if isinstance(verification, dict):
        verify_cmd = verification.get("command")
        if isinstance(verify_cmd, str) and verify_cmd.strip():
            required_targets = (
                _extract_packet_targets(packet) if handoff.get("status") == "completed" else []
            )
            _check_verification_command(
                verify_cmd,
                json_path="$.verification.command",
                expected_targets=required_targets,
                forbid_repo_wide=forbid_wide,
                issues=issues,
            )
            if isinstance(declared_policy, str) and declared_policy in (
                "fix-existing",
                "block-new",
            ):
                _check_policy_command_match(
                    verify_cmd,
                    declared_policy=declared_policy,
                    json_path="$.verification.command",
                    issues=issues,
                )


# ---------------------------------------------------------------------------
# CLI subcommands
# ---------------------------------------------------------------------------


def _emit_result(
    *,
    kind: str,
    target_path: Path,
    packet_id: object,
    issues: list[ValidationIssue],
    output_format: str,
) -> int:
    valid = len(issues) == 0
    if output_format == "json":
        payload = {
            "valid": valid,
            "kind": kind,
            "path": str(target_path),
            "packet_id": packet_id,
            "errors": [asdict(item) for item in issues],
        }
        sys.stdout.write(json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
        return EXIT_OK if valid else EXIT_INVALID

    if valid:
        sys.stdout.write(f"OK: {kind} {target_path} (packet_id={packet_id})\n")
        return EXIT_OK

    for item in issues:
        sys.stderr.write(f"ERROR [{item.code}] {item.path}: {item.message}\n")
    sys.stderr.write(f"INVALID: {len(issues)} error(s) in {target_path}\n")
    return EXIT_INVALID


def _cmd_validate_packet(args: argparse.Namespace) -> int:
    path = Path(args.path)
    if not path.is_file():
        sys.stderr.write(f"error: file not found: {path}\n")
        return EXIT_USAGE
    try:
        schema = load_schema()
        doc = load_document(path)
    except OSError as exc:
        sys.stderr.write(f"error: unable to read {path}: {exc}\n")
        return EXIT_USAGE
    except DocumentParseError as exc:
        return _emit_result(
            kind="packet",
            target_path=path,
            packet_id=None,
            issues=[ValidationIssue("PARSE_ERROR", "$", str(exc))],
            output_format=args.output_format,
        )
    issues = validate_packet_doc(doc, schema)
    return _emit_result(
        kind="packet",
        target_path=path,
        packet_id=doc.data.get("packet_id"),
        issues=issues,
        output_format=args.output_format,
    )


def _resolve_handoff_paths(
    args: argparse.Namespace,
) -> tuple[tuple[Path, Path | None, Path | None, Path | None] | None, str]:
    handoff_path = Path(args.handoff_path)
    if not handoff_path.is_file():
        return None, f"error: handoff file not found: {handoff_path}\n"

    packet_path: Path | None = Path(args.packet) if args.packet else None
    if packet_path is not None and not packet_path.is_file():
        return None, f"error: packet file not found: {packet_path}\n"

    screener_report_path: Path | None = Path(args.screener_report) if args.screener_report else None
    if screener_report_path is not None and not screener_report_path.is_file():
        return None, f"error: screener report file not found: {screener_report_path}\n"

    repo_root: Path | None = Path(args.repo_root) if args.repo_root else REPO_ROOT_DEFAULT
    if repo_root is not None and not repo_root.is_dir():
        return None, f"error: repo root directory not found: {repo_root}\n"

    if args.base_commit is not None and (
        not args.base_commit.strip() or args.base_commit.startswith("-")
    ):
        return None, f"error: invalid --base-commit revision: {args.base_commit!r}\n"

    return (handoff_path, packet_path, screener_report_path, repo_root), ""


def _cmd_validate_handoff(args: argparse.Namespace) -> int:
    resolved, err_msg = _resolve_handoff_paths(args)
    if resolved is None:
        sys.stderr.write(err_msg)
        return EXIT_USAGE
    handoff_path, packet_path, screener_report_path, repo_root = resolved

    try:
        schema = load_schema()
        handoff_doc = load_document(handoff_path)
        packet_doc = load_document(packet_path) if packet_path is not None else None
        screener_candidates: list[dict[str, Any]] | None = None
        if screener_report_path is not None:
            screener_candidates = _parse_screener_candidates_text(
                screener_report_path.read_text(encoding="utf-8"),
                source=str(screener_report_path),
            )
        elif args.base_commit is not None and repo_root is not None:
            screener_candidates = _run_behavior_screener_for_base(repo_root, args.base_commit)
    except OSError as exc:
        sys.stderr.write(f"error: unable to read file: {exc}\n")
        return EXIT_USAGE
    except DocumentParseError as exc:
        return _emit_result(
            kind="handoff",
            target_path=handoff_path,
            packet_id=None,
            issues=[ValidationIssue("PARSE_ERROR", "$", str(exc))],
            output_format=args.output_format,
        )

    issues = validate_handoff_doc(
        handoff_doc,
        schema,
        packet_doc=packet_doc,
        screener_candidates=screener_candidates,
        repo_root=repo_root,
    )
    if args.base_commit and args.base_commit != "HEAD":
        allow_generated = (
            packet_doc is not None
            and isinstance(packet_doc.data.get("scope"), dict)
            and packet_doc.data["scope"].get("allow_generated_code_edits") is True
        )
        base_gen_issues: list[ValidationIssue] = []
        _validate_generated_code_edits(
            handoff_doc.data,
            repo_root=repo_root,
            allow_generated=allow_generated,
            issues=base_gen_issues,
            base_commit=args.base_commit,
        )
        existing_gen_paths = {
            item.path for item in issues if item.code == "GENERATED_CODE_MODIFIED"
        }
        issues.extend(item for item in base_gen_issues if item.path not in existing_gen_paths)
    return _emit_result(
        kind="handoff",
        target_path=handoff_path,
        packet_id=handoff_doc.data.get("packet_id"),
        issues=issues,
        output_format=args.output_format,
    )


# ---------------------------------------------------------------------------
# LOOP-02 (measure), LOOP-01 (verify-handoff), VERIFY-01 (verify-sequence)
# ---------------------------------------------------------------------------

_SCREENER_MOD_CACHE: dict[str, ModuleType] = {}


def _load_behavior_screener() -> ModuleType:
    cached = _SCREENER_MOD_CACHE.get("mod")
    if cached is not None:
        return cached
    if not SCREENER_SCRIPT_PATH.is_file():
        msg = f"behavior screener script not found: {SCREENER_SCRIPT_PATH}"
        raise DocumentParseError(msg)
    spec = importlib.util.spec_from_file_location("alt_behavior_screener", SCREENER_SCRIPT_PATH)
    if spec is None or spec.loader is None:
        msg = f"unable to load behavior screener module from {SCREENER_SCRIPT_PATH}"
        raise DocumentParseError(msg)
    mod = importlib.util.module_from_spec(spec)
    with unittest.mock.patch.dict(sys.modules, {spec.name: mod}):
        spec.loader.exec_module(mod)
    _SCREENER_MOD_CACHE["mod"] = mod
    return mod


def _load_harness_run() -> ModuleType:
    cached = _SCREENER_MOD_CACHE.get("harness_run")
    if cached is not None:
        return cached
    if not HARNESS_RUN_SCRIPT_PATH.is_file():
        msg = f"harness_run script not found: {HARNESS_RUN_SCRIPT_PATH}"
        raise DocumentParseError(msg)
    spec = importlib.util.spec_from_file_location("alt_harness_run", HARNESS_RUN_SCRIPT_PATH)
    if spec is None or spec.loader is None:
        msg = f"unable to load harness_run module from {HARNESS_RUN_SCRIPT_PATH}"
        raise DocumentParseError(msg)
    mod = importlib.util.module_from_spec(spec)
    with unittest.mock.patch.dict(sys.modules, {spec.name: mod}):
        spec.loader.exec_module(mod)
    _SCREENER_MOD_CACHE["harness_run"] = mod
    return mod


def _require_git_bin() -> str:
    git_bin = shutil.which("git")
    if git_bin is None:
        msg = "git executable not found in PATH"
        raise DocumentParseError(msg)
    return git_bin


def _resolve_git_commit(repo_root: Path, rev: str) -> str | None:
    if not rev or not rev.strip() or rev.startswith("-"):
        return None
    git_bin = _require_git_bin()
    res = _run_git_bytes(
        repo_root, git_bin, "rev-parse", "--verify", "--end-of-options", f"{rev}^{{commit}}"
    )
    if res.returncode != 0:
        return None
    return res.stdout.decode("utf-8", errors="replace").strip()


def _current_head_sha(repo_root: Path) -> str:
    resolved = _resolve_git_commit(repo_root, "HEAD") if (repo_root / ".git").exists() else None
    return resolved or "0000000"


def _current_branch_sanitized(repo_root: Path) -> str:
    hr = _load_harness_run()
    raw_branch = hr.resolve_git_branch(repo_root)
    return str(hr.sanitize_branch_name(raw_branch))


def _yaml_scalar_str(val: object) -> str:
    if isinstance(val, bool):
        return "true" if val else "false"
    if isinstance(val, (int, float)):
        return str(val)
    if val is None:
        return "null"
    return json.dumps(str(val), ensure_ascii=False)


def _format_measured_yaml_lines(indent: str, measured: dict[str, Any]) -> list[str]:
    child = indent + "  "
    lines = [
        f"{indent}measured:",
        f"{child}duration_s: {measured['duration_s']}",
        f"{child}exit_code: {measured['exit_code']}",
        f'{child}at_commit: "{measured["at_commit"]}"',
    ]
    if "command_sha256" in measured:
        lines.append(f'{child}command_sha256: "{measured["command_sha256"]}"')
    return lines


def _format_execution_policy_yaml_lines(
    ep_indent: int,
    policy_data: dict[str, Any],
    measured: dict[str, Any],
) -> list[str]:
    base = " " * ep_indent
    child = " " * (ep_indent + 2)
    lines = [f"{base}execution_policy:"]
    for key, val in policy_data.items():
        if key == "measured":
            continue
        lines.append(f"{child}{key}: {_yaml_scalar_str(val)}")
    lines.extend(_format_measured_yaml_lines(child, measured))
    return lines


def _strip_measured_from_frontmatter(fm_lines: list[str]) -> list[str]:
    ep_idx = next(
        (i for i, line in enumerate(fm_lines) if line.lstrip(" ").startswith("execution_policy:")),
        None,
    )
    if ep_idx is None:
        return fm_lines
    ep_indent = len(fm_lines[ep_idx]) - len(fm_lines[ep_idx].lstrip(" "))
    block_end = ep_idx + 1
    while block_end < len(fm_lines):
        raw = fm_lines[block_end]
        if raw.strip() and (len(raw) - len(raw.lstrip(" "))) <= ep_indent:
            break
        block_end += 1
    cleaned: list[str] = []
    idx = ep_idx + 1
    while idx < block_end:
        raw = fm_lines[idx]
        stripped = raw.lstrip(" ")
        cur_indent = len(raw) - len(stripped)
        if stripped.startswith("measured:") and cur_indent > ep_indent:
            idx += 1
            while idx < block_end:
                nxt = fm_lines[idx]
                if nxt.strip() and (len(nxt) - len(nxt.lstrip(" "))) <= cur_indent:
                    break
                idx += 1
            continue
        cleaned.append(raw)
        idx += 1
    return [*fm_lines[: ep_idx + 1], *cleaned, *fm_lines[block_end:]]


def _update_yaml_frontmatter_measured(
    fm_lines: list[str],
    measured: dict[str, Any],
    policy_data: dict[str, Any],
) -> list[str]:
    ep_idx = next(
        (i for i, line in enumerate(fm_lines) if line.lstrip(" ").startswith("execution_policy:")),
        None,
    )
    if ep_idx is None:
        msg = "execution_policy block not found in YAML frontmatter"
        raise DocumentParseError(msg)

    ep_line = fm_lines[ep_idx]
    ep_indent = len(ep_line) - len(ep_line.lstrip(" "))
    ep_stripped = _strip_yaml_comment(ep_line.lstrip(" "))
    colon_idx = _find_unquoted_colon(ep_stripped)
    inline_val = ep_stripped[colon_idx + 1 :].strip() if colon_idx != -1 else ""
    if inline_val:
        expanded = _format_execution_policy_yaml_lines(ep_indent, policy_data, measured)
        return [*fm_lines[:ep_idx], *expanded, *fm_lines[ep_idx + 1 :]]

    block_end = ep_idx + 1
    while block_end < len(fm_lines):
        raw = fm_lines[block_end]
        if raw.strip() and (len(raw) - len(raw.lstrip(" "))) <= ep_indent:
            break
        block_end += 1

    child_indent = " " * (ep_indent + 2)
    cleaned_ep: list[str] = []
    idx = ep_idx + 1
    while idx < block_end:
        raw = fm_lines[idx]
        stripped = raw.lstrip(" ")
        cur_indent = len(raw) - len(stripped)
        if stripped.startswith("measured:") and cur_indent > ep_indent:
            child_indent = " " * cur_indent
            idx += 1
            while idx < block_end:
                nxt = fm_lines[idx]
                if nxt.strip() and (len(nxt) - len(nxt.lstrip(" "))) <= cur_indent:
                    break
                idx += 1
            continue
        if raw.strip() and cur_indent > ep_indent and not cleaned_ep:
            child_indent = " " * cur_indent
        cleaned_ep.append(raw)
        idx += 1

    measured_lines = _format_measured_yaml_lines(child_indent, measured)
    return [*fm_lines[: ep_idx + 1], *cleaned_ep, *measured_lines, *fm_lines[block_end:]]


def _write_measured_to_packet(path: Path, doc: ParsedDocument, measured: dict[str, Any]) -> None:
    if not doc.is_markdown:
        doc.data["execution_policy"]["measured"] = measured
        path.write_text(json.dumps(doc.data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        return

    raw_text = path.read_text(encoding="utf-8")
    lines = raw_text.splitlines()
    fm_lines, body_lines = _extract_frontmatter(lines)
    if fm_lines is not None and (set(parse_yaml_subset(fm_lines)) & STRUCTURED_PAYLOAD_KEYS):
        policy_data = doc.data.get("execution_policy", {})
        updated_fm = _update_yaml_frontmatter_measured(
            fm_lines, measured, policy_data if isinstance(policy_data, dict) else {}
        )
        rebuilt = ["---", *updated_fm, "---", *body_lines]
        path.write_text("\n".join(rebuilt) + "\n", encoding="utf-8")
        return

    doc.data["execution_policy"]["measured"] = measured
    replacement_json = json.dumps(doc.data, ensure_ascii=False, indent=2)
    updated = re.sub(
        r"```json\s*\n.*?\n```",
        lambda _m: f"```json\n{replacement_json}\n```",
        raw_text,
        count=1,
        flags=re.DOTALL,
    )
    path.write_text(updated, encoding="utf-8")


def _resolve_measure_inputs(
    args: argparse.Namespace,
) -> tuple[tuple[Path, Path, ParsedDocument] | None, str]:
    packet_path = Path(args.packet_path)
    if not packet_path.is_file():
        return None, f"error: packet file not found: {packet_path}\n"
    repo_root = Path(args.repo_root).resolve() if args.repo_root else REPO_ROOT_DEFAULT
    if not repo_root.is_dir():
        return None, f"error: repo root directory not found: {repo_root}\n"
    try:
        doc = load_document(packet_path)
    except (OSError, DocumentParseError) as exc:
        return None, f"error: unable to load packet {packet_path}: {exc}\n"
    return (packet_path, repo_root, doc), ""


def _cmd_measure(args: argparse.Namespace) -> int:
    resolved, err_msg = _resolve_measure_inputs(args)
    if resolved is None:
        sys.stderr.write(err_msg)
        return EXIT_USAGE
    packet_path, repo_root, doc = resolved

    policy = doc.data.get("execution_policy")
    verify_cmd = policy.get("verification_command") if isinstance(policy, dict) else None
    pre_issues: list[ValidationIssue] = []
    if not isinstance(policy, dict) or not isinstance(verify_cmd, str) or not verify_cmd.strip():
        pre_issues.append(
            ValidationIssue(
                "MISSING_VERIFICATION_COMMAND",
                "$.execution_policy.verification_command",
                "packet missing non-empty execution_policy.verification_command",
            )
        )
        return _emit_result(
            kind="measure",
            target_path=packet_path,
            packet_id=doc.data.get("packet_id"),
            issues=pre_issues,
            output_format=args.output_format,
        )

    _check_verification_command(
        verify_cmd,
        json_path="$.execution_policy.verification_command",
        expected_targets=[],
        forbid_repo_wide=policy.get("forbid_repo_wide_test_build") is True,
        issues=pre_issues,
    )
    if pre_issues:
        return _emit_result(
            kind="measure",
            target_path=packet_path,
            packet_id=doc.data.get("packet_id"),
            issues=pre_issues,
            output_format=args.output_format,
        )

    budget = _resolve_command_budget(policy)
    try:
        exit_code, duration_s, _, _ = _run_bash_command(
            verify_cmd,
            cwd=repo_root,
            timeout=budget + MEASURE_TIMEOUT_SLACK_S,
        )
        measured: dict[str, Any] = {
            "duration_s": duration_s,
            "exit_code": exit_code,
            "at_commit": _current_head_sha(repo_root),
            "command_sha256": _command_sha256(verify_cmd),
        }
        _write_measured_to_packet(packet_path, doc, measured)
    except (OSError, DocumentParseError) as exc:
        sys.stderr.write(f"error: measure failed: {exc}\n")
        return EXIT_USAGE

    issues: list[ValidationIssue] = []
    if duration_s > budget:
        issues.append(
            ValidationIssue(
                "VERIFICATION_BUDGET_EXCEEDED",
                "$.execution_policy.measured.duration_s",
                f"verification_command took {duration_s}s, exceeding max_command_budget_s={budget}s",
            )
        )
    return _emit_result(
        kind="measure",
        target_path=packet_path,
        packet_id=doc.data.get("packet_id"),
        issues=issues,
        output_format=args.output_format,
    )


def _git_actual_modified_files(repo_root: Path, resolved_base: str) -> set[str]:
    git_bin = _require_git_bin()
    diff_res = _run_git_bytes(
        repo_root, git_bin, "diff", "--name-only", "--no-renames", "-z", resolved_base, "--"
    )
    if diff_res.returncode != 0:
        err = diff_res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"git diff failed against {resolved_base}: {err}"
        raise DocumentParseError(msg)
    cached_res = _run_git_bytes(
        repo_root,
        git_bin,
        "diff",
        "--cached",
        "--name-only",
        "--no-renames",
        "-z",
        resolved_base,
        "--",
    )
    if cached_res.returncode != 0:
        err = cached_res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"git diff --cached failed against {resolved_base}: {err}"
        raise DocumentParseError(msg)
    untracked_res = _run_git_bytes(
        repo_root, git_bin, "ls-files", "--others", "--exclude-standard", "-z"
    )
    if untracked_res.returncode != 0:
        err = untracked_res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"git ls-files failed: {err}"
        raise DocumentParseError(msg)

    paths: set[str] = set()
    for raw_stdout in (diff_res.stdout, cached_res.stdout, untracked_res.stdout):
        for entry in raw_stdout.split(b"\x00"):
            if not entry:
                continue
            rel = _normalize_repo_rel_path(entry.decode("utf-8", errors="surrogateescape"))
            if not rel or rel == ".harness" or rel.startswith(HARNESS_STATE_PREFIX):
                continue
            paths.add(rel)
    return paths


def _git_file_text_at_rev(repo_root: Path, rev: str, rel: str) -> str:
    git_bin = _require_git_bin()
    res = _run_git_bytes(repo_root, git_bin, "show", "--end-of-options", f"{rev}:{rel}")
    if res.returncode != 0:
        return ""
    return res.stdout.decode("utf-8", errors="replace")


def _read_worktree_text(repo_root: Path, rel: str) -> str:
    full = repo_root / rel
    if not full.is_file() or full.is_symlink():
        return ""
    if not full.resolve().is_relative_to(repo_root.resolve()):
        return ""
    return full.read_text(encoding="utf-8", errors="replace")


def _py_func_fingerprint(fn: ast.FunctionDef | ast.AsyncFunctionDef) -> str:
    filtered_body = [
        stmt
        for stmt in fn.body
        if not isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
    ]
    parts = [
        type(fn).__name__,
        fn.name,
        ast.dump(fn.args, include_attributes=False),
        ast.dump(fn.returns, include_attributes=False) if fn.returns is not None else "",
        *(ast.dump(dec, include_attributes=False) for dec in fn.decorator_list),
        *(ast.dump(stmt, include_attributes=False) for stmt in filtered_body),
    ]
    return hashlib.sha256("\n".join(parts).encode("utf-8")).hexdigest()


def _extract_python_func_bodies(source: str, rel: str) -> dict[str, str]:
    if not source.strip():
        return {}
    try:
        tree = ast.parse(source)
    except SyntaxError as exc:
        msg = f"failed to parse Python file {rel}: {exc}"
        raise DocumentParseError(msg) from exc

    funcs: dict[str, str] = {}
    counts: Counter[str] = Counter()

    def walk(stmts: list[ast.stmt], prefix: str) -> None:
        for stmt in stmts:
            if isinstance(stmt, ast.ClassDef):
                cls_prefix = f"{prefix}.{stmt.name}" if prefix else stmt.name
                walk(stmt.body, cls_prefix)
            elif isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef)):
                base = f"{prefix}.{stmt.name}" if prefix else stmt.name
                counts[base] += 1
                key = f"{base}#{counts[base]}" if counts[base] > 1 else base
                funcs[key] = _py_func_fingerprint(stmt)
                walk(stmt.body, base)

    walk(tree.body, "")
    return funcs


def _mask_span_preserve_newlines(segment: str) -> str:
    return "".join("\n" if ch == "\n" else " " for ch in segment)


def _scan_block_comment_end(src: str, start: int, *, nested: bool) -> int:
    depth = 1
    idx = start + 2
    n = len(src)
    while idx < n and depth > 0:
        if nested and src[idx : idx + 2] == "/*":
            depth += 1
            idx += 2
        elif src[idx : idx + 2] == "*/":
            depth -= 1
            idx += 2
        else:
            idx += 1
    return idx


def _mask_comments_and_strings(source: str, language: str) -> tuple[str, str]:
    """Return same-length (masked_all, comments_only_masked) views of C-style source."""
    screener = _load_behavior_screener()
    masked_all: list[str] = []
    comments_masked: list[str] = []
    n = len(source)
    i = 0
    rust_nested = language == "rust"
    while i < n:
        str_end = screener._scan_string_literal_end(source, i, language)
        if str_end is not None:
            seg = source[i:str_end]
            masked_all.append(_mask_span_preserve_newlines(seg))
            comments_masked.append(seg)
            i = str_end
            continue
        ch = source[i]
        nxt = source[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            nl = source.find("\n", i + 2)
            end = n if nl == -1 else nl
            blank = " " * (end - i)
            masked_all.append(blank)
            comments_masked.append(blank)
            i = end
        elif ch == "/" and nxt == "*":
            end = _scan_block_comment_end(source, i, nested=rust_nested)
            blank = _mask_span_preserve_newlines(source[i:end])
            masked_all.append(blank)
            comments_masked.append(blank)
            i = end
        else:
            masked_all.append(ch)
            comments_masked.append(ch)
            i += 1
    return "".join(masked_all), "".join(comments_masked)


def _find_masked_brace_close(masked: str, open_idx: int) -> int:
    depth = 1
    idx = open_idx + 1
    n = len(masked)
    while idx < n and depth > 0:
        ch = masked[idx]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
        idx += 1
    return idx


def _find_masked_paren_close(masked: str, open_idx: int) -> int:
    depth = 1
    idx = open_idx + 1
    n = len(masked)
    while idx < n and depth > 0:
        ch = masked[idx]
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        idx += 1
    return idx


def _find_body_brace_after_params(masked: str, open_paren_idx: int, language: str) -> int | None:
    pos = _find_masked_paren_close(masked, open_paren_idx)
    n = len(masked)
    paren_depth = 0
    while pos < n:
        ch = masked[pos]
        if ch == "(":
            paren_depth += 1
        elif ch == ")":
            paren_depth = max(0, paren_depth - 1)
        elif paren_depth == 0:
            if ch == "{":
                return pos
            if ch == ";" and language != "go":
                return None
            if ch == "\n" and language == "go" and pos + 1 < n:
                nxt_line = masked[pos + 1 :].lstrip(" \t")
                if nxt_line.startswith(("func ", "type ", "var ", "const ", "package ", "import ")):
                    return None
        pos += 1
    return None


def _normalize_go_receiver_type(raw_recv: str) -> str:
    tokens = raw_recv.strip().split()
    if not tokens:
        return ""
    type_tok = tokens[-1]
    while type_tok.startswith("(") and type_tok.endswith(")"):
        type_tok = type_tok[1:-1].strip()
    type_tok = type_tok.lstrip("*").strip()
    while type_tok.startswith("(") and type_tok.endswith(")"):
        type_tok = type_tok[1:-1].strip()
    return type_tok


def _extract_go_func_bodies(source: str) -> dict[str, str]:
    if not source.strip():
        return {}
    masked, keep_str = _mask_comments_and_strings(source, "go")
    funcs: dict[str, str] = {}
    counts: Counter[str] = Counter()
    pos = 0
    n = len(masked)
    while pos < n:
        match = GO_FUNC_DECL_RE.search(masked, pos)
        if match is None:
            break
        prefix_depth = masked[: match.start()].count("{") - masked[: match.start()].count("}")
        if prefix_depth != 0:
            pos = match.end()
            continue
        body_open = _find_body_brace_after_params(masked, match.end() - 1, "go")
        if body_open is None:
            pos = match.end()
            continue
        body_close = _find_masked_brace_close(masked, body_open)
        recv_type = _normalize_go_receiver_type(match.group(1) or "")
        name = match.group(2)
        base = f"({recv_type}).{name}" if recv_type else name
        counts[base] += 1
        key = f"{base}#{counts[base]}" if counts[base] > 1 else base
        normalized = " ".join(keep_str[match.start() : body_close].split())
        funcs[key] = hashlib.sha256(normalized.encode("utf-8")).hexdigest()
        pos = max(body_close, match.end())
    return funcs


def _extract_rust_impl_ranges(masked: str) -> list[tuple[int, int, str]]:
    ranges: list[tuple[int, int, str]] = []
    for match in RUST_IMPL_DECL_RE.finditer(masked):
        open_idx = match.end() - 1
        close_idx = _find_masked_brace_close(masked, open_idx)
        target = " ".join(match.group(1).split())
        if " where " in target:
            target = target.split(" where ", 1)[0].strip()
        ranges.append((open_idx, close_idx, target))
    return ranges


def _rust_fn_decl_start(masked: str, fn_start: int) -> int:
    line_start = masked.rfind("\n", 0, fn_start) + 1
    prefix = masked[line_start:fn_start]
    pre_match = RUST_PRE_FN_RE.search(prefix)
    return line_start + pre_match.start() if pre_match is not None else fn_start


def _extract_rust_func_bodies(source: str) -> dict[str, str]:
    if not source.strip():
        return {}
    screener = _load_behavior_screener()
    masked, keep_str = _mask_comments_and_strings(source, "rust")
    impl_ranges = _extract_rust_impl_ranges(masked)
    funcs: dict[str, str] = {}
    counts: Counter[str] = Counter()
    for match in screener.RUST_FN_DECL_RE.finditer(masked):
        body_open = screener._find_rust_fn_body_open(masked, match.end())
        if body_open is None:
            continue
        body_close = _find_masked_brace_close(masked, body_open)
        fn_name = match.group(1)
        enclosing = [target for start, end, target in impl_ranges if start < match.start() < end]
        base = f"{enclosing[-1]}::{fn_name}" if enclosing else fn_name
        counts[base] += 1
        key = f"{base}#{counts[base]}" if counts[base] > 1 else base
        decl_start = _rust_fn_decl_start(masked, match.start())
        normalized = " ".join(keep_str[decl_start:body_close].split())
        funcs[key] = hashlib.sha256(normalized.encode("utf-8")).hexdigest()
    return funcs


def _extract_svelte_scripts(source: str) -> str:
    blocks = [match.group(1) for match in SVELTE_SCRIPT_RE.finditer(source)]
    return "\n".join(blocks)


def _record_ts_func(
    funcs: dict[str, str],
    counts: Counter[str],
    base: str,
    span_text: str,
) -> None:
    counts[base] += 1
    key = f"{base}#{counts[base]}" if counts[base] > 1 else base
    normalized = " ".join(span_text.split())
    funcs[key] = hashlib.sha256(normalized.encode("utf-8")).hexdigest()


def _extract_ts_func_bodies(source: str, *, is_svelte: bool = False) -> dict[str, str]:
    effective = _extract_svelte_scripts(source) if is_svelte else source
    if not effective.strip():
        return {}
    masked, keep_str = _mask_comments_and_strings(effective, "typescript")
    funcs: dict[str, str] = {}
    counts: Counter[str] = Counter()
    seen_spans: set[tuple[int, int]] = set()

    class_ranges: list[tuple[int, int, str]] = []
    for cls_m in TS_CLASS_DECL_RE.finditer(masked):
        c_open = cls_m.end() - 1
        c_close = _find_masked_brace_close(masked, c_open)
        class_ranges.append((c_open, c_close, cls_m.group(1)))

    for match in TS_FUNC_DECL_RE.finditer(masked):
        body_open = _find_body_brace_after_params(masked, match.end() - 1, "typescript")
        if body_open is None:
            continue
        body_close = _find_masked_brace_close(masked, body_open)
        seen_spans.add((body_open, body_close))
        _record_ts_func(funcs, counts, match.group(1), keep_str[match.start() : body_close])

    for match in TS_ARROW_DECL_RE.finditer(masked):
        tail_pos = match.end()
        while tail_pos < len(masked) and masked[tail_pos].isspace():
            tail_pos += 1
        if tail_pos < len(masked) and masked[tail_pos] == "{":
            body_close = _find_masked_brace_close(masked, tail_pos)
            seen_spans.add((tail_pos, body_close))
        else:
            semi = masked.find(";", tail_pos)
            nl = masked.find("\n", tail_pos)
            ends = [idx for idx in (semi, nl) if idx != -1]
            body_close = min(ends) if ends else len(masked)
        _record_ts_func(funcs, counts, match.group(1), keep_str[match.start() : body_close])

    for match in TS_METHOD_DECL_RE.finditer(masked):
        name = match.group(1)
        if name in TS_CONTROL_KEYWORDS:
            continue
        body_open = match.end() - 1
        body_close = _find_masked_brace_close(masked, body_open)
        if (body_open, body_close) in seen_spans:
            continue
        enclosing_cls = [
            cname for c_open, c_close, cname in class_ranges if c_open < match.start() < c_close
        ]
        base = f"{enclosing_cls[-1]}.{name}" if enclosing_cls else name
        seen_spans.add((body_open, body_close))
        _record_ts_func(funcs, counts, base, keep_str[match.start() : body_close])
    return funcs


def _count_modified_functions_in_file(rel: str, before_text: str, after_text: str) -> int:
    screener = _load_behavior_screener()
    lang = screener.infer_language(rel)
    if lang == "python":
        b_map = _extract_python_func_bodies(before_text, rel)
        a_map = _extract_python_func_bodies(after_text, rel)
    elif lang == "go":
        b_map = _extract_go_func_bodies(before_text)
        a_map = _extract_go_func_bodies(after_text)
    elif lang == "rust":
        b_map = _extract_rust_func_bodies(before_text)
        a_map = _extract_rust_func_bodies(after_text)
    elif lang == "typescript":
        is_svelte = rel.lower().endswith(".svelte")
        b_map = _extract_ts_func_bodies(before_text, is_svelte=is_svelte)
        a_map = _extract_ts_func_bodies(after_text, is_svelte=is_svelte)
    else:
        return 0

    changed = set(b_map) ^ set(a_map)
    for key in set(b_map) & set(a_map):
        if b_map[key] != a_map[key]:
            changed.add(key)
    return len(changed)


def _count_actual_modified_functions(
    repo_root: Path,
    resolved_base: str,
    actual_files: set[str],
) -> int:
    total = 0
    for rel in sorted(actual_files):
        before_text = _git_file_text_at_rev(repo_root, resolved_base, rel)
        after_text = _read_worktree_text(repo_root, rel)
        if before_text == after_text:
            continue
        total += _count_modified_functions_in_file(rel, before_text, after_text)
    return total


@dataclass(frozen=True)
class _VerifyDiffContext:
    repo_root: Path
    resolved_base: str
    actual_files: set[str]


def _verify_handoff_actual_files(
    handoff: dict[str, Any],
    packet: dict[str, Any],
    repo_root: Path,
    resolved_base: str,
    issues: list[ValidationIssue],
) -> set[str]:
    actual_files = _git_actual_modified_files(repo_root, resolved_base)
    declared_raw = handoff.get("modified_files", [])
    declared_files: set[str] = {
        _normalize_repo_rel_path(entry["path"])
        for entry in declared_raw
        if isinstance(entry, dict) and isinstance(entry.get("path"), str)
    }

    issues.extend(
        ValidationIssue(
            "UNDECLARED_MODIFIED_FILE",
            "$.modified_files",
            f"git worktree modified/added '{rel}' since {resolved_base[:12]} "
            "but it is missing from handoff.modified_files",
        )
        for rel in sorted(actual_files - declared_files)
    )
    issues.extend(
        ValidationIssue(
            "PHANTOM_MODIFIED_FILE",
            "$.modified_files",
            f"handoff declares modified file '{rel}' with no diff against {resolved_base[:12]}",
        )
        for rel in sorted(declared_files - actual_files)
    )

    scope = packet.get("scope")
    if not isinstance(scope, dict):
        return actual_files

    max_files = scope.get("max_files")
    if (
        isinstance(max_files, int)
        and not isinstance(max_files, bool)
        and len(actual_files) > max_files
        and not any(item.code == "MAX_FILES_EXCEEDED" for item in issues)
    ):
        issues.append(
            ValidationIssue(
                "MAX_FILES_EXCEEDED",
                "$.modified_files",
                f"actual git modified files count {len(actual_files)} exceeds packet max_files={max_files}",
            )
        )

    allowed_paths: list[str] = scope.get("allowed_paths", [])
    forbidden_paths: list[str] = scope.get("forbidden_paths", [])
    allow_generated = scope.get("allow_generated_code_edits") is True
    for rel in sorted(actual_files - declared_files):
        if allowed_paths and not _matches_any_glob(rel, allowed_paths):
            issues.append(
                ValidationIssue(
                    "PATH_OUTSIDE_ALLOWED_PATHS",
                    f"git:{rel}",
                    f"actual modified file '{rel}' is outside packet scope.allowed_paths {allowed_paths!r}",
                )
            )
        if forbidden_paths and _matches_any_glob(rel, forbidden_paths):
            issues.append(
                ValidationIssue(
                    "PATH_IN_FORBIDDEN_PATHS",
                    f"git:{rel}",
                    f"actual modified file '{rel}' matches packet scope.forbidden_paths {forbidden_paths!r}",
                )
            )
        if not allow_generated and _repo_rel_is_generated_code(
            repo_root, rel, base_commit=resolved_base
        ):
            issues.append(
                ValidationIssue(
                    "GENERATED_CODE_MODIFIED",
                    f"git:{rel}",
                    f"actual modified file '{rel}' carries a generated-code DO NOT EDIT header",
                )
            )
    return actual_files


def _verify_handoff_ast_functions(
    handoff: dict[str, Any],
    packet: dict[str, Any],
    diff_ctx: _VerifyDiffContext,
    issues: list[ValidationIssue],
) -> None:
    actual_count = _count_actual_modified_functions(
        diff_ctx.repo_root, diff_ctx.resolved_base, diff_ctx.actual_files
    )
    declared_count = handoff.get("modified_functions_count")
    if (
        isinstance(declared_count, int)
        and not isinstance(declared_count, bool)
        and declared_count != actual_count
    ):
        issues.append(
            ValidationIssue(
                "MODIFIED_FUNCTIONS_MISMATCH",
                "$.modified_functions_count",
                f"handoff declares modified_functions_count={declared_count}, "
                f"but AST diff found {actual_count} modified function(s)",
            )
        )
    scope = packet.get("scope")
    max_functions = scope.get("max_functions") if isinstance(scope, dict) else None
    if (
        isinstance(max_functions, int)
        and not isinstance(max_functions, bool)
        and actual_count > max_functions
        and not any(item.code == "MAX_FUNCTIONS_EXCEEDED" for item in issues)
    ):
        issues.append(
            ValidationIssue(
                "MAX_FUNCTIONS_EXCEEDED",
                "$.modified_functions_count",
                f"actual AST modified functions count {actual_count} exceeds packet max_functions={max_functions}",
            )
        )


@dataclass(frozen=True)
class _VerifyRerunContext:
    repo_root: Path
    runs_dir: Path
    resolved_base: str


def _safe_evidence_path(runs_dir: Path, branch_dir: str, packet_id: str) -> Path | None:
    if not PACKET_ID_RE.match(packet_id):
        return None
    runs_resolved = runs_dir.resolve()
    raw_dir = runs_dir / branch_dir / "verify"
    raw_path = raw_dir / f"{packet_id}.json"
    cur = runs_dir
    for part in (branch_dir, "verify", f"{packet_id}.json"):
        cur = cur / part
        if cur.is_symlink():
            return None
    if not raw_path.resolve().is_relative_to(runs_resolved):
        return None
    return raw_path


def _verify_handoff_host_rerun(
    handoff: dict[str, Any],
    packet: dict[str, Any],
    ctx: _VerifyRerunContext,
    issues: list[ValidationIssue],
) -> None:
    if any(item.code.startswith("PACKET_") for item in issues):
        return
    policy = packet.get("execution_policy")
    if not isinstance(policy, dict):
        return
    verify_cmd = policy.get("verification_command")
    if not isinstance(verify_cmd, str) or not verify_cmd.strip():
        return
    if _match_forbidden_autofix(verify_cmd) or _has_background_ampersand(verify_cmd):
        return

    raw_packet_id = packet.get("packet_id") or handoff.get("packet_id")
    if not isinstance(raw_packet_id, str) or not PACKET_ID_RE.match(raw_packet_id):
        issues.append(
            ValidationIssue(
                "INVALID_PACKET_ID",
                "$.packet_id",
                f"refusing to write rerun evidence for invalid packet_id {raw_packet_id!r}",
            )
        )
        return

    branch_dir = _current_branch_sanitized(ctx.repo_root)
    evidence_path = _safe_evidence_path(ctx.runs_dir, branch_dir, raw_packet_id)
    if evidence_path is None:
        issues.append(
            ValidationIssue(
                "UNSAFE_EVIDENCE_PATH",
                "$.packet_id",
                f"evidence path for packet_id={raw_packet_id!r} escapes runs_dir or hits symlink",
            )
        )
        return

    budget = _resolve_command_budget(policy)
    host_rc, duration_s, out_bytes, err_bytes = _run_bash_command(
        verify_cmd,
        cwd=ctx.repo_root,
        timeout=budget + MEASURE_TIMEOUT_SLACK_S,
    )
    output_sha = hashlib.sha256(out_bytes + b"\n" + err_bytes).hexdigest()
    evidence_path.parent.mkdir(parents=True, exist_ok=True)
    if evidence_path.is_symlink() or not evidence_path.resolve().is_relative_to(
        ctx.runs_dir.resolve()
    ):
        msg = f"evidence path escapes runs_dir: {evidence_path}"
        raise DocumentParseError(msg)
    evidence_payload = {
        "packet_id": raw_packet_id,
        "command": verify_cmd,
        "exit_code": host_rc,
        "duration_s": duration_s,
        "output_sha256": output_sha,
        "base_commit": ctx.resolved_base,
        "head_commit": _current_head_sha(ctx.repo_root),
    }
    evidence_path.write_text(
        json.dumps(evidence_payload, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )

    verification = handoff.get("verification")
    claimed_rc = verification.get("exit_code") if isinstance(verification, dict) else None
    if isinstance(claimed_rc, int) and not isinstance(claimed_rc, bool) and host_rc != claimed_rc:
        issues.append(
            ValidationIssue(
                "VERIFICATION_EXIT_CODE_MISMATCH",
                "$.verification.exit_code",
                f"host re-execution exited with {host_rc}, but handoff claimed exit_code={claimed_rc}",
            )
        )
    if handoff.get("status") == "completed" and host_rc != 0:
        issues.append(
            ValidationIssue(
                "VERIFICATION_RERUN_FAILED",
                "$.verification.exit_code",
                f"host re-execution of verification_command failed with exit_code={host_rc} "
                "while handoff status='completed'",
            )
        )


def _extract_python_comments(source: str) -> Counter[str]:
    if not source.strip():
        return Counter()
    counts: Counter[str] = Counter()
    try:
        for tok in tokenize.generate_tokens(io.StringIO(source).readline):
            if tok.type == tokenize.COMMENT:
                text = tok.string.strip()
                if text:
                    counts[text] += 1
    except (tokenize.TokenError, IndentationError):
        return Counter()
    return counts


def _extract_c_style_comments(source: str, language: str) -> Counter[str]:
    if not source.strip():
        return Counter()
    screener = _load_behavior_screener()
    counts: Counter[str] = Counter()
    n = len(source)
    i = 0
    rust_nested = language == "rust"
    while i < n:
        str_end = screener._scan_string_literal_end(source, i, language)
        if str_end is not None:
            i = str_end
            continue
        ch = source[i]
        nxt = source[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            nl = source.find("\n", i + 2)
            end = n if nl == -1 else nl
            text = source[i:end].strip()
            if text:
                counts[text] += 1
            i = end
        elif ch == "/" and nxt == "*":
            end = _scan_block_comment_end(source, i, nested=rust_nested)
            text = source[i:end].strip()
            if text:
                counts[text] += 1
            i = end
        else:
            i += 1
    return counts


def _extract_shell_comments(source: str) -> Counter[str]:
    counts: Counter[str] = Counter()
    for line in source.splitlines():
        in_quote: str | None = None
        escaped = False
        for idx, ch in enumerate(line):
            if escaped:
                escaped = False
                continue
            if in_quote == '"' and ch == "\\":
                escaped = True
                continue
            if in_quote is not None:
                if ch == in_quote:
                    in_quote = None
                continue
            if ch in ('"', "'"):
                in_quote = ch
            elif ch == "#" and (idx == 0 or line[idx - 1].isspace()):
                text = line[idx:].strip()
                if text:
                    counts[text] += 1
                break
    return counts


def _extract_code_comments(rel: str, source: str) -> Counter[str]:
    suffix = Path(rel).suffix.lower()
    if suffix == ".py":
        return _extract_python_comments(source)
    if suffix in C_STYLE_COMMENT_EXTS:
        effective = _extract_svelte_scripts(source) if suffix == ".svelte" else source
        lang = "go" if suffix == ".go" else ("rust" if suffix == ".rs" else "typescript")
        return _extract_c_style_comments(effective, lang)
    if suffix in SHELL_COMMENT_EXTS:
        return _extract_shell_comments(source)
    return Counter()


def _verify_handoff_lint_fix_extras(
    packet: dict[str, Any],
    diff_ctx: _VerifyDiffContext,
    screener_candidates: list[dict[str, Any]],
    issues: list[ValidationIssue],
) -> None:
    task_kind = packet.get("task_kind", "lint-fix")
    if task_kind != "lint-fix":
        return

    for rel in sorted(diff_ctx.actual_files):
        if SEQUENCE_TEST_FILE_RE.search(rel) is not None:
            continue
        before_text = _git_file_text_at_rev(diff_ctx.repo_root, diff_ctx.resolved_base, rel)
        after_text = _read_worktree_text(diff_ctx.repo_root, rel)
        added_comments = _extract_code_comments(rel, after_text) - _extract_code_comments(
            rel, before_text
        )
        for comment_text in sorted(added_comments):
            if ALLOWED_LINT_FIX_COMMENT_RE.search(comment_text) is not None:
                continue
            issues.append(
                ValidationIssue(
                    "UNREQUESTED_COMMENT_ADDED",
                    f"diff:{rel}",
                    f"lint-fix task added unrequested comment: {comment_text[:80]!r}",
                )
            )
            break

    scope = packet.get("scope")
    allow_test_changes = (
        isinstance(scope, dict) and scope.get("allow_test_expectation_changes") is True
    )
    if not allow_test_changes:
        for cand in screener_candidates:
            kind = cand.get("kind")
            if kind in ("test.expectation_change", "test.file_deleted"):
                issues.append(
                    ValidationIssue(
                        "FORBIDDEN_TEST_EXPECTATION_CHANGE",
                        f"diff:{cand.get('file')}",
                        f"lint-fix task forbids {kind} in '{cand.get('file')}' unless "
                        "scope.allow_test_expectation_changes=true",
                    )
                )


def _resolve_verify_handoff_paths(
    args: argparse.Namespace,
) -> tuple[tuple[Path, Path, Path] | None, str]:
    handoff_path = Path(args.handoff_path)
    packet_path = Path(args.packet)
    repo_root = Path(args.repo_root).resolve() if args.repo_root else REPO_ROOT_DEFAULT
    if not handoff_path.is_file():
        return None, f"error: handoff file not found: {handoff_path}\n"
    if not packet_path.is_file():
        return None, f"error: packet file not found: {packet_path}\n"
    if not repo_root.is_dir():
        return None, f"error: repo root directory not found: {repo_root}\n"
    if args.base_commit is not None and (
        not args.base_commit.strip() or args.base_commit.startswith("-")
    ):
        return None, f"error: invalid --base-commit revision: {args.base_commit!r}\n"
    return (handoff_path, packet_path, repo_root), ""


def _resolve_verify_base(
    args: argparse.Namespace,
    packet_doc: ParsedDocument,
    repo_root: Path,
) -> tuple[str | None, ValidationIssue | None]:
    raw_base = args.base_commit or packet_doc.data.get("base_commit")
    if not isinstance(raw_base, str) or not raw_base.strip():
        return None, ValidationIssue(
            "MISSING_BASE_COMMIT",
            "$.base_commit",
            "verify-handoff requires --base-commit or packet.base_commit",
        )
    resolved_base = _resolve_git_commit(repo_root, raw_base.strip())
    if resolved_base is None:
        return None, ValidationIssue(
            "INVALID_BASE_COMMIT",
            "$.base_commit",
            f"unable to resolve base_commit {raw_base!r} in {repo_root}",
        )
    return resolved_base, None


def _cmd_verify_handoff(args: argparse.Namespace) -> int:
    resolved_paths, err_msg = _resolve_verify_handoff_paths(args)
    if resolved_paths is None:
        sys.stderr.write(err_msg)
        return EXIT_USAGE
    handoff_path, packet_path, repo_root = resolved_paths

    try:
        schema = load_schema()
        handoff_doc = load_document(handoff_path)
        packet_doc = load_document(packet_path)
    except OSError as exc:
        sys.stderr.write(f"error: unable to read file: {exc}\n")
        return EXIT_USAGE
    except DocumentParseError as exc:
        return _emit_result(
            kind="verify-handoff",
            target_path=handoff_path,
            packet_id=None,
            issues=[ValidationIssue("PARSE_ERROR", "$", str(exc))],
            output_format=args.output_format,
        )

    resolved_base, base_issue = _resolve_verify_base(args, packet_doc, repo_root)
    if resolved_base is None or base_issue is not None:
        return _emit_result(
            kind="verify-handoff",
            target_path=handoff_path,
            packet_id=handoff_doc.data.get("packet_id"),
            issues=[base_issue] if base_issue is not None else [],
            output_format=args.output_format,
        )

    issues: list[ValidationIssue] = []
    try:
        screener_candidates = _run_behavior_screener_for_base(repo_root, resolved_base)
        issues = validate_handoff_doc(
            handoff_doc,
            schema,
            packet_doc=packet_doc,
            screener_candidates=screener_candidates,
            repo_root=repo_root,
        )
        allow_generated = (
            isinstance(packet_doc.data.get("scope"), dict)
            and packet_doc.data["scope"].get("allow_generated_code_edits") is True
        )
        base_gen_issues: list[ValidationIssue] = []
        _validate_generated_code_edits(
            handoff_doc.data,
            repo_root=repo_root,
            allow_generated=allow_generated,
            issues=base_gen_issues,
            base_commit=resolved_base,
        )
        existing_gen_paths = {
            item.path for item in issues if item.code == "GENERATED_CODE_MODIFIED"
        }
        issues.extend(item for item in base_gen_issues if item.path not in existing_gen_paths)
        actual_files = _verify_handoff_actual_files(
            handoff_doc.data, packet_doc.data, repo_root, resolved_base, issues
        )
        diff_ctx = _VerifyDiffContext(
            repo_root=repo_root, resolved_base=resolved_base, actual_files=actual_files
        )
        _verify_handoff_ast_functions(handoff_doc.data, packet_doc.data, diff_ctx, issues)
        if not args.no_rerun:
            runs_dir = (
                Path(args.runs_dir).resolve()
                if args.runs_dir
                else (repo_root / ".harness" / "runs")
            )
            _verify_handoff_host_rerun(
                handoff_doc.data,
                packet_doc.data,
                _VerifyRerunContext(
                    repo_root=repo_root, runs_dir=runs_dir, resolved_base=resolved_base
                ),
                issues,
            )
        _verify_handoff_lint_fix_extras(packet_doc.data, diff_ctx, screener_candidates, issues)
    except (OSError, DocumentParseError) as exc:
        issues.append(ValidationIssue("VERIFY_HANDOFF_ERROR", "$", str(exc)))

    return _emit_result(
        kind="verify-handoff",
        target_path=handoff_path,
        packet_id=handoff_doc.data.get("packet_id"),
        issues=issues,
        output_format=args.output_format,
    )


def _git_commit_files_with_status(repo_root: Path, git_bin: str, rev: str) -> list[tuple[str, str]]:
    res = _run_git_bytes(
        repo_root,
        git_bin,
        "diff-tree",
        "--no-commit-id",
        "--name-status",
        "--no-renames",
        "-r",
        "--root",
        "-z",
        rev,
    )
    if res.returncode != 0:
        err = res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"git diff-tree failed for {rev}: {err}"
        raise DocumentParseError(msg)
    tokens = [tok.decode("utf-8", errors="surrogateescape") for tok in res.stdout.split(b"\x00")]
    entries: list[tuple[str, str]] = []
    idx = 0
    while idx + 1 < len(tokens):
        status = tokens[idx]
        if not status:
            idx += 1
            continue
        rel = _normalize_repo_rel_path(tokens[idx + 1])
        if rel:
            entries.append((status, rel))
        idx += 2
    return entries


def _git_commit_files(repo_root: Path, git_bin: str, rev: str) -> list[str]:
    return [rel for _, rel in _git_commit_files_with_status(repo_root, git_bin, rev)]


def _git_dirty_files(repo_root: Path, git_bin: str) -> set[str]:
    dirty: set[str] = set()
    for diff_args in (
        ("diff", "--name-only", "--no-renames", "-z", "HEAD", "--"),
        ("diff", "--cached", "--name-only", "--no-renames", "-z", "--"),
        ("ls-files", "--others", "--exclude-standard", "-z"),
    ):
        res = _run_git_bytes(repo_root, git_bin, *diff_args)
        if res.returncode == 0:
            for entry in res.stdout.split(b"\x00"):
                if not entry:
                    continue
                rel = _normalize_repo_rel_path(entry.decode("utf-8", errors="surrogateescape"))
                if not rel or rel == ".harness" or rel.startswith(HARNESS_STATE_PREFIX):
                    continue
                dirty.add(rel)
    return dirty


def _git_blob_mode(repo_root: Path, git_bin: str, rev: str, rel: str) -> str:
    res = _run_git_bytes(repo_root, git_bin, "ls-tree", rev, "--", rel)
    if res.returncode != 0:
        return ""
    line = res.stdout.decode("utf-8", errors="replace").strip()
    return line.split(" ", 1)[0] if line else ""


def _overlay_commit_files(
    repo_root: Path,
    git_bin: str,
    rev: str,
    files: list[str],
    wt_dir: Path,
) -> None:
    wt_resolved = wt_dir.resolve()
    for rel in files:
        if not rel or rel.startswith("/") or "\\" in rel or ".." in PurePosixPath(rel).parts:
            msg = f"refusing to overlay unsafe path: {rel!r}"
            raise DocumentParseError(msg)
        cur = wt_dir
        for part in PurePosixPath(rel).parts[:-1]:
            cur = cur / part
            if cur.is_symlink():
                msg = f"refusing to overlay through symlink parent directory: {rel!r}"
                raise DocumentParseError(msg)
        raw_dest = wt_dir / rel
        if raw_dest.is_symlink():
            raw_dest.unlink()
        raw_dest.parent.mkdir(parents=True, exist_ok=True)
        if not raw_dest.parent.resolve().is_relative_to(wt_resolved):
            msg = f"refusing to overlay path outside worktree: {rel}"
            raise DocumentParseError(msg)
        mode = _git_blob_mode(repo_root, git_bin, rev, rel)
        if mode == GIT_SYMLINK_MODE:
            msg = f"refusing to overlay symlink from commit {rev[:12]}: {rel}"
            raise DocumentParseError(msg)
        show_res = _run_git_bytes(repo_root, git_bin, "show", "--end-of-options", f"{rev}:{rel}")
        if show_res.returncode != 0:
            if raw_dest.exists():
                raw_dest.unlink()
            continue
        raw_dest.write_bytes(show_res.stdout)
        if mode == GIT_EXEC_MODE:
            raw_dest.chmod(0o755)


def _run_test_in_detached_worktree(
    repo_root: Path,
    git_bin: str,
    checkout_rev: str,
    test_command: str,
    *,
    overlay: tuple[str, tuple[str, ...]] | None = None,
) -> tuple[int, str]:
    tmp_parent = Path(tempfile.mkdtemp(prefix="alt-verify-seq-"))
    wt_dir = tmp_parent / "wt"
    added = False
    try:
        add_res = _run_git_bytes(
            repo_root, git_bin, "worktree", "add", "--detach", str(wt_dir), checkout_rev
        )
        if add_res.returncode != 0:
            err = add_res.stderr.decode("utf-8", errors="replace").strip()
            msg = f"git worktree add failed for {checkout_rev}: {err}"
            raise DocumentParseError(msg)
        added = True
        if overlay is not None and overlay[1]:
            _overlay_commit_files(repo_root, git_bin, overlay[0], list(overlay[1]), wt_dir)
        rc, _, out_b, err_b = _run_bash_command(
            test_command,
            cwd=wt_dir,
            timeout=DEFAULT_COMMAND_BUDGET_S + MEASURE_TIMEOUT_SLACK_S,
        )
        snippet = (err_b or out_b).decode("utf-8", errors="replace").strip()[-240:]
        return rc, snippet
    finally:
        if added:
            _run_git_bytes(repo_root, git_bin, "worktree", "remove", "--force", str(wt_dir))
            _run_git_bytes(repo_root, git_bin, "worktree", "prune")
        shutil.rmtree(tmp_parent, ignore_errors=True)


def _resolve_sequence_mode(args: argparse.Namespace) -> tuple[bool, str | None]:
    if args.bugfix:
        return False, None
    if args.behavior_preserving:
        return True, None
    if args.packet:
        pkt_doc = load_document(Path(args.packet))
        pkt_id = pkt_doc.data.get("packet_id")
        bp = pkt_doc.data.get("behavior_preserving")
        if isinstance(bp, bool):
            return bp, str(pkt_id) if pkt_id else None
        if pkt_doc.data.get("task_kind") == "bugfix":
            return False, str(pkt_id) if pkt_id else None
        return True, str(pkt_id) if pkt_id else None
    return True, None


def _check_c1_integrity(
    repo_root: Path,
    git_bin: str,
    base_sha: str,
    c1_rev: str,
    issues: list[ValidationIssue],
) -> list[str]:
    c1_entries = _git_commit_files_with_status(repo_root, git_bin, c1_rev)
    c1_files = [rel for _, rel in c1_entries]
    non_test_c1 = [rel for rel in c1_files if SEQUENCE_TEST_FILE_RE.search(rel) is None]
    if not c1_files or non_test_c1:
        issues.append(
            ValidationIssue(
                "SEQUENCE_FIRST_COMMIT_NOT_TEST_ONLY",
                f"commit:{c1_rev[:12]}",
                f"first commit {c1_rev[:12]} must modify only test files "
                f"(files={c1_files!r}, non_test={non_test_c1!r})",
            )
        )
        return c1_files

    deleted_c1 = [rel for status, rel in c1_entries if status == "D"]
    if deleted_c1:
        issues.append(
            ValidationIssue(
                "SEQUENCE_FIRST_COMMIT_DELETED_TEST",
                f"commit:{c1_rev[:12]}",
                f"first commit {c1_rev[:12]} deleted test file(s): {deleted_c1!r}",
            )
        )
    symlink_c1 = [
        rel
        for rel in c1_files
        if _git_blob_mode(repo_root, git_bin, c1_rev, rel) == GIT_SYMLINK_MODE
    ]
    if symlink_c1:
        issues.append(
            ValidationIssue(
                "SEQUENCE_FIRST_COMMIT_SYMLINK",
                f"commit:{c1_rev[:12]}",
                f"first commit {c1_rev[:12]} introduced symlink(s): {symlink_c1!r}",
            )
        )
    screener = _load_behavior_screener()
    for status, rel in c1_entries:
        if status != "M" or _git_blob_mode(repo_root, git_bin, base_sha, rel) == GIT_SYMLINK_MODE:
            continue
        lang = screener.infer_language(rel)
        if lang is None:
            continue
        before_text = _git_file_text_at_rev(repo_root, base_sha, rel)
        after_text = _git_file_text_at_rev(repo_root, c1_rev, rel)
        cands = screener.screen_test_expectations(before_text, after_text, rel, lang)
        issues.extend(
            ValidationIssue(
                "SEQUENCE_FIRST_COMMIT_WEAKENED_TEST",
                f"commit:{c1_rev[:12]}",
                f"first commit {c1_rev[:12]} weakened existing test ({cand.kind}) in '{cand.file}'",
            )
            for cand in cands
        )
    return c1_files


def _check_sequence_commits(
    repo_root: Path,
    git_bin: str,
    base_sha: str,
) -> tuple[list[str], list[str], list[ValidationIssue]]:
    issues: list[ValidationIssue] = []
    dirty_all = sorted(_git_dirty_files(repo_root, git_bin))
    if dirty_all:
        issues.append(
            ValidationIssue(
                "SEQUENCE_DIRTY_WORKTREE",
                "worktree",
                f"repo_root has uncommitted or untracked changes: {dirty_all!r}",
            )
        )
        return [], [], issues

    rev_res = _run_git_bytes(
        repo_root, git_bin, "rev-list", "--reverse", "--no-merges", f"{base_sha}..HEAD"
    )
    if rev_res.returncode != 0:
        err = rev_res.stderr.decode("utf-8", errors="replace").strip()
        msg = f"git rev-list failed: {err}"
        raise DocumentParseError(msg)
    commits = [
        line.strip()
        for line in rev_res.stdout.decode("utf-8", errors="replace").splitlines()
        if line.strip()
    ]
    if len(commits) < SEQUENCE_MIN_COMMITS:
        issues.append(
            ValidationIssue(
                "SEQUENCE_TOO_FEW_COMMITS",
                "$.commits",
                f"verify-sequence requires at least {SEQUENCE_MIN_COMMITS} non-merge commits in "
                f"{base_sha[:12]}..HEAD (found {len(commits)})",
            )
        )
        return commits, [], issues

    c1_files = _check_c1_integrity(repo_root, git_bin, base_sha, commits[0], issues)
    if issues:
        return commits, c1_files, issues

    c1_set = set(c1_files)
    for commit in commits[1:]:
        mutated = sorted(c1_set & set(_git_commit_files(repo_root, git_bin, commit)))
        if mutated:
            issues.append(
                ValidationIssue(
                    "SEQUENCE_TEST_MUTATED_IN_IMPL",
                    f"commit:{commit[:12]}",
                    f"implementation commit {commit[:12]} mutated commit 1 test file(s): {mutated!r}",
                )
            )
    return commits, c1_files, issues


def _validate_verify_sequence_args(args: argparse.Namespace) -> tuple[Path | None, str]:
    repo_root = Path(args.repo_root).resolve() if args.repo_root else REPO_ROOT_DEFAULT
    if not repo_root.is_dir():
        return None, f"error: repo root directory not found: {repo_root}\n"
    if not args.base_commit or not args.base_commit.strip() or args.base_commit.startswith("-"):
        return None, f"error: invalid --base-commit revision: {args.base_commit!r}\n"
    if not args.test_command or not args.test_command.strip():
        return None, "error: --test-command must be non-empty\n"
    if args.packet and not Path(args.packet).is_file():
        return None, f"error: packet file not found: {args.packet}\n"
    return repo_root, ""


def _cmd_verify_sequence(args: argparse.Namespace) -> int:
    repo_root, err_msg = _validate_verify_sequence_args(args)
    if repo_root is None:
        sys.stderr.write(err_msg)
        return EXIT_USAGE

    try:
        behavior_preserving, packet_id = _resolve_sequence_mode(args)
        git_bin = _require_git_bin()
        base_sha = _resolve_git_commit(repo_root, args.base_commit.strip())
        head_sha = _resolve_git_commit(repo_root, "HEAD")
    except (OSError, DocumentParseError) as exc:
        sys.stderr.write(f"error: {exc}\n")
        return EXIT_USAGE

    cmd_issues: list[ValidationIssue] = []
    _check_verification_command(
        args.test_command,
        json_path="$.test_command",
        expected_targets=[],
        forbid_repo_wide=False,
        issues=cmd_issues,
    )
    if cmd_issues:
        return _emit_result(
            kind="verify-sequence",
            target_path=repo_root,
            packet_id=packet_id,
            issues=cmd_issues,
            output_format=args.output_format,
        )

    if base_sha is None or head_sha is None:
        return _emit_result(
            kind="verify-sequence",
            target_path=repo_root,
            packet_id=packet_id,
            issues=[
                ValidationIssue(
                    "INVALID_BASE_COMMIT",
                    "$.base_commit",
                    f"unable to resolve --base-commit {args.base_commit!r} or HEAD in {repo_root}",
                )
            ],
            output_format=args.output_format,
        )

    try:
        commits, c1_files, issues = _check_sequence_commits(repo_root, git_bin, base_sha)
        if not issues:
            base_rc, base_snippet = _run_test_in_detached_worktree(
                repo_root,
                git_bin,
                base_sha,
                args.test_command,
                overlay=(commits[0], tuple(c1_files)),
            )
            if behavior_preserving and base_rc != 0:
                issues.append(
                    ValidationIssue(
                        "SEQUENCE_BASELINE_TEST_FAILED",
                        f"base:{base_sha[:12]}",
                        f"commit 1 test failed on base_commit {base_sha[:12]} (rc={base_rc}): {base_snippet}",
                    )
                )
            elif not behavior_preserving and base_rc == 0:
                issues.append(
                    ValidationIssue(
                        "SEQUENCE_BUGFIX_NOT_RED",
                        f"base:{base_sha[:12]}",
                        f"bugfix commit 1 test unexpectedly passed (rc=0) on base_commit {base_sha[:12]}",
                    )
                )
        if not issues:
            final_rc, final_snippet = _run_test_in_detached_worktree(
                repo_root,
                git_bin,
                head_sha,
                args.test_command,
            )
            if final_rc != 0:
                issues.append(
                    ValidationIssue(
                        "SEQUENCE_FINAL_TEST_FAILED",
                        f"head:{head_sha[:12]}",
                        f"test command failed on final HEAD {head_sha[:12]} (rc={final_rc}): {final_snippet}",
                    )
                )
    except (OSError, DocumentParseError) as exc:
        issues = [ValidationIssue("VERIFY_SEQUENCE_ERROR", "$", str(exc))]

    return _emit_result(
        kind="verify-sequence",
        target_path=repo_root,
        packet_id=packet_id,
        issues=issues,
        output_format=args.output_format,
    )


def _cmd_list_generated_forbidden_paths(args: argparse.Namespace) -> int:
    repo_root = Path(args.repo_root).resolve()
    if not repo_root.is_dir():
        sys.stderr.write(f"error: repo root directory not found: {repo_root}\n")
        return EXIT_USAGE
    paths = list_generated_forbidden_paths(repo_root)
    if args.output_format == "json":
        sys.stdout.write(json.dumps(paths, ensure_ascii=False, indent=2) + "\n")
    else:
        for rel in paths:
            sys.stdout.write(f"{rel}\n")
    return EXIT_OK


def _cmd_render_template(args: argparse.Namespace) -> int:
    packet_id: str = args.packet_id or DEFAULT_PACKET_ID
    if PACKET_ID_RE.match(packet_id) is None:
        sys.stderr.write(
            f"error: invalid --packet-id {packet_id!r} (must match {PACKET_ID_RE.pattern})\n"
        )
        return EXIT_USAGE

    target: str | None = args.target
    if target is not None:
        norm_target = _normalize_target(target)
        if not norm_target or TARGET_RE.match(norm_target) is None:
            sys.stderr.write(f"error: invalid --target {target!r}\n")
            return EXIT_USAGE
        target = norm_target

    tmpl_path = PACKET_TEMPLATE_PATH if args.kind == "packet" else HANDOFF_TEMPLATE_PATH
    if not tmpl_path.is_file():
        sys.stderr.write(f"error: template not found: {tmpl_path}\n")
        return EXIT_USAGE

    rendered = tmpl_path.read_text(encoding="utf-8").replace(DEFAULT_PACKET_ID, packet_id)
    if target is not None:
        new_verify_cmd = f"./scripts/lint/run.sh {target}"
        rendered = rendered.replace(f'target: "{DEFAULT_TARGET}"', f'target: "{target}"')
        rendered = rendered.replace(DEFAULT_TEMPLATE_VERIFY_CMD, new_verify_cmd)

    if args.kind == "packet":
        lines = rendered.splitlines()
        fm_lines, body_lines = _extract_frontmatter(lines)
        if fm_lines is not None:
            stripped_fm = _strip_measured_from_frontmatter(fm_lines)
            rendered = "\n".join(["---", *stripped_fm, "---", *body_lines]) + "\n"

    if args.output_format == "json":
        doc = parse_document_text(rendered)
        if args.kind == "packet" and isinstance(doc.data.get("execution_policy"), dict):
            doc.data["execution_policy"].pop("measured", None)
        sys.stdout.write(json.dumps(doc.data, ensure_ascii=False, indent=2) + "\n")
        return EXIT_OK

    sys.stdout.write(rendered)
    return EXIT_OK


def _cmd_check_templates(_args: argparse.Namespace) -> int:
    issues: list[ValidationIssue] = []
    if not SCHEMA_PATH.is_file():
        issues.append(ValidationIssue("MISSING_SCHEMA", str(SCHEMA_PATH), "schema file missing"))
    if not PACKET_TEMPLATE_PATH.is_file():
        issues.append(
            ValidationIssue(
                "MISSING_TEMPLATE", str(PACKET_TEMPLATE_PATH), "packet template missing"
            )
        )
    if not HANDOFF_TEMPLATE_PATH.is_file():
        issues.append(
            ValidationIssue(
                "MISSING_TEMPLATE",
                str(HANDOFF_TEMPLATE_PATH),
                "handoff template missing",
            )
        )
    if issues:
        for item in issues:
            sys.stderr.write(f"ERROR [{item.code}] {item.path}: {item.message}\n")
        return EXIT_INVALID

    try:
        schema = load_schema(SCHEMA_PATH)
        if schema.get("$schema") != SCHEMA_DRAFT_2020_12:
            issues.append(
                ValidationIssue(
                    "SCHEMA_DRAFT",
                    "$schema",
                    f"expected {SCHEMA_DRAFT_2020_12}, got {schema.get('$schema')!r}",
                )
            )
        defs = schema.get("$defs", {})
        issues.extend(
            ValidationIssue("SCHEMA_DEFS", f"$defs.{name}", f"missing definition '{name}'")
            for name in ("TaskPacket", "TaskHandoff")
            if not isinstance(defs, dict) or name not in defs
        )

        packet_doc = load_document(PACKET_TEMPLATE_PATH)
        handoff_doc = load_document(HANDOFF_TEMPLATE_PATH)
        issues.extend(validate_packet_doc(packet_doc, schema))
        issues.extend(validate_handoff_doc(handoff_doc, schema, packet_doc=packet_doc))
    except (OSError, ValueError) as exc:
        issues.append(ValidationIssue("SELF_CHECK_ERROR", "$", str(exc)))

    if issues:
        for item in issues:
            sys.stderr.write(f"ERROR [{item.code}] {item.path}: {item.message}\n")
        sys.stderr.write(f"INVALID: {len(issues)} template self-check error(s)\n")
        return EXIT_INVALID

    sys.stdout.write(
        "OK: schema and bundled templates validated "
        f"({PACKET_TEMPLATE_PATH.name}, {HANDOFF_TEMPLATE_PATH.name})\n"
    )
    return EXIT_OK


def _add_loop_and_verify_subparsers(subparsers: argparse._SubParsersAction[Any]) -> None:
    p_meas = subparsers.add_parser(
        "measure",
        help="LOOP-02: run verification_command synchronously and record execution_policy.measured",
    )
    p_meas.add_argument("packet_path", help="Path to TaskPacket JSON or Markdown file")
    p_meas.add_argument(
        "--repo-root",
        default=None,
        help="Repository root directory in which to run verification_command (default: repo root)",
    )
    p_meas.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_meas.set_defaults(func=_cmd_measure)

    p_vh = subparsers.add_parser(
        "verify-handoff",
        help="LOOP-01: ground-truth verify TaskHandoff against git diff, AST functions, CHECK-01, and host rerun",
    )
    p_vh.add_argument("handoff_path", help="Path to TaskHandoff JSON or Markdown file")
    p_vh.add_argument("--packet", required=True, help="Path to corresponding TaskPacket")
    p_vh.add_argument(
        "--base-commit",
        default=None,
        help="Base git revision (falls back to packet.base_commit)",
    )
    p_vh.add_argument(
        "--repo-root",
        default=None,
        help="Repository root directory (default: repo root)",
    )
    p_vh.add_argument(
        "--runs-dir",
        default=None,
        help="Directory for persisted rerun evidence JSON (default: <repo-root>/.harness/runs)",
    )
    p_vh.add_argument(
        "--no-rerun",
        action="store_true",
        default=False,
        help="Skip host re-execution of verification_command",
    )
    p_vh.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_vh.set_defaults(func=_cmd_verify_handoff)

    p_seq = subparsers.add_parser(
        "verify-sequence",
        help="VERIFY-01: verify test-first commit sequence in isolated detached git worktrees",
    )
    p_seq.add_argument("--base-commit", required=True, help="Base git revision")
    p_seq.add_argument(
        "--test-command", required=True, help="Test command to run on base+c1 and HEAD"
    )
    p_seq.add_argument("--packet", default=None, help="Optional TaskPacket to infer sequence mode")
    seq_mode = p_seq.add_mutually_exclusive_group()
    seq_mode.add_argument(
        "--behavior-preserving",
        action="store_true",
        default=False,
        help="Require c1 test to pass on base_commit and stay unmodified in c2..HEAD (default)",
    )
    seq_mode.add_argument(
        "--bugfix",
        action="store_true",
        default=False,
        help="Require c1 test to fail (RED) on base_commit and pass (GREEN) on HEAD",
    )
    p_seq.add_argument(
        "--repo-root",
        default=None,
        help="Repository root directory (default: repo root)",
    )
    p_seq.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_seq.set_defaults(func=_cmd_verify_sequence)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Validate and render Alt coding-agent Task Packets and Handoffs.",
    )
    subparsers = parser.add_subparsers(dest="subcommand", required=True)

    p_pkt = subparsers.add_parser(
        "validate-packet", help="Validate a TaskPacket (JSON or Markdown)"
    )
    p_pkt.add_argument("path", help="Path to TaskPacket JSON or Markdown file")
    p_pkt.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_pkt.set_defaults(func=_cmd_validate_packet)

    p_ho = subparsers.add_parser(
        "validate-handoff",
        help="Validate a TaskHandoff (optionally cross-checked against its TaskPacket)",
    )
    p_ho.add_argument("handoff_path", help="Path to TaskHandoff JSON or Markdown file")
    p_ho.add_argument("--packet", default=None, help="Optional TaskPacket path for cross-check")
    screener_group = p_ho.add_mutually_exclusive_group()
    screener_group.add_argument(
        "--screener-report",
        default=None,
        help="Optional CHECK-01 behavior_screener.py JSON report path",
    )
    screener_group.add_argument(
        "--base-commit",
        default=None,
        help="Optional base git commit to auto-run CHECK-01 behavior_screener.py against",
    )
    p_ho.add_argument(
        "--repo-root",
        default=None,
        help="Optional repository root for CHECK-01 --base-commit and CHECK-05 generated-file checks",
    )
    p_ho.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_ho.set_defaults(func=_cmd_validate_handoff)

    p_gen = subparsers.add_parser(
        "list-generated-forbidden-paths",
        help="CHECK-05: list tracked files carrying a generated-code DO NOT EDIT header",
    )
    p_gen.add_argument(
        "--repo-root",
        default=str(REPO_ROOT_DEFAULT),
        help="Repository root directory to scan (default: repo root)",
    )
    p_gen.add_argument(
        "--format",
        dest="output_format",
        choices=("text", "json"),
        default="text",
        help="Output format (default: text)",
    )
    p_gen.set_defaults(func=_cmd_list_generated_forbidden_paths)

    p_tmpl = subparsers.add_parser("render-template", help="Render a standard packet or handoff")
    p_tmpl.add_argument("--kind", choices=("packet", "handoff"), required=True)
    p_tmpl.add_argument("--packet-id", default=None, help="Packet identifier to substitute")
    p_tmpl.add_argument("--target", default=None, help="Lint target to substitute")
    p_tmpl.add_argument(
        "--format",
        dest="output_format",
        choices=("markdown", "json"),
        default="markdown",
        help="Output format (default: markdown)",
    )
    p_tmpl.set_defaults(func=_cmd_render_template)

    p_chk = subparsers.add_parser(
        "check-templates",
        help="Self-check schema and bundled Markdown templates",
    )
    p_chk.set_defaults(func=_cmd_check_templates)
    _add_loop_and_verify_subparsers(subparsers)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    handler = args.func
    return int(handler(args))


if __name__ == "__main__":
    _launcher = Path(__file__).resolve().parents[2] / "scripts" / "altharness" / "alt-harness"
    if _launcher.is_file():
        os.execv(_launcher, [str(_launcher), "packet", *sys.argv[1:]])  # noqa: S606
    sys.exit(main())
