"""Shared include-chain loader for production compose audits.

Copied in spirit from scripts/compose-netns-cascade-audit.py: the same
`include:` walk, the same `!override` constructor, and the same "later
definition wins" merge. A naive `^  [a-z].*:` grep of compose/*.yaml
counts networks as services (compose/backup.yaml `backup-docker-proxy`)
and is how 93 vs 94 appeared; this loader only reads `services:`.
"""

from __future__ import annotations

import sys
from pathlib import Path

try:
    import yaml
except ImportError as exc:  # pragma: no cover - CI sets this up
    sys.stderr.write(
        "PyYAML is required to load compose files. "
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


def load_yaml(path: Path) -> dict:
    data = yaml.load(path.read_text(encoding="utf-8"), Loader=OverrideLoader)
    return data or {}


def resolve_included(path: Path, seen: set[Path] | None = None) -> list[Path]:
    """Recursively resolve `include:` directives in a compose file."""
    if seen is None:
        seen = set()
    if path in seen:
        return []
    seen.add(path)
    out: list[Path] = [path]
    data = load_yaml(path)
    for inc in data.get("include") or []:
        inc_path = (path.parent / inc).resolve()
        if inc_path.is_file() and inc_path.suffix in (".yml", ".yaml"):
            out.extend(resolve_included(inc_path, seen))
    return out


def production_compose_files(root: Path | None = None) -> list[Path]:
    compose = root or ROOT_COMPOSE
    if not compose.is_file():
        raise SystemExit(f"missing root compose file: {compose}")
    return resolve_included(compose, set())


def production_services(root: Path | None = None) -> dict[str, dict]:
    """Merge `services:` across compose.yaml and everything it includes.

    Only the `services:` mapping is read. Top-level `networks:` / `volumes:`
    / `secrets:` keys are ignored, so a network named like a service cannot
    inflate the inventory.
    """
    services: dict[str, dict] = {}
    for path in production_compose_files(root):
        data = load_yaml(path)
        for name, svc in (data.get("services") or {}).items():
            if isinstance(svc, dict):
                services[name] = svc
    return services


def iter_production_services(
    root: Path | None = None,
) -> list[tuple[Path, str, dict]]:
    """Each declared service with the compose file that defined it."""
    found: list[tuple[Path, str, dict]] = []
    for path in production_compose_files(root):
        data = load_yaml(path)
        for name, svc in (data.get("services") or {}).items():
            if isinstance(svc, dict):
                found.append((path, name, svc))
    return found
