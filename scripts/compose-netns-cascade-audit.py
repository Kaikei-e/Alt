#!/usr/bin/env python3
"""Fail when a forbidden netns-piggyback pki sidecar exists.

Wave 4 Pattern B cutover moved inbound TLS into the parent process.
`network_mode: service:<parent>` is forbidden for pki-agent sidecars:
sharing the parent's netns is what made :9443 a sidecar listener that
a parent-only force-recreate could orphan (ADR-000782 / ADR-000802).

scripts/cascade-pki-sidecars.sh is a hard-failing tombstone. There is
no NETNS_SIDECARS array to keep in lockstep. This audit is the signal
that the forbidden topology has not come back:

- Zero `network_mode: service:<parent>` pki-agent services in the
  production include chain. An empty success is the Wave 4 contract,
  not a parser no-op.
- Re-adding a netns sidecar fails this job with the service name.
- Re-adding a NETNS_SIDECARS array to the tombstone also fails.

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

# Matches the COMPOSE_PROJECT default the retired cascade script used.
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
    """Parse a NETNS_SIDECARS array if present.

    A missing array is the tombstone form (zero rows). A present-but
    unparseable array still raises: that is how a silently empty parser
    is prevented from reporting "full coverage" of a list it cannot see.
    """
    block = ARRAY_RE.search(text)
    if block is None:
        return []
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
    violations: list[str] = []

    for svc, parent in sorted(sidecars.items()):
        if svc.startswith("pki-agent-"):
            violations.append(
                f"{svc} uses network_mode: service:{parent}; Wave 4 forbids "
                "pki-agent netns sharing. Convert to cert-only (independent netns)."
            )
        else:
            violations.append(
                f"{svc} uses network_mode: service:{parent}; no cascade repair "
                "surface remains. Do not reintroduce netns sharing."
            )

    if CASCADE_SCRIPT.is_file():
        text = CASCADE_SCRIPT.read_text(encoding="utf-8")
        rows = parse_cascade_rows(text)
        for row in rows:
            violations.append(
                f"{row[0]} has a NETNS_SIDECARS row in the retired cascade "
                "script; drop the array — Wave 4 left no netns to repair"
            )

    return violations


def main() -> int:
    violations = audit()
    if violations:
        sys.stderr.write(
            "compose-netns-cascade-audit FAILED — Wave 4 forbids "
            "`network_mode: service:<parent>` on pki-agent sidecars. "
            "Inbound TLS lives in the parent process; a netns-sharing "
            "sidecar re-opens the orphan class this audit exists to close.\n\n"
        )
        for v in violations:
            sys.stderr.write(f"  - {v}\n")
        return 1

    print("compose-netns-cascade-audit OK — 0 forbidden netns pki sidecar(s).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
