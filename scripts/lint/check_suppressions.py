#!/usr/bin/env python3
"""Enforce lint suppression governance and config allowlists across the repository.

Checks two surfaces:
1. Configuration files (`pyproject.toml`, `.golangci.yml`, `Cargo.toml`) against
   the frozen ledger in `scripts/lint/suppression_allowlist.toml`.
2. Tracked source files (Python, Go, Rust, TypeScript/JavaScript/Svelte/Deno):
   - File/crate/module-wide or blanket suppressions require a verified
     `ADR-NNNNNN` reference (where `docs/ADR/NNNNNN.md` exists) plus a reason.
   - Inline suppressions require concrete rule/linter names and reasons where
     mandated by policy.

Run:
    python3 scripts/lint/check_suppressions.py
"""

from __future__ import annotations

import argparse
import datetime
import hashlib
import io
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tokenize
import tomllib
from collections import Counter
from dataclasses import dataclass

MIN_REASON_LENGTH = 5
EXIT_CLEAN = 0
EXIT_VIOLATIONS = 1
EXIT_USAGE_ERROR = 2
FROZEN_BASELINE_MAX_EXPIRES = datetime.date(2027, 5, 22)
MAX_EXPIRES_HORIZON_DAYS = 366
ADR_PATH_RE = re.compile(r"^docs/ADR/(\d{6})\.md$")

LEGACY_RUFF_KEYS = ("ignore", "extend-ignore", "per-file-ignores", "extend-per-file-ignores")
SOURCE_EXTENSIONS = (".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")
TS_EXTENSIONS = (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte")

MAX_RUFF_GLOBAL_IGNORES = frozenset({"E501", "RUF001", "RUF002", "RUF003"})
MAX_RUFF_TEST_PER_FILE_IGNORES = frozenset({"S101"})

FROZEN_BANDIT_BASELINES: dict[str, tuple[frozenset[str], frozenset[str]]] = {
    "tag-generator/app/pyproject.toml": (
        frozenset({"B101", "B404", "B603", "B110", "B104", "B108", "B602"}),
        frozenset({"tests", ".venv", "__pycache__", "*.egg-info", "build", "dist"}),
    ),
    "tts-speaker/pyproject.toml": (
        frozenset({"B101", "B104"}),
        frozenset({"tests", ".venv", "__pycache__", "*.egg-info", "build", "dist"}),
    ),
}

FROZEN_GOLANGCI_BASELINES: frozenset[
    tuple[str, str | None, str | None, str | None, str | None, frozenset[str]]
] = frozenset(
    {
        (".golangci.yml", r"\.pb\.go$", None, None, None, frozenset({"all"})),
        (".golangci.yml", r"\.connect\.go$", None, None, None, frozenset({"all"})),
        (".golangci.yml", r"server\.gen\.go$", None, None, None, frozenset({"all"})),
        (".golangci.yml", r"internal/pki/.*_test\.go$", None, None, None, frozenset({"gosec"})),
        (".golangci.yml", r"_test\.go$", None, "G101", None, frozenset({"gosec"})),
        (
            ".golangci.yml",
            None,
            r"^knowledge-sovereign/app/usecase/(knowledge_home_projector|knowledge_trail_projector)/[^/]+\.go$",
            None,
            None,
            frozenset({"forbidigo"}),
        ),
        (".golangci.yml", r"_test\.go$", None, None, None, frozenset({"forbidigo"})),
    }
)

FROZEN_CARGO_BASELINES: dict[tuple[str, str], frozenset[str]] = {
    ("recap-worker/recap-worker/Cargo.toml", "lints.clippy"): frozenset(
        {
            "assert_is_empty",
            "cast_possible_truncation",
            "cast_possible_wrap",
            "cast_precision_loss",
            "collapsible_if",
            "doc_markdown",
            "double_must_use",
            "explicit_iter_loop",
            "items_after_statements",
            "missing_errors_doc",
            "missing_panics_doc",
            "module_name_repetitions",
            "must_use_candidate",
            "needless_pass_by_value",
            "option_if_let_else",
            "or_fun_call",
            "redundant_closure",
            "uninlined_format_args",
        }
    ),
}

ADR_ID_RE = re.compile(r"\bADR-(\d{6})\b")
ADR_EXACT_RE = re.compile(r"^ADR-(\d{6})$")
ISO_DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
EXPIRES_INLINE_RE = re.compile(r"\bexpires\s*[:=]\s*([^\s),;\]]+)", re.IGNORECASE)
NON_REASON_CHARS_RE = re.compile(r"[\s#/*<>!:\-—.,;()\[\]{}]+")
GENERATED_HEADER_RE = re.compile(r"Code generated .* DO NOT EDIT\.?", re.IGNORECASE)
GENERATED_HEADER_LINES = 40

PY_FILE_NOQA_RE = re.compile(r"^#\s*(?:ruff|flake8)\s*:\s*noqa\b(.*)$", re.IGNORECASE)
PY_INLINE_NOQA_RE = re.compile(r"#\s*noqa\b(.*)$", re.IGNORECASE)
PY_NOQA_CODES_RE = re.compile(
    r"^:\s*([A-Z]+[0-9]+(?:\s*,\s*[A-Z]+[0-9]+)*)(?:$|[\s#\-—(;:])",
)
PY_TRAILING_BAD_CODE_RE = re.compile(r",\s*(?:ALL|\*)", re.IGNORECASE)
PY_NOSEC_RE = re.compile(r"#\s*nosec\b(.*)$", re.IGNORECASE)
PY_NOSEC_CODES_RE = re.compile(
    r"^[\s:]*((?:B\d{3})(?:[\s,]+B\d{3})*)(?=$|[\s#\-—(;:])",
    re.IGNORECASE,
)
PY_NOSEC_SECONDARY_RE = re.compile(
    r"#\s*(?:noqa|type:\s*ignore|pyright:\s*ignore|pyrefly:\s*ignore)\b.*$",
    re.IGNORECASE,
)
PY_SECONDARY_DIRECTIVE_RE = re.compile(
    r"#\s*(?:nosec|type:\s*ignore|pyright:\s*ignore|pyrefly:\s*ignore)\b.*$",
    re.IGNORECASE,
)

SQL_SUPPRESSION_CODES = frozenset({"S608", "G201", "G202", "B608"})
SQL_FORBIDDEN_VALUE_RE = re.compile(r"(?:\bvalue\b|値)", re.IGNORECASE)
SQL_FORBIDDEN_UNTRUSTED_RE = re.compile(r"(?:\buntrusted\b|\bexternal\b|信頼境界外)", re.IGNORECASE)
SQL_ALLOWED_KIND_RE = re.compile(
    r"(?:\bidentifier\b|識別子|\bnot[-_\s]?sql\b|非\s*SQL)",
    re.IGNORECASE,
)
SQL_ALLOWED_TRUST_RE = re.compile(
    r"(?:\btrusted\b|\binternal\b|\bconstant\b|\ballowlist\b|信頼境界内)",
    re.IGNORECASE,
)

GO_NOLINT_RE = re.compile(
    r"^(?://|/\*)\s*nolint(?::\s*([A-Za-z0-9_\-]+(?:\s*,\s*[A-Za-z0-9_\-]+)*)\s*,?)?(?=$|[\s*/])(.*)$",
    re.DOTALL,
)
GO_NOSEC_RE = re.compile(r"^(?://|/\*)\s*#nosec\b(.*)$", re.DOTALL)
GO_NOSEC_RULES_RE = re.compile(
    r"^((?:G\d{3}(?:[\s,]+)?)+)(?:--|//)(.*)$",
    re.DOTALL,
)
GO_NOSEC_RULE_ONLY_RE = re.compile(r"^((?:G\d{3}(?:[\s,]+)?)+)(?:\*/)?$")

RUST_INNER_ALLOW_RE = re.compile(r"^#\s*!\s*\[\s*(?:allow|expect)\s*\(", re.DOTALL)
RUST_OUTER_ALLOW_RE = re.compile(r"^#\s*\[\s*(?:allow|expect)\s*\(", re.DOTALL)

TS_BIOME_ALL_RE = re.compile(r"\bbiome-ignore-all\b(.*)$", re.DOTALL)
TS_BIOME_LINE_RE = re.compile(r"\bbiome-ignore\b(?!-all)(.*)$", re.DOTALL)
TS_DENO_FILE_RE = re.compile(r"\bdeno-lint-ignore-file\b(.*)$", re.DOTALL)
TS_DENO_LINE_RE = re.compile(r"\bdeno-lint-ignore\b(?!-file)(.*)$", re.DOTALL)
TS_ESLINT_BROAD_RE = re.compile(r"\beslint-disable\b(?!-next-line\b|-line\b)(.*)$", re.DOTALL)
TS_BIOME_SPEC_RE = re.compile(r"^([A-Za-z][A-Za-z0-9/_-]*)(?::(.*))?$", re.DOTALL)
TS_DENO_RULE_RE = re.compile(r"^([a-z][a-z0-9-]*)")


@dataclass(frozen=True)
class Violation:
    path: str
    line: int
    rule: str
    message: str


@dataclass(frozen=True)
class BanditAllowEntry:
    path: str
    skips: frozenset[str]
    exclude_dirs: frozenset[str]


@dataclass(frozen=True)
class GolangciExclusionEntry:
    config: str
    path: str | None
    path_except: str | None
    text: str | None
    source: str | None
    linters: frozenset[str]


@dataclass(frozen=True)
class Allowlist:
    ruff_global_ignores: frozenset[str]
    ruff_test_per_file_ignores: frozenset[str]
    bandit: dict[str, BanditAllowEntry]
    golangci_exclusions: dict[str, tuple[GolangciExclusionEntry, ...]]
    cargo_allows: dict[tuple[str, str], frozenset[str]]


@dataclass(frozen=True)
class _RustToken:
    kind: str
    text: str
    start_line: int
    end_line: int


@dataclass(frozen=True)
class _GoNolintCtx:
    rel_path: str
    line_no: int
    before_package: bool
    preceding: str
    trailing: str


@dataclass(frozen=True)
class _BroadCtx:
    rel_path: str
    line_no: int
    directive_desc: str
    context_text: str


PythonNoqaKey = tuple[str, tuple[str, ...], str]


def default_today() -> datetime.date:
    """Return today's UTC date for deterministic expiration checks."""
    return datetime.datetime.now(tz=datetime.UTC).date()


def _parse_iso_date(raw: str) -> datetime.date | None:
    cleaned = raw.strip()
    if not ISO_DATE_RE.match(cleaned):
        return None
    try:
        return datetime.date.fromisoformat(cleaned)
    except ValueError:
        return None


def normalize_source_line(line: str) -> str:
    """Collapse internal and surrounding whitespace for stable line fingerprinting."""
    return " ".join(line.split())


def line_fingerprint_sha256(line: str) -> str:
    """Return SHA-256 hex digest of a whitespace-normalized source line."""
    return hashlib.sha256(normalize_source_line(line).encode("utf-8")).hexdigest()


def is_generated_source_text(text: str) -> bool:
    """Return True if the first 40 lines contain a standard DO NOT EDIT header."""
    for line in text.splitlines()[:GENERATED_HEADER_LINES]:
        if GENERATED_HEADER_RE.search(line):
            return True
    return False


def _reason_len(text: str) -> int:
    cleaned = ADR_ID_RE.sub("", text)
    cleaned = EXPIRES_INLINE_RE.sub("", cleaned)
    cleaned = NON_REASON_CHARS_RE.sub("", cleaned)
    return len(cleaned)


def _has_sufficient_reason(text: str) -> bool:
    return len(re.sub(r"\s+", "", text)) >= MIN_REASON_LENGTH


def _strip_comment_delimiters(raw: str) -> str:
    text = raw.strip()
    if text.startswith("<!--"):
        text = text[4:]
        if text.endswith("-->"):
            text = text[:-3]
    elif text.startswith("/*"):
        text = text[2:]
        if text.endswith("*/"):
            text = text[:-2]
    elif text.startswith("//"):
        text = text[2:]
    elif text.startswith("#"):
        text = text[1:]
    return text.strip()


def _validate_ledger_expires(
    entry: dict[str, object],
    label: str,
    allowlist_rel: str,
    today: datetime.date,
) -> list[Violation]:
    expires = entry.get("expires")
    if expires is None or (isinstance(expires, str) and not expires.strip()):
        return [
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.missing-expires",
                f"{label}: missing required 'expires = \"YYYY-MM-DD\"'",
            )
        ]
    if not isinstance(expires, str):
        return [
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.invalid",
                f"{label}: 'expires' must be a YYYY-MM-DD string",
            )
        ]
    exp_date = _parse_iso_date(expires)
    if exp_date is None:
        return [
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.invalid",
                f"{label}: invalid 'expires' date {expires!r} (expected YYYY-MM-DD)",
            )
        ]
    if exp_date < today:
        return [
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.expired",
                f"{label}: suppression expired on {exp_date.isoformat()} "
                f"(today is {today.isoformat()})",
            )
        ]
    max_expires = today + datetime.timedelta(days=MAX_EXPIRES_HORIZON_DAYS)
    if exp_date > max_expires:
        return [
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.expires-too-far",
                f"{label}: 'expires' ({exp_date.isoformat()}) exceeds maximum horizon of "
                f"{MAX_EXPIRES_HORIZON_DAYS} days from {today.isoformat()} "
                f"(max {max_expires.isoformat()})",
            )
        ]
    return []


