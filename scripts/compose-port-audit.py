#!/usr/bin/env python3
"""Fail when a production compose service publishes a port on every NIC.

The house style is `- "9000:9000"`, whose Compose short syntax means
0.0.0.0 — every interface, not localhost. Applied to an application port
that repeats for every service, that turns each plaintext listener into a
second entrance next to the mTLS one, and the mTLS port is the one that
is *not* published. Docker's published ports are DNAT in PREROUTING, so a
host INPUT firewall does not cover them either.

The rule: a published port must bind inside 127.0.0.0/8 or ::1 unless it
is in EDGE_ALLOWLIST below, which names the surfaces that are meant to
answer off-host and says why. Loopback binding keeps every localhost
workflow (hurl e2e, altctl, curl health probes) working unchanged, and
east-west callers were always using the container DNS name.

A second rule covers the services for which loopback is not good enough:
ZERO_PUBLISH_SERVICES must have no `ports:` at all. The first rule alone
cannot express that — adding `- "127.0.0.1:9443:9443"` to an mTLS-only
service passes the loopback check while handing anything on the host a
way past the peer allowlist.

Usage:
  python3 scripts/compose-port-audit.py [-f compose/compose.yaml]
  python3 scripts/compose-port-audit.py --overlays
  python3 scripts/compose-port-audit.py --overlays-only
Exit 0 when clean, 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import argparse
import contextlib
import ipaddress
import json
import os
import subprocess
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
REPO_ROOT = _SCRIPTS.parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import resolve_included  # noqa: E402

# (service, container-side port) -> why it may answer off-host.
EDGE_ALLOWLIST = {
    ("plecto-proxy", 8443): "the edge proxy; every browser entry point terminates here",
    ("pact-broker", 9292): "operator-supplied private-network binding, see compose/pact.yaml",
}
SHORT_PORT_HOST_IP_PARTS = 3
SHORT_PORT_HOST_PORT_PARTS = 2

# service -> why it may publish nothing at all, not even on loopback.
ZERO_PUBLISH_SERVICES = {
    "alt-data-hub": (
        "mTLS-only data plane. Its authorisation is DATAHUB_ALLOWED_PEERS "
        "keyed off the client certificate; a published port reaches the "
        "listener from the host and makes that allowlist decorative"
    ),
}

# Overlay files may legitimately publish ports for local development or LAN GPU host access.
# Keyed strictly by repo-relative path -> dict of "service:container_port" -> reason.
# Note: compose.dev.yaml and compose/compose.dev.yaml depend on base stack services and skip as non-standalone.
OVERLAY_ALLOWLIST: dict[str, dict[str, str]] = {
    "compose.augur.yaml": {
        "knowledge-augur:11434": (
            "standalone GPU-host overlay whose ports must be reachable from the main host over the LAN "
            "(guarded by the host firewall)"
        ),
        "knowledge-embedder:11434": (
            "standalone GPU-host overlay whose ports must be reachable from the main host over the LAN "
            "(guarded by the host firewall)"
        ),
    },
    "compose/dev.yaml": {
        "mock-auth:4001": "local-development overlays never deployed",
        "mock-auth:4002": "local-development overlays never deployed",
        "alt-frontend-sv:4173": "local-development overlays never deployed",
        "alt-frontend-sv:24678": "local-development overlays never deployed",
        "alt-backend:9000": "local-development overlays never deployed",
        "alt-backend:9101": "local-development overlays never deployed",
        "db:5432": "local-development overlays never deployed",
    },
    "compose/frontend-dev.yaml": {
        "mock-auth:4001": "local-development overlays never deployed",
        "mock-auth:4002": "local-development overlays never deployed",
        "mock-auth:4003": "local-development overlays never deployed",
        "alt-frontend-sv:5173": "local-development overlays never deployed",
        "alt-frontend-sv:24678": "local-development overlays never deployed",
    },
}


class ComposeConfigError(RuntimeError):
    """Raised when docker compose config fails on a compose file."""

    def __init__(self, message: str, returncode: int = 1) -> None:
        super().__init__(message)
        self.message = message
        self.returncode = returncode


def relative_path_str(path: Path, repo_root: Path = REPO_ROOT) -> str:
    abs_path = Path(os.path.normpath(path.absolute()))
    abs_root = Path(os.path.normpath(repo_root.absolute()))
    try:
        return str(abs_path.relative_to(abs_root))
    except ValueError:
        return str(path)


def get_overlay_allowlist(path: Path | str | None, repo_root: Path = REPO_ROOT) -> dict[str, str]:
    if path is None:
        return {}
    if isinstance(path, str):
        path = Path(path)
    rel = relative_path_str(path, repo_root=repo_root)
    return OVERLAY_ALLOWLIST.get(rel, {})


def find_overlay_files(root: Path = REPO_ROOT) -> list[Path]:
    """Enumerate overlay compose files not pulled in by compose/compose.yaml include:.

    Discovers:
    1. Any compose/*.yaml or compose/*.yml not included in compose/compose.yaml.
    2. Any compose.*.yaml or compose.*.yml at the repo root (including compose.augur.yaml).
    """
    root_compose = root / "compose" / "compose.yaml"
    included = (
        {p.resolve() for p in resolve_included(root_compose)} if root_compose.is_file() else set()
    )

    overlays: list[Path] = []

    # 1. Overlay files in compose/ directory not pulled in by include:
    compose_dir = root / "compose"
    if compose_dir.is_dir():
        overlays.extend(
            p
            for p in sorted(compose_dir.iterdir())
            if p.suffix in (".yaml", ".yml")
            and p.is_file()
            and p.resolve() not in included
            and p.resolve() != root_compose.resolve()
        )

    # 2. Repo-root overlay compose files like compose.augur.yaml
    for p in sorted(root.glob("compose.*.yaml")):
        if p.is_file() and p.resolve() not in included and p not in overlays:
            overlays.append(p)
    for p in sorted(root.glob("compose.*.yml")):
        if p.is_file() and p.resolve() not in included and p not in overlays:
            overlays.append(p)

    return overlays


def resolved_config(compose_file: Path, repo_root: Path = REPO_ROOT) -> dict:
    # --env-file is not optional: the compose file lives under compose/, so
    # that is the project directory compose would otherwise search for .env,
    # and the repo keeps it at the root instead.
    cmd = ["docker", "compose"]
    env_file = repo_root / ".env"
    if env_file.exists():
        cmd += ["--env-file", str(env_file)]
    cmd += ["-f", str(compose_file), "config", "--format", "json"]

    # Supply default placeholders for environment variables that may be referenced.
    env = {**os.environ}
    env.setdefault("DOCKER_GROUP_ID", "0")

    proc = subprocess.run(  # noqa: S603 -- fixed `docker compose` CLI resolved from PATH (patched in unit tests)
        cmd,
        capture_output=True,
        text=True,
        cwd=repo_root,
        env=env,
        check=False,
    )
    if proc.returncode != 0:
        err_msg = proc.stderr or proc.stdout or "compose config failed\n"
        raise ComposeConfigError(err_msg, proc.returncode)
    return json.loads(proc.stdout)


def is_loopback(host_ip: str) -> bool:
    try:
        return ipaddress.ip_address(host_ip).is_loopback
    except ValueError:
        return False


def zero_publish_violations(cfg: dict, is_overlay: bool = False) -> list[str]:
    """Report ZERO_PUBLISH_SERVICES that publish anything.

    For base production stacks, also reports entries naming a service
    the config does not define (preventing silent renames). For overlay files,
    services not defined in the overlay are expected and skipped.
    """
    services = cfg.get("services") or {}
    found = []
    for name, reason in sorted(ZERO_PUBLISH_SERVICES.items()):
        svc = services.get(name)
        if svc is None:
            if not is_overlay:
                found.append(
                    f"{name} is in ZERO_PUBLISH_SERVICES but no such service is "
                    f"defined; drop the entry or fix the name"
                )
            continue
        published = svc.get("ports") or []
        if published:
            spec_parts = []
            for p in published:
                if isinstance(p, dict):
                    h = p.get("host_ip") or "0.0.0.0"  # noqa: S104 — represents Compose's default bind-all IP when omitted, not a socket bind
                    spec_parts.append(f"{h}:{p.get('published')}->{p.get('target')}")
                else:
                    spec_parts.append(str(p))
            spec = ", ".join(spec_parts)
            found.append(f"{name} publishes {spec} but must publish nothing — {reason}")
    return found


def violations(
    cfg: dict, overlay_path: Path | str | None = None, repo_root: Path = REPO_ROOT
) -> list[str]:
    found = []
    overlay_allow = get_overlay_allowlist(overlay_path, repo_root=repo_root) if overlay_path else {}
    for name, svc in sorted((cfg.get("services") or {}).items()):
        for port in svc.get("ports") or []:
            if isinstance(port, dict):
                host_ip = port.get("host_ip") or "0.0.0.0"  # noqa: S104 — represents Compose's default bind-all IP when omitted, not a socket bind
                published = port.get("published")
                target = port.get("target")
            else:
                p_str = str(port).strip()
                parts = p_str.split(":")
                if len(parts) == SHORT_PORT_HOST_IP_PARTS:
                    host_ip, published, target = parts[0], parts[1], parts[2]
                elif len(parts) == SHORT_PORT_HOST_PORT_PARTS:
                    host_ip, published, target = "0.0.0.0", parts[0], parts[1]  # noqa: S104 — represents Compose's default bind-all IP when omitted, not a socket bind
                else:
                    host_ip, published, target = "0.0.0.0", None, parts[0]  # noqa: S104 — represents Compose's default bind-all IP when omitted, not a socket bind
                with contextlib.suppress(ValueError, TypeError):
                    target = int(target)
            if is_loopback(host_ip):
                continue
            if (name, target) in EDGE_ALLOWLIST:
                continue
            if f"{name}:{target}" in overlay_allow:
                continue
            found.append(f"{name} publishes {host_ip}:{published} -> {target} on every interface")
    return found


def audit_file(
    path: Path, is_overlay: bool = False, repo_root: Path | None = None
) -> tuple[bool, int]:
    effective_root = REPO_ROOT if repo_root is None else repo_root
    rel_path = relative_path_str(path, repo_root=effective_root)
    try:
        cfg = (
            resolved_config(path)
            if repo_root is None
            else resolved_config(path, repo_root=repo_root)
        )
    except (ComposeConfigError, RuntimeError) as exc:
        err_msg = getattr(exc, "message", str(exc))
        if is_overlay:
            first_line = (
                err_msg.strip().splitlines()[0] if err_msg.strip() else "compose config failed"
            )
            sys.stdout.write(f"SKIP (not standalone): {rel_path} — {first_line}\n")
            return False, 0
        sys.stderr.write(f"{err_msg}\n")
        code = getattr(exc, "returncode", 1)
        raise SystemExit(code) from exc

    file_failed = False
    overlay_path = path if is_overlay else None
    found = violations(cfg, overlay_path=overlay_path, repo_root=effective_root)
    zero = zero_publish_violations(cfg, is_overlay=is_overlay)

    if found:
        file_failed = True
        sys.stdout.write(f"Published ports that are not loopback-bound ({rel_path}):\n")
        for v in found:
            sys.stdout.write(f"  - {v}\n")
        sys.stdout.write(
            '\nPrefix the mapping with 127.0.0.1: (e.g. "127.0.0.1:9000:9000"), '
            "or add the service to EDGE_ALLOWLIST in this script with the reason "
            "it must answer off-host.\n"
        )

    if zero:
        file_failed = True
        sys.stdout.write(f"\nServices that must publish no ports at all ({rel_path}):\n")
        for v in zero:
            sys.stdout.write(f"  - {v}\n")
        sys.stdout.write(
            "\nDelete the ports: block. Callers reach these services by "
            "container DNS name over mTLS; if a host-side workflow needs one, "
            "run it inside the network (docker compose exec) rather than "
            "opening a port.\n"
        )

    total = sum(len(s.get("ports") or []) for s in (cfg.get("services") or {}).values())
    if not file_failed:
        if is_overlay:
            sys.stdout.write(
                f"OK ({rel_path}): {total} published ports checked, "
                f"0 zero-publish services applicable (overlay), 0 violations\n"
            )
        else:
            sys.stdout.write(
                f"OK: {total} published ports checked, "
                f"{len(ZERO_PUBLISH_SERVICES)} zero-publish services verified, 0 violations\n"
            )
    return file_failed, total


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-f", "--file", default="compose/compose.yaml")
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=REPO_ROOT,
        help="repository/staging root containing compose/ and .env",
    )
    parser.add_argument(
        "--overlays",
        action="store_true",
        dest="overlays",
        help="Audit overlay compose files in addition to the specified compose file",
    )
    parser.add_argument(
        "--overlays-only",
        action="store_true",
        dest="overlays_only",
        help="Audit only overlay compose files not included in compose/compose.yaml",
    )
    args = parser.parse_args(argv)
    repo_root = args.repo_root

    failed = False

    if args.overlays_only:
        for ov in find_overlay_files(repo_root):
            ov_failed, _ = audit_file(ov, is_overlay=True, repo_root=repo_root)
            if ov_failed:
                failed = True
        return 1 if failed else 0

    base_path = repo_root / args.file
    is_base_overlay = base_path.resolve() != (repo_root / "compose" / "compose.yaml").resolve()

    base_failed, _ = audit_file(base_path, is_overlay=is_base_overlay, repo_root=repo_root)
    if base_failed:
        failed = True

    if args.overlays:
        for ov in find_overlay_files(repo_root):
            ov_failed, _ = audit_file(ov, is_overlay=True, repo_root=repo_root)
            if ov_failed:
                failed = True

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
