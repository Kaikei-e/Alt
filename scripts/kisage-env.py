#!/usr/bin/env python3
"""Host environment LKAL reconciliation & MCP setup helper for kisage v0.1.1.

Bridges Alt's repository-owned environment declaration (`deploy/desired-env.lkal`)
and host-local settings (`/local/*` in `.kisage/host-env.lkal`) with `kisage`
CLI and MCP server:

  * `init`: Provisions a protected `.kisage/` sandbox (`0700` directories,
    `0600` `host.json` and `host-env.lkal`), binds the target file's physical
    `deviceId`, grants `Scope` only to `/upstreams`, `/mtls_allowlists`,
    `/rate_limits`, and `/feature_flags` (preserving `/local/*` as undeclared
    unowned siblings), and initializes the `kisage` ownership store.
  * `verify`: Checks that `deploy/desired-env.lkal` stays in lockstep with
    `deploy/host-prereqs.yaml` and ADR-000982's 5s rate-limit floor.
  * `render-dotenv`: Projects a converged `host-env.lkal` into deterministic
    `KEY=VALUE` dotenv lines validated against `scripts/check-env-overrides.py`.
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
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parent.parent
DEFAULT_SANDBOX = ROOT / ".kisage"
DEFAULT_DECLARATION = ROOT / "deploy" / "desired-env.lkal"
DEFAULT_MANIFEST = ROOT / "deploy" / "host-prereqs.yaml"
CHECK_OVERRIDES_PATH = ROOT / "scripts" / "check-env-overrides.py"

MANAGED_SECTIONS = ("upstreams", "mtls_allowlists", "rate_limits", "feature_flags")
ALL_SECTIONS = (*MANAGED_SECTIONS, "local")
MIN_EXTERNAL_API_INTERVAL_SECONDS = 5.0

_IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_-]*$")
_LABELED_ASSIGN_RE = re.compile(
    r'^(?P<sec>[A-Za-z_][A-Za-z0-9_-]*)\s+"(?P<key>[A-Za-z_][A-Za-z0-9_-]*)"\s*=\s*(?P<rhs>.+)$'
)
_DURATION_RE = re.compile(r"^(?P<num>[0-9]+(?:\.[0-9]+)?)(?P<unit>ms|s|m|h)$")

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


def _load_overrides_module():
    spec = importlib.util.spec_from_file_location(
        "check_env_overrides", CHECK_OVERRIDES_PATH
    )
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {CHECK_OVERRIDES_PATH}")
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


def _validate_kisage_binary(candidate: str | None) -> str:
    allowed_paths: set[pathlib.Path] = set()
    for known in (
        shutil.which("kisage"),
        str(pathlib.Path.home() / ".local" / "bin" / "kisage"),
        "/usr/local/bin/kisage",
        "/usr/bin/kisage",
        "/tmp/kisage",
    ):
        if known:
            p = pathlib.Path(known)
            if p.is_file():
                allowed_paths.add(p.resolve(strict=True))

    chosen = (
        candidate
        or shutil.which("kisage")
        or str(pathlib.Path.home() / ".local" / "bin" / "kisage")
    )
    resolved = pathlib.Path(chosen).resolve(strict=True)
    if resolved not in allowed_paths:
        raise ValueError(
            f"kisage binary path {resolved} is not in the trusted allow-list"
        )
    if not os.access(resolved, os.X_OK):
        raise ValueError(f"kisage binary {resolved} is not executable")
    return str(resolved)


def parse_lkal_sections(text: str) -> dict[str, dict[str, Any]]:
    """Parse a 2-level LKAL v0.1 configuration document into {section: {key: value}}.

    Supports both block syntax (`section { KEY = VALUE }`) and LKAL labeled-entry
    syntax (`section "KEY" = VALUE`), which produce the same LKAL Record data model.
    """
    lines = text.splitlines()
    sections: dict[str, dict[str, Any]] = {}
    current_section: str | None = None
    in_list_section: str | None = None
    in_list_key: str | None = None
    list_acc: list[str] = []
    seen_header = False

    for lineno, raw in enumerate(lines, start=1):
        stripped = raw.strip()
        if not stripped or stripped.startswith("###"):
            continue
        if not seen_header:
            if stripped != '@lkal "0.1"':
                raise ValueError(
                    f"line {lineno}: expected '@lkal \"0.1\"' header, got {stripped!r}"
                )
            seen_header = True
            continue

        if in_list_key is not None:
            if stripped.endswith("]"):
                body = stripped[:-1].strip()
                if body:
                    list_acc.extend(_parse_string_list_items(body, lineno))
                assert in_list_section is not None
                sections.setdefault(in_list_section, {})[in_list_key] = list(list_acc)
                in_list_section = None
                in_list_key = None
                list_acc = []
            else:
                list_acc.extend(_parse_string_list_items(stripped, lineno))
            continue

        if current_section is None:
            labeled = _LABELED_ASSIGN_RE.match(stripped)
            if labeled:
                sec_name = labeled.group("sec")
                key = labeled.group("key")
                rhs = labeled.group("rhs").strip()
                sec_map = sections.setdefault(sec_name, {})
                if key in sec_map:
                    raise ValueError(
                        f"line {lineno}: duplicate key {key!r} in section {sec_name!r}"
                    )
                if rhs.startswith("[") and not rhs.endswith("]"):
                    in_list_section = sec_name
                    in_list_key = key
                    list_acc = []
                    rem = rhs[1:].strip()
                    if rem:
                        list_acc.extend(_parse_string_list_items(rem, lineno))
                else:
                    sec_map[key] = _parse_lkal_scalar_or_list(rhs, lineno)
                continue

            if not stripped.endswith("{"):
                raise ValueError(
                    f"line {lineno}: expected top-level block '<section> {{', got {stripped!r}"
                )
            sec_name = stripped[:-1].strip()
            if not _IDENT_RE.match(sec_name):
                raise ValueError(
                    f"line {lineno}: invalid section identifier {sec_name!r}"
                )
            sections.setdefault(sec_name, {})
            current_section = sec_name
            continue

        if stripped == "}":
            current_section = None
            continue

        key, sep, rhs = stripped.partition("=")
        if not sep:
            raise ValueError(
                f"line {lineno}: expected 'KEY = VALUE' inside {current_section!r}"
            )
        key = key.strip()
        rhs = rhs.strip()
        if not _IDENT_RE.match(key):
            raise ValueError(f"line {lineno}: invalid key identifier {key!r}")
        if key in sections[current_section]:
            raise ValueError(
                f"line {lineno}: duplicate key {key!r} in section {current_section!r}"
            )

        if rhs.startswith("[") and not rhs.endswith("]"):
            in_list_section = current_section
            in_list_key = key
            list_acc = []
            rem = rhs[1:].strip()
            if rem:
                list_acc.extend(_parse_string_list_items(rem, lineno))
            continue

        sections[current_section][key] = _parse_lkal_scalar_or_list(rhs, lineno)

    if not seen_header:
        raise ValueError("empty LKAL document: missing '@lkal \"0.1\"' header")
    if current_section is not None or in_list_key is not None:
        raise ValueError("unclosed block or list in LKAL document")
    return sections


def _parse_string_list_items(fragment: str, lineno: int) -> list[str]:
    items: list[str] = []
    for part in fragment.split(","):
        tok = part.strip()
        if not tok:
            continue
        val = json.loads(tok)
        if not isinstance(val, str):
            raise ValueError(
                f"line {lineno}: expected string element in list, got {tok!r}"
            )
        items.append(val)
    return items


def _parse_lkal_scalar_or_list(rhs: str, lineno: int) -> Any:
    if rhs == "true":
        return True
    if rhs == "false":
        return False
    if rhs.startswith('"') and rhs.endswith('"'):
        return json.loads(rhs)
    if rhs.startswith("[") and rhs.endswith("]"):
        inner = rhs[1:-1].strip()
        return _parse_string_list_items(inner, lineno) if inner else []
    if re.fullmatch(r"-?[0-9]+", rhs):
        return int(rhs)
    raise ValueError(f"line {lineno}: unsupported LKAL value literal {rhs!r}")


def _parse_duration_seconds(raw: str) -> float:
    m = _DURATION_RE.fullmatch(raw.strip())
    if not m:
        raise ValueError(f"invalid duration string: {raw!r}")
    num = float(m.group("num"))
    unit = m.group("unit")
    if unit == "ms":
        return num / 1000.0
    if unit == "s":
        return num
    if unit == "m":
        return num * 60.0
    return num * 3600.0


def sections_to_env_dict(sections: dict[str, dict[str, Any]]) -> dict[str, str]:
    env: dict[str, str] = {}
    for sec in ALL_SECTIONS:
        entries = sections.get(sec) or {}
        for key in sorted(entries):
            val = entries[key]
            if isinstance(val, list):
                env[key] = ",".join(str(x) for x in val)
            elif isinstance(val, bool):
                env[key] = "true" if val else "false"
            else:
                env[key] = str(val)
    return env


def render_dotenv(lkal_path: pathlib.Path) -> str:
    sections = parse_lkal_sections(lkal_path.read_text(encoding="utf-8"))
    out_lines = [
        "# Generated from kisage-reconciled LKAL configuration.",
        "# Managed sections: upstreams, mtls_allowlists, rate_limits, feature_flags.",
        "# Preserved sibling section: local.",
    ]
    for sec in ALL_SECTIONS:
        entries = sections.get(sec) or {}
        if not entries:
            continue
        out_lines.append("")
        out_lines.append(f"# [{sec}]")
        for key in sorted(entries):
            val = entries[key]
            if isinstance(val, list):
                rendered = ",".join(str(x) for x in val)
            elif isinstance(val, bool):
                rendered = "true" if val else "false"
            else:
                rendered = str(val)
            out_lines.append(f"{key}={rendered}")
    return "\n".join(out_lines) + "\n"


def verify_declaration(
    decl_path: pathlib.Path = DEFAULT_DECLARATION,
    manifest_path: pathlib.Path = DEFAULT_MANIFEST,
) -> list[str]:
    errors: list[str] = []
    try:
        sections = parse_lkal_sections(decl_path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        return [f"cannot parse {decl_path}: {exc}"]

    if "local" in sections:
        errors.append(
            "deploy/desired-env.lkal must not declare /local/* (reserved for host-local operator settings)"
        )

    overrides = _load_overrides_module()
    rules = overrides.load_rules(manifest_path)

    decl_allowlists = sections.get("mtls_allowlists") or {}
    if set(decl_allowlists.keys()) != set(rules.allowlists.keys()):
        missing = sorted(set(rules.allowlists.keys()) - set(decl_allowlists.keys()))
        extra = sorted(set(decl_allowlists.keys()) - set(rules.allowlists.keys()))
        errors.append(
            f"mtls_allowlists keys differ from deploy/host-prereqs.yaml: missing={missing} extra={extra}"
        )
    for key, required_peers in rules.allowlists.items():
        got_peers = decl_allowlists.get(key)
        if not isinstance(got_peers, list) or set(got_peers) != set(required_peers):
            errors.append(
                f"mtls_allowlists/{key} peers {got_peers!r} != required {list(required_peers)!r}"
            )

    rate_limits = sections.get("rate_limits") or {}
    interval_raw = rate_limits.get("RATE_LIMIT_EXTERNAL_API_INTERVAL")
    if not isinstance(interval_raw, str):
        errors.append(
            "rate_limits/RATE_LIMIT_EXTERNAL_API_INTERVAL must be a duration string"
        )
    else:
        try:
            seconds = _parse_duration_seconds(interval_raw)
            if seconds < MIN_EXTERNAL_API_INTERVAL_SECONDS:
                errors.append(
                    f"RATE_LIMIT_EXTERNAL_API_INTERVAL={interval_raw!r} violates the "
                    f"{MIN_EXTERNAL_API_INTERVAL_SECONDS}s floor (ADR-000982 / Critical Rule 2)"
                )
        except ValueError as exc:
            errors.append(str(exc))

    env_map = sections_to_env_dict(sections)
    findings = overrides.evaluate(env_map, rules)
    for finding in findings:
        errors.append(
            f"check-env-overrides {finding.severity}: {finding.key} tripped {finding.rule}"
        )

    return errors


def init_sandbox(
    sandbox_dir: pathlib.Path,
    decl_path: pathlib.Path,
    kisage_bin: str,
    actor: str = "alt-operator",
) -> int:
    if not _IDENT_RE.match(actor):
        raise ValueError(f"invalid actor identifier: {actor!r}")

    decl_resolved = decl_path.resolve(strict=True)
    sandbox_abs = sandbox_dir.resolve(strict=False)
    parent_resolved = sandbox_abs.parent.resolve(strict=True)
    if not str(sandbox_abs).startswith(str(parent_resolved) + os.sep):
        raise ValueError(
            f"sandbox path {sandbox_abs} escapes parent directory {parent_resolved}"
        )

    sandbox_abs.mkdir(mode=0o700, exist_ok=True)
    os.chmod(sandbox_abs, 0o700)
    for sub in ("state", "locks"):
        sub_dir = sandbox_abs / sub
        sub_dir.mkdir(mode=0o700, exist_ok=True)
        os.chmod(sub_dir, 0o700)

    target_lkal = sandbox_abs / "host-env.lkal"
    if not target_lkal.exists():
        _write_owner_only_file(target_lkal, INITIAL_TARGET_LKAL)
    else:
        os.chmod(target_lkal, 0o600)

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
        proc = subprocess.run(
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


def main(argv: list[str] | None = None) -> int:
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
        "verify", help="Verify deploy/desired-env.lkal against deploy/host-prereqs.yaml"
    )
    p_verify.add_argument(
        "--declaration", type=pathlib.Path, default=DEFAULT_DECLARATION
    )
    p_verify.add_argument("--manifest", type=pathlib.Path, default=DEFAULT_MANIFEST)

    p_render = sub.add_parser(
        "render-dotenv",
        help="Render converged host-env.lkal to KEY=VALUE dotenv format",
    )
    p_render.add_argument(
        "--target", type=pathlib.Path, default=DEFAULT_SANDBOX / "host-env.lkal"
    )
    p_render.add_argument("--output", type=pathlib.Path, default=None)

    args = parser.parse_args(argv)

    if args.cmd == "verify":
        errs = verify_declaration(args.declaration, args.manifest)
        if errs:
            for err in errs:
                sys.stderr.write(f"ERROR: {err}\n")
            return 1
        print(
            "OK: deploy/desired-env.lkal is in lockstep with deploy/host-prereqs.yaml"
        )
        return 0

    if args.cmd == "render-dotenv":
        dotenv_text = render_dotenv(args.target.resolve(strict=True))
        if args.output is not None:
            _write_owner_only_file(args.output, dotenv_text)
        else:
            sys.stdout.write(dotenv_text)
        return 0

    if args.cmd == "init":
        kisage_bin = _validate_kisage_binary(args.kisage_bin)
        return init_sandbox(args.sandbox_dir, args.declaration, kisage_bin, args.actor)

    return 2


if __name__ == "__main__":
    raise SystemExit(main())
