#!/usr/bin/env python3
"""Host environment LKAL reconciliation, MCP setup & read-only preflight helper.

Bridges Alt's repository-owned environment declaration (`deploy/desired-env.lkal`)
and host-local settings (`/local/*` in `.kisage/host-env.lkal`) with the official
`lkal` CLI (`lkal json -plain`) and `kisage` v0.1.2 CLI / MCP server:

  * `init`: Provisions a protected `.kisage/` sandbox (`0700` directories,
    `0600` `host.json` and `host-env.lkal`), binds the target file's physical
    `deviceId`, grants `Scope` only to `/upstreams`, `/mtls_allowlists`,
    `/rate_limits`, `/feature_flags`, `/mtls_policy`, and `/llm_budgets`
    (preserving `/local/*` as undeclared unowned siblings), and initializes
    the `kisage` ownership store.
  * `verify`: Checks that `deploy/desired-env.lkal` parses via official `lkal`,
    is in canonical `lkal fmt` format, and stays in lockstep with
    `deploy/host-prereqs.yaml`, production Compose bindings, `.env.template` /
    `.env.example`, and ADR-000982's 5s rate-limit floor.
  * `render-dotenv`: Projects a converged `host-env.lkal` into deterministic,
    escaped `KEY=VALUE` dotenv lines (rejecting unrepresentable values)
    validated against `scripts/check-env-overrides.py`.
  * `preflight`: Read-only comparison of repository-managed keys from
    `deploy/desired-env.lkal` against an explicitly supplied actual `.env`
    file (subset check; preserves local/unmanaged keys, emits KEY names only,
    never secret/actual values, and does not inspect running containers).
  * `audit-host-config`: Explicit read-only structural audit of gitignored
    host-local agent configuration indicators (`.claude/settings.json`
    `PreToolUse` entry matching `Edit` and `Write` with a command hook
    referencing `.kisage/*`, and `.mcp.json` `kisage` MCP server entry with
    non-empty `command`, `args == ["mcp"]`, and `env.KISAGE_HOST_CONFIG`).
    Fails closed when invoked if either file is missing, malformed, or lacks
    the expected structural indicators, without executing hook/MCP commands or
    echoing raw file contents/secrets. Checks structural configuration presence
    only; does not prove runtime hook blocking or OS-level isolation.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import types
from typing import Any, NamedTuple

ASCII_PRINTABLE_MIN = 0x20
ASCII_DEL = 0x7F
LKAL_DIAG_CODE_LEN = 4

ROOT = pathlib.Path(__file__).resolve().parent.parent
DEFAULT_SANDBOX = ROOT / ".kisage"
DEFAULT_DECLARATION = ROOT / "deploy" / "desired-env.lkal"
DEFAULT_MANIFEST = ROOT / "deploy" / "host-prereqs.yaml"
DEFAULT_CLAUDE_SETTINGS = ROOT / ".claude" / "settings.json"
DEFAULT_MCP_CONFIG = ROOT / ".mcp.json"
DEFAULT_COMPOSE_ROOT = ROOT / "compose" / "compose.yaml"
DEFAULT_ENV_TEMPLATE = ROOT / ".env.template"
DEFAULT_ENV_EXAMPLE = ROOT / ".env.example"
CHECK_OVERRIDES_PATH = ROOT / "scripts" / "check-env-overrides.py"
COMPOSE_INCLUDE_PATH = ROOT / "scripts" / "compose_include.py"

EXPECTED_KISAGE_VERSION = "0.1.2"
EXPECTED_LKAL_SPEC_VERSION = "0.1"

MANAGED_SECTIONS = (
    "upstreams",
    "mtls_allowlists",
    "rate_limits",
    "feature_flags",
    "mtls_policy",
    "llm_budgets",
)
# alt-backend receives the host dotenv via env_file and exits at startup when any of these variables is set.
DOTENV_FORBIDDEN_KEYS = (
    "MTLS_LISTEN",
    "MTLS_PORT",
    "MTLS_CLIENT_AUTH",
    "MTLS_ALLOWED_PEERS",
)
ALL_SECTIONS = (*MANAGED_SECTIONS, "local")
MIN_EXTERNAL_API_INTERVAL_SECONDS = 5.0

_IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_-]*$")
_ENV_KEY_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_DURATION_RE = re.compile(r"^(?P<num>[0-9]+(?:\.[0-9]+)?)(?P<unit>ms|s|m|h)$")
_SAFE_UNQUOTED_VALUE_RE = re.compile(r"^[A-Za-z0-9_./:@,+=-]*$")
_SAFE_LIST_ITEM_RE = re.compile(r"^[A-Za-z0-9_./:@+-]+$")

INITIAL_TARGET_LKAL = """@lkal "0.1"

