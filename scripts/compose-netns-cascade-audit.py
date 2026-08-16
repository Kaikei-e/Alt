#!/usr/bin/env python3
"""Fail when a netns-piggyback sidecar has no row in the cascade script.

`network_mode: "service:X"` freezes the *container id* of X into the
sidecar's netns. When the deploy tool force-recreates X alone, the sidecar
keeps pointing at the dead netns and its reverse-proxy listener disappears
from the network — and `depends_on.restart: true` does not fire on a
force-recreate, so compose never notices. scripts/cascade-pki-sidecars.sh
closes that hole by recreating each sidecar whose netns no longer matches
its parent, but only for the rows hand-written in its NETNS_SIDECARS array.

A sidecar added to compose and forgotten in that array is therefore an
armed netns-orphan incident with no signal until east-west mTLS traffic to
the parent starts failing in production. This audit is that signal: it
compares the two lists in both directions and checks that each row names
container names that actually exist by compose's own naming rules.

Rules:

- Every service in the production include chain with
  `network_mode: service:<parent>` MUST have a NETNS_SIDECARS row.
- Every NETNS_SIDECARS row MUST name a service that still uses
  `network_mode: service:<parent>` (no stale rows — a stale row makes the
  script inspect a container that no longer exists and report "skip").
- Each row's container names MUST match what compose actually creates:
  the service's own `container_name:` when it declares one, otherwise
  `<project>-<service>-1`. A row naming a container that never exists
  fails open: the script prints "skip: ... not running" and exits 0.

Usage: python3 scripts/compose-netns-cascade-audit.py
Exit 0 when clean, 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError as exc:  # pragma: no cover - CI sets this up
    sys.stderr.write(
        "PyYAML is required to run compose-netns-cascade-audit. "
        "Install it with `pip install pyyaml` or run in a repo dev shell.\n"
    )
    raise SystemExit(2) from exc


class OverrideLoader(yaml.SafeLoader):
    """Tolerate the `!override` tag that compose files use for anchor merges."""


def _construct_override(loader, node):  # type: ignore[no-untyped-def]
    if isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node, deep=True)
    if isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node, deep=True)
    return loader.construct_scalar(node)


OverrideLoader.add_constructor("!override", _construct_override)


REPO_ROOT = Path(__file__).resolve().parent.parent
ROOT_COMPOSE = REPO_ROOT / "compose" / "compose.yaml"
CASCADE_SCRIPT = REPO_ROOT / "scripts" / "cascade-pki-sidecars.sh"

# Matches the COMPOSE_PROJECT default in cascade-pki-sidecars.sh, which is
# also what scripts/deploy.sh runs with. Compose derives the container name
# of a service without `container_name:` as `<project>-<service>-<index>`.
COMPOSE_PROJECT = "alt"

NETNS_PARENT_RE = re.compile(r"^service:(.+)$")
# One NETNS_SIDECARS entry: a double-quoted colon-separated triplet.
ROW_RE = re.compile(r'"([^"]*)"')
ARRAY_RE = re.compile(r"^NETNS_SIDECARS=\(\s*$(.*?)^\)\s*$", re.MULTILINE | re.DOTALL)


def resolve_included(path: Path, seen: set[Path]) -> list[Path]:
    """Recursively resolve `include:` directives in a compose file."""
    if path in seen:
        return []
    seen.add(path)
    out: list[Path] = [path]
    data = yaml.load(path.read_text(encoding="utf-8"), Loader=OverrideLoader)
    for inc in (data or {}).get("include", []) or []:
        inc_path = (path.parent / inc).resolve()
        if inc_path.is_file() and inc_path.suffix in (".yml", ".yaml"):
            out.extend(resolve_included(inc_path, seen))
    return out


def production_services() -> dict[str, dict]:
    """Merge `services:` across compose.yaml and everything it includes."""
    if not ROOT_COMPOSE.is_file():
        raise SystemExit(f"missing root compose file: {ROOT_COMPOSE}")
    services: dict[str, dict] = {}
    for path in resolve_included(ROOT_COMPOSE, set()):
        data = yaml.load(path.read_text(encoding="utf-8"), Loader=OverrideLoader)
        for name, svc in ((data or {}).get("services") or {}).items():
            if isinstance(svc, dict):
                services[name] = svc
    return services


def container_name(service: str, services: dict[str, dict]) -> str:
    """The name compose gives a service's container.

    An explicit `container_name:` wins; otherwise compose composes
    `<project>-<service>-<index>` and the whole stack runs at index 1.
    """
    declared = (services.get(service) or {}).get("container_name")
    if isinstance(declared, str) and declared:
        return declared
    return f"{COMPOSE_PROJECT}-{service}-1"


def netns_sidecars(services: dict[str, dict]) -> dict[str, str]:
    """Return {sidecar_service: parent_service} for `network_mode: service:X`."""
    found: dict[str, str] = {}
    for name, svc in services.items():
        mode = svc.get("network_mode")
        if not isinstance(mode, str):
            continue
        match = NETNS_PARENT_RE.match(mode.strip())
        if match:
            found[name] = match.group(1).strip()
    return found


def parse_cascade_rows(text: str) -> list[tuple[str, str, str]]:
    """Parse the NETNS_SIDECARS array of cascade-pki-sidecars.sh.

    Raises SystemExit rather than returning empty when the array cannot be
    found: a parser that silently sees nothing turns this audit green for
    every sidecar at once, which is the failure mode it exists to prevent.
    """
    block = ARRAY_RE.search(text)
    if block is None:
        raise SystemExit(
            f"NETNS_SIDECARS=( ... ) array not found in {CASCADE_SCRIPT}; "
            "the audit parser and the script have diverged"
        )
    rows: list[tuple[str, str, str]] = []
    for line in block.group(1).splitlines():
        code = line.split("#", 1)[0]
        for raw in ROW_RE.findall(code):
            parts = raw.split(":")
            if len(parts) != 3:
                raise SystemExit(
                    f"malformed NETNS_SIDECARS row {raw!r}: expected "
                    "'sidecar-service:sidecar-container:parent-container'"
                )
            rows.append((parts[0], parts[1], parts[2]))
    return rows


def audit() -> list[str]:
    services = production_services()
    sidecars = netns_sidecars(services)
    if not sidecars:
        raise SystemExit(
            "no `network_mode: service:<parent>` service found in the "
            "production compose chain; the audit parser is broken"
        )

    rows = parse_cascade_rows(CASCADE_SCRIPT.read_text(encoding="utf-8"))
    by_service = {row[0]: row for row in rows}

    violations: list[str] = []

    for svc, parent in sorted(sidecars.items()):
        expected = (svc, container_name(svc, services), container_name(parent, services))
        row = by_service.get(svc)
        if row is None:
            violations.append(
                f"{svc} shares the netns of {parent} but has no NETNS_SIDECARS row; "
                f"a force-recreate of {parent} orphans it silently. Add:\n"
                f'      "{":".join(expected)}"'
            )
            continue
        if row[1] != expected[1]:
            violations.append(
                f"{svc}: row names sidecar container {row[1]!r} but compose creates "
                f"{expected[1]!r}; the cascade skips a container it cannot inspect"
            )
        if row[2] != expected[2]:
            violations.append(
                f"{svc}: row names parent container {row[2]!r} but compose creates "
                f"{expected[2]!r}; the cascade skips a container it cannot inspect"
            )

    for svc in sorted(by_service):
        if svc not in sidecars:
            violations.append(
                f"{svc} has a NETNS_SIDECARS row but no longer uses "
                "`network_mode: service:<parent>`; drop the stale row"
            )

    return violations


def main() -> int:
    violations = audit()
    if violations:
        sys.stderr.write(
            "compose-netns-cascade-audit FAILED — scripts/cascade-pki-sidecars.sh\n"
            "must list every service that joins a parent's network namespace, with\n"
            "the container names compose actually creates. Anything missing here is\n"
            "a sidecar that survives a parent force-recreate pointing at a dead\n"
            "netns, with no listener and no error.\n\n"
        )
        for v in violations:
            sys.stderr.write(f"  - {v}\n")
        return 1

    rows = parse_cascade_rows(CASCADE_SCRIPT.read_text(encoding="utf-8"))
    print(f"compose-netns-cascade-audit OK — {len(rows)} netns sidecar(s) covered.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
