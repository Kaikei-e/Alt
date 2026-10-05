#!/usr/bin/env python3
"""Fail when a service holds a URL for a compose service it shares no network with.

Compose DNS answers a service name only on the networks that service joined, so
a URL whose host sits on a network the caller is not attached to fails at
connect time — after deploy, and only on the code path that dials it. The raw
YAML cannot show this: the URL usually comes from `${VAR:-default}`, and the
value that wins is decided at render time. A stale `.env.template` entry
pointing callers on alt-network straight at a backend that lives only on an
internal network behind its auth proxy is exactly the shape this catches.

The audit renders the stack as deploy does (default profiles; --profile adds
opt-in stacks whose host files exist), then for each environment value
containing `scheme://host` checks that the host resolves on at least one
network the caller is attached to. Hosts that are not a compose service name,
container_name or network alias are ignored (external hosts, loopback).
`network_mode` is respected: `service:X` shares X's networks, while `host`,
`none`, `bridge` and `container:` resolve no service names — and a service
that lives in another service's netns has no DNS name of its own.

Two origins are told apart with a second, uninterpolated render:

* a variable the service declares in `environment:` is wiring someone wrote
  for that service, so an unreachable host there fails the audit;
* a variable that only arrives through `env_file: ../.env` is the shared
  template injected into every such service, read by few of them. Those are
  reported once per variable and host, with how many services cannot reach
  it, and do not change the exit status.

`env_file` entries resolve against the files on disk (the repo-root `.env`),
not against --env-file; CI stages `.env` from the same template.

Output names only services, variables and hosts — never a value, which may
carry credentials.

Usage:
  python3 scripts/compose-reachability-audit.py [--env-file .env.template] [-f compose/compose.yaml] [--profile P]...
Exit 0 when every declared URL is reachable, 1 otherwise, 2 when compose cannot render.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from typing import NamedTuple
from urllib.parse import urlsplit

REPO_ROOT = Path(__file__).resolve().parent.parent
URL_TOKEN = re.compile(r"[A-Za-z][A-Za-z0-9+.-]*://[^\s,;]+")


class Finding(NamedTuple):
    service: str
    variable: str
    host: str


class RenderError(RuntimeError):
    def __init__(self, message: str, returncode: int):
        super().__init__(message)
        self.returncode = returncode


def render(
    compose_file: Path,
    env_file: Path,
    profiles: list[str] | None = None,
    interpolate: bool = True,
) -> dict:
    cmd = ["docker", "compose", "--env-file", str(env_file), "-f", str(compose_file)]
    for profile in profiles or []:
        cmd += ["--profile", profile]
    cmd += ["config"]
    if not interpolate:
        # Leaves env_file unmerged, so `environment` holds only declared keys.
        cmd += ["--no-interpolate"]
    cmd += ["--format", "json"]
    proc = subprocess.run(cmd, capture_output=True, text=True, cwd=REPO_ROOT)
    if proc.returncode != 0:
        raise RenderError(proc.stderr or "docker compose config failed", proc.returncode)
    return json.loads(proc.stdout)


def _environment(service: dict) -> dict[str, str | None]:
    values = service.get("environment") or {}
    if isinstance(values, list):
        return dict(item.split("=", 1) if "=" in item else (item, None) for item in values)
    return dict(values)


def declared_keys(raw_cfg: dict) -> dict[str, set[str]]:
    """Per service, the variables written in `environment:` rather than env_file."""
    return {name: set(_environment(svc)) for name, svc in (raw_cfg.get("services") or {}).items()}


def _network_name(cfg: dict, key: str) -> str:
    declared = (cfg.get("networks") or {}).get(key) or {}
    return declared.get("name") or key


def _attached(service: dict) -> dict[str, dict]:
    networks = service.get("networks")
    if networks is None:
        return {"default": {}}
    if isinstance(networks, list):
        return {key: {} for key in networks}
    return {key: (value or {}) for key, value in networks.items()}


def caller_networks(cfg: dict, name: str, seen: frozenset[str] = frozenset()) -> frozenset[str]:
    service = cfg["services"][name]
    mode = service.get("network_mode")
    if mode:
        parent = mode[len("service:"):] if mode.startswith("service:") else None
        if parent in cfg["services"] and parent not in seen:
            return caller_networks(cfg, parent, seen | {name})
        return frozenset()
    return frozenset(_network_name(cfg, key) for key in _attached(service))


def dns_index(cfg: dict) -> dict[str, set[str]]:
    """Every name compose DNS answers, mapped to the networks it answers on."""
    index: dict[str, set[str]] = {}
    for name, service in cfg["services"].items():
        names = {name}
        if service.get("container_name"):
            names.add(service["container_name"])
        if service.get("network_mode"):
            # No endpoint of its own: neither name resolves anywhere.
            for host in names:
                index.setdefault(host.lower(), set())
            continue
        for key, options in _attached(service).items():
            network = _network_name(cfg, key)
            for host in names | set(options.get("aliases") or []):
                index.setdefault(host.lower(), set()).add(network)
    return index


def _extra_hosts(service: dict) -> set[str]:
    entries = service.get("extra_hosts") or []
    if isinstance(entries, dict):
        return {host.lower() for host in entries}
    return {re.split(r"[=:]", entry, maxsplit=1)[0].lower() for entry in entries}


def url_hosts(value: str) -> list[str]:
    hosts = []
    for token in URL_TOKEN.findall(value):
        try:
            host = urlsplit(token).hostname
        except ValueError:
            continue
        if host:
            hosts.append(host.lower())
    return hosts


def findings(cfg: dict) -> list[Finding]:
    index = dns_index(cfg)
    found: set[Finding] = set()
    for name, service in cfg.get("services", {}).items():
        networks = caller_networks(cfg, name)
        static = _extra_hosts(service)
        for variable, value in _environment(service).items():
            if not isinstance(value, str):
                continue
            for host in url_hosts(value):
                if host not in index or host in static:
                    continue
                if not networks & index[host]:
                    found.add(Finding(name, variable, host))
    return sorted(found)


def split_by_origin(
    found: list[Finding], declared: dict[str, set[str]]
) -> tuple[list[Finding], list[Finding]]:
    own = [f for f in found if f.variable in declared.get(f.service, set())]
    shared = [f for f in found if f.variable not in declared.get(f.service, set())]
    return own, shared


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--env-file", type=Path, default=REPO_ROOT / ".env.template")
    parser.add_argument("-f", "--file", type=Path, default=REPO_ROOT / "compose" / "compose.yaml")
    parser.add_argument(
        "--profile", action="append", default=[],
        help="also activate this compose profile (repeatable); the default is what deploy renders",
    )
    args = parser.parse_args(argv)

    try:
        cfg = render(args.file, args.env_file, args.profile)
        raw = render(args.file, args.env_file, args.profile, interpolate=False)
    except RenderError as error:
        print(f"compose-reachability-audit: docker compose config exited {error.returncode}", file=sys.stderr)
        print(str(error).rstrip(), file=sys.stderr)
        return 2

    own, shared = split_by_origin(findings(cfg), declared_keys(raw))

    if shared:
        receivers: dict[tuple[str, str], int] = defaultdict(int)
        for finding in shared:
            receivers[(finding.variable, finding.host)] += 1
        print("compose-reachability-audit: shared env_file values some receivers cannot reach (not failing):")
        print("variable\thost\tservices")
        for (variable, host), count in sorted(receivers.items()):
            print(f"{variable}\t{host}\t{count}")

    if not own:
        print(f"compose-reachability-audit: {len(cfg.get('services', {}))} services, every declared compose-host URL shares a network")
        return 0

    print(f"compose-reachability-audit: {len(own)} declared URL(s) name a service the caller shares no network with:")
    print("service\tvariable\thost")
    for finding in own:
        print(f"{finding.service}\t{finding.variable}\t{finding.host}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