### Host-local settings owned by the local operator / developer.
### Outside kisage Scope: preserved untouched across kisage apply and prune.
local {
  KRATOS_COOKIE_DOMAIN = ""
  ORIGIN = "http://localhost:4173"
  ALTCTL_DOCKER_CONTEXT = "default"
  ALTCTL_AI_REQUIRES_GPU = true
}
"""


def _load_overrides_module() -> types.ModuleType:
    spec = importlib.util.spec_from_file_location("check_env_overrides", CHECK_OVERRIDES_PATH)
    if spec is None or spec.loader is None:
        msg = f"cannot load {CHECK_OVERRIDES_PATH}"
        raise RuntimeError(msg)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _write_owner_only_file(path: pathlib.Path, content: str) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    fd = os.open(path, flags, 0o600)
    try:
        os.fchmod(fd, 0o600)
        os.write(fd, content.encode("utf-8"))
        os.fsync(fd)
    finally:
        os.close(fd)


def _validate_kisage_binary(
    candidate: str | None = None, expected_version: str | None = None
) -> str:
    allowed_paths: set[pathlib.Path] = set()
    for known in (
        os.environ.get("KISAGE_BIN"),
        shutil.which("kisage"),
        str(pathlib.Path.home() / ".local" / "bin" / "kisage"),
        "/usr/local/bin/kisage",
        "/usr/bin/kisage",
        "/tmp/kisage",  # noqa: S108 — fixed trusted binary install fallback path pinned by tests/test-kisage-env.py
    ):
        if known:
            p = pathlib.Path(known)
            if p.is_file():
                allowed_paths.add(p.resolve(strict=True))

    chosen = (
        candidate
        or os.environ.get("KISAGE_BIN")
        or shutil.which("kisage")
        or str(pathlib.Path.home() / ".local" / "bin" / "kisage")
    )
    chosen_path = pathlib.Path(chosen)
    if not chosen_path.is_file():
        msg = f"kisage binary not found at {chosen!r}; install pinned kisage via 'make install-kisage' or set KISAGE_BIN"
        raise ValueError(msg)
    resolved = chosen_path.resolve(strict=True)
    if resolved not in allowed_paths:
        msg = f"kisage binary path {resolved} is not in the trusted allow-list"
        raise ValueError(msg)
    if not os.access(resolved, os.X_OK):
        msg = f"kisage binary {resolved} is not executable"
        raise ValueError(msg)

    exp_ver = expected_version or os.environ.get("KISAGE_EXPECTED_VERSION", EXPECTED_KISAGE_VERSION)
    ver_proc = subprocess.run(  # noqa: S603 -- binary path allow-listed by _validate_kisage_binary
        [str(resolved), "--version"],
        capture_output=True,
        text=True,
        check=False,
    )
    actual_ver = ver_proc.stdout.strip()
    if ver_proc.returncode != 0 or actual_ver != f"kisage version {exp_ver}":
        msg = f"kisage binary {resolved} version mismatch: expected 'kisage version {exp_ver}', got {actual_ver!r}"
        raise ValueError(msg)
    return str(resolved)


def _validate_lkal_binary(candidate: str | None = None) -> str:
    allowed_paths: set[pathlib.Path] = set()
    for known in (
        os.environ.get("LKAL_BIN"),
        shutil.which("lkal"),
        str(pathlib.Path.home() / ".local" / "bin" / "lkal"),
        "/usr/local/bin/lkal",
        "/usr/bin/lkal",
        "/tmp/lkal",  # noqa: S108 — fixed trusted binary install fallback path pinned by tests/test-kisage-env.py
    ):
        if known:
            p = pathlib.Path(known)
            if p.is_file():
                allowed_paths.add(p.resolve(strict=True))

    if candidate is not None:
        chosen = candidate
    else:
        chosen = None
        for fallback in (
            os.environ.get("LKAL_BIN"),
            shutil.which("lkal"),
            str(pathlib.Path.home() / ".local" / "bin" / "lkal"),
            "/usr/local/bin/lkal",
            "/usr/bin/lkal",
            "/tmp/lkal",  # noqa: S108 — fixed trusted binary install fallback path pinned by tests/test-kisage-env.py
        ):
            if fallback and pathlib.Path(fallback).is_file():
                chosen = fallback
                break
        if chosen is None:
            chosen = str(pathlib.Path.home() / ".local" / "bin" / "lkal")

    chosen_path = pathlib.Path(chosen)
    if not chosen_path.is_file():
        msg = f"lkal binary not found at {chosen!r}; install official lkal via 'make install-lkal' or set LKAL_BIN"
        raise ValueError(msg)
    resolved = chosen_path.resolve(strict=True)
    if resolved not in allowed_paths:
        msg = f"lkal binary path {resolved} is not in the trusted allow-list"
        raise ValueError(msg)
    if not os.access(resolved, os.X_OK):
        msg = f"lkal binary {resolved} is not executable"
        raise ValueError(msg)
    return str(resolved)


def _validate_normalized_leaf_value(
    sec_name: str, key: str, val: object
) -> bool | int | str | list[str]:
    if isinstance(val, bool):
        return val
    if isinstance(val, int):
        return val
    if isinstance(val, str):
        return val
    if isinstance(val, list):
        for item in val:
            if not isinstance(item, str):
                msg = f"section {sec_name!r} key {key!r}: list elements must be strings, got {type(item).__name__}"
                raise ValueError(msg)  # noqa: TRY004 — callers/tests catch ValueError
        return list(val)
    msg = f"section {sec_name!r} key {key!r}: unsupported LKAL normalized value type {type(val).__name__}"
    raise ValueError(msg)


def _format_lkal_cli_error(stderr: str) -> str:
    unparseable_msg = "lkal json -plain --json failed (lkal reported an unparseable diagnostic)"
    try:
        raw_diags = json.loads(stderr)
    except (TypeError, json.JSONDecodeError) as exc:
        raise ValueError(unparseable_msg) from exc
    if not isinstance(raw_diags, list) or not raw_diags:
        raise ValueError(unparseable_msg)
    formatted_entries: list[str] = []
    for entry in raw_diags:
        if not isinstance(entry, dict):
            raise ValueError(unparseable_msg)  # noqa: TRY004 — callers/tests catch ValueError
        code = entry.get("code")
        line = entry.get("line")
        col = entry.get("col")
        if (
            not isinstance(code, str)
            or len(code) != LKAL_DIAG_CODE_LEN
            or code[0] != "L"
            or not code[1:].isdigit()
            or isinstance(line, bool)
            or not isinstance(line, int)
            or line < 0
            or isinstance(col, bool)
            or not isinstance(col, int)
            or col < 0
        ):
            raise ValueError(unparseable_msg)
        formatted_entries.append(f"{code} at {line}:{col}")
    return f"lkal json -plain --json failed ({', '.join(formatted_entries)})"


def parse_lkal_sections(text: str, *, lkal_bin: str | None = None) -> dict[str, dict[str, Any]]:
    """Parse a 2-level LKAL v0.1 configuration document into {section: {key: value}}
    using the official `lkal json -plain --json` CLI normalizer (no regex fallback).

    Both block syntax (`section { KEY = VALUE }`) and LKAL labeled-entry syntax
    (`section "KEY" = VALUE`) normalize to the same Record structure, and LKAL
    `Quantity` literals (`7.5s`, `250ms`) normalize to canonical string payloads.
    """
    resolved_lkal = _validate_lkal_binary(lkal_bin)
    proc = subprocess.run(  # noqa: S603 -- binary path allow-listed by _validate_lkal_binary
        [resolved_lkal, "json", "-plain", "--json", "-"],
        input=text,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        msg = _format_lkal_cli_error(proc.stderr)
        raise ValueError(msg)

    try:
        raw_doc = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        msg = f"lkal json -plain produced invalid JSON at pos {exc.pos}"
        raise ValueError(msg) from exc

    if not isinstance(raw_doc, dict):
        msg = "expected top-level LKAL Record object"
        raise ValueError(msg)  # noqa: TRY004 — callers/tests catch ValueError

    sections: dict[str, dict[str, Any]] = {}
    for sec_name, sec_val in raw_doc.items():
        if not isinstance(sec_name, str) or not _IDENT_RE.fullmatch(sec_name):
            msg = "invalid top-level section identifier"
            raise ValueError(msg)
        if not isinstance(sec_val, dict):
            msg = f"top-level entry {sec_name!r} must be a 2-level Record block, got {type(sec_val).__name__}"
            raise ValueError(msg)  # noqa: TRY004 — callers/tests catch ValueError
        sec_map: dict[str, Any] = {}
        for key, val in sec_val.items():
            if not isinstance(key, str) or not _ENV_KEY_RE.fullmatch(key):
                msg = f"invalid environment key identifier in section {sec_name!r}"
                raise ValueError(msg)
            sec_map[key] = _validate_normalized_leaf_value(sec_name, key, val)
        sections[sec_name] = sec_map

    return sections


def check_lkal_format(
    decl_path: pathlib.Path = DEFAULT_DECLARATION,
    *,
    lkal_bin: str | None = None,
) -> list[str]:
    """Verify that `decl_path` is in canonical `lkal fmt` form via `lkal fmt -l`.

    `lkal fmt -l` exits 0 when a valid file differs from canonical form and
    prints the file path to stdout. Decide by stdout content while failing
    closed on non-zero exit status or unexpected output.
    """
    try:
        resolved_lkal = _validate_lkal_binary(lkal_bin)
    except ValueError as exc:
        return [f"cannot check format of {decl_path.name}: {exc}"]

    target = str(decl_path)
    proc = subprocess.run(  # noqa: S603 -- binary path allow-listed by _validate_lkal_binary
        [resolved_lkal, "fmt", "-l", target],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        return [f"{decl_path.name}: lkal fmt -l failed with exit code {proc.returncode}"]
    if proc.stderr:
        return [f"{decl_path.name}: lkal fmt -l produced unexpected stderr output"]
    if proc.stdout == "":
        return []
    if proc.stdout.splitlines() == [target]:
        return [f"{decl_path.name} is not in canonical lkal fmt format"]
    return [f"{decl_path.name}: lkal fmt -l produced unexpected stdout output"]


def _parse_duration_seconds(raw: str) -> float:
    m = _DURATION_RE.fullmatch(raw.strip())
    if not m:
        msg = "invalid duration quantity syntax"
        raise ValueError(msg)
    num = float(m.group("num"))
    unit = m.group("unit")
    if unit == "ms":
        return num / 1000.0
    if unit == "s":
        return num
    if unit == "m":
        return num * 60.0
    return num * 3600.0


def _contains_control_chars(s: str) -> bool:
    return any(ord(ch) < ASCII_PRINTABLE_MIN or ord(ch) == ASCII_DEL for ch in s)


def _format_dotenv_list(key: str, val: list[object]) -> str:
    items: list[str] = []
    for item in val:
        if not isinstance(item, str) or not item:
            msg = f"key {key}: list elements must be non-empty strings"
            raise ValueError(msg)
        if _contains_control_chars(item) or not _SAFE_LIST_ITEM_RE.fullmatch(item):
            msg = f"key {key}: list element contains characters unrepresentable in comma-separated dotenv"
            raise ValueError(msg)
        items.append(item)
    return ",".join(items)


def _format_dotenv_str(key: str, val: str, *, quote_for_file: bool) -> str:
    if _contains_control_chars(val):
        msg = f"key {key}: value contains control or newline characters unrepresentable in single-line dotenv"
        raise ValueError(msg)
    if "$" in val or "\\" in val:
        msg = f"key {key}: value contains '$' or '\\' which is ambiguous across dotenv parsers"
        raise ValueError(msg)
    if not quote_for_file or val == "" or _SAFE_UNQUOTED_VALUE_RE.fullmatch(val):
        return val
    if '"' not in val:
        return f'"{val}"'
    if "'" not in val:
        return f"'{val}'"
    msg = f"key {key}: value contains both single and double quotes and cannot be losslessly escaped in dotenv"
    raise ValueError(msg)


def _format_dotenv_value(key: str, val: object, *, quote_for_file: bool) -> str:
    if not _ENV_KEY_RE.fullmatch(key):
        msg = f"invalid dotenv key identifier: {key!r}"
        raise ValueError(msg)
    if isinstance(val, bool):
        return "true" if val else "false"
    if isinstance(val, int):
        return str(val)
    if isinstance(val, list):
        return _format_dotenv_list(key, val)
    if isinstance(val, str):
        return _format_dotenv_str(key, val, quote_for_file=quote_for_file)
    msg = f"key {key}: unsupported value type {type(val).__name__}"
    raise ValueError(msg)


def _collect_section_env(
    sections: dict[str, dict[str, Any]],
    allowed_sections: tuple[str, ...],
    *,
    quote_for_file: bool,
) -> dict[str, str]:
    unknown = sorted(set(sections.keys()) - set(allowed_sections))
    if unknown:
        msg = f"unsupported LKAL section count={len(unknown)}"
        raise ValueError(msg)

    env: dict[str, str] = {}
    owner_section: dict[str, str] = {}
    for sec in allowed_sections:
        entries = sections.get(sec) or {}
        for key in sorted(entries):
            if key in owner_section:
                msg = f"duplicate environment key {key!r} across sections {owner_section[key]!r} and {sec!r}"
                raise ValueError(msg)
            owner_section[key] = sec
            env[key] = _format_dotenv_value(key, entries[key], quote_for_file=quote_for_file)
    return env


def sections_to_env_dict(sections: dict[str, dict[str, Any]]) -> dict[str, str]:
    return _collect_section_env(sections, ALL_SECTIONS, quote_for_file=False)


def render_dotenv(lkal_path: pathlib.Path, *, lkal_bin: str | None = None) -> str:
    sections = parse_lkal_sections(lkal_path.read_text(encoding="utf-8"), lkal_bin=lkal_bin)
    # Validate cross-section uniqueness and value representability up front.
    _collect_section_env(sections, ALL_SECTIONS, quote_for_file=True)

    out_lines = [
        "# Generated from kisage-reconciled LKAL configuration.",
        "# Managed sections: upstreams, mtls_allowlists, rate_limits, feature_flags, mtls_policy, llm_budgets.",
        "# Preserved sibling section: local.",
    ]
    for sec in ALL_SECTIONS:
        entries = sections.get(sec) or {}
        if not entries:
            continue
        out_lines.append("")
        out_lines.append(f"# [{sec}]")
        for key in sorted(entries):
            rendered = _format_dotenv_value(key, entries[key], quote_for_file=True)
            out_lines.append(f"{key}={rendered}")
    return "\n".join(out_lines) + "\n"


_COMPOSE_DEFAULTED_INTERP_RE = re.compile(r"(?<!\$)\$\{([A-Z_][A-Z0-9_]*):?-(.*?)\}")
_COMPOSE_REQUIRED_BRACE_RE = re.compile(r"(?<!\$)\$\{([A-Z_][A-Z0-9_]*)(?::?\?[^}]*)?\}")
_COMPOSE_REQUIRED_BARE_RE = re.compile(r"(?<!\$)\$([A-Z_][A-Z0-9_]*)\b")


def _load_compose_include_module() -> types.ModuleType:
    spec = importlib.util.spec_from_file_location("compose_include", COMPOSE_INCLUDE_PATH)
    if spec is None or spec.loader is None:
        msg = f"cannot load {COMPOSE_INCLUDE_PATH}"
        raise RuntimeError(msg)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _iter_service_env_entries(svc: dict[str, Any]) -> list[tuple[str, str]]:
    env = svc.get("environment")
    out: list[tuple[str, str]] = []
    if isinstance(env, dict):
        for k, v in env.items():
            if v is None:
                continue
            out.append((str(k).strip(), str(v)))
    elif isinstance(env, list):
        for item in env:
            if isinstance(item, str) and "=" in item:
                k, v = item.split("=", 1)
                out.append((k.strip(), v))
    return out


class _BindingAccumulator(NamedTuple):
    managed_set: set[str]
    defaults_by_key: dict[str, list[str]]
    required_by_key: dict[str, bool]
    literals_by_key: dict[str, list[str]]


def _load_production_compose_services(
    compose_root: pathlib.Path,
) -> tuple[dict[str, Any] | None, list[str]]:
    if not compose_root.is_file():
        return None, [f"compose-root-missing: cannot read compose root {compose_root.name}"]
    try:
        compose_mod = _load_compose_include_module()
        services = compose_mod.production_services(compose_root)
    except (SystemExit, Exception):  # noqa: BLE001 — boundary converts compose_include load/YAML failures to diagnostics
        return None, [
            f"compose-parse-error: cannot load production compose services from {compose_root.name}"
        ]
    if not isinstance(services, dict) or not services:
        return None, [
            f"compose-empty-services: no production compose services found in {compose_root.name}"
        ]
    return services, []


def _record_compose_env_entry(acc: _BindingAccumulator, var_name: str, raw_val: str) -> None:
    interpolated_in_val: set[str] = set()
    for match in _COMPOSE_DEFAULTED_INTERP_RE.finditer(raw_val):
        ref_key, default_val = match.group(1), match.group(2)
        interpolated_in_val.add(ref_key)
        if ref_key in acc.managed_set:
            acc.defaults_by_key[ref_key].append(default_val)
    for regex in (_COMPOSE_REQUIRED_BRACE_RE, _COMPOSE_REQUIRED_BARE_RE):
        for match in regex.finditer(raw_val):
            ref_key = match.group(1)
            interpolated_in_val.add(ref_key)
            if ref_key in acc.managed_set:
                acc.required_by_key[ref_key] = True

    if var_name in acc.managed_set and var_name not in interpolated_in_val:
        acc.literals_by_key[var_name].append(raw_val)


def _classify_compose_binding(
    *, required: bool, defaults: list[str], literals: list[str]
) -> dict[str, Any]:
    if required:
        return {"class": "required", "defaults": [], "literals": []}
    if defaults:
        return {"class": "defaulted", "defaults": defaults, "literals": []}
    if literals:
        return {"class": "pinned", "defaults": [], "literals": literals}
    return {"class": "passthrough", "defaults": [], "literals": []}


def resolve_compose_bindings(
    managed_keys: set[str] | list[str] | tuple[str, ...],
    compose_root: pathlib.Path = DEFAULT_COMPOSE_ROOT,
) -> tuple[dict[str, dict[str, Any]] | None, list[str]]:
    """Classify each managed key from production compose services' environment entries."""
    services, load_errors = _load_production_compose_services(compose_root)
    if load_errors or services is None:
        return None, load_errors

    managed_set = set(managed_keys)
    acc = _BindingAccumulator(
        managed_set=managed_set,
        defaults_by_key={k: [] for k in managed_set},
        required_by_key=dict.fromkeys(managed_set, False),
        literals_by_key={k: [] for k in managed_set},
    )

    for svc in services.values():
        if not isinstance(svc, dict):
            continue
        for var_name, raw_val in _iter_service_env_entries(svc):
            _record_compose_env_entry(acc, var_name, raw_val)

    bindings = {
        key: _classify_compose_binding(
            required=acc.required_by_key[key],
            defaults=acc.defaults_by_key[key],
            literals=acc.literals_by_key[key],
        )
        for key in sorted(managed_set)
    }

    return bindings, []


