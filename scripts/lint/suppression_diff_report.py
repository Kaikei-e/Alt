#!/usr/bin/env python3
"""Generate a GitHub Flavored Markdown PR Job Summary diff table for lint suppressions (CHECK-03).

Compares `<base_rev>` against the working tree (or `--head <rev>`), extracts all
config allowlist, broad source, and inline source suppressions before and after,
and prints a Markdown summary table with totals, net delta, added suppressions
(`path:line`, `kind/rule`, `reason`, `expires`), and removed suppressions.

Run:
    python3 scripts/lint/suppression_diff_report.py --base HEAD~1
    python3 scripts/lint/suppression_diff_report.py --base origin/main --summary-file "$GITHUB_STEP_SUMMARY"
"""

from __future__ import annotations

import argparse
import io
import pathlib
import re
import shutil
import subprocess
import sys
import tokenize
import tomllib
from collections import Counter
from dataclasses import dataclass

EXIT_CLEAN = 0
EXIT_USAGE_ERROR = 2

ALLOWLIST_REL_PATH = "scripts/lint/suppression_allowlist.toml"
SOURCE_EXTENSIONS = (".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")
TS_EXTENSIONS = (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")
GENERATED_HEADER_RE = re.compile(r"Code generated .* DO NOT EDIT\.?", re.IGNORECASE)
GENERATED_HEADER_LINES = 40
MAX_REASON_DISPLAY = 90

EXPIRES_INLINE_RE = re.compile(r"\bexpires\s*[:=]\s*(\d{4}-\d{2}-\d{2})\b", re.IGNORECASE)

PY_FILE_NOQA_RE = re.compile(r"^#\s*(?:ruff|flake8)\s*:\s*noqa\b(.*)$", re.IGNORECASE)
PY_INLINE_NOQA_RE = re.compile(r"#\s*noqa\b(.*)$", re.IGNORECASE)
PY_NOQA_CODES_RE = re.compile(
    r"^:\s*([A-Z]+[0-9]+(?:\s*,\s*[A-Z]+[0-9]+)*)(?:$|[\s#\-—(;:])",
)
PY_SECONDARY_DIRECTIVE_RE = re.compile(
    r"#\s*(?:nosec|type:\s*ignore|pyright:\s*ignore|pyrefly:\s*ignore)\b.*$",
    re.IGNORECASE,
)

GO_NOLINT_RE = re.compile(
    r"^(?://|/\*)\s*nolint(?::\s*([A-Za-z0-9_\-]+(?:\s*,\s*[A-Za-z0-9_\-]+)*)\s*,?)?(?=$|[\s*/])(.*)$",
    re.DOTALL,
)
GO_NOSEC_RE = re.compile(r"^(?://|/\*)\s*#nosec\b(.*)$", re.DOTALL)
GO_NOSEC_RULES_RE = re.compile(r"^((?:G\d{3}(?:[\s,]+)?)+)(?:--|//)(.*)$", re.DOTALL)

RUST_INNER_ALLOW_RE = re.compile(r"^#!\[\s*allow\s*\((.*)\)\s*\]$", re.DOTALL)
RUST_OUTER_ALLOW_RE = re.compile(r"^#\[\s*allow\s*\((.*)\)\s*\]$", re.DOTALL)

TS_BIOME_ALL_RE = re.compile(r"\bbiome-ignore-all\b(.*)$", re.DOTALL)
TS_BIOME_LINE_RE = re.compile(r"\bbiome-ignore\b(?!-all)(.*)$", re.DOTALL)
TS_DENO_FILE_RE = re.compile(r"\bdeno-lint-ignore-file\b(.*)$", re.DOTALL)
TS_DENO_LINE_RE = re.compile(r"\bdeno-lint-ignore\b(?!-file)(.*)$", re.DOTALL)
TS_ESLINT_BROAD_RE = re.compile(r"\beslint-disable\b(?!-next-line\b|-line\b)(.*)$", re.DOTALL)
TS_BIOME_SPEC_RE = re.compile(r"^([A-Za-z][A-Za-z0-9/_-]*)(?::(.*))?$", re.DOTALL)


@dataclass(frozen=True)
class SuppressionRecord:
    surface: str  # "config" | "broad" | "inline"
    path: str
    line: int
    kind: str
    rule: str
    reason: str
    expires: str

    def identity_key(self) -> tuple[str, str, str, str, str, str]:
        return (self.surface, self.path, self.kind, self.rule, self.reason, self.expires)


@dataclass(frozen=True)
class _RustToken:
    kind: str
    text: str
    start_line: int
    end_line: int


def is_excluded_source_path(rel_path: str) -> bool:
    norm = rel_path.lstrip("./")
    if norm.startswith(("stopped-using-k8s/", "scripts/harness/corpus/")):
        return True
    if norm.endswith((".pb.go", ".connect.go", "server.gen.go", ".md")):
        return True
    if norm.startswith(("gen/", "alt-frontend-sv/src/lib/gen/")) or "/gen/" in norm:
        return True
    return not norm.endswith(SOURCE_EXTENSIONS)


def is_generated_source_text(text: str) -> bool:
    return any(
        GENERATED_HEADER_RE.search(line) for line in text.splitlines()[:GENERATED_HEADER_LINES]
    )


def _strip_comment_delimiters(raw: str) -> str:
    text = raw.strip()
    if text.startswith("<!--"):
        text = text[4:-3] if text.endswith("-->") else text[4:]
    elif text.startswith("/*"):
        text = text[2:-2] if text.endswith("*/") else text[2:]
    elif text.startswith("//"):
        text = text[2:]
    elif text.startswith("#"):
        text = text[1:]
    return text.strip()


def _extract_expires(text: str) -> str:
    match = EXPIRES_INLINE_RE.search(text)
    return match.group(1) if match else "-"


def _preceding_comment_text(comment_by_line: dict[int, str], start_line: int) -> str:
    parts: list[str] = []
    cursor = start_line - 1
    while cursor in comment_by_line:
        parts.append(comment_by_line[cursor])
        cursor -= 1
    parts.reverse()
    return "\n".join(parts)


def _clean_reason(raw: str) -> str:
    compact = " ".join(raw.split())
    return compact or "-"


def extract_allowlist_records(text: str) -> list[SuppressionRecord]:
    """Extract config-level suppression entries from suppression_allowlist.toml."""
    try:
        data = tomllib.loads(text)
    except tomllib.TOMLDecodeError:
        return []
    records: list[SuppressionRecord] = []

    for entry in data.get("bandit", []):
        if not isinstance(entry, dict):
            continue
        skips = ", ".join(str(s) for s in entry.get("skips", []))
        records.append(
            SuppressionRecord(
                surface="config",
                path=str(entry.get("path", ALLOWLIST_REL_PATH)),
                line=0,
                kind="config_bandit",
                rule=skips or "bandit",
                reason=_clean_reason(str(entry.get("reason", ""))),
                expires=str(entry.get("expires", "-")),
            )
        )

    for entry in data.get("golangci_exclusions", []):
        if not isinstance(entry, dict):
            continue
        linters = ", ".join(str(x) for x in entry.get("linters", []))
        scope = str(entry.get("path") or entry.get("path_except") or "*")
        records.append(
            SuppressionRecord(
                surface="config",
                path=str(entry.get("config", ".golangci.yml")),
                line=0,
                kind="config_golangci",
                rule=f"{linters} ({scope})",
                reason=_clean_reason(str(entry.get("reason", ""))),
                expires=str(entry.get("expires", "-")),
            )
        )

    for entry in data.get("cargo_lint_allows", []):
        if not isinstance(entry, dict):
            continue
        section = str(entry.get("section", "lints"))
        allows = ", ".join(str(a) for a in entry.get("allows", []))
        records.append(
            SuppressionRecord(
                surface="config",
                path=str(entry.get("path", "Cargo.toml")),
                line=0,
                kind="config_cargo",
                rule=f"{section}: {allows}",
                reason=_clean_reason(str(entry.get("reason", ""))),
                expires=str(entry.get("expires", "-")),
            )
        )
    return records


def _extract_python_records(rel_path: str, text: str) -> list[SuppressionRecord]:
    try:
        comments = [
            (tok.start[0], tok.string)
            for tok in tokenize.generate_tokens(io.StringIO(text).readline)
            if tok.type == tokenize.COMMENT
        ]
    except (tokenize.TokenError, IndentationError):
        comments = [
            (idx, line.lstrip())
            for idx, line in enumerate(text.splitlines(), 1)
            if line.lstrip().startswith("#")
        ]
    comment_by_line = {ln: _strip_comment_delimiters(raw) for ln, raw in comments}
    records: list[SuppressionRecord] = []

    for line_no, raw in comments:
        file_match = PY_FILE_NOQA_RE.match(raw)
        if file_match:
            preceding = _preceding_comment_text(comment_by_line, line_no)
            context = f"{preceding} {file_match.group(1)}".strip()
            rule = file_match.group(1).strip().lstrip(":").strip() or "all"
            records.append(
                SuppressionRecord(
                    surface="broad",
                    path=rel_path,
                    line=line_no,
                    kind="broad_python_noqa",
                    rule=rule,
                    reason=_clean_reason(preceding or file_match.group(1)),
                    expires=_extract_expires(context),
                )
            )
            continue

        inline_match = PY_INLINE_NOQA_RE.search(raw)
        if inline_match:
            rest = inline_match.group(1)
            codes_match = PY_NOQA_CODES_RE.match(rest)
            if codes_match:
                rule = ", ".join(
                    c.strip().upper() for c in codes_match.group(1).split(",") if c.strip()
                )
                tail = PY_SECONDARY_DIRECTIVE_RE.sub("", rest[codes_match.end(1) :]).strip()
                reason = re.sub(r"^(?:--|—|-|#|:|;|\s)+", "", tail).strip()
            else:
                rule = "bare"
                reason = rest.strip()
            records.append(
                SuppressionRecord(
                    surface="inline",
                    path=rel_path,
                    line=line_no,
                    kind="python_noqa",
                    rule=rule,
                    reason=_clean_reason(reason),
                    expires=_extract_expires(raw),
                )
            )
    return records


def _skip_quoted(text: str, pos: int, quote: str) -> tuple[int, int]:
    n = len(text)
    pos += 1
    newlines = 0
    while pos < n:
        ch = text[pos]
        if ch == "\\" and pos + 1 < n:
            if text[pos + 1] == "\n":
                newlines += 1
            pos += 2
            continue
        if ch == "\n":
            return pos, newlines
        if ch == quote:
            return pos + 1, newlines
        pos += 1
    return pos, newlines


def _scan_go_comments(text: str) -> list[tuple[int, int, str, bool]]:
    comments: list[tuple[int, int, str, bool]] = []
    n = len(text)
    i = 0
    line = 1
    seen_package = False
    while i < n:
        ch = text[i]
        if ch == "\n":
            line += 1
            i += 1
        elif ch in ('"', "'"):
            i, nl = _skip_quoted(text, i, ch)
            line += nl
        elif ch == "`":
            end = text.find("`", i + 1)
            if end == -1:
                break
            line += text.count("\n", i, end + 1)
            i = end + 1
        elif ch == "/" and i + 1 < n and text[i + 1] == "/":
            end = text.find("\n", i)
            end = n if end == -1 else end
            comments.append((line, line, text[i:end], not seen_package))
            i = end
        elif ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            end = n if end == -1 else end + 2
            end_line = line + text.count("\n", i, end)
            comments.append((line, end_line, text[i:end], not seen_package))
            line = end_line
            i = end
        elif not seen_package and (ch.isalpha() or ch == "_"):
            start = i
            while i < n and (text[i].isalnum() or text[i] == "_"):
                i += 1
            if text[start:i] == "package":
                seen_package = True
        else:
            i += 1
    return comments


def _extract_go_records(rel_path: str, text: str) -> list[SuppressionRecord]:
    comments = _scan_go_comments(text)
    comment_by_line = {s: _strip_comment_delimiters(r) for s, e, r, _ in comments if s == e}
    records: list[SuppressionRecord] = []
    for start_line, _, raw, before_package in comments:
        nolint_match = GO_NOLINT_RE.match(raw)
        if nolint_match:
            linters = (nolint_match.group(1) or "all").strip()
            rest = nolint_match.group(2)
            if rest.endswith("*/"):
                rest = rest[:-2]
            reason = rest.split("//", 1)[1].strip() if "//" in rest else ""
            is_broad = before_package or linters.lower() == "all"
            preceding = _preceding_comment_text(comment_by_line, start_line) if is_broad else ""
            combined = f"{preceding} {reason}".strip()
            records.append(
                SuppressionRecord(
                    surface="broad" if is_broad else "inline",
                    path=rel_path,
                    line=start_line,
                    kind="broad_go_nolint" if is_broad else "go_nolint",
                    rule=linters,
                    reason=_clean_reason(combined if is_broad else reason),
                    expires=_extract_expires(combined),
                )
            )
            continue

        nosec_match = GO_NOSEC_RE.match(raw)
        if nosec_match:
            cleaned = nosec_match.group(1).rstrip("*/").strip()
            parsed = GO_NOSEC_RULES_RE.match(cleaned)
            if parsed:
                rule = ", ".join(re.findall(r"G\d{3}", parsed.group(1))) or "nosec"
                reason = parsed.group(2).strip()
            else:
                rule = cleaned or "nosec"
                reason = ""
            records.append(
                SuppressionRecord(
                    surface="inline",
                    path=rel_path,
                    line=start_line,
                    kind="go_nosec",
                    rule=rule,
                    reason=_clean_reason(reason),
                    expires=_extract_expires(raw),
                )
            )
    return records


def _skip_rust_raw_string(text: str, pos: int) -> tuple[int, int] | None:
    n = len(text)
    j = pos
    if j < n and text[j] in ("b", "c"):
        j += 1
    if j >= n or text[j] != "r":
        return None
    j += 1
    hashes = 0
    while j < n and text[j] == "#":
        hashes += 1
        j += 1
    if j >= n or text[j] != '"':
        return None
    closing = '"' + ("#" * hashes)
    end = text.find(closing, j + 1)
    end = n if end == -1 else end + len(closing)
    return end, text.count("\n", pos, end)


def _skip_rust_block_comment(text: str, pos: int) -> tuple[int, int]:
    n = len(text)
    depth = 1
    i = pos + 2
    newlines = 0
    while i < n and depth > 0:
        if text[i] == "\n":
            newlines += 1
            i += 1
        elif i + 1 < n and text[i : i + 2] == "/*":
            depth += 1
            i += 2
        elif i + 1 < n and text[i : i + 2] == "*/":
            depth -= 1
            i += 2
        else:
            i += 1
    return i, newlines


def _skip_rust_char_literal(text: str, pos: int) -> int | None:
    n = len(text)
    if pos + 2 >= n or text[pos] != "'":
        return None
    if text[pos + 1] == "\\":
        j = pos + 2
        while j < n and text[j] not in ("'", "\n"):
            j += 1
        return j + 1 if j < n and text[j] == "'" else None
    if text[pos + 1] != "\n" and text[pos + 2] == "'":
        return pos + 3
    return None


def _read_rust_attribute(text: str, pos: int, start_line: int) -> tuple[_RustToken, int, int]:
    n = len(text)
    is_inner = pos + 1 < n and text[pos + 1] == "!"
    i = pos + (2 if is_inner else 1)
    depth = 0
    line = start_line
    while i < n:
        ch = text[i]
        if ch == "\n":
            line += 1
            i += 1
        elif ch == '"':
            i, nl = _skip_quoted(text, i, '"')
            line += nl
        elif ch == "[":
            depth += 1
            i += 1
        elif ch == "]":
            depth -= 1
            i += 1
            if depth == 0:
                break
        else:
            i += 1
    return (
        _RustToken("ATTR_INNER" if is_inner else "ATTR_OUTER", text[pos:i], start_line, line),
        i,
        line,
    )


def _scan_rust_comment_or_attr(
    text: str,
    pos: int,
    line: int,
) -> tuple[_RustToken | None, int, int] | None:
    n = len(text)
    ch = text[pos]
    if ch == "/" and pos + 1 < n and text[pos + 1] == "/":
        end = text.find("\n", pos)
        end = n if end == -1 else end
        return _RustToken("COMMENT", text[pos:end], line, line), end, line
    if ch == "/" and pos + 1 < n and text[pos + 1] == "*":
        end, nl = _skip_rust_block_comment(text, pos)
        return _RustToken("COMMENT", text[pos:end], line, line + nl), end, line + nl
    if ch == "#" and (
        (pos + 1 < n and text[pos + 1] == "[") or (pos + 2 < n and text[pos + 1 : pos + 3] == "![")
    ):
        return _read_rust_attribute(text, pos, line)
    return None


def _scan_rust_string_or_word(
    text: str,
    pos: int,
    line: int,
) -> tuple[_RustToken | None, int, int]:
    n = len(text)
    ch = text[pos]
    if ch == '"':
        end, nl = _skip_quoted(text, pos, '"')
        return None, end, line + nl
    if ch in ("r", "b", "c"):
        raw = _skip_rust_raw_string(text, pos)
        if raw is not None:
            return None, raw[0], line + raw[1]
        if ch in ("b", "c") and pos + 1 < n and text[pos + 1] == '"':
            end, nl = _skip_quoted(text, pos + 1, '"')
            return None, end, line + nl
    if ch == "'":
        char_end = _skip_rust_char_literal(text, pos)
        return None, (char_end if char_end is not None else pos + 1), line
    if ch.isalpha() or ch == "_":
        end = pos
        while end < n and (text[end].isalnum() or text[end] == "_"):
            end += 1
        return _RustToken("WORD", text[pos:end], line, line), end, line
    return _RustToken("PUNCT", ch, line, line), pos + 1, line


def _scan_rust_tokens(text: str) -> list[_RustToken]:
    tokens: list[_RustToken] = []
    n = len(text)
    i = 0
    line = 1
    while i < n:
        ch = text[i]
        if ch == "\n":
            line += 1
            i += 1
            continue
        if ch.isspace():
            i += 1
            continue
        special = _scan_rust_comment_or_attr(text, i, line)
        tok, i, line = special if special is not None else _scan_rust_string_or_word(text, i, line)
        if tok is not None:
            tokens.append(tok)
    return tokens


def _rust_attr_applies_to_mod(tokens: list[_RustToken], attr_index: int) -> bool:
    j = attr_index + 1
    n = len(tokens)
    while j < n and tokens[j].kind in ("COMMENT", "ATTR_OUTER"):
        j += 1
    if j < n and tokens[j].kind == "WORD" and tokens[j].text == "pub":
        j += 1
        if j < n and tokens[j].kind == "PUNCT" and tokens[j].text == "(":
            depth = 1
            j += 1
            while j < n and depth > 0:
                if tokens[j].kind == "PUNCT" and tokens[j].text == "(":
                    depth += 1
                elif tokens[j].kind == "PUNCT" and tokens[j].text == ")":
                    depth -= 1
                j += 1
    while j < n and tokens[j].kind == "COMMENT":
        j += 1
    return (
        j + 1 < n
        and tokens[j].kind == "WORD"
        and tokens[j].text == "mod"
        and tokens[j + 1].kind == "WORD"
    )


def _rust_context_for_attr(tokens: list[_RustToken], attr_index: int) -> str:
    attr = tokens[attr_index]
    top_line = attr.start_line
    k = attr_index - 1
    while k >= 0 and tokens[k].kind == "ATTR_OUTER" and tokens[k].end_line >= top_line - 1:
        top_line = tokens[k].start_line
        k -= 1
    preceding: list[str] = []
    expected_line = top_line - 1
    while k >= 0 and tokens[k].kind == "COMMENT" and tokens[k].end_line == expected_line:
        preceding.append(_strip_comment_delimiters(tokens[k].text))
        expected_line = tokens[k].start_line - 1
        k -= 1
    preceding.reverse()
    j = attr_index + 1
    while j < len(tokens) and tokens[j].start_line == attr.end_line:
        if tokens[j].kind == "COMMENT":
            preceding.append(_strip_comment_delimiters(tokens[j].text))
        j += 1
    return "\n".join(preceding)


def _extract_rust_records(rel_path: str, text: str) -> list[SuppressionRecord]:
    tokens = _scan_rust_tokens(text)
    records: list[SuppressionRecord] = []
    for idx, tok in enumerate(tokens):
        inner = RUST_INNER_ALLOW_RE.match(tok.text) if tok.kind == "ATTR_INNER" else None
        if inner:
            ctx = _rust_context_for_attr(tokens, idx)
            records.append(
                SuppressionRecord(
                    surface="broad",
                    path=rel_path,
                    line=tok.start_line,
                    kind="broad_rust_inner_allow",
                    rule=" ".join(inner.group(1).split()),
                    reason=_clean_reason(ctx),
                    expires=_extract_expires(ctx),
                )
            )
            continue
        outer = RUST_OUTER_ALLOW_RE.match(tok.text) if tok.kind == "ATTR_OUTER" else None
        if outer:
            is_mod = _rust_attr_applies_to_mod(tokens, idx)
            ctx = _rust_context_for_attr(tokens, idx)
            records.append(
                SuppressionRecord(
                    surface="broad" if is_mod else "inline",
                    path=rel_path,
                    line=tok.start_line,
                    kind="broad_rust_mod_allow" if is_mod else "rust_allow",
                    rule=" ".join(outer.group(1).split()),
                    reason=_clean_reason(ctx),
                    expires=_extract_expires(ctx),
                )
            )
    return records


def _skip_ts_template(text: str, pos: int) -> tuple[int, int]:
    n = len(text)
    i = pos + 1
    newlines = 0
    while i < n:
        ch = text[i]
        if ch == "\\" and i + 1 < n:
            if text[i + 1] == "\n":
                newlines += 1
            i += 2
            continue
        if ch == "\n":
            newlines += 1
        elif ch == "`":
            return i + 1, newlines
        i += 1
    return i, newlines


def _scan_ts_comments(text: str, *, is_svelte: bool) -> list[tuple[int, int, str]]:
    comments: list[tuple[int, int, str]] = []
    n = len(text)
    i = 0
    line = 1
    while i < n:
        ch = text[i]
        if ch == "\n":
            line += 1
            i += 1
        elif ch in ('"', "'"):
            i, nl = _skip_quoted(text, i, ch)
            line += nl
        elif ch == "`":
            i, nl = _skip_ts_template(text, i)
            line += nl
        elif ch == "/" and i + 1 < n and text[i + 1] == "/":
            end = text.find("\n", i)
            end = n if end == -1 else end
            comments.append((line, line, text[i:end]))
            i = end
        elif ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            end = n if end == -1 else end + 2
            end_line = line + text.count("\n", i, end)
            comments.append((line, end_line, text[i:end]))
            line = end_line
            i = end
        elif is_svelte and text[i : i + 4] == "<!--":
            end = text.find("-->", i + 4)
            end = n if end == -1 else end + 3
            end_line = line + text.count("\n", i, end)
            comments.append((line, end_line, text[i:end]))
            line = end_line
            i = end
        else:
            i += 1
    return comments


def _extract_ts_records(rel_path: str, text: str) -> list[SuppressionRecord]:
    comments = _scan_ts_comments(text, is_svelte=rel_path.endswith(".svelte"))
    comment_by_line = {s: _strip_comment_delimiters(r) for s, e, r in comments if s == e}
    records: list[SuppressionRecord] = []
    for start_line, _, raw in comments:
        body = _strip_comment_delimiters(raw)
        preceding = _preceding_comment_text(comment_by_line, start_line)
        for regex, kind in (
            (TS_BIOME_ALL_RE, "broad_ts_biome"),
            (TS_DENO_FILE_RE, "broad_ts_deno"),
            (TS_ESLINT_BROAD_RE, "broad_ts_eslint"),
        ):
            m = regex.search(body)
            if m:
                combined = f"{preceding} {m.group(1)}".strip()
                records.append(
                    SuppressionRecord(
                        surface="broad",
                        path=rel_path,
                        line=start_line,
                        kind=kind,
                        rule=m.group(1).strip() or "all",
                        reason=_clean_reason(combined),
                        expires=_extract_expires(combined),
                    )
                )
                break
        else:
            biome_line = TS_BIOME_LINE_RE.search(body)
            if biome_line:
                spec = TS_BIOME_SPEC_RE.match(biome_line.group(1).strip())
                rule = spec.group(1) if spec else "biome"
                reason = (spec.group(2) or "").strip() if spec else biome_line.group(1).strip()
                records.append(
                    SuppressionRecord(
                        surface="inline",
                        path=rel_path,
                        line=start_line,
                        kind="ts_biome_ignore",
                        rule=rule,
                        reason=_clean_reason(reason),
                        expires=_extract_expires(body),
                    )
                )
                continue
            deno_line = TS_DENO_LINE_RE.search(body)
            if deno_line:
                rest = deno_line.group(1).strip()
                parts = rest.split(None, 1)
                rule = parts[0] if parts else "deno"
                reason = parts[1] if len(parts) > 1 else ""
                records.append(
                    SuppressionRecord(
                        surface="inline",
                        path=rel_path,
                        line=start_line,
                        kind="ts_deno_ignore",
                        rule=rule,
                        reason=_clean_reason(reason),
                        expires=_extract_expires(body),
                    )
                )
    return records


def extract_file_records(rel_path: str, text: str) -> list[SuppressionRecord]:
    """Extract all config, broad, or inline suppressions from a single file's contents."""
    if rel_path == ALLOWLIST_REL_PATH:
        return extract_allowlist_records(text)
    if is_excluded_source_path(rel_path) or is_generated_source_text(text):
        return []
    records: list[SuppressionRecord] = []
    if rel_path.endswith(".py"):
        records = _extract_python_records(rel_path, text)
    elif rel_path.endswith(".go"):
        records = _extract_go_records(rel_path, text)
    elif rel_path.endswith(".rs"):
        records = _extract_rust_records(rel_path, text)
    elif rel_path.endswith(TS_EXTENSIONS):
        records = _extract_ts_records(rel_path, text)
    return records


def _run_git(
    repo_root: pathlib.Path, *args: str, check: bool = True
) -> subprocess.CompletedProcess[str]:
    git_bin = shutil.which("git")
    if git_bin is None:
        raise FileNotFoundError("git")
    return subprocess.run(  # noqa: S603 - fixed git command invocation
        [git_bin, "-C", str(repo_root), *args],
        capture_output=True,
        text=True,
        check=check,
    )


def _git_show_optional(repo_root: pathlib.Path, rev: str, rel_path: str) -> str | None:
    proc = _run_git(repo_root, "show", f"{rev}:{rel_path}", check=False)
    return proc.stdout if proc.returncode == 0 else None


def _read_after_text(repo_root: pathlib.Path, head_rev: str | None, rel_path: str) -> str | None:
    if head_rev is not None:
        return _git_show_optional(repo_root, head_rev, rel_path)
    full = repo_root / rel_path
    return full.read_text(encoding="utf-8", errors="replace") if full.is_file() else None


def _collect_changed_paths(
    repo_root: pathlib.Path,
    base_rev: str,
    head_rev: str | None,
) -> set[str]:
    diff_args = (
        ["diff", "--name-only", base_rev, head_rev]
        if head_rev
        else ["diff", "--name-only", base_rev]
    )
    paths = {line for line in _run_git(repo_root, *diff_args).stdout.splitlines() if line}
    if head_rev is None:
        untracked = _run_git(
            repo_root, "ls-files", "--others", "--exclude-standard"
        ).stdout.splitlines()
        paths.update(line for line in untracked if line)
    return paths


def _collect_after_surface_totals(repo_root: pathlib.Path, head_rev: str | None) -> Counter[str]:
    if head_rev is not None:
        files = _run_git(repo_root, "ls-tree", "-r", "--name-only", head_rev).stdout.splitlines()
    else:
        files = _run_git(
            repo_root, "ls-files", "--cached", "--others", "--exclude-standard"
        ).stdout.splitlines()

    totals: Counter[str] = Counter()
    for rel_path in files:
        if rel_path != ALLOWLIST_REL_PATH and is_excluded_source_path(rel_path):
            continue
        text = _read_after_text(repo_root, head_rev, rel_path)
        if text is None:
            continue
        for rec in extract_file_records(rel_path, text):
            totals[rec.surface] += 1
    return totals


def _diff_records(
    before_records: list[SuppressionRecord],
    after_records: list[SuppressionRecord],
) -> tuple[list[SuppressionRecord], list[SuppressionRecord]]:
    before_counts = Counter(r.identity_key() for r in before_records)
    after_counts = Counter(r.identity_key() for r in after_records)

    added: list[SuppressionRecord] = []
    seen_after: Counter[tuple[str, str, str, str, str, str]] = Counter()
    for rec in after_records:
        key = rec.identity_key()
        seen_after[key] += 1
        if seen_after[key] > before_counts.get(key, 0):
            added.append(rec)

    removed: list[SuppressionRecord] = []
    seen_before: Counter[tuple[str, str, str, str, str, str]] = Counter()
    for rec in before_records:
        key = rec.identity_key()
        seen_before[key] += 1
        if seen_before[key] > after_counts.get(key, 0):
            removed.append(rec)

    added.sort(key=lambda r: (r.surface, r.path, r.line, r.kind, r.rule))
    removed.sort(key=lambda r: (r.surface, r.path, r.line, r.kind, r.rule))
    return added, removed


def _md_escape(text: str) -> str:
    clipped = text if len(text) <= MAX_REASON_DISPLAY else f"{text[: MAX_REASON_DISPLAY - 3]}..."
    return clipped.replace("|", "\\|").replace("\n", " ")


def _fmt_delta(delta: int) -> str:
    return f"+{delta}" if delta >= 0 else str(delta)


def _render_record_table(records: list[SuppressionRecord]) -> list[str]:
    if not records:
        return ["_None_", ""]
    lines = [
        "| Location | Kind / Rule | Reason | Expires |",
        "| --- | --- | --- | --- |",
    ]
    for rec in records:
        loc = f"`{rec.path}:{rec.line}`" if rec.line > 0 else f"`{rec.path}`"
        kind_rule = f"`{rec.kind}` (`{_md_escape(rec.rule)}`)"
        exp = f"`{rec.expires}`" if rec.expires != "-" else "-"
        lines.append(f"| {loc} | {kind_rule} | {_md_escape(rec.reason)} | {exp} |")
    lines.append("")
    return lines


def build_diff_report(
    repo_root: pathlib.Path,
    base_rev: str,
    head_rev: str | None = None,
) -> str:
    """Compute before/after suppression diff and render GitHub Flavored Markdown."""
    changed_paths = sorted(_collect_changed_paths(repo_root, base_rev, head_rev))
    before_changed: list[SuppressionRecord] = []
    after_changed: list[SuppressionRecord] = []

    for rel_path in changed_paths:
        if rel_path != ALLOWLIST_REL_PATH and is_excluded_source_path(rel_path):
            continue
        before_text = _git_show_optional(repo_root, base_rev, rel_path)
        if before_text is not None:
            before_changed.extend(extract_file_records(rel_path, before_text))
        after_text = _read_after_text(repo_root, head_rev, rel_path)
        if after_text is not None:
            after_changed.extend(extract_file_records(rel_path, after_text))

    added, removed = _diff_records(before_changed, after_changed)
    after_totals = _collect_after_surface_totals(repo_root, head_rev)
    added_by_surface = Counter(r.surface for r in added)
    removed_by_surface = Counter(r.surface for r in removed)

    head_label = head_rev if head_rev is not None else "working-tree"
    lines = [
        "## Suppression Governance Diff Report",
        "",
        f"Base: `{base_rev}` -> Head: `{head_label}`",
        "",
        "| Surface | Before | After | Delta |",
        "| --- | ---: | ---: | ---: |",
    ]
    total_before = 0
    total_after = 0
    for surface, label in (
        ("config", "Config allowlist"),
        ("broad", "Broad source"),
        ("inline", "Inline source"),
    ):
        after_n = after_totals.get(surface, 0)
        delta = added_by_surface.get(surface, 0) - removed_by_surface.get(surface, 0)
        before_n = after_n - delta
        total_before += before_n
        total_after += after_n
        lines.append(f"| {label} | {before_n} | {after_n} | {_fmt_delta(delta)} |")

    total_delta = total_after - total_before
    lines.append(
        f"| **Total** | **{total_before}** | **{total_after}** | **{_fmt_delta(total_delta)}** |"
    )
    lines.extend(["", f"### Added Suppressions ({len(added)})", ""])
    lines.extend(_render_record_table(added))
    lines.extend([f"### Removed Suppressions ({len(removed)})", ""])
    lines.extend(_render_record_table(removed))
    return "\n".join(lines)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    default_root = pathlib.Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(
        description="Render a Markdown PR Job Summary table of suppression diffs.",
    )
    parser.add_argument(
        "base_pos",
        nargs="?",
        default=None,
        help="Optional positional base git revision (alternative to --base).",
    )
    parser.add_argument(
        "--base",
        type=str,
        default=None,
        help="Base git revision to compare against (default: HEAD).",
    )
    parser.add_argument(
        "--head",
        type=str,
        default=None,
        help="Optional head git revision (default: compare against working tree).",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=default_root,
        help="Repository root directory (default: detected repo root).",
    )
    parser.add_argument(
        "--summary-file",
        type=pathlib.Path,
        default=None,
        help="Append Markdown report to this file (e.g. $GITHUB_STEP_SUMMARY).",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    base_rev: str = args.base or args.base_pos or "HEAD"
    repo_root: pathlib.Path = args.repo_root.resolve()

    try:
        report = build_diff_report(repo_root, base_rev, args.head)
    except (subprocess.CalledProcessError, FileNotFoundError, OSError) as exc:
        sys.stderr.write(f"ERROR: failed to build suppression diff report: {exc}\n")
        return EXIT_USAGE_ERROR

    sys.stdout.write(report)
    if args.summary_file is not None:
        summary_path: pathlib.Path = args.summary_file
        summary_path.parent.mkdir(parents=True, exist_ok=True)
        existing = summary_path.read_text(encoding="utf-8") if summary_path.is_file() else ""
        separator = "\n" if existing and not existing.endswith("\n") else ""
        with summary_path.open("a", encoding="utf-8") as fh:
            if separator:
                fh.write(separator)
            fh.write(report)
    return EXIT_CLEAN


if __name__ == "__main__":
    sys.exit(main())
