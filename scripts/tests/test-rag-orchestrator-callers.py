#!/usr/bin/env python3
"""Verify every mTLS caller of rag-orchestrator is listed in all peer allowlists.

Checks that every compose service under compose/*.yaml configured with an https
RAG_ORCHESTRATOR_URL or RAG_ORCHESTRATOR_CONNECT_URL has its CERT_SUBJECT listed
in all three rag-orchestrator peer allowlist declarations:
  1. compose/rag.yaml (PEER_IDENTITY_ALLOWED_PEERS default)
  2. deploy/desired-env.lkal (mtls_allowlists.RAG_ORCHESTRATOR_ALLOWED_PEERS)
  3. deploy/host-prereqs.yaml (env_allowlists RAG_ORCHESTRATOR_ALLOWED_PEERS required)

Run:
    python3 scripts/tests/test-rag-orchestrator-callers.py
"""

from __future__ import annotations

import argparse
import importlib.util
import pathlib
import re
from typing import Any

ROOT = pathlib.Path(__file__).resolve().parents[2]
DEFAULT_COMPOSE_DIR = ROOT / "compose"
DEFAULT_RAG_COMPOSE = DEFAULT_COMPOSE_DIR / "rag.yaml"
DEFAULT_DECLARATION = ROOT / "deploy" / "desired-env.lkal"
DEFAULT_MANIFEST = ROOT / "deploy" / "host-prereqs.yaml"

CALLER_ENV_KEYS = ("RAG_ORCHESTRATOR_URL", "RAG_ORCHESTRATOR_CONNECT_URL")
ALLOWLIST_KEY = "RAG_ORCHESTRATOR_ALLOWED_PEERS"

_DEFAULTED_VAR_RE = re.compile(r"^\$\{([A-Za-z_][A-Za-z0-9_]*):?-(.*?)\}$")
_REQUIRED_BRACE_RE = re.compile(r"^\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?\?[^}]*)?\}$")
_BARE_VAR_RE = re.compile(r"^\$([A-Za-z_][A-Za-z0-9_]*)$")
_RAG_DEFAULT_ALLOWLIST_RE = re.compile(
    r"^\$\{RAG_ORCHESTRATOR_ALLOWED_PEERS:-(?P<default>[^}]*)\}$"
)

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}" + (f" ({detail})" if detail else ""))
        FAIL += 1


def _load_module(name: str, path: pathlib.Path) -> Any:
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _display_path(path: pathlib.Path) -> str:
    try:
        return str(path.resolve().relative_to(ROOT))
    except ValueError:
        return path.name


def _service_env(svc: dict[str, Any]) -> dict[str, str]:
    raw = svc.get("environment")
    out: dict[str, str] = {}
    if isinstance(raw, dict):
        for key, val in raw.items():
            if val is not None:
                out[str(key).strip()] = str(val)
    elif isinstance(raw, list):
        for item in raw:
            if isinstance(item, str) and "=" in item:
                key, val = item.split("=", 1)
                out[key.strip()] = val
    return out


def _resolve_value(raw: str, upstreams: dict[str, Any]) -> str:
    defaulted = _DEFAULTED_VAR_RE.fullmatch(raw)
    if defaulted is not None:
        return defaulted.group(2)
    req_brace = _REQUIRED_BRACE_RE.fullmatch(raw)
    if req_brace is not None:
        val = upstreams.get(req_brace.group(1))
        return val if isinstance(val, str) else ""
    bare = _BARE_VAR_RE.fullmatch(raw)
    if bare is not None:
        val = upstreams.get(bare.group(1))
        return val if isinstance(val, str) else ""
    return raw


def _read_lkal_declaration(
    kisage_env: Any,
    decl_path: pathlib.Path,
    *,
    lkal_bin: str | None = None,
) -> tuple[set[str] | None, dict[str, Any]]:
    label = _display_path(decl_path)
    try:
        sections = kisage_env.parse_lkal_sections(
            decl_path.read_text(encoding="utf-8"), lkal_bin=lkal_bin
        )
    except OSError as exc:
        check(
            f"read {ALLOWLIST_KEY} from {label}",
            False,
            f"cannot read {label}: {exc.strerror}",
        )
        return None, {}
    except UnicodeDecodeError:
        check(
            f"read {ALLOWLIST_KEY} from {label}",
            False,
            f"{label} is not valid UTF-8",
        )
        return None, {}
    except ValueError as exc:
        check(f"read {ALLOWLIST_KEY} from {label}", False, str(exc))
        return None, {}

    upstreams = sections.get("upstreams")
    upstream_map = upstreams if isinstance(upstreams, dict) else {}
    allowlists = sections.get("mtls_allowlists")
    raw_peers = allowlists.get(ALLOWLIST_KEY) if isinstance(allowlists, dict) else None
    if (
        not isinstance(raw_peers, list)
        or not raw_peers
        or not all(isinstance(p, str) and p.strip() for p in raw_peers)
    ):
        check(
            f"read {ALLOWLIST_KEY} from {label}",
            False,
            f"missing or empty {ALLOWLIST_KEY} in {label}",
        )
        return None, upstream_map
    return {p.strip() for p in raw_peers}, upstream_map