def _value_matches_declaration(
    key: str,
    candidate: str,
    env_map: dict[str, str],
    decl_allowlists: dict[str, Any],
) -> bool:
    if key in decl_allowlists and isinstance(decl_allowlists[key], list):
        peers = [p.strip() for p in candidate.split(",") if p.strip()]
        decl_peers = decl_allowlists[key]
        return len(peers) == len(set(peers)) and set(peers) == set(decl_peers)
    return candidate == env_map.get(key)


class _EnvFileSnapshot(NamedTuple):
    name: str
    values: dict[str, str]


def _parse_declaration_sections(
    decl_path: pathlib.Path, *, lkal_bin: str | None
) -> tuple[dict[str, dict[str, Any]] | None, list[str]]:
    try:
        sections = parse_lkal_sections(decl_path.read_text(encoding="utf-8"), lkal_bin=lkal_bin)
    except OSError as exc:
        return None, [f"cannot read declaration {decl_path.name}: {exc.strerror}"]
    except UnicodeDecodeError:
        return None, [f"declaration {decl_path.name} is not valid UTF-8"]
    except ValueError as exc:
        return None, [f"cannot parse {decl_path.name}: {exc}"]
    return sections, []


def _check_declaration_allowlists(
    sections: dict[str, dict[str, Any]], rules_allowlists: dict[str, tuple[str, ...]]
) -> list[str]:
    errors: list[str] = []
    decl_allowlists = sections.get("mtls_allowlists") or {}
    if set(decl_allowlists.keys()) != set(rules_allowlists.keys()):
        missing = sorted(set(rules_allowlists.keys()) - set(decl_allowlists.keys()))
        extra = sorted(
            k
            for k in (set(decl_allowlists.keys()) - set(rules_allowlists.keys()))
            if _ENV_KEY_RE.fullmatch(k)
        )
        errors.append(
            f"mtls_allowlists keys differ from deploy/host-prereqs.yaml: missing={missing} extra={extra}"
        )
    for key, required_peers in rules_allowlists.items():
        got_peers = decl_allowlists.get(key)
        if not isinstance(got_peers, list) or set(got_peers) != set(required_peers):
            errors.append(
                f"mtls_allowlists/{key} peer allowlist mismatch against deploy/host-prereqs.yaml"
            )
        if isinstance(got_peers, list) and len(got_peers) != len(set(got_peers)):
            errors.append(f"mtls_allowlists/{key} contains duplicate peer entries")
    return errors