def _validate_ledger_metadata(
    entry: dict[str, object],
    label: str,
    allowlist_rel: str,
    repo_root: pathlib.Path,
    today: datetime.date,
) -> list[Violation]:
    violations: list[Violation] = []
    reason = entry.get("reason")
    if not isinstance(reason, str) or not _has_sufficient_reason(reason):
        violations.append(
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.invalid",
                f"{label}: missing or too-short 'reason' (>= {MIN_REASON_LENGTH} non-space chars)",
            )
        )

    violations.extend(_validate_ledger_expires(entry, label, allowlist_rel, today))

    status = entry.get("status")
    adr = entry.get("adr")
    adr_valid_existing = False
    if status != "frozen-baseline" and not adr:
        violations.append(
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.invalid",
                f"{label}: non-baseline entry must specify 'adr = \"ADR-NNNNNN\"'",
            )
        )
    if adr is not None:
        if not isinstance(adr, str):
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.invalid",
                    f"{label}: 'adr' must be a string",
                )
            )
        else:
            match = ADR_EXACT_RE.match(adr.strip())
            if not match:
                violations.append(
                    Violation(
                        allowlist_rel,
                        0,
                        "config.allowlist.invalid",
                        f"{label}: invalid ADR id {adr!r} (expected ADR-NNNNNN)",
                    )
                )
            else:
                adr_file = repo_root / "docs" / "ADR" / f"{match.group(1)}.md"
                if not adr_file.is_file():
                    violations.append(
                        Violation(
                            allowlist_rel,
                            0,
                            "config.allowlist.invalid",
                            f"{label}: {adr} referenced but docs/ADR/{match.group(1)}.md missing",
                        )
                    )
                else:
                    adr_valid_existing = True
    if status == "frozen-baseline" and not adr_valid_existing:
        raw_expires = entry.get("expires")
        exp_date = _parse_iso_date(raw_expires) if isinstance(raw_expires, str) else None
        if exp_date is not None and exp_date > FROZEN_BASELINE_MAX_EXPIRES:
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.expires-extended-without-adr",
                    f"{label}: frozen-baseline 'expires' ({exp_date.isoformat()}) exceeds ceiling "
                    f"{FROZEN_BASELINE_MAX_EXPIRES.isoformat()} and requires a valid existing 'adr = \"ADR-NNNNNN\"'",
                )
            )
    return violations


def _load_bandit_entries(
    raw_entries: list[dict[str, object]],
    allowlist_rel: str,
    repo_root: pathlib.Path,
    violations: list[Violation],
    today: datetime.date,
) -> dict[str, BanditAllowEntry]:
    result: dict[str, BanditAllowEntry] = {}
    for idx, entry in enumerate(raw_entries):
        label = f"bandit[{idx}]"
        violations.extend(_validate_ledger_metadata(entry, label, allowlist_rel, repo_root, today))
        path_val = entry.get("path")
        skips = entry.get("skips", [])
        exclude_dirs = entry.get("exclude_dirs", [])
        if (
            not isinstance(path_val, str)
            or not isinstance(skips, list)
            or not isinstance(exclude_dirs, list)
        ):
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.invalid",
                    f"{label}: invalid path/skips/exclude_dirs types",
                )
            )
            continue
        item = BanditAllowEntry(
            path=path_val,
            skips=frozenset(str(s) for s in skips),
            exclude_dirs=frozenset(str(d) for d in exclude_dirs),
        )
        if entry.get("status") == "frozen-baseline" and not entry.get("adr"):
            baseline = FROZEN_BANDIT_BASELINES.get(path_val)
            if baseline is None or not (
                item.skips <= baseline[0] and item.exclude_dirs <= baseline[1]
            ):
                violations.append(
                    Violation(
                        allowlist_rel,
                        0,
                        "config.allowlist.invalid",
                        f"{label}: {path_val!r} exceeds frozen-baseline ceiling and requires 'adr = \"ADR-NNNNNN\"'",
                    )
                )
        result[path_val] = item
    return result


def _load_golangci_entries(
    raw_entries: list[dict[str, object]],
    allowlist_rel: str,
    repo_root: pathlib.Path,
    violations: list[Violation],
    today: datetime.date,
) -> dict[str, tuple[GolangciExclusionEntry, ...]]:
    grouped: dict[str, list[GolangciExclusionEntry]] = {}
    for idx, entry in enumerate(raw_entries):
        label = f"golangci_exclusions[{idx}]"
        violations.extend(_validate_ledger_metadata(entry, label, allowlist_rel, repo_root, today))
        config = entry.get("config", ".golangci.yml")
        linters = entry.get("linters", [])
        if not isinstance(config, str) or not isinstance(linters, list):
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.invalid",
                    f"{label}: invalid config/linters types",
                )
            )
            continue
        path_val = entry.get("path")
        path_except_val = entry.get("path_except")
        text_val = entry.get("text")
        source_val = entry.get("source")
        item = GolangciExclusionEntry(
            config=config,
            path=str(path_val) if path_val is not None else None,
            path_except=str(path_except_val) if path_except_val is not None else None,
            text=str(text_val) if text_val is not None else None,
            source=str(source_val) if source_val is not None else None,
            linters=frozenset(str(linter) for linter in linters),
        )
        sig = (item.config, item.path, item.path_except, item.text, item.source, item.linters)
        if (
            entry.get("status") == "frozen-baseline"
            and not entry.get("adr")
            and sig not in FROZEN_GOLANGCI_BASELINES
        ):
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.invalid",
                    f"{label}: exclusion is not part of frozen-baseline and requires 'adr = \"ADR-NNNNNN\"'",
                )
            )
        grouped.setdefault(config, []).append(item)
    return {cfg: tuple(items) for cfg, items in grouped.items()}


def _load_cargo_entries(
    raw_entries: list[dict[str, object]],
    allowlist_rel: str,
    repo_root: pathlib.Path,
    violations: list[Violation],
    today: datetime.date,
) -> dict[tuple[str, str], frozenset[str]]:
    result: dict[tuple[str, str], frozenset[str]] = {}
    for idx, entry in enumerate(raw_entries):
        label = f"cargo_lint_allows[{idx}]"
        violations.extend(_validate_ledger_metadata(entry, label, allowlist_rel, repo_root, today))
        path_val = entry.get("path")
        section = entry.get("section")
        allows = entry.get("allows", [])
        if (
            not isinstance(path_val, str)
            or not isinstance(section, str)
            or not isinstance(allows, list)
        ):
            violations.append(
                Violation(
                    allowlist_rel,
                    0,
                    "config.allowlist.invalid",
                    f"{label}: invalid path/section/allows types",
                )
            )
            continue
        allow_set = frozenset(str(a) for a in allows)
        if entry.get("status") == "frozen-baseline" and not entry.get("adr"):
            baseline_allows = FROZEN_CARGO_BASELINES.get((path_val, section))
            if baseline_allows is None or not (allow_set <= baseline_allows):
                violations.append(
                    Violation(
                        allowlist_rel,
                        0,
                        "config.allowlist.invalid",
                        f"{label}: {(path_val, section)!r} exceeds frozen-baseline ceiling and requires 'adr = \"ADR-NNNNNN\"'",
                    )
                )
        result[(path_val, section)] = allow_set
    return result


def load_allowlist(
    allowlist_path: pathlib.Path,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
) -> tuple[Allowlist, list[Violation]]:
    """Load and validate the frozen suppression allowlist TOML file."""
    ref_today = today if today is not None else default_today()
    data = tomllib.loads(allowlist_path.read_text(encoding="utf-8"))
    try:
        rel = allowlist_path.resolve().relative_to(repo_root.resolve()).as_posix()
    except ValueError:
        rel = allowlist_path.as_posix()

    violations: list[Violation] = []
    ruff_cfg = data.get("ruff", {})
    ruff_global = frozenset(str(x) for x in ruff_cfg.get("allowed_global_ignores", []))
    ruff_test_pfi = frozenset(str(x) for x in ruff_cfg.get("allowed_test_per_file_ignores", []))
    if not (
        ruff_global <= MAX_RUFF_GLOBAL_IGNORES and ruff_test_pfi <= MAX_RUFF_TEST_PER_FILE_IGNORES
    ):
        if not isinstance(ruff_cfg, dict) or not ruff_cfg.get("adr"):
            violations.append(
                Violation(
                    rel,
                    0,
                    "config.allowlist.invalid",
                    f"[ruff] exceeds policy ceiling ({sorted(MAX_RUFF_GLOBAL_IGNORES)} / "
                    f"{sorted(MAX_RUFF_TEST_PER_FILE_IGNORES)}) without 'adr = \"ADR-NNNNNN\"'",
                )
            )
        else:
            violations.extend(
                _validate_ledger_metadata(ruff_cfg, "ruff", rel, repo_root, ref_today)
            )

    bandit = _load_bandit_entries(data.get("bandit", []), rel, repo_root, violations, ref_today)
    golangci = _load_golangci_entries(
        data.get("golangci_exclusions", []),
        rel,
        repo_root,
        violations,
        ref_today,
    )
    cargo = _load_cargo_entries(
        data.get("cargo_lint_allows", []),
        rel,
        repo_root,
        violations,
        ref_today,
    )
    return (
        Allowlist(
            ruff_global_ignores=ruff_global,
            ruff_test_per_file_ignores=ruff_test_pfi,
            bandit=bandit,
            golangci_exclusions=golangci,
            cargo_allows=cargo,
        ),
        violations,
    )


