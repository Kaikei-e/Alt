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

Usage: python3 scripts/compose-port-audit.py [-f compose/compose.yaml]
Exit 0 when clean, 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

# (service, container-side port) -> why it may answer off-host.
EDGE_ALLOWLIST = {
    ("plecto-proxy", 8443): "the edge proxy; every browser entry point terminates here",
    ("grafana", 3000): "operator dashboard behind its own login",
    ("pact-broker", 9292): "operator-supplied private-network binding, see compose/pact.yaml",
}


def resolved_config(compose_file: Path) -> dict:
    # --env-file is not optional: the compose file lives under compose/, so
    # that is the project directory compose would otherwise search for .env,
    # and the repo keeps it at the root instead. Without this, interpolation
    # fails on DOCKER_GROUP_ID, which has no default by design.
    cmd = ["docker", "compose"]
    env_file = REPO_ROOT / ".env"
    if env_file.exists():
        cmd += ["--env-file", str(env_file)]
    cmd += ["-f", str(compose_file), "config", "--format", "json"]

    # logging.yaml fail-fasts on an unset DOCKER_GROUP_ID by design, and the
    # canonical .env does not carry it (start.sh injects the host GID at run
    # time). The value cannot affect a port audit, so supply a placeholder
    # rather than making every caller export one first.
    env = {**os.environ}
    env.setdefault("DOCKER_GROUP_ID", "0")

    proc = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        cwd=REPO_ROOT,
        env=env,
    )
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr or proc.stdout or "compose config failed\n")
        raise SystemExit(proc.returncode)
    return json.loads(proc.stdout)


def is_loopback(host_ip: str) -> bool:
    try:
        return ipaddress.ip_address(host_ip).is_loopback
    except ValueError:
        return False


def violations(cfg: dict) -> list[str]:
    found = []
    for name, svc in sorted((cfg.get("services") or {}).items()):
        for port in svc.get("ports") or []:
            host_ip = port.get("host_ip") or "0.0.0.0"
            target = port.get("target")
            if is_loopback(host_ip):
                continue
            if (name, target) in EDGE_ALLOWLIST:
                continue
            found.append(
                f"{name} publishes {host_ip}:{port.get('published')} -> {target} "
                f"on every interface"
            )
    return found


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-f", "--file", default="compose/compose.yaml")
    args = parser.parse_args()

    cfg = resolved_config(REPO_ROOT / args.file)
    found = violations(cfg)
    if found:
        print("Published ports that are not loopback-bound:")
        for v in found:
            print(f"  - {v}")
        print(
            "\nPrefix the mapping with 127.0.0.1: (e.g. \"127.0.0.1:9000:9000\"), "
            "or add the service to EDGE_ALLOWLIST in this script with the reason "
            "it must answer off-host."
        )
        return 1

    total = sum(len(s.get("ports") or []) for s in (cfg.get("services") or {}).values())
    print(f"OK: {total} published ports checked, 0 violations")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