def _check_rate_limit_floor(sections: dict[str, dict[str, Any]]) -> list[str]:
    rate_limits = sections.get("rate_limits") or {}
    interval_raw = rate_limits.get("RATE_LIMIT_EXTERNAL_API_INTERVAL")
    if not isinstance(interval_raw, str):
        return ["rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL must be a duration quantity/string"]
    try:
        seconds = _parse_duration_seconds(interval_raw)
    except ValueError:
        return ["rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL has invalid duration quantity syntax"]
    if seconds < MIN_EXTERNAL_API_INTERVAL_SECONDS:
        return [
            f"rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL violates the "
            f"{MIN_EXTERNAL_API_INTERVAL_SECONDS}s floor (ADR-000982 / Critical Rule 2)"
        ]
    return []


def _read_overrides_env_file(
    path: pathlib.Path, label: str, overrides: types.ModuleType
) -> tuple[_EnvFileSnapshot | None, str | None]:
    try:
        raw = path.read_text(encoding="utf-8")
    except OSError as exc:
        return None, f"{label}-unreadable: cannot read {path.name} ({exc.strerror})"
    except UnicodeDecodeError:
        return None, f"{label}-encoding: {path.name} is not valid UTF-8"
    return _EnvFileSnapshot(name=path.name, values=overrides.parse_env_file(raw)), None