def is_excluded_source_path(rel_path: str) -> bool:
    """Return True if a repository-relative path is outside source governance."""
    norm = rel_path.lstrip("./")
    if norm.startswith(("stopped-using-k8s/", "scripts/harness/corpus/")):
        return True
    if norm.endswith((".pb.go", ".connect.go", "server.gen.go", ".md")):
        return True
    if norm.startswith(("gen/", "alt-frontend-sv/src/lib/gen/")) or "/gen/" in norm:
        return True
    return not norm.endswith(SOURCE_EXTENSIONS)


def is_test_path_pattern(pattern: str) -> bool:
    """Return True only when a ruff per-file-ignores glob targets test paths."""
    pat = pattern.strip().lstrip("./")
    if not pat or any(ch in pat for ch in ("{", "}", ",")):
        return False
    segments = [seg for seg in pat.split("/") if seg]
    if "tests" in segments:
        return True
    filename = segments[-1]
    return (filename.startswith("test_") or filename.endswith("_test.py")) and filename.endswith(
        ".py"
    )


def _check_ruff_per_file_ignores(
    rel_path: str,
    table_name: str,
    mapping: object,
    allowlist: Allowlist,
) -> list[Violation]:
    violations: list[Violation] = []
    if not isinstance(mapping, dict):
        return violations
    for pattern, rules in mapping.items():
        if not is_test_path_pattern(str(pattern)):
            violations.append(
                Violation(
                    rel_path,
                    0,
                    "config.pyproject.ruff-per-file-ignores-path",
                    f"[tool.ruff.lint.{table_name}] pattern {pattern!r} is not a test-only path",
                )
            )
        rule_list = rules if isinstance(rules, list) else [rules]
        disallowed = [
            str(r)
            for r in rule_list
            if str(r) not in allowlist.ruff_test_per_file_ignores
            or str(r).upper() == "ALL"
            or "*" in str(r)
        ]
        if disallowed:
            violations.append(
                Violation(
                    rel_path,
                    0,
                    "config.pyproject.ruff-per-file-ignores-rule",
                    f"[tool.ruff.lint.{table_name}] pattern {pattern!r} has disallowed rules "
                    f"{disallowed} (allowed: {sorted(allowlist.ruff_test_per_file_ignores)})",
                )
            )
    return violations


def _check_bandit_section(
    rel_path: str,
    bandit_cfg: object,
    allowlist: Allowlist,
) -> list[Violation]:
    if not isinstance(bandit_cfg, dict):
        return []
    violations: list[Violation] = []
    allowed = allowlist.bandit.get(rel_path)
    if allowed is None:
        return [
            Violation(
                rel_path,
                0,
                "config.pyproject.bandit-unallowlisted",
                "[tool.bandit] is not allowlisted for this project in suppression_allowlist.toml",
            )
        ]
    skips = {str(s) for s in bandit_cfg.get("skips", [])}
    extra_skips = sorted(skips - allowed.skips)
    if extra_skips:
        violations.append(
            Violation(
                rel_path,
                0,
                "config.pyproject.bandit-skip-disallowed",
                f"[tool.bandit].skips adds unallowlisted skips {extra_skips}",
            )
        )
    exclude_dirs = {str(d) for d in bandit_cfg.get("exclude_dirs", [])}
    extra_dirs = sorted(exclude_dirs - allowed.exclude_dirs)
    if extra_dirs:
        violations.append(
            Violation(
                rel_path,
                0,
                "config.pyproject.bandit-exclude-dir-disallowed",
                f"[tool.bandit].exclude_dirs adds unallowlisted dirs {extra_dirs}",
            )
        )
    return violations


def check_pyproject(rel_path: str, text: str, allowlist: Allowlist) -> list[Violation]:
    """Validate Ruff and Bandit suppression settings in a pyproject.toml file."""
    data = tomllib.loads(text)
    tool = data.get("tool", {})
    if not isinstance(tool, dict):
        return []

    violations: list[Violation] = []
    ruff = tool.get("ruff", {})
    if isinstance(ruff, dict):
        violations.extend(
            Violation(
                rel_path,
                0,
                "config.pyproject.legacy-ruff-key",
                f"Legacy [tool.ruff].{key} is forbidden; use [tool.ruff.lint]",
            )
            for key in LEGACY_RUFF_KEYS
            if key in ruff
        )
        lint_cfg = ruff.get("lint", {})
        if isinstance(lint_cfg, dict):
            if "extend-ignore" in lint_cfg:
                violations.append(
                    Violation(
                        rel_path,
                        0,
                        "config.pyproject.ruff-extend-ignore",
                        "[tool.ruff.lint].extend-ignore is forbidden",
                    )
                )
            ignores = lint_cfg.get("ignore", [])
            if isinstance(ignores, list):
                bad_ignores = [
                    str(code)
                    for code in ignores
                    if str(code) not in allowlist.ruff_global_ignores
                    or str(code).upper() == "ALL"
                    or "*" in str(code)
                ]
                if bad_ignores:
                    violations.append(
                        Violation(
                            rel_path,
                            0,
                            "config.pyproject.ruff-ignore-disallowed",
                            f"[tool.ruff.lint].ignore contains disallowed codes {bad_ignores} "
                            f"(allowed: {sorted(allowlist.ruff_global_ignores)})",
                        )
                    )
            for table_name in ("per-file-ignores", "extend-per-file-ignores"):
                if table_name in lint_cfg:
                    violations.extend(
                        _check_ruff_per_file_ignores(
                            rel_path,
                            table_name,
                            lint_cfg[table_name],
                            allowlist,
                        )
                    )

    if "bandit" in tool:
        violations.extend(_check_bandit_section(rel_path, tool["bandit"], allowlist))

    return violations


def _golangci_rule_allowed(
    rule: dict[str, object],
    allowed_rules: tuple[GolangciExclusionEntry, ...],
) -> bool:
    known_keys = {"path", "path-except", "text", "source", "linters"}
    if not set(rule.keys()) <= known_keys:
        return False
    rule_linters = rule.get("linters")
    if not isinstance(rule_linters, list) or not rule_linters:
        return False
    rule_linter_set = {str(x) for x in rule_linters}
    for entry in allowed_rules:
        if (
            rule.get("path") == entry.path
            and rule.get("path-except") == entry.path_except
            and rule.get("text") == entry.text
            and rule.get("source") == entry.source
            and (rule_linter_set <= entry.linters or "all" in entry.linters)
        ):
            return True
    return False


def check_golangci(rel_path: str, text: str, allowlist: Allowlist) -> list[Violation]:
    """Validate .golangci.yml disables and exclusion rules against the allowlist."""
    import yaml  # noqa: PLC0415 - lazy import keeps non-YAML checks usable without PyYAML

    data = yaml.safe_load(text) or {}
    if not isinstance(data, dict):
        return []

    violations: list[Violation] = []
    linters = data.get("linters", {})
    if isinstance(linters, dict):
        if linters.get("disable"):
            violations.append(
                Violation(
                    rel_path,
                    0,
                    "config.golangci.disable-forbidden",
                    "linters.disable is forbidden in .golangci.yml",
                )
            )
        if linters.get("disable-all"):
            violations.append(
                Violation(
                    rel_path,
                    0,
                    "config.golangci.disable-forbidden",
                    "linters.disable-all is forbidden in .golangci.yml",
                )
            )
        exclusions = linters.get("exclusions", {})
        if isinstance(exclusions, dict):
            violations.extend(
                Violation(
                    rel_path,
                    0,
                    "config.golangci.exclusion-unallowlisted",
                    f"linters.exclusions.{extra_key} is not allowlisted",
                )
                for extra_key in ("paths", "presets")
                if exclusions.get(extra_key)
            )
            allowed_rules = allowlist.golangci_exclusions.get(rel_path, ())
            for idx, rule in enumerate(exclusions.get("rules") or []):
                if not isinstance(rule, dict) or not _golangci_rule_allowed(rule, allowed_rules):
                    violations.append(
                        Violation(
                            rel_path,
                            0,
                            "config.golangci.exclusion-unallowlisted",
                            f"linters.exclusions.rules[{idx}] is not in suppression_allowlist.toml: "
                            f"{rule!r}",
                        )
                    )

    issues = data.get("issues", {})
    if isinstance(issues, dict) and issues.get("exclude-rules"):
        violations.append(
            Violation(
                rel_path,
                0,
                "config.golangci.exclusion-unallowlisted",
                "legacy issues.exclude-rules is forbidden; use frozen linters.exclusions.rules",
            )
        )
    return violations


def _is_cargo_allow(value: object) -> bool:
    if isinstance(value, str):
        return value.strip() == "allow"
    if isinstance(value, dict):
        return str(value.get("level", "")).strip() == "allow"
    return False


def _collect_cargo_lint_tables(data: dict[str, object]) -> list[tuple[str, dict[str, object]]]:
    tables: list[tuple[str, dict[str, object]]] = []
    workspace = data.get("workspace")
    ws_lints = workspace.get("lints") if isinstance(workspace, dict) else None
    for prefix, root_obj in (
        ("lints", data.get("lints")),
        ("workspace.lints", ws_lints),
    ):
        if not isinstance(root_obj, dict):
            continue
        for tool_name, tool_table in root_obj.items():
            if isinstance(tool_table, dict):
                tables.append((f"{prefix}.{tool_name}", tool_table))
    return tables


def check_cargo_toml(rel_path: str, text: str, allowlist: Allowlist) -> list[Violation]:
    """Validate [lints.*] and [workspace.lints.*] allow entries in Cargo.toml."""
    data = tomllib.loads(text)
    violations: list[Violation] = []
    for section, tool_table in _collect_cargo_lint_tables(data):
        allowed_set = allowlist.cargo_allows.get((rel_path, section), frozenset())
        for lint_name, value in tool_table.items():
            if _is_cargo_allow(value) and lint_name not in allowed_set:
                violations.append(
                    Violation(
                        rel_path,
                        0,
                        "config.cargo.lint-allow-unallowlisted",
                        f"[{section}].{lint_name} = allow is not allowlisted in "
                        f"suppression_allowlist.toml",
                    )
                )
    return violations


def _extract_broad_expires_dates(context_text: str) -> list[datetime.date]:
    dates: list[datetime.date] = []
    for raw_token in EXPIRES_INLINE_RE.findall(context_text):
        parsed = _parse_iso_date(raw_token.rstrip("),.;]}"))
        if parsed is not None:
            dates.append(parsed)
    return dates


