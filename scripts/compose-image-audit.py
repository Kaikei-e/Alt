#!/usr/bin/env python3
"""Fail when a production compose service builds a GHCR-registered image
without declaring `image:` — that combination silently keeps the running
container on a stale local image during pull-deploy, which is exactly the
failure mode that bit Acolyte report-regeneration on 2026-04-19 (only
search-indexer updated; every other service kept running the pre-fix
binary).

Rules:

- Services in services.yaml with kind in {pacticipant, runtime} are the
  canonical GHCR-pushed set. The image coordinate is
  ``ghcr.io/<GHCR_OWNER>/alt-<name>:<IMAGE_TAG>``.
- For every compose/*.yaml under the production include set (resolved
  recursively from compose/compose.yaml), each service that carries a
  ``build:`` and whose name matches a canonical entry MUST also carry an
  ``image:`` that references the expected GHCR coordinate.
- Staging/dev compose files are out of scope because alt-deploy uses the
  production file path.

Exit 0 when clean, exit 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError as exc:  # pragma: no cover - CI sets this up
    sys.stderr.write(
        "PyYAML is required to run compose-image-audit. "
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
COMPOSE_DIR = REPO_ROOT / "compose"
SERVICES_YAML = REPO_ROOT / "services.yaml"
ROOT_COMPOSE = COMPOSE_DIR / "compose.yaml"

# Compose fragments that describe staging / dev / pact-only topologies.
# alt-deploy never deploys from these, so missing image: is harmless.
SKIP_FRAGMENTS = {
    "compose.dev.yaml",
    "compose.staging.yaml",
    "dev.yaml",
    "frontend-dev.yaml",
    "pact.yaml",
    "load-test.yaml",
}


def load_registry() -> dict[str, str]:
    """Return {service_name: ghcr_coordinate_prefix} for GHCR-built services."""
    data = yaml.safe_load(SERVICES_YAML.read_text(encoding="utf-8"))
    registry: dict[str, str] = {}
    for svc in data.get("services", []):
        kind = svc.get("kind", "")
        if kind not in ("pacticipant", "runtime"):
            continue
        name = svc["name"]
        registry[name] = f"ghcr.io/${{GHCR_OWNER:-kaikei-e}}/alt-{name}:"
    return registry


def resolve_included(path: Path, seen: set[Path]) -> list[Path]:
    """Recursively resolve `include:` directives in a compose file."""
    if path in seen:
        return []
    seen.add(path)
    out: list[Path] = [path]
    try:
        data = yaml.load(path.read_text(encoding="utf-8"), Loader=OverrideLoader)
    except yaml.YAMLError as exc:  # malformed compose file surfaces elsewhere
        sys.stderr.write(f"[warn] failed to parse {path.name}: {exc}\n")
        return out
    for inc in (data or {}).get("include", []) or []:
        inc_path = (path.parent / inc).resolve()
        if inc_path.is_file() and inc_path.suffix in (".yml", ".yaml"):
            out.extend(resolve_included(inc_path, seen))
    return out


def production_compose_files() -> list[Path]:
    """Production fragments = compose.yaml + everything it transitively includes."""
    if not ROOT_COMPOSE.is_file():
        raise SystemExit(f"missing root compose file: {ROOT_COMPOSE}")
    resolved = resolve_included(ROOT_COMPOSE, set())
    return [p for p in resolved if p.name not in SKIP_FRAGMENTS]


def audit_file(path: Path, registry: dict[str, str]) -> list[str]:
    """Return a list of human-readable violations for one compose file."""
    violations: list[str] = []
    data = yaml.load(path.read_text(encoding="utf-8"), Loader=OverrideLoader)
    services = (data or {}).get("services") or {}
    for name, svc in services.items():
        if not isinstance(svc, dict):
            continue
        if "build" not in svc:
            continue
        if name not in registry:
            # Service doesn't go through GHCR build (migrators, local DBs).
            continue
        image = svc.get("image", "")
        if not isinstance(image, str) or not image.startswith(registry[name]):
            violations.append(
                f"{path.name}: service {name!r} has build: but "
                f"image: does not reference {registry[name]}<tag>"
                + (f" (found {image!r})" if image else " (no image: key)")
            )
    return violations


def audit_pki_anchor(path: Path) -> list[str]:
    """pki.yaml uses a YAML anchor for pki-agent — check the raw text."""
    if path.name != "pki.yaml":
        return []
    raw = path.read_text(encoding="utf-8")
    match = re.search(r"x-pki-agent:\s*&pki-agent\n((?:\s{2,}.*\n)+)", raw)
    if not match:
        return [f"{path.name}: x-pki-agent anchor block not found"]
    body = match.group(1)
    if "image: ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-pki-agent:" not in body:
        return [
            f"{path.name}: x-pki-agent anchor must set "
            "image: ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-pki-agent:${IMAGE_TAG:-main}"
        ]
    return []


def main() -> int:
    registry = load_registry()
    if not registry:
        sys.stderr.write("services.yaml yielded no pacticipant/runtime entries\n")
        return 2

    files = production_compose_files()
    all_violations: list[str] = []
    for path in files:
        all_violations.extend(audit_file(path, registry))
        all_violations.extend(audit_pki_anchor(path))

    if all_violations:
        sys.stderr.write(
            "compose-image-audit FAILED — every GHCR-registered service must set\n"
            "`image: ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-<name>:${IMAGE_TAG:-main}`\n"
            "alongside its `build:` block so pull-deploy actually rolls the new\n"
            "container. Add the missing lines and re-run.\n\n"
        )
        for v in all_violations:
            sys.stderr.write(f"  - {v}\n")
        return 1

    print(f"compose-image-audit OK — {len(files)} compose fragment(s), "
          f"{len(registry)} GHCR-registered service(s) verified.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