def _check_compose_template_lockstep(
    env_map: dict[str, str],
    bindings: dict[str, dict[str, Any]],
    tpl: _EnvFileSnapshot,
    example: _EnvFileSnapshot,
    decl_allowlists: dict[str, Any],
) -> list[str]:
    errors: list[str] = []
    for key in sorted(env_map):
        b = bindings[key]
        b_class = b["class"]
        if b_class == "defaulted":
            defaults = b["defaults"]
            if len(set(defaults)) > 1:
                errors.append(
                    f"compose-default-inconsistent: {key} has conflicting defaults across production compose services"
                )
            elif not _value_matches_declaration(key, defaults[0], env_map, decl_allowlists):
                errors.append(
                    f"compose-default-mismatch: {key} compose default differs from declaration"
                )
        elif b_class == "pinned":
            literals = b["literals"]
            if len(set(literals)) > 1 or not _value_matches_declaration(
                key, literals[0], env_map, decl_allowlists
            ):
                errors.append(
                    f"compose-pinned-mismatch: {key} compose pinned literal differs from declaration"
                )
        elif b_class == "passthrough":
            if key not in tpl.values:
                errors.append(
                    f"passthrough-missing-in-template: {key} is not referenced in production compose and is missing from {tpl.name}"
                )
            elif not _value_matches_declaration(key, tpl.values[key], env_map, decl_allowlists):
                errors.append(
                    f"passthrough-template-mismatch: {key} in {tpl.name} differs from declaration"
                )

        if (
            b_class != "passthrough"
            and key in tpl.values
            and not _value_matches_declaration(key, tpl.values[key], env_map, decl_allowlists)
        ):
            errors.append(f"env-template-mismatch: {key} in {tpl.name} differs from declaration")
        if key in example.values and not _value_matches_declaration(
            key, example.values[key], env_map, decl_allowlists
        ):
            errors.append(f"env-example-mismatch: {key} in {example.name} differs from declaration")
    return errors


def verify_declaration(  # noqa: PLR0913 — public API and argparse-forwarded options pinned by tests
    decl_path: pathlib.Path = DEFAULT_DECLARATION,
    manifest_path: pathlib.Path = DEFAULT_MANIFEST,
    *,
    lkal_bin: str | None = None,
    compose_root: pathlib.Path = DEFAULT_COMPOSE_ROOT,
    env_template_path: pathlib.Path = DEFAULT_ENV_TEMPLATE,
    env_example_path: pathlib.Path = DEFAULT_ENV_EXAMPLE,
) -> list[str]:
    sections, parse_errors = _parse_declaration_sections(decl_path, lkal_bin=lkal_bin)
    if parse_errors or sections is None:
        return parse_errors

    errors: list[str] = []
    if "local" in sections:
        errors.append(
            "deploy/desired-env.lkal must not declare /local/* (reserved for host-local operator settings)"
        )

    unknown_sections = sorted(set(sections.keys()) - set(ALL_SECTIONS))
    if unknown_sections:
        errors.append(
            f"deploy/desired-env.lkal declares {len(unknown_sections)} unsupported section(s)"
        )

    overrides = _load_overrides_module()
    rules = overrides.load_rules(manifest_path)
    errors.extend(_check_declaration_allowlists(sections, rules.allowlists))
    errors.extend(_check_rate_limit_floor(sections))

    try:
        env_map = _collect_section_env(
            {k: v for k, v in sections.items() if k in MANAGED_SECTIONS},
            MANAGED_SECTIONS,
            quote_for_file=False,
        )
    except ValueError as exc:
        errors.append(str(exc))
        return errors

    errors.extend(
        f"dotenv-forbidden-key: {forbidden_key} cannot be managed in the host dotenv because alt-backend rejects it at startup"
        for forbidden_key in sorted(set(env_map) & set(DOTENV_FORBIDDEN_KEYS))
    )
    findings = overrides.evaluate(env_map, rules)
    errors.extend(f"check-env-overrides {finding.severity}: {finding.key}" for finding in findings)

    bindings, binding_errors = resolve_compose_bindings(
        set(env_map.keys()), compose_root=compose_root
    )
    if binding_errors or bindings is None:
        errors.extend(binding_errors)
        return errors

    tpl_snapshot, tpl_err = _read_overrides_env_file(env_template_path, "env-template", overrides)
    if tpl_err is not None or tpl_snapshot is None:
        errors.append(tpl_err or f"env-template-unreadable: cannot read {env_template_path.name}")
        return errors

    example_snapshot, ex_err = _read_overrides_env_file(env_example_path, "env-example", overrides)
    if ex_err is not None or example_snapshot is None:
        errors.append(ex_err or f"env-example-unreadable: cannot read {env_example_path.name}")
        return errors

    decl_allowlists = sections.get("mtls_allowlists") or {}
    errors.extend(
        _check_compose_template_lockstep(
            env_map, bindings, tpl_snapshot, example_snapshot, decl_allowlists
        )
    )
    return errors


_DOUBLE_QUOTE_ESCAPES = {
    "n": "\n",
    "r": "\r",
    "t": "\t",
    '"': '"',
    "\\": "\\",
    "$": "$",
}
_VALID_VAR_REF_RE = re.compile(
    r"\$(?:\$|[A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*(?:(?::-|-|:\?|\?|:\+|\+)[^}\"'\r\n]*)?\})"
)


def _has_valid_var_refs(val: str) -> bool:
    pos = 0
    while True:
        dollar = val.find("$", pos)
        if dollar < 0:
            return True
        m = _VALID_VAR_REF_RE.match(val, dollar)
        if m is None:
            return False
        pos = m.end()


def _parse_double_quoted_dotenv_rhs(
    rhs: str, lineno: int, key: str, *, allow_var_refs: bool = False
) -> tuple[str | None, str | None]:
    """Parse a double-quoted dotenv RHS supporting backslash escapes (`\\"`, `\\\\`, `\\$`, `\\n`, `\\r`, `\\t`).

    Rejects unescaped `$` interpolation on repository-managed keys (when `allow_var_refs=False`),
    permits valid Compose `$VAR` / `${VAR}` references on unmanaged keys (when `allow_var_refs=True`)
    without evaluating them, and rejects unclosed/trailing/malformed syntax with key-only diagnostics.
    """
    buf: list[str] = []
    escaped = False
    close_idx = -1
    idx = 1

    while idx < len(rhs):
        ch = rhs[idx]
        if escaped:
            buf.append(_DOUBLE_QUOTE_ESCAPES.get(ch, ch))
            escaped = False
            idx += 1
            continue
        if ch == "\\":
            escaped = True
            idx += 1
            continue
        if ch == '"':
            close_idx = idx
            break
        if ch == "$":
            if not allow_var_refs:
                return (
                    None,
                    f"line {lineno}: unsupported unescaped '$' interpolation in value for key {key}",
                )
            m = _VALID_VAR_REF_RE.match(rhs, idx)
            if m is None:
                return (
                    None,
                    f"line {lineno}: malformed '$' variable reference in value for key {key}",
                )
            buf.append(m.group(0))
            idx = m.end()
            continue
        buf.append(ch)
        idx += 1

    if close_idx < 0 or escaped:
        return (
            None,
            f"line {lineno}: malformed unclosed double-quoted value for key {key}",
        )

    trailing = rhs[close_idx + 1 :].strip()
    if trailing and not trailing.startswith("#"):
        return (
            None,
            f"line {lineno}: malformed trailing characters after quoted value for key {key}",
        )
    return "".join(buf), None


