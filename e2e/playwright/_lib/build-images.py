#!/usr/bin/env python3
"""Build the container images one Playwright E2E suite's staging slice needs.

    python3 e2e/playwright/_lib/build-images.py <suite> [--dry-run]

Reads `e2e/playwright/suites.yaml` for *which* images a suite needs and
`services.yaml` — the repository's existing registry — for *how* to build each
one. Nothing about a build lives in two places: the alt-backend trio's
`--build-arg BINARY=` is declared once, in services.yaml, and both CI and a
developer's laptop reach it through here.

Environment
-----------
    IMAGE_TAG   tag to apply (default: ci). `run.sh` defaults to the same, so
                `build-images.py mq-hub && bash e2e/playwright/mq-hub/run.sh`
                tests what you just built.
    GHCR_OWNER  registry namespace (default: kaikei-e)
    NO_CACHE=1  skip the registry layer-cache import

Layer cache
-----------
Each build imports `ghcr.io/<owner>/alt-<cache-key>:buildcache`, the cache
docker-build.yaml publishes on pushes to main. The cache key is the *source
directory*, not the image name: alt-backend, alt-harvester and alt-data-hub
are three images out of one Go module whose dependency layers sit above
`ARG BINARY`, so builds 2 and 3 are nearly free once build 1 is warm. Keying on
the image name instead would give each of them a cold cache.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
SUITES_YAML = ROOT / "e2e" / "playwright" / "suites.yaml"
SERVICES_YAML = ROOT / "services.yaml"


class BuildError(RuntimeError):
    """A build could not be described or did not succeed."""


def load(path: Path) -> dict:
    try:
        with path.open() as handle:
            return yaml.safe_load(handle) or {}
    except OSError as exc:
        raise BuildError(f"cannot read {path}: {exc}") from exc


def service_index(registry: dict) -> dict[str, dict]:
    return {entry["name"]: entry for entry in registry.get("services", [])}


def discover_dockerfile(directory: Path, name: str) -> Path:
    """Find the Dockerfile for a service that declares no explicit path.

    services.yaml's fallback rule is "Dockerfile discovered under that
    directory". In practice that means `Dockerfile` or `Dockerfile.<name>`;
    anything else is ambiguous and must be declared, so this raises rather than
    guessing — a guessed Dockerfile builds the wrong image and the suite then
    fails somewhere far from the cause.
    """
    for candidate in (directory / "Dockerfile", directory / f"Dockerfile.{name}"):
        if candidate.is_file():
            return candidate
    found = sorted(p for p in directory.glob("Dockerfile*") if p.is_file())
    if len(found) == 1:
        return found[0]
    raise BuildError(
        f"cannot determine the Dockerfile for '{name}' under {directory} "
        f"(candidates: {[p.name for p in found] or 'none'}). Declare "
        f"build.dockerfile for it in services.yaml."
    )


def build_command(
    image: str,
    suites: dict,
    services: dict[str, dict],
    tag: str,
    owner: str,
    use_cache: bool,
) -> list[str]:
    stub = suites.get("stubs", {}).get(image)
    entry = services.get(image)

    if stub is not None:
        context = ROOT / stub["context"]
        dockerfile = context / "Dockerfile"
        # No cache refs: docker-build.yaml builds registered services only, so
        # no `:buildcache` is ever published for a stub. Importing one that does
        # not exist costs a registry round-trip and a warning on every build for
        # nothing — and these contexts are a few hundred lines each.
        cache_keys: list[str] = []
        build_args: dict[str, str] = {}
    elif entry is not None:
        spec = entry.get("build") or {}
        # The historical fallback: the service name is its directory.
        source_dir = ROOT / spec.get("dir", image)
        context = source_dir
        declared = spec.get("dockerfile")
        dockerfile = ROOT / declared if declared else discover_dockerfile(source_dir, image)
        # Mirrors docker-build.yaml: the image's own buildcache, plus the
        # source directory's when they differ. alt-backend, alt-harvester and
        # alt-data-hub are three images out of one Go module whose dependency
        # layers sit above `ARG BINARY`, so the shared `alt-alt-backend` cache
        # is what makes builds 2 and 3 nearly free. Keying only on the image
        # name would give each of them a cold cache.
        cache_keys = [image] if source_dir.name == image else [image, source_dir.name]
        build_args = {str(k): str(v) for k, v in (spec.get("args") or {}).items()}
    else:
        raise BuildError(
            f"image '{image}' is neither a service in services.yaml nor a stub in "
            f"{SUITES_YAML.relative_to(ROOT)}. Add it to one of them."
        )

    if not dockerfile.is_file():
        raise BuildError(f"{dockerfile} does not exist (image '{image}')")

    command = ["docker", "buildx", "build"]
    if use_cache:
        for key in cache_keys:
            command += [
                "--cache-from",
                f"type=registry,ref=ghcr.io/{owner}/alt-{key}:buildcache",
            ]
    for key, value in build_args.items():
        command += ["--build-arg", f"{key}={value}"]
    command += [
        "-t",
        f"ghcr.io/{owner}/alt-{image}:{tag}",
        "-f",
        str(dockerfile),
        str(context),
    ]
    return command


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("suite", help="a key under `suites:` in e2e/playwright/suites.yaml")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print the docker commands instead of running them",
    )
    parser.add_argument(
        "--print-images",
        action="store_true",
        help=(
            "print the fully-qualified image references, space-separated, and "
            "build nothing. CI feeds this to `docker save` so the list of "
            "images handed to a suite's shards comes from the same manifest "
            "that built them"
        ),
    )
    args = parser.parse_args()

    tag = os.environ.get("IMAGE_TAG", "ci")
    owner = os.environ.get("GHCR_OWNER", "kaikei-e")
    use_cache = os.environ.get("NO_CACHE", "") != "1"

    suites = load(SUITES_YAML)
    services = service_index(load(SERVICES_YAML))

    entry = suites.get("suites", {}).get(args.suite)
    if entry is None:
        known = ", ".join(sorted(suites.get("suites", {})))
        print(f"unknown suite '{args.suite}'. Known suites: {known}", file=sys.stderr)
        return 2

    images = entry.get("images", [])
    if not images:
        print(f"suite '{args.suite}' declares no images to build", file=sys.stderr)
        return 0

    if args.print_images:
        print(" ".join(f"ghcr.io/{owner}/alt-{image}:{tag}" for image in images))
        return 0

    for image in images:
        command = build_command(image, suites, services, tag, owner, use_cache)
        print(f"==> {' '.join(command)}", file=sys.stderr)
        if args.dry_run:
            continue
        result = subprocess.run(command, cwd=ROOT, check=False)
        if result.returncode != 0:
            print(f"build failed for image '{image}'", file=sys.stderr)
            return result.returncode

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except BuildError as error:
        print(f"error: {error}", file=sys.stderr)
        sys.exit(1)
