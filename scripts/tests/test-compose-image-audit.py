#!/usr/bin/env python3
"""Tests for scripts/compose-image-audit.py.

Pins the three rules: a registered service with build: must use its GHCR
image, every GHCR image must be built by docker-build (services.yaml), and a
long-running non-profiled service with build: must not stay a host-local
build unless it is allowlisted with a reason.

Run:
    python3 scripts/tests/test-compose-image-audit.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys
from collections.abc import Callable

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

from compose_include import production_services  # noqa: E402 - needs the sys.path entry added above

spec = importlib.util.spec_from_file_location(
    "compose_image_audit", SCRIPTS / "compose-image-audit.py"
)
assert spec is not None
assert spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()

GHCR = "ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-"
REGISTRY = {"known": f"{GHCR}known:", "shared-proxy": f"{GHCR}shared-proxy:"}


def check(name: str, condition: Callable[[], bool]) -> None:
    try:
        ok = bool(condition())
    except Exception as exc:  # noqa: BLE001 - test check wrapper converts unexpected errors into FAIL
        sys.stdout.write(f"  FAIL  {name} ({type(exc).__name__}: {exc})\n")
        RESULTS.failed += 1
        return
    if ok:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        sys.stdout.write(f"  FAIL  {name}\n")
        RESULTS.failed += 1


def run(
    services: dict, long_running: set[str] | None = None, allowlist: dict[str, str] | None = None
) -> list[str]:
    return audit.audit_services(
        "fixture.yaml",
        services,
        REGISTRY,
        set(services) if long_running is None else long_running,
        allowlist or {},
    )


BUILD = {"context": "../somewhere", "dockerfile": "Dockerfile"}

sys.stdout.write("registered service keeps its GHCR coordinate\n")
check(
    "registered service with matching image is clean",
    lambda: run({"known": {"build": BUILD, "image": f"{GHCR}known:${{IMAGE_TAG:-main}}"}}) == [],
)
check(
    "registered service with build and no image is flagged",
    lambda: any("known" in v for v in run({"known": {"build": BUILD}})),
)

sys.stdout.write("every GHCR image is one docker-build pushes\n")
check(
    "image shared by two services resolves to one registry entry",
    lambda: (
        run(
            {
                "a-proxy": {"build": BUILD, "image": f"{GHCR}shared-proxy:${{IMAGE_TAG:-main}}"},
                "b-proxy": {"build": BUILD, "image": f"{GHCR}shared-proxy:${{IMAGE_TAG:-main}}"},
            }
        )
        == []
    ),
)
check(
    "GHCR image with no services.yaml entry is flagged",
    lambda: any(
        "ghost" in v
        for v in run({"svc": {"build": BUILD, "image": f"{GHCR}ghost:${{IMAGE_TAG:-main}}"}})
    ),
)
check(
    "unregistered GHCR image is flagged even without build:",
    lambda: any("ghost" in v for v in run({"svc": {"image": f"{GHCR}ghost:${{IMAGE_TAG:-main}}"}})),
)
check(
    "third-party image is not a GHCR coordinate",
    lambda: run({"db": {"image": "postgres:18"}}) == [],
)

sys.stdout.write("long-running build-only services must ship through GHCR\n")
check(
    "long-running build without image is flagged",
    lambda: any("relay" in v for v in run({"relay": {"build": BUILD}})),
)
check(
    "long-running build with a host-local tag is flagged",
    lambda: any("relay" in v for v in run({"relay": {"build": BUILD, "image": "alt-relay:local"}})),
)
check(
    "non-long-running build (one-shot / profiled) is out of scope",
    lambda: run({"relay-migrator": {"build": BUILD}}, long_running=set()) == [],
)
check(
    "allowlisted long-running build is accepted",
    lambda: (
        run(
            {"model-server": {"build": BUILD}},
            allowlist={"model-server": "weights stay on the host"},
        )
        == []
    ),
)

sys.stdout.write("allowlist cannot rot\n")
check(
    "allowlist entry for an unknown service is stale",
    lambda: any("gone" in v for v in audit.stale_allowlist({}, set(), {"gone": "reason"})),
)
check(
    "allowlist entry for a service that already ships via GHCR is stale",
    lambda: any(
        "fixed" in v
        for v in audit.stale_allowlist(
            {"fixed": {"build": BUILD, "image": f"{GHCR}fixed:${{IMAGE_TAG:-main}}"}},
            {"fixed"},
            {"fixed": "reason"},
        )
    ),
)
check(
    "allowlist entry still needed is not stale",
    lambda: (
        audit.stale_allowlist(
            {"model-server": {"build": BUILD}}, {"model-server"}, {"model-server": "reason"}
        )
        == []
    ),
)
check(
    "every allowlist entry states a reason",
    lambda: all(reason.strip() for reason in audit.BUILD_ONLY_ALLOWLIST.values()),
)

sys.stdout.write("real repository\n")
prod = production_services()
for name in ("otel-relay", "docker-socket-proxy-ro"):
    check(
        f"{name} declares its GHCR image",
        lambda name=name: str(prod[name].get("image", "")).startswith(f"{GHCR}{name}:"),
    )
for name in ("generation-proxy", "embedding-proxy"):
    check(
        f"{name} image is built by docker-build",
        lambda name=name: (
            "inference-proxy" in audit.load_registry()
            and str(prod[name].get("image", "")).startswith(f"{GHCR}inference-proxy:")
        ),
    )
check(
    "production compose has zero violations",
    lambda: audit.collect_violations() == [],
)

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