def _parse_single_quoted_dotenv_rhs(
    rhs: str, lineno: int, key: str
) -> tuple[str | None, str | None]:
    end = rhs.find("'", 1)
    if end < 0:
        return None, f"line {lineno}: malformed unclosed single-quoted value for key {key}"
    trailing = rhs[end + 1 :].strip()
    if trailing and not trailing.startswith("#"):
        return (
            None,
            f"line {lineno}: malformed trailing characters after quoted value for key {key}",
        )
    return rhs[1:end], None


def _parse_unquoted_dotenv_rhs(
    rhs: str, lineno: int, key: str, *, is_managed: bool
) -> tuple[str | None, str | None]:
    val = re.split(r"\s+#", rhs, maxsplit=1)[0].rstrip()
    if '"' in val:
        return None, f"line {lineno}: malformed unquoted double-quote in value for key {key}"
    if "$" in val:
        if is_managed:
            return (
                None,
                f"line {lineno}: unsupported unquoted '$' interpolation in value for key {key}",
            )
        if not _has_valid_var_refs(val):
            return None, f"line {lineno}: malformed '$' variable reference in value for key {key}"
    return val, None


def _parse_dotenv_rhs(
    rhs: str, lineno: int, key: str, *, is_managed: bool
) -> tuple[str | None, str | None]:
    rhs = rhs.strip()
    if rhs.startswith("'"):
        return _parse_single_quoted_dotenv_rhs(rhs, lineno, key)
    if rhs.startswith('"'):
        parsed_val, err = _parse_double_quoted_dotenv_rhs(
            rhs, lineno, key, allow_var_refs=not is_managed
        )
        if err is not None or parsed_val is None:
            return None, err or f"line {lineno}: malformed quoted value for key {key}"
        return parsed_val, None
    return _parse_unquoted_dotenv_rhs(rhs, lineno, key, is_managed=is_managed)


def _parse_actual_dotenv_strict(
    text: str, *, managed_keys: set[str] | frozenset[str] | None = None
) -> tuple[dict[str, str], list[str]]:
    """Strictly parse a raw Compose-compatible dotenv file for preflight comparison.

    Supports:
      - Unquoted values (including apostrophes `'`) with inline ` #...` comment stripping
      - Single-quoted literal values (`'...'`)
      - Double-quoted values (`"..."`) with backslash escapes (`\\"`, `\\\\`, `\\$`, `\\n`, `\\r`, `\\t`)
    Explicitly rejects unescaped `$` variable interpolation on repository-managed keys
    (when `managed_keys` is None or `key in managed_keys`), while permitting valid
    Compose variable reference syntax (`$VAR`, `${VAR}`, `${VAR:-default}`, `$$`) on
    unmanaged keys without evaluating them. Rejects malformed `$` references, unclosed
    quotes, control characters, trailing garbage, duplicate keys, or malformed lines.
    Never includes raw lines or values in diagnostics so credentials cannot leak.
    """
    out: dict[str, str] = {}
    seen_keys: set[str] = set()
    errors: list[str] = []

    for lineno, raw in enumerate(text.splitlines(), start=1):
        if any(
            (ord(ch) < ASCII_PRINTABLE_MIN and ch != "\t") or ord(ch) == ASCII_DEL for ch in raw
        ):
            errors.append(f"line {lineno}: malformed control character in dotenv entry")
            continue
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export ") :].lstrip()
        key, sep, rhs = line.partition("=")
        if not sep:
            errors.append(f"line {lineno}: malformed dotenv entry (missing '=')")
            continue
        key = key.strip()
        if not _ENV_KEY_RE.fullmatch(key):
            errors.append(f"line {lineno}: malformed dotenv key name")
            continue
        if key in seen_keys:
            errors.append(f"line {lineno}: duplicate key {key} in dotenv file")
            continue
        seen_keys.add(key)

        is_managed = managed_keys is None or key in managed_keys
        val, err = _parse_dotenv_rhs(rhs, lineno, key, is_managed=is_managed)
        if err is not None or val is None:
            errors.append(err or f"line {lineno}: malformed value for key {key}")
            continue
        out[key] = val

    return out, errors


def preflight_dotenv(  # noqa: PLR0913 — public API and argparse-forwarded options pinned by tests
    env_file: pathlib.Path,
    decl_path: pathlib.Path = DEFAULT_DECLARATION,
    manifest_path: pathlib.Path = DEFAULT_MANIFEST,
    *,
    lkal_bin: str | None = None,
    compose_root: pathlib.Path = DEFAULT_COMPOSE_ROOT,
    env_template_path: pathlib.Path = DEFAULT_ENV_TEMPLATE,
    env_example_path: pathlib.Path = DEFAULT_ENV_EXAMPLE,
) -> list[str]:
    """Read-only comparison of repository-managed keys against an actual dotenv file.

    Preserves local/unmanaged keys in `env_file`, accepts absent `defaulted` and
    `pinned` keys, fails closed on absent `passthrough` or `required` keys, value
    mismatches, duplicate keys, or malformed dotenv syntax, and reports KEY names
    only (never actual values or secrets).
    """
    decl_errors = verify_declaration(
        decl_path,
        manifest_path,
        lkal_bin=lkal_bin,
        compose_root=compose_root,
        env_template_path=env_template_path,
        env_example_path=env_example_path,
    )
    if decl_errors:
        return [f"declaration verification failed: {e}" for e in decl_errors]

    sections = parse_lkal_sections(decl_path.read_text(encoding="utf-8"), lkal_bin=lkal_bin)
    expected_managed = _collect_section_env(
        {k: v for k, v in sections.items() if k in MANAGED_SECTIONS},
        MANAGED_SECTIONS,
        quote_for_file=False,
    )

    bindings, binding_errors = resolve_compose_bindings(
        set(expected_managed), compose_root=compose_root
    )
    if binding_errors or bindings is None:
        return [f"declaration verification failed: {e}" for e in binding_errors]

    try:
        raw_env_text = env_file.read_text(encoding="utf-8")
    except OSError as exc:
        return [f"cannot read dotenv file {env_file}: {exc.strerror}"]
    except UnicodeDecodeError:
        return [f"dotenv file {env_file} is not valid UTF-8 text"]

    actual_env, parse_errors = _parse_actual_dotenv_strict(
        raw_env_text, managed_keys=set(expected_managed)
    )
    errors = list(parse_errors)
    decl_allowlists = sections.get("mtls_allowlists") or {}

    for key in sorted(expected_managed):
        b_class = bindings[key]["class"]
        if key not in actual_env:
            if b_class in ("passthrough", "required"):
                errors.append(f"missing managed key in dotenv: {key}")
        elif not _value_matches_declaration(
            key, actual_env[key], expected_managed, decl_allowlists
        ):
            errors.append(f"managed key drift (value mismatch) in dotenv: {key}")

    return errors


