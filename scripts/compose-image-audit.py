#!/usr/bin/env python3
"""Fail when a production compose service would run an image that pull-deploy
cannot roll.

Two incidents shaped this gate. On 2026-04-19 a service carried `build:`
without `image:`, so `docker compose pull` was a no-op and the container
kept the pre-fix local build (only search-indexer updated; every other
service kept running the old binary). Later, the inference proxies,
otel-relay and docker-socket-proxy-ro shipped as host-local builds — or as a
GHCR coordinate nothing ever pushed — so a fresh host could not even pull
them.

Rules:

- Services in services.yaml with kind in {pacticipant, runtime} are the
  canonical GHCR-pushed set. The image coordinate is
  ``ghcr.io/<GHCR_OWNER>/alt-<name>:<IMAGE_TAG>``.
- A: a production service whose name is a registry entry and that carries
  ``build:`` MUST also carry ``image:`` with that entry's coordinate.
- B: every ``image: ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-<x>:`` MUST name a
  registry entry, otherwise docker-build never pushes it.
- C: a long-running, non-profiled service (classification shared with
  compose-ops-surface-audit.py) that carries ``build:`` MUST ship through
  a GHCR image, unless BUILD_ONLY_ALLOWLIST names it with a reason. An
  allowlist entry that no longer matches such a service fails too.
- Production fragments are compose/compose.yaml and everything it
  includes, minus the staging / dev / pact-only files that alt-deploy
  never deploys from.

Exit 0 when clean, exit 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import (  # noqa: E402
    REPO_ROOT,
    load_yaml,
    production_services,
)
from compose_include import production_compose_files as include_chain  # noqa: E402

SERVICES_YAML = REPO_ROOT / "services.yaml"
GHCR_PREFIX = "ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-"

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

# Long-running services that stay host-local builds on purpose.
BUILD_ONLY_ALLOWLIST: dict[str, str] = {
    "news-creator-backend": "Ollama GPU runtime; model weights live in a host volume",
    "knowledge-embedder-local": "Ollama embedding runtime; model weights live in a host volume",
    "rerank-local": "ONNX reranker; the quantized model is produced into a host volume at first boot",
    "rag-db": "stateful PostgreSQL extended with pgvector",
    "recap-db": "stateful PostgreSQL extended with pg_cron",
}


def _load_ops_surface():  # type: ignore[no-untyped-def]
    spec = importlib.util.spec_from_file_location(
        "compose_ops_surface_audit", _SCRIPTS / "compose-ops-surface-audit.py"
    )
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_registry() -> dict[str, str]:
    """Return {service_name: ghcr_coordinate_prefix} for GHCR-built services."""
    data = load_yaml(SERVICES_YAML)
    registry: dict[str, str] = {}
    for svc in data.get("services", []):
        if svc.get("kind", "") not in ("pacticipant", "runtime"):
            continue
        name = svc["name"]
        registry[name] = f"{GHCR_PREFIX}{name}:"
    return registry


def production_compose_files() -> list[Path]:
    return [p for p in include_chain() if p.name not in SKIP_FRAGMENTS]


def long_running_services(services: dict[str, dict]) -> set[str]:
    return set(_load_ops_surface().classify_services(services)["long_running"])


def ghcr_name(image: object) -> str | None:
    """`<x>` of a ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-<x>:<tag> image, else None."""
    if not isinstance(image, str) or not image.startswith(GHCR_PREFIX):
        return None
    return image[len(GHCR_PREFIX):].split(":", 1)[0]


def audit_services(
    label: str,
    services: dict[str, dict],
    registry: dict[str, str],
    long_running: set[str],
    allowlist: dict[str, str],
) -> list[str]:
    """Return human-readable violations of rules A, B and C for one file."""
    violations: list[str] = []
    for name, svc in services.items():
        if not isinstance(svc, dict):
            continue
        image = svc.get("image", "")
        built = "build" in svc
        pushed = ghcr_name(image)
        if built and name in registry:
            if not isinstance(image, str) or not image.startswith(registry[name]):
                violations.append(
                    f"{label}: service {name!r} has build: but "
                    f"image: does not reference {registry[name]}<tag>"
                    + (f" (found {image!r})" if image else " (no image: key)")
                )
        elif built and pushed is None and name in long_running and name not in allowlist:
            violations.append(
                f"{label}: long-running service {name!r} has build: but no GHCR "
                f"image:, so pull-deploy can never roll it"
                + (f" (found {image!r})" if image else "")
                + f". Register it in services.yaml and set "
                f"image: {GHCR_PREFIX}<name>:${{IMAGE_TAG:-main}}, or add it to "
                f"BUILD_ONLY_ALLOWLIST with a reason"
            )
        if pushed is not None and pushed not in registry:
            violations.append(
                f"{label}: service {name!r} uses {image!r} but services.yaml has no "
                f"pacticipant/runtime entry {pushed!r}, so docker-build never pushes it"
            )
    return violations


def stale_allowlist(
    services: dict[str, dict],
    long_running: set[str],
    allowlist: dict[str, str],
) -> list[str]:
    stale: list[str] = []
    for name in sorted(allowlist):
        svc = services.get(name)
        if not isinstance(svc, dict) or "build" not in svc or name not in long_running:
            stale.append(
                f"BUILD_ONLY_ALLOWLIST: {name!r} is not a long-running production "
                f"service with build:; drop the entry"
            )
        elif ghcr_name(svc.get("image")) is not None:
            stale.append(
                f"BUILD_ONLY_ALLOWLIST: {name!r} already ships through GHCR; drop the entry"
            )
    return stale


def audit_pki_workload_fleet(path: Path) -> list[str]:
    """Final cutover: pki.yaml must not declare pki-agent-* workload services."""
    if path.name != "pki.yaml":
        return []
    data = load_yaml(path)
    leftovers = [
        name
        for name in (data.get("services") or {})
        if str(name).startswith("pki-agent-")
    ]
    if leftovers:
        return [
            f"{path.name}: pki-agent workload services still declared: "
            f"{sorted(leftovers)} (dual writer after in-process cutover)"
        ]
    return []


def collect_violations() -> list[str]:
    registry = load_registry()
    if not registry:
        raise SystemExit("services.yaml yielded no pacticipant/runtime entries")
    long_running = long_running_services(production_services())

    audited: dict[str, dict] = {}
    violations: list[str] = []
    for path in production_compose_files():
        services = load_yaml(path).get("services") or {}
        audited.update(services)
        violations.extend(
            audit_services(path.name, services, registry, long_running, BUILD_ONLY_ALLOWLIST)
        )
        violations.extend(audit_pki_workload_fleet(path))
    violations.extend(stale_allowlist(audited, long_running, BUILD_ONLY_ALLOWLIST))
    return violations


def main() -> int:
    violations = collect_violations()
    if violations:
        sys.stderr.write(
            "compose-image-audit FAILED — every image pull-deploy runs must come\n"
            "from `ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-<name>:${IMAGE_TAG:-main}`,\n"
            "with <name> registered in services.yaml so docker-build pushes it.\n\n"
        )
        for v in violations:
            sys.stderr.write(f"  - {v}\n")
        return 1

    print(
        f"compose-image-audit OK — {len(production_compose_files())} compose "
        f"fragment(s), {len(load_registry())} GHCR-registered service(s), "
        f"{len(BUILD_ONLY_ALLOWLIST)} allowlisted build-only service(s) verified."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