def _check_broad_suppression(
    ctx: _BroadCtx,
    repo_root: pathlib.Path,
    today: datetime.date | None = None,
) -> list[Violation]:
    ref_today = today if today is not None else default_today()
    adr_matches = ADR_ID_RE.findall(ctx.context_text)
    if not adr_matches:
        return [
            Violation(
                ctx.rel_path,
                ctx.line_no,
                "source.broad.missing-adr",
                f"{ctx.directive_desc} requires an ADR-NNNNNN reference, reason comment, "
                f"and 'expires: YYYY-MM-DD'",
            )
        ]
    violations: list[Violation] = []
    for adr_num in adr_matches:
        adr_path = repo_root / "docs" / "ADR" / f"{adr_num}.md"
        if not adr_path.is_file():
            violations.append(
                Violation(
                    ctx.rel_path,
                    ctx.line_no,
                    "source.broad.nonexistent-adr",
                    f"{ctx.directive_desc} cites ADR-{adr_num} but docs/ADR/{adr_num}.md does not exist",
                )
            )
    if _reason_len(ctx.context_text) < MIN_REASON_LENGTH:
        violations.append(
            Violation(
                ctx.rel_path,
                ctx.line_no,
                "source.broad.missing-reason",
                f"{ctx.directive_desc} requires a reason comment (>= {MIN_REASON_LENGTH} non-space "
                f"chars alongside ADR-NNNNNN)",
            )
        )
    exp_dates = _extract_broad_expires_dates(ctx.context_text)
    if not exp_dates:
        violations.append(
            Violation(
                ctx.rel_path,
                ctx.line_no,
                "source.broad.missing-expires",
                f"{ctx.directive_desc} requires 'expires: YYYY-MM-DD' in comment context",
            )
        )
    else:
        if min(exp_dates) < ref_today:
            earliest = min(exp_dates)
            violations.append(
                Violation(
                    ctx.rel_path,
                    ctx.line_no,
                    "source.broad.expired",
                    f"{ctx.directive_desc} expired on {earliest.isoformat()} "
                    f"(today is {ref_today.isoformat()})",
                )
            )
        max_expires = ref_today + datetime.timedelta(days=MAX_EXPIRES_HORIZON_DAYS)
        if max(exp_dates) > max_expires:
            furthest = max(exp_dates)
            violations.append(
                Violation(
                    ctx.rel_path,
                    ctx.line_no,
                    "source.broad.expires-too-far",
                    f"{ctx.directive_desc} 'expires' ({furthest.isoformat()}) exceeds maximum "
                    f"horizon of {MAX_EXPIRES_HORIZON_DAYS} days from {ref_today.isoformat()} "
                    f"(max {max_expires.isoformat()})",
                )
            )
    return violations


def extract_python_noqa_reason(tail: str) -> str:
    """Extract human reason text following '# noqa: CODE(S)' after removing secondary directives."""
    cleaned = PY_SECONDARY_DIRECTIVE_RE.sub("", tail).strip()
    cleaned = re.sub(r"^(?:--|—|-|#|:|;|\s)+", "", cleaned).strip()
    if cleaned.startswith("(") and cleaned.endswith(")"):
        cleaned = cleaned[1:-1].strip()
    return cleaned


def check_sql_suppression_reason(
    rel_path: str,
    line_no: int,
    codes: tuple[str, ...],
    reason: str,
) -> list[Violation]:
    """Enforce SQL kind and trust-boundary classification on S608/G201/G202/B608 suppressions."""
    sql_codes = [c for c in codes if c.upper() in SQL_SUPPRESSION_CODES]
    if not sql_codes:
        return []
    label = ",".join(sql_codes)
    violations: list[Violation] = []
    if SQL_FORBIDDEN_VALUE_RE.search(reason):
        violations.append(
            Violation(
                rel_path,
                line_no,
                "source.sql.value-interpolation-forbidden",
                f"SQL suppression ({label}) forbids value interpolation; use bind parameters",
            )
        )
    elif not SQL_ALLOWED_KIND_RE.search(reason):
        violations.append(
            Violation(
                rel_path,
                line_no,
                "source.sql.missing-kind",
                f"SQL suppression ({label}) reason must classify kind "
                f"(identifier/識別子 or not-sql/非SQL)",
            )
        )
    if SQL_FORBIDDEN_UNTRUSTED_RE.search(reason):
        violations.append(
            Violation(
                rel_path,
                line_no,
                "source.sql.untrusted-boundary-forbidden",
                f"SQL suppression ({label}) forbids untrusted/external trust boundary",
            )
        )
    elif not SQL_ALLOWED_TRUST_RE.search(reason):
        violations.append(
            Violation(
                rel_path,
                line_no,
                "source.sql.missing-trust-boundary",
                f"SQL suppression ({label}) reason must state trust boundary "
                f"(trusted/internal/constant/allowlist/信頼境界内)",
            )
        )
    return violations


def load_python_noqa_baseline(baseline_path: pathlib.Path) -> dict[PythonNoqaKey, int]:
    """Load frozen multiplicity ledger of pre-existing reasonless Python # noqa occurrences."""
    data = tomllib.loads(baseline_path.read_text(encoding="utf-8"))
    result: dict[PythonNoqaKey, int] = {}
    for entry in data.get("entries", []):
        if not isinstance(entry, dict):
            continue
        path_val = entry.get("path")
        codes_val = entry.get("codes")
        sha_val = entry.get("line_sha256")
        count_val = entry.get("count", 1)
        if (
            isinstance(path_val, str)
            and isinstance(codes_val, list)
            and isinstance(sha_val, str)
            and isinstance(count_val, int)
            and count_val > 0
        ):
            codes = tuple(sorted({str(c).strip().upper() for c in codes_val if str(c).strip()}))
            key: PythonNoqaKey = (path_val, codes, sha_val)
            result[key] = result.get(key, 0) + count_val
    return result


def _tokenize_python_comments(text: str) -> list[tuple[int, str]]:
    try:
        return [
            (tok.start[0], tok.string)
            for tok in tokenize.generate_tokens(io.StringIO(text).readline)
            if tok.type == tokenize.COMMENT
        ]
    except (tokenize.TokenError, IndentationError):
        return [
            (idx, line.lstrip())
            for idx, line in enumerate(text.splitlines(), 1)
            if line.lstrip().startswith("#")
        ]


def _preceding_comment_text(comment_by_line: dict[int, str], start_line: int) -> str:
    parts: list[str] = []
    cursor = start_line - 1
    while cursor in comment_by_line:
        parts.append(comment_by_line[cursor])
        cursor -= 1
    parts.reverse()
    return "\n".join(parts)


def _python_file_noqa_trailing_reason(rest: str) -> str:
    stripped = rest.strip()
    if stripped.startswith(":"):
        after_colon = stripped[1:].strip()
        for sep in ("#", "--", " - ", "—"):
            if sep in after_colon:
                return after_colon.split(sep, 1)[1].strip()
        return ""
    return stripped


def _extract_python_nosec_reason(tail: str) -> str:
    cleaned = PY_NOSEC_SECONDARY_RE.sub("", tail).strip()
    cleaned = re.sub(r"^(?:--|—|-|#|:|;|\s)+", "", cleaned).strip()
    if cleaned.startswith("(") and cleaned.endswith(")"):
        cleaned = cleaned[1:-1].strip()
    return cleaned


def _check_python_inline_noqa(
    rel_path: str,
    line_no: int,
    rest: str,
    src_line: str,
    state: tuple[dict[PythonNoqaKey, int], Counter[PythonNoqaKey]],
) -> tuple[list[Violation], str]:
    codes_match = PY_NOQA_CODES_RE.match(rest)
    if not codes_match or PY_TRAILING_BAD_CODE_RE.search(rest):
        return (
            [
                Violation(
                    rel_path,
                    line_no,
                    "source.python.noqa-missing-code",
                    "inline '# noqa' must specify concrete rule code(s) (e.g. '# noqa: S603')",
                )
            ],
            "",
        )
    codes = tuple(sorted({c.strip().upper() for c in codes_match.group(1).split(",") if c.strip()}))
    tail = rest[codes_match.end(1) :]
    reason = extract_python_noqa_reason(tail)
    violations: list[Violation] = []
    baseline, seen_reasonless = state
    if not _has_sufficient_reason(reason):
        key: PythonNoqaKey = (rel_path, codes, line_fingerprint_sha256(src_line))
        seen_reasonless[key] += 1
        if seen_reasonless[key] > baseline.get(key, 0):
            violations.append(
                Violation(
                    rel_path,
                    line_no,
                    "source.python.noqa-missing-reason",
                    f"inline '# noqa: {', '.join(codes)}' requires a reason comment "
                    f"(e.g. '-- <reason>' or '# <reason>', >= {MIN_REASON_LENGTH} non-space chars)",
                )
            )
    violations.extend(check_sql_suppression_reason(rel_path, line_no, codes, reason))
    return violations, reason


@dataclass(frozen=True)
class _PythonLineLoc:
    rel_path: str
    line_no: int
    src_line: str


def _check_python_nosec(
    loc: _PythonLineLoc,
    rest: str,
    shared_reason: str,
    state: tuple[dict[PythonNoqaKey, int], Counter[PythonNoqaKey]],
) -> list[Violation]:
    codes_match = PY_NOSEC_CODES_RE.match(rest)
    if not codes_match:
        return [
            Violation(
                loc.rel_path,
                loc.line_no,
                "source.python.nosec-missing-code",
                "inline '# nosec' must specify concrete Bandit rule code(s) (e.g. '# nosec B310 -- <reason>')",
            )
        ]
    codes = tuple(
        sorted({c.upper() for c in re.findall(r"B\d{3}", codes_match.group(1), re.IGNORECASE)})
    )
    own_reason = _extract_python_nosec_reason(rest[codes_match.end(1) :])
    effective_reason = own_reason if _has_sufficient_reason(own_reason) else shared_reason
    violations: list[Violation] = []
    baseline, seen_reasonless = state
    if not _has_sufficient_reason(effective_reason):
        key: PythonNoqaKey = (loc.rel_path, codes, line_fingerprint_sha256(loc.src_line))
        seen_reasonless[key] += 1
        if seen_reasonless[key] > baseline.get(key, 0):
            violations.append(
                Violation(
                    loc.rel_path,
                    loc.line_no,
                    "source.python.nosec-missing-reason",
                    f"inline '# nosec {' '.join(codes)}' requires a reason comment "
                    f"(e.g. '-- <reason>' or '# <reason>', >= {MIN_REASON_LENGTH} non-space chars)",
                )
            )
    violations.extend(
        check_sql_suppression_reason(loc.rel_path, loc.line_no, codes, effective_reason)
    )
    return violations