def _validate_claude_pre_tool_use_structure(filename: str, claude_doc: dict[str, Any]) -> list[str]:
    hooks = claude_doc.get("hooks")
    pre_tool_use = hooks.get("PreToolUse") if isinstance(hooks, dict) else None
    if not isinstance(pre_tool_use, list) or not pre_tool_use:
        return [f"{filename} hooks.PreToolUse must be a non-empty list of hook entries"]

    entry_errors: list[str] = []
    has_kisage_guard_indicator = False

    for idx, entry in enumerate(pre_tool_use):
        if not isinstance(entry, dict):
            entry_errors.append(f"{filename} hooks.PreToolUse[{idx}] must be a JSON object")
            continue
        matcher = entry.get("matcher")
        if not isinstance(matcher, str) or not matcher.strip():
            entry_errors.append(
                f"{filename} hooks.PreToolUse[{idx}].matcher must be a non-empty regex string"
            )
            continue
        try:
            pattern = re.compile(matcher)
        except re.error:
            entry_errors.append(
                f"{filename} hooks.PreToolUse[{idx}].matcher is not a valid regular expression"
            )
            continue

        hook_nodes = entry.get("hooks")
        if not isinstance(hook_nodes, list) or not hook_nodes:
            entry_errors.append(
                f"{filename} hooks.PreToolUse[{idx}].hooks must be a non-empty list"
            )
            continue

        matches_edit_and_write = bool(pattern.search("Edit")) and bool(pattern.search("Write"))
        for h_idx, hook in enumerate(hook_nodes):
            if not isinstance(hook, dict):
                entry_errors.append(
                    f"{filename} hooks.PreToolUse[{idx}].hooks[{h_idx}] must be a JSON object"
                )
                continue
            cmd = hook.get("command")
            if (
                matches_edit_and_write
                and hook.get("type") == "command"
                and isinstance(cmd, str)
                and bool(cmd.strip())
                and ".kisage/*" in cmd
            ):
                has_kisage_guard_indicator = True

    if not has_kisage_guard_indicator:
        entry_errors.append(
            f"{filename} hooks.PreToolUse lacks a command-type hook entry matching Edit and Write with '.kisage/*' guard indicator in command"
        )
    return entry_errors


def _read_json_object_safe(
    path: pathlib.Path, label: str
) -> tuple[dict[str, Any] | None, str | None]:
    try:
        raw = path.read_text(encoding="utf-8")
    except OSError as exc:
        return None, f"cannot read {label} {path.name}: {exc.strerror}"
    except UnicodeDecodeError:
        return None, f"{label} {path.name} is not valid UTF-8 text"
    try:
        doc = json.loads(raw)
    except json.JSONDecodeError as exc:
        return None, f"malformed JSON in {path.name} (line {exc.lineno} col {exc.colno})"
    if not isinstance(doc, dict):
        return None, f"{path.name} root must be a JSON object"
    return doc, None


def _validate_mcp_kisage_server(filename: str, mcp_doc: dict[str, Any]) -> list[str]:
    servers = mcp_doc.get("mcpServers")
    kisage_srv = servers.get("kisage") if isinstance(servers, dict) else None
    if not isinstance(kisage_srv, dict):
        return [f"{filename} mcpServers.kisage registration is missing"]

    errors: list[str] = []
    cmd_val = kisage_srv.get("command")
    if not isinstance(cmd_val, str) or not cmd_val.strip():
        errors.append(f"{filename} mcpServers.kisage command must be a non-empty string")
    if kisage_srv.get("args") != ["mcp"]:
        errors.append(f"{filename} mcpServers.kisage args must be ['mcp']")
    env_map = kisage_srv.get("env")
    host_cfg = env_map.get("KISAGE_HOST_CONFIG") if isinstance(env_map, dict) else None
    if not isinstance(host_cfg, str) or not host_cfg.strip():
        errors.append(f"{filename} mcpServers.kisage env.KISAGE_HOST_CONFIG is missing or empty")
    return errors


def audit_host_config(
    claude_settings_path: pathlib.Path = DEFAULT_CLAUDE_SETTINGS,
    mcp_config_path: pathlib.Path = DEFAULT_MCP_CONFIG,
) -> list[str]:
    """Explicit read-only structural indicator audit of gitignored host-local agent files.

    Fails closed when either `.claude/settings.json` or `.mcp.json` is missing,
    unreadable, malformed JSON, non-object JSON root (including `null`), or lacks
    the expected structural indicators (`PreToolUse` entry matching `Edit` and
    `Write` with a non-empty `type == "command"` hook command referencing
    `.kisage/*`; `mcpServers.kisage` with non-empty `command`, `args == ["mcp"]`,
    and non-empty `env.KISAGE_HOST_CONFIG`). Never executes hook or MCP commands
    and never echoes raw file contents or secret values. Checks configuration
    presence only and does not verify runtime hook execution or OS isolation.
    """
    errors: list[str] = []

    claude_doc, claude_err = _read_json_object_safe(claude_settings_path, "host Claude settings")
    if claude_err is not None:
        errors.append(claude_err)
    elif claude_doc is not None:
        errors.extend(
            _validate_claude_pre_tool_use_structure(claude_settings_path.name, claude_doc)
        )

    mcp_doc, mcp_err = _read_json_object_safe(mcp_config_path, "host MCP config")
    if mcp_err is not None:
        errors.append(mcp_err)
    elif mcp_doc is not None:
        errors.extend(_validate_mcp_kisage_server(mcp_config_path.name, mcp_doc))

    return errors