def _read_rag_compose_allowlist(
    compose_include: Any, rag_compose_path: pathlib.Path
) -> set[str] | None:
    label = _display_path(rag_compose_path)
    try:
        doc = compose_include.load_yaml(rag_compose_path)
    except OSError as exc:
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"cannot read {label}: {exc.strerror}",
        )
        return None
    except UnicodeDecodeError:
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"{label} is not valid UTF-8",
        )
        return None
    except compose_include.yaml.YAMLError:
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"invalid YAML in {label}",
        )
        return None

    services = doc.get("services") if isinstance(doc, dict) else None
    rag_svc = services.get("rag-orchestrator") if isinstance(services, dict) else None
    if not isinstance(rag_svc, dict):
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"missing rag-orchestrator service in {label}",
        )
        return None

    raw_entry = _service_env(rag_svc).get("PEER_IDENTITY_ALLOWED_PEERS", "")
    match = _RAG_DEFAULT_ALLOWLIST_RE.fullmatch(raw_entry)
    if match is None:
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"expected PEER_IDENTITY_ALLOWED_PEERS=${{{ALLOWLIST_KEY}:-...}} in {label}",
        )
        return None

    peers = {p.strip() for p in match.group("default").split(",") if p.strip()}
    if not peers:
        check(
            f"read PEER_IDENTITY_ALLOWED_PEERS default from {label}",
            False,
            f"empty PEER_IDENTITY_ALLOWED_PEERS default in {label}",
        )
        return None
    return peers


def _read_host_prereqs_allowlist(
    compose_include: Any, manifest_path: pathlib.Path
) -> set[str] | None:
    label = _display_path(manifest_path)
    try:
        doc = compose_include.load_yaml(manifest_path)
    except OSError as exc:
        check(
            f"read {ALLOWLIST_KEY} required list from {label}",
            False,
            f"cannot read {label}: {exc.strerror}",
        )
        return None
    except UnicodeDecodeError:
        check(
            f"read {ALLOWLIST_KEY} required list from {label}",
            False,
            f"{label} is not valid UTF-8",
        )
        return None
    except compose_include.yaml.YAMLError:
        check(
            f"read {ALLOWLIST_KEY} required list from {label}",
            False,
            f"invalid YAML in {label}",
        )
        return None

    entries = doc.get("env_allowlists") if isinstance(doc, dict) else None
    matches = [
        entry
        for entry in (entries or [])
        if isinstance(entry, dict) and entry.get("key") == ALLOWLIST_KEY
    ]
    if len(matches) != 1:
        check(
            f"read {ALLOWLIST_KEY} required list from {label}",
            False,
            f"expected exactly one env_allowlists entry for {ALLOWLIST_KEY} in {label}",
        )
        return None

    required = matches[0].get("required")
    if (
        not isinstance(required, list)
        or not required
        or not all(isinstance(p, str) and p.strip() for p in required)
    ):
        check(
            f"read {ALLOWLIST_KEY} required list from {label}",
            False,
            f"missing or empty required list for {ALLOWLIST_KEY} in {label}",
        )
        return None
    return {p.strip() for p in required}