def check_python_source(
    rel_path: str,
    text: str,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
    python_noqa_baseline: dict[PythonNoqaKey, int] | None = None,
) -> list[Violation]:
    """Enforce Python # ruff: noqa, # flake8: noqa, inline # noqa, and # nosec rules."""
    comments = _tokenize_python_comments(text)
    lines = text.splitlines()
    comment_by_line = {ln: _strip_comment_delimiters(raw) for ln, raw in comments}
    baseline = python_noqa_baseline if python_noqa_baseline is not None else {}
    seen_reasonless: Counter[PythonNoqaKey] = Counter()
    violations: list[Violation] = []

    for line_no, raw in comments:
        file_match = PY_FILE_NOQA_RE.match(raw)
        if file_match:
            trailing = _python_file_noqa_trailing_reason(file_match.group(1))
            preceding = _preceding_comment_text(comment_by_line, line_no)
            context = f"{preceding}\n{trailing}".strip()
            violations.extend(
                _check_broad_suppression(
                    _BroadCtx(rel_path, line_no, "file-level Python noqa", context),
                    repo_root,
                    today,
                )
            )
            continue

        src_line = lines[line_no - 1] if 1 <= line_no <= len(lines) else raw
        shared_reason = ""
        inline_match = PY_INLINE_NOQA_RE.search(raw)
        if inline_match:
            noqa_violations, shared_reason = _check_python_inline_noqa(
                rel_path,
                line_no,
                inline_match.group(1),
                src_line,
                (baseline, seen_reasonless),
            )
            violations.extend(noqa_violations)

        nosec_match = PY_NOSEC_RE.search(raw)
        if nosec_match:
            violations.extend(
                _check_python_nosec(
                    _PythonLineLoc(rel_path, line_no, src_line),
                    nosec_match.group(1),
                    shared_reason,
                    (baseline, seen_reasonless),
                )
            )
    return violations


def _skip_quoted(text: str, pos: int, quote: str) -> tuple[int, int]:
    """Advance past a single-line or escaped quoted string; return (new_pos, newlines)."""
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
    """Extract (start_line, end_line, raw_comment, before_package) from Go source."""
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
            if end == -1:
                end = n
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


def _extract_go_nolint_reason(rest: str, trailing_same_line: str) -> str:
    body = rest
    if body.endswith("*/"):
        body = body[:-2]
    if "//" in body:
        return body.split("//", 1)[1].strip()
    if "//" in trailing_same_line:
        return trailing_same_line.split("//", 1)[1].strip()
    return ""


def _check_go_nolint_comment(
    ctx: _GoNolintCtx,
    match: re.Match[str],
    repo_root: pathlib.Path,
    today: datetime.date | None = None,
) -> list[Violation]:
    linters_raw = match.group(1)
    rest = match.group(2)
    linter_names = (
        [name.strip().lower() for name in linters_raw.split(",") if name.strip()]
        if linters_raw
        else []
    )
    if "gosec" in linter_names:
        return [
            Violation(
                ctx.rel_path,
                ctx.line_no,
                "source.go.nolint-gosec-forbidden",
                "use '// #nosec G<NNN> -- <reason>' instead of '//nolint:gosec' "
                "(ARCH-01 single gosec suppression format)",
            )
        ]
    if ctx.before_package or not linter_names or "all" in linter_names:
        reason_part = _extract_go_nolint_reason(rest, ctx.trailing)
        combined = f"{ctx.preceding}\n{reason_part}".strip()
        desc = "file-level Go nolint" if ctx.before_package else "blanket Go nolint"
        return _check_broad_suppression(
            _BroadCtx(ctx.rel_path, ctx.line_no, desc, combined),
            repo_root,
            today,
        )

    reason = _extract_go_nolint_reason(rest, ctx.trailing)
    if not _has_sufficient_reason(reason):
        return [
            Violation(
                ctx.rel_path,
                ctx.line_no,
                "source.go.nolint-missing-reason",
                f"//nolint:{linters_raw} requires same-line '// <reason>' "
                f"(>= {MIN_REASON_LENGTH} non-space chars)",
            )
        ]
    return []


def _check_go_nosec_comment(rel_path: str, line_no: int, rest: str) -> list[Violation]:
    cleaned = rest[:-2].strip() if rest.rstrip().endswith("*/") else rest.strip()
    with_reason = GO_NOSEC_RULES_RE.match(cleaned)
    if with_reason:
        reason = with_reason.group(2).strip()
        if not _has_sufficient_reason(reason):
            return [
                Violation(
                    rel_path,
                    line_no,
                    "source.go.nosec-missing-reason",
                    f"#nosec requires '-- <reason>' or '// <reason>' "
                    f"(>= {MIN_REASON_LENGTH} non-space chars)",
                )
            ]
        codes = tuple(re.findall(r"G\d{3}", with_reason.group(1)))
        return check_sql_suppression_reason(rel_path, line_no, codes, reason)
    if GO_NOSEC_RULE_ONLY_RE.match(cleaned):
        return [
            Violation(
                rel_path,
                line_no,
                "source.go.nosec-missing-reason",
                f"#nosec requires '-- <reason>' or '// <reason>' "
                f"(>= {MIN_REASON_LENGTH} non-space chars)",
            )
        ]
    return [
        Violation(
            rel_path,
            line_no,
            "source.go.nosec-missing-rule",
            "#nosec must specify concrete G<NNN> rule(s) and '-- <reason>' or '// <reason>'",
        )
    ]


def check_go_source(
    rel_path: str,
    text: str,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
) -> list[Violation]:
    """Enforce Go //nolint and // #nosec governance rules."""
    comments = _scan_go_comments(text)
    comment_by_line: dict[int, str] = {}
    comments_on_line: dict[int, list[str]] = {}
    for start_line, end_line, raw, _ in comments:
        comments_on_line.setdefault(start_line, []).append(raw)
        if start_line == end_line:
            comment_by_line[start_line] = _strip_comment_delimiters(raw)

    violations: list[Violation] = []
    for start_line, _, raw, before_package in comments:
        same_line_list = comments_on_line.get(start_line, [])
        idx_in_line = same_line_list.index(raw)
        trailing = " ".join(same_line_list[idx_in_line + 1 :])

        nolint_match = GO_NOLINT_RE.match(raw)
        if nolint_match:
            ctx = _GoNolintCtx(
                rel_path=rel_path,
                line_no=start_line,
                before_package=before_package,
                preceding=_preceding_comment_text(comment_by_line, start_line),
                trailing=trailing,
            )
            violations.extend(_check_go_nolint_comment(ctx, nolint_match, repo_root, today))
            continue

        nosec_match = GO_NOSEC_RE.match(raw)
        if nosec_match:
            violations.extend(_check_go_nosec_comment(rel_path, start_line, nosec_match.group(1)))
    return violations


def _skip_rust_raw_string(text: str, pos: int) -> tuple[int, int] | None:
    """If pos starts a Rust raw string (r#"... "#, br#..., cr#...), return (end, newlines)."""
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
    """Skip a possibly nested Rust /* ... */ block comment."""
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
    """Return end index if pos starts a Rust char literal, else None (lifetime)."""
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


def _match_rust_attr_start(text: str, pos: int) -> tuple[bool, int] | None:
    """If pos ('#') starts a Rust attribute (allowing optional whitespace), return (is_inner, bracket_pos)."""
    n = len(text)
    if pos >= n or text[pos] != "#":
        return None
    j = pos + 1
    while j < n and text[j].isspace():
        j += 1
    is_inner = False
    if j < n and text[j] == "!":
        is_inner = True
        j += 1
        while j < n and text[j].isspace():
            j += 1
    if j < n and text[j] == "[":
        return is_inner, j
    return None


def _read_rust_attribute(
    text: str,
    pos: int,
    start_line: int,
    *,
    is_inner: bool,
    bracket_pos: int,
) -> tuple[_RustToken, int, int]:
    """Read a balanced Rust #[...] or #![...] attribute token (supporting whitespace after #/!)."""
    n = len(text)
    line = start_line + text.count("\n", pos, bracket_pos)
    i = bracket_pos
    depth = 0
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
    kind = "ATTR_INNER" if is_inner else "ATTR_OUTER"
    return _RustToken(kind, text[pos:i], start_line, line), i, line


def _read_rust_word(text: str, pos: int) -> int:
    n = len(text)
    while pos < n and (text[pos].isalnum() or text[pos] == "_"):
        pos += 1
    return pos


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
    if ch == "#":
        attr_start = _match_rust_attr_start(text, pos)
        if attr_start is not None:
            is_inner, bracket_pos = attr_start
            return _read_rust_attribute(text, pos, line, is_inner=is_inner, bracket_pos=bracket_pos)
    return None


def _scan_rust_string_or_word(
    text: str,
    pos: int,
    line: int,
) -> tuple[_RustToken | None, int, int]:
    ch = text[pos]
    if ch == '"':
        end, nl = _skip_quoted(text, pos, '"')
        return None, end, line + nl
    if ch in ("r", "b", "c"):
        raw = _skip_rust_raw_string(text, pos)
        if raw is not None:
            return None, raw[0], line + raw[1]
        if ch in ("b", "c") and pos + 1 < len(text) and text[pos + 1] == '"':
            end, nl = _skip_quoted(text, pos + 1, '"')
            return None, end, line + nl
    if ch == "'":
        char_end = _skip_rust_char_literal(text, pos)
        return None, (char_end if char_end is not None else pos + 1), line
    if ch.isalpha() or ch == "_":
        end = _read_rust_word(text, pos)
        return _RustToken("WORD", text[pos:end], line, line), end, line
    return _RustToken("PUNCT", ch, line, line), pos + 1, line


def _scan_rust_tokens(text: str) -> list[_RustToken]:
    """Tokenize Rust source into comments, attributes, words, and punctuation."""
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
    """Return True if the outer attribute at attr_index decorates a `mod <ident>` item."""
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
    if j + 1 < n and tokens[j].kind == "WORD" and tokens[j].text == "mod":
        return tokens[j + 1].kind == "WORD"
    return False


def _rust_context_for_attr(tokens: list[_RustToken], attr_index: int) -> str:
    """Collect same-line trailing comments and contiguous comments above an attribute block."""
    attr = tokens[attr_index]
    parts: list[str] = []

    # Walk backward past adjacent outer attributes and contiguous comment lines.
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
    parts.extend(preceding)

    # Trailing comment on the same line as the attribute end.
    j = attr_index + 1
    while j < len(tokens) and tokens[j].start_line == attr.end_line:
        if tokens[j].kind == "COMMENT":
            parts.append(_strip_comment_delimiters(tokens[j].text))
        j += 1
    return "\n".join(parts)


def check_rust_source(
    rel_path: str,
    text: str,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
) -> list[Violation]:
    """Enforce Rust #![allow(...)] and mod-level #[allow(...)] governance."""
    tokens = _scan_rust_tokens(text)
    violations: list[Violation] = []
    for idx, tok in enumerate(tokens):
        if tok.kind == "ATTR_INNER" and RUST_INNER_ALLOW_RE.match(tok.text):
            ctx = _rust_context_for_attr(tokens, idx)
            violations.extend(
                _check_broad_suppression(
                    _BroadCtx(
                        rel_path,
                        tok.start_line,
                        "Rust crate/module inner #![allow(...)]",
                        ctx,
                    ),
                    repo_root,
                    today,
                )
            )
        elif (
            tok.kind == "ATTR_OUTER"
            and RUST_OUTER_ALLOW_RE.match(tok.text)
            and _rust_attr_applies_to_mod(tokens, idx)
        ):
            ctx = _rust_context_for_attr(tokens, idx)
            violations.extend(
                _check_broad_suppression(
                    _BroadCtx(
                        rel_path,
                        tok.start_line,
                        "Rust module-level #[allow(...)] on mod",
                        ctx,
                    ),
                    repo_root,
                    today,
                )
            )
    return violations