def init_sandbox(
    sandbox_dir: pathlib.Path,
    decl_path: pathlib.Path,
    kisage_bin: str,
    actor: str = "alt-operator",
) -> int:
    if not _IDENT_RE.fullmatch(actor):
        msg = f"invalid actor identifier: {actor!r}"
        raise ValueError(msg)

    decl_resolved = decl_path.resolve(strict=True)
    sandbox_abs = sandbox_dir.resolve(strict=False)
    parent_resolved = sandbox_abs.parent.resolve(strict=True)
    if not str(sandbox_abs).startswith(str(parent_resolved) + os.sep):
        msg = f"sandbox path {sandbox_abs} escapes parent directory {parent_resolved}"
        raise ValueError(msg)

    sandbox_abs.mkdir(mode=0o700, exist_ok=True)
    sandbox_abs.chmod(0o700)
    for sub in ("state", "locks"):
        sub_dir = sandbox_abs / sub
        sub_dir.mkdir(mode=0o700, exist_ok=True)
        sub_dir.chmod(0o700)

    target_lkal = sandbox_abs / "host-env.lkal"
    if not target_lkal.exists():
        _write_owner_only_file(target_lkal, INITIAL_TARGET_LKAL)
    else:
        target_lkal.chmod(0o600)

    device_id = target_lkal.stat().st_dev
    host_cfg = {
        "version": 1,
        "actor": actor,
        "storeId": "alt-env-store",
        "storeDirectory": str(sandbox_abs / "state"),
        "lockDirectory": str(sandbox_abs / "locks"),
        "resources": [
            {
                "id": "alt-host-env",
                "managementRoot": str(sandbox_abs),
                "targetPath": str(target_lkal),
                "deviceId": device_id,
                "declarationFile": str(decl_resolved),
                "scope": [
                    {
                        "path": f"/{sec}",
                        "match": "subtree",
                        "permissions": ["Set", "Own", "Delete", "Resolve"],
                    }
                    for sec in MANAGED_SECTIONS
                ],
            }
        ],
    }
    host_json_path = sandbox_abs / "host.json"
    _write_owner_only_file(host_json_path, json.dumps(host_cfg, indent=2) + "\n")

    ownership_file = sandbox_abs / "state" / "ownership.json"
    if not ownership_file.exists():
        env = {**os.environ, "KISAGE_HOST_CONFIG": str(host_json_path)}
        proc = subprocess.run(  # noqa: S603 -- binary path allow-listed by _validate_kisage_binary
            [kisage_bin, "init-store"],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        if proc.returncode != 0:
            sys.stderr.write(f"kisage init-store failed: {proc.stderr}\n")
            return proc.returncode
    return 0


def _build_cli_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_init = sub.add_parser(
        "init",
        help="Provision a protected kisage sandbox and initialize the ownership store",
    )
    p_init.add_argument("--sandbox-dir", type=pathlib.Path, default=DEFAULT_SANDBOX)
    p_init.add_argument("--declaration", type=pathlib.Path, default=DEFAULT_DECLARATION)
    p_init.add_argument("--kisage-bin", type=str, default=None)
    p_init.add_argument("--actor", type=str, default="alt-operator")

    p_verify = sub.add_parser(
        "verify",
        help="Verify deploy/desired-env.lkal canonical format and lockstep with deploy/host-prereqs.yaml, compose, and env templates",
    )
    p_verify.add_argument("--declaration", type=pathlib.Path, default=DEFAULT_DECLARATION)
    p_verify.add_argument("--manifest", type=pathlib.Path, default=DEFAULT_MANIFEST)
    p_verify.add_argument("--lkal-bin", type=str, default=None)
    p_verify.add_argument("--compose-root", type=pathlib.Path, default=DEFAULT_COMPOSE_ROOT)
    p_verify.add_argument("--env-template", type=pathlib.Path, default=DEFAULT_ENV_TEMPLATE)
    p_verify.add_argument("--env-example", type=pathlib.Path, default=DEFAULT_ENV_EXAMPLE)

    p_render = sub.add_parser(
        "render-dotenv",
        help="Render converged host-env.lkal to KEY=VALUE dotenv format",
    )
    p_render.add_argument("--target", type=pathlib.Path, default=DEFAULT_SANDBOX / "host-env.lkal")
    p_render.add_argument("--output", type=pathlib.Path, default=None)
    p_render.add_argument("--lkal-bin", type=str, default=None)

    p_preflight = sub.add_parser(
        "preflight",
        help="Read-only preflight comparing repository-managed keys against an actual dotenv file",
    )
    p_preflight.add_argument(
        "--env-file",
        type=pathlib.Path,
        required=True,
        help="Actual dotenv file to inspect (read-only)",
    )
    p_preflight.add_argument("--declaration", type=pathlib.Path, default=DEFAULT_DECLARATION)
    p_preflight.add_argument("--manifest", type=pathlib.Path, default=DEFAULT_MANIFEST)
    p_preflight.add_argument("--lkal-bin", type=str, default=None)

    p_audit_host = sub.add_parser(
        "audit-host-config",
        help="Explicit read-only audit of gitignored host-local .claude/settings.json and .mcp.json",
    )
    p_audit_host.add_argument(
        "--claude-settings",
        type=pathlib.Path,
        default=DEFAULT_CLAUDE_SETTINGS,
        help="Host Claude settings file to inspect (read-only)",
    )
    p_audit_host.add_argument(
        "--mcp-config",
        type=pathlib.Path,
        default=DEFAULT_MCP_CONFIG,
        help="Host MCP config file to inspect (read-only)",
    )
    return parser


def _cmd_verify(args: argparse.Namespace) -> int:
    errs = verify_declaration(
        args.declaration,
        args.manifest,
        lkal_bin=args.lkal_bin,
        compose_root=args.compose_root,
        env_template_path=args.env_template,
        env_example_path=args.env_example,
    )
    errs.extend(check_lkal_format(args.declaration, lkal_bin=args.lkal_bin))
    if errs:
        for err in errs:
            sys.stderr.write(f"ERROR: {err}\n")
        return 1
    sys.stdout.write(
        "OK: deploy/desired-env.lkal is in canonical lkal fmt format and in lockstep with deploy/host-prereqs.yaml, production compose, .env.template, and .env.example\n"
    )
    return 0


def _cmd_render_dotenv(args: argparse.Namespace) -> int:
    dotenv_text = render_dotenv(args.target.resolve(strict=True), lkal_bin=args.lkal_bin)
    if args.output is not None:
        _write_owner_only_file(args.output, dotenv_text)
    else:
        sys.stdout.write(dotenv_text)
    return 0


def _cmd_preflight(args: argparse.Namespace) -> int:
    errs = preflight_dotenv(
        args.env_file,
        args.declaration,
        args.manifest,
        lkal_bin=args.lkal_bin,
    )
    if errs:
        for err in errs:
            sys.stderr.write(f"PREFLIGHT ERROR: {err}\n")
        return 1
    sys.stdout.write(
        f"OK: {args.env_file} matches managed keys from {args.declaration} "
        "(read-only preflight; unmanaged/local keys preserved, running containers not inspected)\n"
    )
    return 0


def _cmd_audit_host_config(args: argparse.Namespace) -> int:
    errs = audit_host_config(args.claude_settings, args.mcp_config)
    if errs:
        for err in errs:
            sys.stderr.write(f"HOST CONFIG AUDIT ERROR: {err}\n")
        return 1
    sys.stdout.write(
        "OK: expected host-local configuration entries present in .claude/settings.json and .mcp.json "
        "(structural configuration indicator check only; hook execution and OS isolation not verified)\n"
    )
    return 0


def _cmd_init(args: argparse.Namespace) -> int:
    kisage_bin = _validate_kisage_binary(args.kisage_bin)
    return init_sandbox(args.sandbox_dir, args.declaration, kisage_bin, args.actor)


def main(argv: list[str] | None = None) -> int:
    args = _build_cli_parser().parse_args(argv)
    handlers = {
        "verify": _cmd_verify,
        "render-dotenv": _cmd_render_dotenv,
        "preflight": _cmd_preflight,
        "audit-host-config": _cmd_audit_host_config,
        "init": _cmd_init,
    }
    handler = handlers.get(args.cmd)
    if handler is None:
        return 2
    return handler(args)


if __name__ == "__main__":
    raise SystemExit(main())