def _discover_mtls_callers(
    compose_include: Any,
    compose_dir: pathlib.Path,
    upstreams: dict[str, Any],
) -> dict[str, list[str]]:
    compose_files = sorted(compose_dir.glob("*.yaml"))
    if not compose_files:
        check(
            f"discover compose/*.yaml files in {_display_path(compose_dir)}",
            False,
            f"no *.yaml files found in {_display_path(compose_dir)}",
        )
        return {}

    callers: dict[str, list[str]] = {}
    for compose_file in compose_files:
        file_label = _display_path(compose_file)
        try:
            doc = compose_include.load_yaml(compose_file)
        except OSError as exc:
            check(
                f"parse {file_label}",
                False,
                f"cannot read {file_label}: {exc.strerror}",
            )
            continue
        except UnicodeDecodeError:
            check(
                f"parse {file_label}",
                False,
                f"{file_label} is not valid UTF-8",
            )
            continue
        except compose_include.yaml.YAMLError:
            check(
                f"parse {file_label}",
                False,
                f"invalid YAML in {file_label}",
            )
            continue

        services = doc.get("services") if isinstance(doc, dict) else None
        if not isinstance(services, dict):
            continue

        for svc_name, svc in services.items():
            if not isinstance(svc, dict):
                continue
            env = _service_env(svc)
            present_keys = [k for k in CALLER_ENV_KEYS if k in env]
            if not present_keys:
                continue

            has_https_target = False
            for key in present_keys:
                resolved = _resolve_value(env[key], upstreams).strip()
                if resolved.startswith("https://"):
                    has_https_target = True
                elif (
                    resolved.startswith("http://")
                    or resolved == "disabled"
                    or resolved == ""
                ):
                    # Plaintext http:// entries belong to E2E stub stacks where inbound mTLS is disabled.
                    continue
                else:
                    check(
                        f"resolve {key} scheme for {file_label}:{svc_name}",
                        False,
                        f"unexpected scheme or unresolved interpolation for {key} in {file_label}:{svc_name}",
                    )

            if not has_https_target:
                continue

            subject = _resolve_value(env.get("CERT_SUBJECT", ""), upstreams).strip()
            if not subject or "$" in subject:
                check(
                    f"read CERT_SUBJECT for {file_label}:{svc_name}",
                    False,
                    f"missing CERT_SUBJECT on mTLS caller {svc_name} in {file_label}",
                )
                continue

            callers.setdefault(subject, []).append(f"{file_label}:{svc_name}")

    return callers


def main(argv: list[str] | None = None) -> int:
    global PASS, FAIL
    PASS = 0
    FAIL = 0

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--compose-dir",
        type=pathlib.Path,
        default=DEFAULT_COMPOSE_DIR,
    )
    parser.add_argument(
        "--rag-compose",
        type=pathlib.Path,
        default=DEFAULT_RAG_COMPOSE,
    )
    parser.add_argument(
        "--declaration",
        type=pathlib.Path,
        default=DEFAULT_DECLARATION,
    )
    parser.add_argument(
        "--manifest",
        type=pathlib.Path,
        default=DEFAULT_MANIFEST,
    )
    parser.add_argument(
        "--lkal-bin",
        type=str,
        default=None,
    )
    args = parser.parse_args(argv)

    compose_include = _load_module(
        "compose_include", ROOT / "scripts" / "compose_include.py"
    )
    kisage_env = _load_module("kisage_env", ROOT / "scripts" / "kisage-env.py")

    print("rag-orchestrator mTLS caller allowlist parity")

    lkal_peers, upstreams = _read_lkal_declaration(
        kisage_env, args.declaration, lkal_bin=args.lkal_bin
    )
    rag_compose_peers = _read_rag_compose_allowlist(compose_include, args.rag_compose)
    prereq_peers = _read_host_prereqs_allowlist(compose_include, args.manifest)
    callers = _discover_mtls_callers(compose_include, args.compose_dir, upstreams)

    if not callers and FAIL == 0:
        check(
            f"discover mTLS callers in {_display_path(args.compose_dir)}/*.yaml",
            False,
            "no https rag-orchestrator callers discovered",
        )

    declaration_sites: tuple[tuple[str, set[str] | None], ...] = (
        (
            f"{_display_path(args.rag_compose)} (PEER_IDENTITY_ALLOWED_PEERS default)",
            rag_compose_peers,
        ),
        (
            f"{_display_path(args.declaration)} ({ALLOWLIST_KEY})",
            lkal_peers,
        ),
        (
            f"{_display_path(args.manifest)} ({ALLOWLIST_KEY} required)",
            prereq_peers,
        ),
    )

    for caller in sorted(callers):
        origins = ", ".join(callers[caller])
        for site_label, allowed_peers in declaration_sites:
            if allowed_peers is None:
                continue
            present = caller in allowed_peers
            check(
                f"caller {caller} ({origins}) in {site_label}",
                present,
                "" if present else f"missing caller {caller} in {site_label}",
            )

    print(f"\n{PASS} passed, {FAIL} failed")
    return 1 if FAIL else 0


if __name__ == "__main__":
    raise SystemExit(main())