def _skip_ts_template_literal(text: str, pos: int) -> tuple[int, int]:
    """Skip a JS/TS backtick template literal; return (new_pos, newlines)."""
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


def _scan_ts_comments(text: str, is_svelte: bool) -> list[tuple[int, int, str]]:
    """Extract (start_line, end_line, raw_comment) from TS/JS/Svelte source."""
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
            i, nl = _skip_ts_template_literal(text, i)
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


def _check_ts_biome_line(rel_path: str, line_no: int, rest: str) -> list[Violation]:
    stripped = rest.strip()
    spec = TS_BIOME_SPEC_RE.match(stripped)
    if not spec:
        return [
            Violation(
                rel_path,
                line_no,
                "source.biome.ignore-missing-rule",
                "biome-ignore must specify a lint rule/category and ': <reason>'",
            )
        ]
    reason = (spec.group(2) or "").strip()
    if not _has_sufficient_reason(reason):
        return [
            Violation(
                rel_path,
                line_no,
                "source.biome.ignore-missing-reason",
                f"biome-ignore requires ': <reason>' (>= {MIN_REASON_LENGTH} non-space chars)",
            )
        ]
    return []


def _check_ts_broad_directive(
    ctx: _BroadCtx,
    preceding: str,
    repo_root: pathlib.Path,
    today: datetime.date | None = None,
) -> list[Violation] | None:
    body = ctx.context_text
    biome_all = TS_BIOME_ALL_RE.search(body)
    if biome_all:
        trailing = biome_all.group(1)
        if ":" in trailing:
            trailing = trailing.split(":", 1)[1]
        return _check_broad_suppression(
            _BroadCtx(
                ctx.rel_path,
                ctx.line_no,
                "file-level biome-ignore-all",
                f"{preceding}\n{trailing}".strip(),
            ),
            repo_root,
            today,
        )
    deno_file = TS_DENO_FILE_RE.search(body)
    if deno_file:
        return _check_broad_suppression(
            _BroadCtx(
                ctx.rel_path,
                ctx.line_no,
                "file-level deno-lint-ignore-file",
                f"{preceding}\n{deno_file.group(1)}".strip(),
            ),
            repo_root,
            today,
        )
    eslint_broad = TS_ESLINT_BROAD_RE.search(body)
    if eslint_broad:
        trailing = eslint_broad.group(1)
        reason_part = trailing.split("--", 1)[1] if "--" in trailing else ""
        return _check_broad_suppression(
            _BroadCtx(
                ctx.rel_path,
                ctx.line_no,
                "file/block-level eslint-disable",
                f"{preceding}\n{reason_part}".strip(),
            ),
            repo_root,
            today,
        )
    return None


def check_ts_source(
    rel_path: str,
    text: str,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
) -> list[Violation]:
    """Enforce Biome, Deno lint, and ESLint suppression governance."""
    is_svelte = rel_path.endswith(".svelte")
    comments = _scan_ts_comments(text, is_svelte=is_svelte)
    comment_by_line = {
        start: _strip_comment_delimiters(raw) for start, end, raw in comments if start == end
    }
    violations: list[Violation] = []
    for start_line, _, raw in comments:
        body = _strip_comment_delimiters(raw)
        preceding = _preceding_comment_text(comment_by_line, start_line)

        broad_result = _check_ts_broad_directive(
            _BroadCtx(rel_path, start_line, "", body),
            preceding,
            repo_root,
            today,
        )
        if broad_result is not None:
            violations.extend(broad_result)
            continue

        biome_line = TS_BIOME_LINE_RE.search(body)
        if biome_line:
            violations.extend(_check_ts_biome_line(rel_path, start_line, biome_line.group(1)))
            continue

        deno_line = TS_DENO_LINE_RE.search(body)
        if deno_line:
            rest = deno_line.group(1).strip()
            if not TS_DENO_RULE_RE.match(rest):
                violations.append(
                    Violation(
                        rel_path,
                        start_line,
                        "source.deno.ignore-missing-rule",
                        "deno-lint-ignore must specify at least one concrete rule name",
                    )
                )
    return violations


def check_source_file(
    rel_path: str,
    text: str,
    repo_root: pathlib.Path,
    *,
    today: datetime.date | None = None,
    python_noqa_baseline: dict[PythonNoqaKey, int] | None = None,
) -> list[Violation]:
    """Dispatch source-code suppression checks by file extension."""
    if is_excluded_source_path(rel_path):
        return []
    if rel_path.endswith(".py"):
        return check_python_source(
            rel_path,
            text,
            repo_root,
            today=today,
            python_noqa_baseline=python_noqa_baseline,
        )
    if rel_path.endswith(".go"):
        return check_go_source(rel_path, text, repo_root, today=today)
    if rel_path.endswith(".rs"):
        return check_rust_source(rel_path, text, repo_root, today=today)
    if rel_path.endswith(TS_EXTENSIONS):
        return check_ts_source(rel_path, text, repo_root, today=today)
    return []


def _is_safe_repo_file(full_path: pathlib.Path, repo_root: pathlib.Path) -> bool:
    if full_path.is_symlink():
        return False
    try:
        resolved = full_path.resolve()
        if not resolved.is_relative_to(repo_root.resolve()):
            return False
    except OSError:
        return False
    return full_path.is_file()


def collect_python_noqa_reasonless(repo_root: pathlib.Path) -> dict[PythonNoqaKey, int]:
    """Collect all reasonless inline Python '# noqa: CODE' and '# nosec B<NNN>' occurrences."""
    counts: Counter[PythonNoqaKey] = Counter()
    for rel_path in _git_ls_files(repo_root):
        if not rel_path.endswith(".py") or is_excluded_source_path(rel_path):
            continue
        full_path = repo_root / rel_path
        if not _is_safe_repo_file(full_path, repo_root):
            continue
        text = full_path.read_text(encoding="utf-8", errors="replace")
        if is_generated_source_text(text):
            continue
        lines = text.splitlines()
        for line_no, raw in _tokenize_python_comments(text):
            if PY_FILE_NOQA_RE.match(raw):
                continue
            src_line = lines[line_no - 1] if 1 <= line_no <= len(lines) else raw
            shared_reason = ""
            inline_match = PY_INLINE_NOQA_RE.search(raw)
            if inline_match:
                rest = inline_match.group(1)
                codes_match = PY_NOQA_CODES_RE.match(rest)
                if codes_match and not PY_TRAILING_BAD_CODE_RE.search(rest):
                    codes = tuple(
                        sorted(
                            {
                                c.strip().upper()
                                for c in codes_match.group(1).split(",")
                                if c.strip()
                            }
                        )
                    )
                    shared_reason = extract_python_noqa_reason(rest[codes_match.end(1) :])
                    if not _has_sufficient_reason(shared_reason):
                        key: PythonNoqaKey = (rel_path, codes, line_fingerprint_sha256(src_line))
                        counts[key] += 1

            nosec_match = PY_NOSEC_RE.search(raw)
            if nosec_match:
                nosec_rest = nosec_match.group(1)
                nosec_codes_match = PY_NOSEC_CODES_RE.match(nosec_rest)
                if nosec_codes_match:
                    nosec_codes = tuple(
                        sorted(
                            {
                                c.upper()
                                for c in re.findall(
                                    r"B\d{3}", nosec_codes_match.group(1), re.IGNORECASE
                                )
                            }
                        )
                    )
                    own_reason = _extract_python_nosec_reason(
                        nosec_rest[nosec_codes_match.end(1) :]
                    )
                    effective_reason = (
                        own_reason if _has_sufficient_reason(own_reason) else shared_reason
                    )
                    if not _has_sufficient_reason(effective_reason):
                        nosec_key: PythonNoqaKey = (
                            rel_path,
                            nosec_codes,
                            line_fingerprint_sha256(src_line),
                        )
                        counts[nosec_key] += 1
    return dict(counts)


def render_python_noqa_baseline_toml(counts: dict[PythonNoqaKey, int]) -> str:
    """Serialize frozen reasonless Python # noqa/# nosec baseline entries to deterministic TOML."""
    out = [
        "# Frozen baseline of pre-existing reasonless Python `# noqa: CODE` / `# nosec B<NNN>` occurrences (CHECK-03).",
        "# Auto-generated by `python3 scripts/lint/check_suppressions.py --update-python-noqa-baseline`.",
        "# New reasonless `# noqa: CODE` / `# nosec B<NNN>` lines are rejected; removing existing ones lowers this ledger.",
        "",
        "version = 1",
        "",
    ]
    for (rel_path, codes, line_sha), count in sorted(counts.items()):
        codes_toml = ", ".join(json.dumps(c, ensure_ascii=False) for c in codes)
        out.extend(
            [
                "[[entries]]",
                f"path = {json.dumps(rel_path, ensure_ascii=False)}",
                f"codes = [{codes_toml}]",
                f"line_sha256 = {json.dumps(line_sha, ensure_ascii=False)}",
                f"count = {count}",
                "",
            ]
        )
    return "\n".join(out)


@dataclass(frozen=True)
class _BroadRecord:
    line_no: int
    directive_desc: str
    adr_ids: tuple[str, ...]
    max_expires: datetime.date | None


def _run_git(
    repo_root: pathlib.Path,
    args: list[str],
    *,
    check: bool = False,
) -> subprocess.CompletedProcess[str]:
    git_bin = shutil.which("git")
    if git_bin is None:
        raise FileNotFoundError("git")
    return subprocess.run(  # noqa: S603 - fixed git invocation
        [git_bin, "-C", str(repo_root), *args],
        capture_output=True,
        text=True,
        check=check,
    )


def _git_ls_files(repo_root: pathlib.Path) -> list[str]:
    proc = _run_git(
        repo_root,
        ["ls-files", "--cached", "--others", "--exclude-standard"],
        check=True,
    )
    return sorted({line for line in proc.stdout.splitlines() if line})


def _git_show_text(repo_root: pathlib.Path, rev: str, rel_path: str) -> str | None:
    proc = _run_git(repo_root, ["show", "--end-of-options", f"{rev}:{rel_path}"])
    if proc.returncode != 0:
        return None
    return proc.stdout


def _validate_git_rev(repo_root: pathlib.Path, rev: str) -> tuple[str | None, str | None]:
    if not rev or not rev.strip() or rev.startswith("-"):
        return None, f"invalid --base git revision: {rev!r}"
    proc = _run_git(
        repo_root,
        ["rev-parse", "--verify", "--end-of-options", f"{rev}^{{commit}}"],
    )
    if proc.returncode != 0:
        err = (proc.stderr or proc.stdout).strip()
        return None, f"invalid --base git revision {rev!r}: {err}"
    return proc.stdout.strip(), None


