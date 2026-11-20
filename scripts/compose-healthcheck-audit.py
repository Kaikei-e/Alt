#!/usr/bin/env python3
"""Fail when a compose service_healthy dependency targets a service without a healthcheck.

Extracted from .github/workflows/compose-audit.yaml so CI and local static checks
run the exact same audit against a staged tree or explicit --env-file without
touching <repo>/.env.

Usage:
  python3 scripts/compose-healthcheck-audit.py --repo-root /path/to/stage --env-file /path/to/stage/.env
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent


def find_violations(cfg: dict) -> tuple[int, list[str]]:
    services = cfg.get("services") or {}
    violations: list[str] = []
    for name, svc in services.items():
        if not isinstance(svc, dict):
            continue
        deps = svc.get("depends_on") or {}
        if not isinstance(deps, dict):
            # Short-form `depends_on: [a, b]` does not specify `service_healthy`.
            continue
        for dep_name, dep_cfg in deps.items():
            if not isinstance(dep_cfg, dict):
                continue
            if dep_cfg.get("condition") != "service_healthy":
                continue
            target = services.get(dep_name)
            if target is None:
                violations.append(f"{name} -> {dep_name} (target service not defined)")
                continue
            if target.get("healthcheck") is None:
                violations.append(
                    f"{name} depends_on.{dep_name}.condition=service_healthy "
                    f"but {dep_name} has no healthcheck"
                )
    return len(services), violations


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=REPO_ROOT,
        help="repository/staging root containing compose/ (default: repo root)",
    )
    parser.add_argument(
        "--env-file",
        type=Path,
        default=None,
        help="env file passed to docker compose --env-file for interpolation",
    )
    parser.add_argument(
        "-f",
        "--file",
        type=Path,
        default=None,
        help="root compose file (default: <repo-root>/compose/compose.yaml)",
    )
    args = parser.parse_args(argv)

    compose_file = (
        args.file if args.file is not None else args.repo_root / "compose" / "compose.yaml"
    )
    cmd = ["docker", "compose"]
    if args.env_file is not None:
        cmd += ["--env-file", str(args.env_file)]
    cmd += ["-f", str(compose_file), "config"]

    proc = subprocess.run(  # noqa: S603
        cmd, capture_output=True, text=True, cwd=args.repo_root, check=False
    )
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr or proc.stdout or "compose config failed\n")
        return proc.returncode

    cfg = yaml.safe_load(proc.stdout) or {}
    total_services, violations = find_violations(cfg)
    if violations:
        sys.stdout.write("service_healthy edges with missing target healthcheck:\n")
        for v in violations:
            sys.stdout.write(f"  - {v}\n")
        sys.stdout.write(
            "Add a `healthcheck:` block to each target service, "
            "or drop the `condition: service_healthy` if the dependency is "
            "start-only. See compose/recap.yaml recap-subworker for a "
            "Python urllib.request reference pattern.\n"
        )
        return 1
    sys.stdout.write(f"OK: {total_services} services checked, 0 violations\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
