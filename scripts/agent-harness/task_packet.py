#!/usr/bin/env python3
"""Deterministic parser, validator, and template CLI for Alt agent Task Packets and Handoffs.

Subcommands:
  validate-packet <path> [--format text|json]
  validate-handoff <handoff-path> [--packet <packet-path>] [--format text|json]
  render-template --kind packet|handoff [--packet-id ID] [--target TARGET] [--format markdown|json]
  check-templates

Exit codes:
  0  Valid / clean
  1  Validation or self-check failure
  2  CLI usage or unreadable input file
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2

SCHEMA_DRAFT_2020_12 = "https://json-schema.org/draft/2020-12/schema"
DEFAULT_PACKET_ID = "EXAMPLE-TASK-001"
DEFAULT_TARGET = "scripts"
QUOTED_MIN_LEN = 2
INT_PATTERN = re.compile(r"^-?[0-9]+$")
PACKET_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$")
TARGET_RE = re.compile(r"^(?!.*\.\.)(?!/)[A-Za-z0-9._/-]+$")

HARNESS_DIR = Path(__file__).resolve().parent
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


def _validate_integer_keywords(
    value: int,
    node: dict[str, Any],
    json_path: str,
    issues: list[ValidationIssue],
) -> None:
    minimum = node.get("minimum")
    if isinstance(minimum, int) and value < minimum:
        issues.append(
            ValidationIssue(
                "SCHEMA_MINIMUM",
                json_path,
                f"value {value} is less than minimum {minimum}",
            )
        )
    maximum = node.get("maximum")
    if isinstance(maximum, int) and value > maximum:
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
    elif isinstance(value, int) and not isinstance(value, bool):
        _validate_integer_keywords(value, node, json_path, issues)
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


def _run_git_bytes(repo_root: Path, git_bin: str, *args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(  # noqa: S603 -- runs resolved git binary inside explicit repo_root
        [git_bin, "-C", str(repo_root), *args],
        capture_output=True,
        timeout=SUBPROCESS_TIMEOUT_SECONDS,
        check=False,
    )


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
        res = subprocess.run(  # noqa: S603 -- invokes repo-owned behavior_screener.py with fixed argv
            [
                sys.executable,
                str(SCREENER_SCRIPT_PATH),
                "--repo-root",
                str(repo_root),
                "--base",
                base_commit,
            ],
            capture_output=True,
            text=True,
            timeout=SUBPROCESS_TIMEOUT_SECONDS,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        msg = f"behavior_screener.py timed out after {SUBPROCESS_TIMEOUT_SECONDS}s"
        raise DocumentParseError(msg) from exc
    if res.returncode != 0:
        msg = f"behavior_screener.py failed (rc={res.returncode}): {res.stderr.strip()}"
        raise DocumentParseError(msg)
    return _parse_screener_candidates_text(res.stdout, source="--base-commit")


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


def _matches_go_test(text: str, query: _TestNameQuery) -> bool:
    if query.go_parent is not None:
        if re.search(rf"\bfunc\s+(?:\([^)]+\)\s+)?{re.escape(query.go_parent)}\s*\(", text) is None:
            return False
        return all(_go_subtest_present(text, sub) for sub in query.go_subtests)
    if IDENT_RE.match(query.leaf) is not None and (
        re.search(rf"\bfunc\s+(?:\([^)]+\)\s+)?{re.escape(query.leaf)}\s*\(", text) is not None
    ):
        return True
    if query.leaf:
        run_pat = re.compile(rf'\bt\.Run\s*\(\s*["`]{re.escape(query.leaf)}["`]')
        return run_pat.search(text) is not None
    return False


def _matches_python_test(text: str, query: _TestNameQuery) -> bool:
    if IDENT_RE.match(query.leaf) is None:
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
        rendered = rendered.replace(f'target: "{DEFAULT_TARGET}"', f'target: "{target}"')
        rendered = rendered.replace(
            f"./scripts/lint/run.sh {DEFAULT_TARGET}",
            f"./scripts/lint/run.sh {target}",
        )

    if args.output_format == "json":
        doc = parse_document_text(rendered)
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
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    handler = args.func
    return int(handler(args))


if __name__ == "__main__":
    sys.exit(main())