def _resolve_auto_base_rev(repo_root: pathlib.Path) -> str | None:
    if not (repo_root / ".git").exists():
        return None
    head_proc = _run_git(repo_root, ["rev-parse", "--verify", "--quiet", "HEAD^{commit}"])
    if head_proc.returncode != 0:
        return None
    head_sha = head_proc.stdout.strip()

    env_base = os.environ.get("LINT_BASE", "").strip()
    candidates: list[str] = []
    if env_base and not re.fullmatch(r"0+", env_base) and not env_base.startswith("-"):
        candidates.append(env_base)
    candidates.append("origin/main")

    for cand in candidates:
        target_proc = _run_git(
            repo_root,
            ["rev-parse", "--verify", "--quiet", "--end-of-options", f"{cand}^{{commit}}"],
        )
        if target_proc.returncode != 0:
            continue
        target_sha = target_proc.stdout.strip()
        mb_proc = _run_git(repo_root, ["merge-base", target_sha, head_sha])
        if mb_proc.returncode == 0 and mb_proc.stdout.strip():
            return mb_proc.stdout.strip()

    return head_sha


def _changed_adr_ids(repo_root: pathlib.Path, base_rev: str) -> set[str]:
    changed: set[str] = set()
    diff_proc = _run_git(
        repo_root,
        ["diff", "--name-only", "--diff-filter=AMCR", base_rev, "--", "docs/ADR"],
    )
    untracked_proc = _run_git(
        repo_root,
        ["ls-files", "--others", "--exclude-standard", "--", "docs/ADR"],
    )
    for out in (diff_proc.stdout, untracked_proc.stdout):
        for raw_line in out.splitlines():
            rel = raw_line.strip()
            match = ADR_PATH_RE.match(rel)
            if match and (repo_root / rel).is_file():
                changed.add(match.group(1))
    return changed


def _extract_allowlist_expires_records(
    data: dict[str, object],
) -> list[tuple[str, tuple[object, ...], tuple[object, ...] | None, datetime.date, str | None]]:
    records: list[
        tuple[str, tuple[object, ...], tuple[object, ...] | None, datetime.date, str | None]
    ] = []

    ruff_cfg = data.get("ruff")
    if isinstance(ruff_cfg, dict):
        exp = (
            _parse_iso_date(str(ruff_cfg["expires"]))
            if isinstance(ruff_cfg.get("expires"), str)
            else None
        )
        adr = str(ruff_cfg["adr"]).strip() if isinstance(ruff_cfg.get("adr"), str) else None
        if exp is not None:
            records.append(("ruff", ("ruff",), None, exp, adr))

    for idx, entry in enumerate(data.get("bandit") or []):
        if not isinstance(entry, dict):
            continue
        exp = _parse_iso_date(entry["expires"]) if isinstance(entry.get("expires"), str) else None
        adr = entry["adr"].strip() if isinstance(entry.get("adr"), str) else None
        if exp is not None and isinstance(entry.get("path"), str):
            records.append((f"bandit[{idx}]", ("bandit", entry["path"]), None, exp, adr))

    for idx, entry in enumerate(data.get("golangci_exclusions") or []):
        if not isinstance(entry, dict):
            continue
        exp = _parse_iso_date(entry["expires"]) if isinstance(entry.get("expires"), str) else None
        adr = entry["adr"].strip() if isinstance(entry.get("adr"), str) else None
        if exp is not None:
            linters = tuple(sorted(str(x) for x in entry.get("linters") or []))
            base_sig = (
                "golangci_exclusions",
                str(entry.get("config", ".golangci.yml")),
                entry.get("path"),
                entry.get("path_except"),
                entry.get("text"),
                entry.get("source"),
            )
            full_sig = (*base_sig, linters)
            records.append((f"golangci_exclusions[{idx}]", full_sig, base_sig, exp, adr))

    for idx, entry in enumerate(data.get("cargo_lint_allows") or []):
        if not isinstance(entry, dict):
            continue
        exp = _parse_iso_date(entry["expires"]) if isinstance(entry.get("expires"), str) else None
        adr = entry["adr"].strip() if isinstance(entry.get("adr"), str) else None
        if (
            exp is not None
            and isinstance(entry.get("path"), str)
            and isinstance(entry.get("section"), str)
        ):
            key = ("cargo_lint_allows", entry["path"], entry["section"])
            records.append((f"cargo_lint_allows[{idx}]", key, None, exp, adr))

    return records


def _check_allowlist_expires_extensions(
    repo_root: pathlib.Path,
    allowlist_path: pathlib.Path,
    base_rev: str,
    changed_adrs: set[str],
) -> list[Violation]:
    try:
        allowlist_rel = allowlist_path.resolve().relative_to(repo_root.resolve()).as_posix()
    except ValueError:
        return []
    base_text = _git_show_text(repo_root, base_rev, allowlist_rel)
    if base_text is None:
        return []
    try:
        base_data = tomllib.loads(base_text)
        cur_data = tomllib.loads(allowlist_path.read_text(encoding="utf-8"))
    except (tomllib.TOMLDecodeError, OSError):
        return []

    base_exact: dict[tuple[object, ...], datetime.date] = {}
    base_loose: dict[tuple[object, ...], datetime.date] = {}
    for _, key, loose_key, exp, _ in _extract_allowlist_expires_records(base_data):
        base_exact[key] = exp
        if loose_key is not None and (loose_key not in base_loose or exp < base_loose[loose_key]):
            base_loose[loose_key] = exp

    violations: list[Violation] = []
    for label, key, loose_key, cur_exp, adr in _extract_allowlist_expires_records(cur_data):
        prev_exp = base_exact.get(key)
        if prev_exp is None and loose_key is not None:
            prev_exp = base_loose.get(loose_key)
        if prev_exp is None or cur_exp <= prev_exp:
            continue
        adr_match = ADR_EXACT_RE.match(adr) if adr else None
        adr_num = adr_match.group(1) if adr_match else None
        if adr_num is not None and adr_num in changed_adrs:
            continue
        cited_desc = f"docs/ADR/{adr_num}.md" if adr_num else "a cited docs/ADR/NNNNNN.md"
        violations.append(
            Violation(
                allowlist_rel,
                0,
                "config.allowlist.expires-extended-without-adr-update",
                f"{label}: 'expires' extended from {prev_exp.isoformat()} to {cur_exp.isoformat()} "
                f"without modifying or adding {cited_desc} in the same diff",
            )
        )
    return violations


def _make_broad_record(line_no: int, desc: str, context_text: str) -> _BroadRecord:
    dates = _extract_broad_expires_dates(context_text)
    return _BroadRecord(
        line_no=line_no,
        directive_desc=desc,
        adr_ids=tuple(ADR_ID_RE.findall(context_text)),
        max_expires=max(dates) if dates else None,
    )


def _collect_broad_suppressions_py(text: str) -> list[_BroadRecord]:
    comments = _tokenize_python_comments(text)
    comment_by_line = {ln: _strip_comment_delimiters(raw) for ln, raw in comments}
    records: list[_BroadRecord] = []
    for line_no, raw in comments:
        file_match = PY_FILE_NOQA_RE.match(raw)
        if file_match:
            trailing = _python_file_noqa_trailing_reason(file_match.group(1))
            preceding = _preceding_comment_text(comment_by_line, line_no)
            records.append(
                _make_broad_record(
                    line_no,
                    "file-level Python noqa",
                    f"{preceding}\n{trailing}".strip(),
                )
            )
    return records


def _collect_broad_suppressions_go(text: str) -> list[_BroadRecord]:
    comments = _scan_go_comments(text)
    comment_by_line: dict[int, str] = {}
    comments_on_line: dict[int, list[str]] = {}
    for start_line, end_line, raw, _ in comments:
        comments_on_line.setdefault(start_line, []).append(raw)
        if start_line == end_line:
            comment_by_line[start_line] = _strip_comment_delimiters(raw)
    records: list[_BroadRecord] = []
    for start_line, _, raw, before_package in comments:
        nolint_match = GO_NOLINT_RE.match(raw)
        if not nolint_match:
            continue
        linters_raw = nolint_match.group(1)
        linter_names = (
            [name.strip().lower() for name in linters_raw.split(",") if name.strip()]
            if linters_raw
            else []
        )
        if "gosec" in linter_names:
            continue
        if before_package or not linter_names or "all" in linter_names:
            same_line_list = comments_on_line.get(start_line, [])
            trailing = " ".join(same_line_list[same_line_list.index(raw) + 1 :])
            reason_part = _extract_go_nolint_reason(nolint_match.group(2), trailing)
            preceding = _preceding_comment_text(comment_by_line, start_line)
            desc = "file-level Go nolint" if before_package else "blanket Go nolint"
            records.append(
                _make_broad_record(start_line, desc, f"{preceding}\n{reason_part}".strip())
            )
    return records


def _collect_broad_suppressions_rs(text: str) -> list[_BroadRecord]:
    tokens = _scan_rust_tokens(text)
    records: list[_BroadRecord] = []
    for idx, tok in enumerate(tokens):
        if tok.kind == "ATTR_INNER" and RUST_INNER_ALLOW_RE.match(tok.text):
            ctx = _rust_context_for_attr(tokens, idx)
            records.append(
                _make_broad_record(tok.start_line, "Rust crate/module inner #![allow(...)]", ctx)
            )
        elif (
            tok.kind == "ATTR_OUTER"
            and RUST_OUTER_ALLOW_RE.match(tok.text)
            and _rust_attr_applies_to_mod(tokens, idx)
        ):
            ctx = _rust_context_for_attr(tokens, idx)
            records.append(
                _make_broad_record(tok.start_line, "Rust module-level #[allow(...)] on mod", ctx)
            )
    return records


def _collect_broad_suppressions_ts(rel_path: str, text: str) -> list[_BroadRecord]:
    comments = _scan_ts_comments(text, is_svelte=rel_path.endswith(".svelte"))
    comment_by_line = {
        start: _strip_comment_delimiters(raw) for start, end, raw in comments if start == end
    }
    records: list[_BroadRecord] = []
    for start_line, _, raw in comments:
        body = _strip_comment_delimiters(raw)
        preceding = _preceding_comment_text(comment_by_line, start_line)
        biome_all = TS_BIOME_ALL_RE.search(body)
        if biome_all:
            trailing = biome_all.group(1)
            if ":" in trailing:
                trailing = trailing.split(":", 1)[1]
            records.append(
                _make_broad_record(
                    start_line,
                    "file-level biome-ignore-all",
                    f"{preceding}\n{trailing}".strip(),
                )
            )
            continue
        deno_file = TS_DENO_FILE_RE.search(body)
        if deno_file:
            records.append(
                _make_broad_record(
                    start_line,
                    "file-level deno-lint-ignore-file",
                    f"{preceding}\n{deno_file.group(1)}".strip(),
                )
            )
            continue
        eslint_broad = TS_ESLINT_BROAD_RE.search(body)
        if eslint_broad:
            trailing = eslint_broad.group(1)
            reason_part = trailing.split("--", 1)[1] if "--" in trailing else ""
            records.append(
                _make_broad_record(
                    start_line,
                    "file/block-level eslint-disable",
                    f"{preceding}\n{reason_part}".strip(),
                )
            )
    return records


def _collect_broad_suppressions(rel_path: str, text: str) -> list[_BroadRecord]:
    if is_excluded_source_path(rel_path):
        return []
    if rel_path.endswith(".py"):
        return _collect_broad_suppressions_py(text)
    if rel_path.endswith(".go"):
        return _collect_broad_suppressions_go(text)
    if rel_path.endswith(".rs"):
        return _collect_broad_suppressions_rs(text)
    if rel_path.endswith(TS_EXTENSIONS):
        return _collect_broad_suppressions_ts(rel_path, text)
    return []


def _compare_broad_group_expires(
    rel_path: str,
    base_group: list[_BroadRecord],
    cur_group: list[_BroadRecord],
    changed_adrs: set[str],
) -> list[Violation]:
    violations: list[Violation] = []
    if len(base_group) == len(cur_group):
        pairs = list(zip(base_group, cur_group, strict=True))
        for prev, cur in pairs:
            if (
                prev.max_expires is not None
                and cur.max_expires is not None
                and cur.max_expires > prev.max_expires
                and not any(adr_id in changed_adrs for adr_id in cur.adr_ids)
            ):
                violations.append(
                    Violation(
                        rel_path,
                        cur.line_no,
                        "source.broad.expires-extended-without-adr-update",
                        f"{cur.directive_desc} extended 'expires' from {prev.max_expires.isoformat()} "
                        f"to {cur.max_expires.isoformat()} without modifying or adding a cited "
                        f"docs/ADR/NNNNNN.md in the same diff",
                    )
                )
        return violations

    base_dates = [rec.max_expires for rec in base_group if rec.max_expires is not None]
    if not base_dates:
        return []
    base_max = max(base_dates)
    return [
        Violation(
            rel_path,
            cur.line_no,
            "source.broad.expires-extended-without-adr-update",
            f"{cur.directive_desc} extended 'expires' from {base_max.isoformat()} "
            f"to {cur.max_expires.isoformat()} without modifying or adding a cited "
            f"docs/ADR/NNNNNN.md in the same diff",
        )
        for cur in cur_group
        if (
            cur.max_expires is not None
            and cur.max_expires > base_max
            and not any(adr_id in changed_adrs for adr_id in cur.adr_ids)
        )
    ]


def _check_source_broad_expires_extensions(
    repo_root: pathlib.Path,
    base_rev: str,
    changed_adrs: set[str],
) -> list[Violation]:
    diff_proc = _run_git(repo_root, ["diff", "--name-only", base_rev, "--"])
    if diff_proc.returncode != 0:
        return []

    violations: list[Violation] = []
    for raw_line in diff_proc.stdout.splitlines():
        rel_path = raw_line.strip()
        if not rel_path or is_excluded_source_path(rel_path):
            continue
        full_path = repo_root / rel_path
        if not _is_safe_repo_file(full_path, repo_root):
            continue
        base_text = _git_show_text(repo_root, base_rev, rel_path)
        if base_text is None:
            continue
        base_records = _collect_broad_suppressions(rel_path, base_text)
        if not base_records:
            continue
        cur_text = full_path.read_text(encoding="utf-8", errors="replace")
        cur_records = _collect_broad_suppressions(rel_path, cur_text)
        if not cur_records:
            continue

        base_by_desc: dict[str, list[_BroadRecord]] = {}
        cur_by_desc: dict[str, list[_BroadRecord]] = {}
        for rec in base_records:
            base_by_desc.setdefault(rec.directive_desc, []).append(rec)
        for rec in cur_records:
            cur_by_desc.setdefault(rec.directive_desc, []).append(rec)

        for desc, cur_group in cur_by_desc.items():
            base_group = base_by_desc.get(desc)
            if base_group:
                violations.extend(
                    _compare_broad_group_expires(rel_path, base_group, cur_group, changed_adrs)
                )
    return violations


def check_repo(
    repo_root: pathlib.Path,
    allowlist_path: pathlib.Path,
    *,
    today: datetime.date | None = None,
    python_noqa_baseline: dict[PythonNoqaKey, int] | None = None,
    base_rev: str | None = None,
) -> list[Violation]:
    """Run all config allowlist and tracked source-code suppression checks."""
    ref_today = today if today is not None else default_today()
    allowlist, violations = load_allowlist(allowlist_path, repo_root, today=ref_today)
    tracked_files = _git_ls_files(repo_root)

    for rel_path in tracked_files:
        if rel_path.startswith(("stopped-using-k8s/", "scripts/harness/corpus/")):
            continue
        full_path = repo_root / rel_path
        if not _is_safe_repo_file(full_path, repo_root):
            continue
        name = full_path.name
        if name == "pyproject.toml":
            text = full_path.read_text(encoding="utf-8")
            violations.extend(check_pyproject(rel_path, text, allowlist))
        elif name in (".golangci.yml", ".golangci.yaml"):
            text = full_path.read_text(encoding="utf-8")
            violations.extend(check_golangci(rel_path, text, allowlist))
        elif name == "Cargo.toml":
            text = full_path.read_text(encoding="utf-8")
            violations.extend(check_cargo_toml(rel_path, text, allowlist))
        elif not is_excluded_source_path(rel_path):
            text = full_path.read_text(encoding="utf-8", errors="replace")
            violations.extend(
                check_source_file(
                    rel_path,
                    text,
                    repo_root,
                    today=ref_today,
                    python_noqa_baseline=python_noqa_baseline,
                )
            )

    if base_rev is not None:
        changed_adrs = _changed_adr_ids(repo_root, base_rev)
        violations.extend(
            _check_allowlist_expires_extensions(repo_root, allowlist_path, base_rev, changed_adrs)
        )
        violations.extend(_check_source_broad_expires_extensions(repo_root, base_rev, changed_adrs))

    violations.sort(key=lambda v: (v.path, v.line, v.rule, v.message))
    return violations


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    default_root = pathlib.Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(
        description="Verify lint suppression allowlists and source-code suppression governance.",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=default_root,
        help="Repository root directory (default: detected repo root).",
    )
    parser.add_argument(
        "--allowlist",
        type=pathlib.Path,
        default=None,
        help="Path to suppression_allowlist.toml (default: <repo-root>/scripts/lint/suppression_allowlist.toml).",
    )
    parser.add_argument(
        "--today",
        type=str,
        default=None,
        help="Override current date (YYYY-MM-DD) for deterministic expiration checks.",
    )
    parser.add_argument(
        "--python-noqa-baseline",
        type=pathlib.Path,
        default=None,
        help="Path to python_noqa_baseline.toml (default: <repo-root>/scripts/lint/ratchet/python_noqa_baseline.toml if present).",
    )
    parser.add_argument(
        "--no-python-noqa-baseline",
        action="store_true",
        default=False,
        help="Disable reasonless Python # noqa baseline (require reasons on every line).",
    )
    parser.add_argument(
        "--update-python-noqa-baseline",
        action="store_true",
        default=False,
        help="Write/update python_noqa_baseline.toml from current reasonless Python # noqa lines.",
    )
    parser.add_argument(
        "--base",
        type=str,
        default=None,
        help="Git base revision for detecting expires extensions without a same-diff ADR update.",
    )
    return parser.parse_args(argv)


def _resolve_today_arg(raw_today: str | None) -> datetime.date | None:
    if raw_today is None:
        return default_today()
    return _parse_iso_date(raw_today)


def _resolve_baseline(
    args: argparse.Namespace,
    repo_root: pathlib.Path,
) -> tuple[dict[PythonNoqaKey, int] | None, str | None]:
    if args.no_python_noqa_baseline:
        return {}, None
    if args.python_noqa_baseline is not None:
        bl_path = args.python_noqa_baseline.resolve()
        if not bl_path.is_file():
            return None, f"python_noqa baseline not found: {bl_path}"
        return load_python_noqa_baseline(bl_path), None
    default_bl = repo_root / "scripts" / "lint" / "ratchet" / "python_noqa_baseline.toml"
    if default_bl.is_file():
        return load_python_noqa_baseline(default_bl), None
    return {}, None


def _update_python_noqa_baseline(args: argparse.Namespace, repo_root: pathlib.Path) -> int:
    target_bl = (
        args.python_noqa_baseline.resolve()
        if args.python_noqa_baseline is not None
        else repo_root / "scripts" / "lint" / "ratchet" / "python_noqa_baseline.toml"
    )
    counts = collect_python_noqa_reasonless(repo_root)
    target_bl.parent.mkdir(parents=True, exist_ok=True)
    target_bl.write_text(render_python_noqa_baseline_toml(counts), encoding="utf-8")
    total = sum(counts.values())
    sys.stdout.write(
        f"Updated {target_bl} ({len(counts)} unique fingerprints, {total} total occurrences).\n"
    )
    return EXIT_CLEAN


def _report_violations(violations: list[Violation]) -> int:
    if violations:
        for v in violations:
            loc = f"{v.path}:{v.line}" if v.line > 0 else v.path
            sys.stderr.write(f"{loc}: [{v.rule}] {v.message}\n")
        sys.stderr.write(f"\nFAIL: {len(violations)} suppression governance violation(s) found.\n")
        return EXIT_VIOLATIONS
    sys.stdout.write("OK: suppression governance and config allowlist checks clean.\n")
    return EXIT_CLEAN


def _resolve_cli_inputs(
    args: argparse.Namespace,
    repo_root: pathlib.Path,
) -> tuple[tuple[datetime.date, pathlib.Path, str | None] | None, str | None]:
    ref_today = _resolve_today_arg(args.today)
    if ref_today is None:
        return None, f"invalid --today date {args.today!r} (expected YYYY-MM-DD)"

    allowlist_path: pathlib.Path = (
        args.allowlist.resolve()
        if args.allowlist is not None
        else repo_root / "scripts" / "lint" / "suppression_allowlist.toml"
    )
    if not allowlist_path.is_file():
        return None, f"allowlist not found: {allowlist_path}"

    if args.base is not None:
        resolved_base, base_err = _validate_git_rev(repo_root, args.base)
        if base_err is not None:
            return None, base_err
    else:
        resolved_base = _resolve_auto_base_rev(repo_root)

    return (ref_today, allowlist_path, resolved_base), None


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    repo_root: pathlib.Path = args.repo_root.resolve()

    if args.update_python_noqa_baseline:
        return _update_python_noqa_baseline(args, repo_root)

    resolved, err = _resolve_cli_inputs(args, repo_root)
    if err is not None or resolved is None:
        sys.stderr.write(f"ERROR: {err}\n")
        return EXIT_USAGE_ERROR
    ref_today, allowlist_path, resolved_base = resolved

    try:
        baseline, bl_err = _resolve_baseline(args, repo_root)
        if bl_err is not None:
            sys.stderr.write(f"ERROR: {bl_err}\n")
            return EXIT_USAGE_ERROR
        violations = check_repo(
            repo_root,
            allowlist_path,
            today=ref_today,
            python_noqa_baseline=baseline,
            base_rev=resolved_base,
        )
    except (subprocess.CalledProcessError, FileNotFoundError, tomllib.TOMLDecodeError) as exc:
        sys.stderr.write(f"ERROR: suppression check failed to execute: {exc}\n")
        return EXIT_USAGE_ERROR

    return _report_violations(violations)


if __name__ == "__main__":
    sys.exit(main())
