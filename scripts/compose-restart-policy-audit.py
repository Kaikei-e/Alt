#!/usr/bin/env python3
"""Fail when a long-running production compose service lacks a restart policy.

PM-2026-023 / ADR-000703: search-indexer had no `restart:` policy and stayed
absent for ~36 hours after stopping. Every long-running service in the
production compose chain — including profiled services — must declare a literal
`restart:` value other than "no" (e.g. `unless-stopped`, `always`, `on-failure`).

A service is exempt only if:
1. The shared classification from `scripts/compose-ops-surface-audit.py` marks
   it ephemeral (name matches `migrat` / `bootstrap$` / `-init$` or it is a
   `service_completed_successfully` target, with `restart` omitted or `"no"`), or
2. It is listed in `ONE_SHOT_ALLOWLIST` below with a one-line reason based on
   its compose command running to completion.

Caveat: because the shared ephemeral rule is name-based, a long-running service
whose name ends in `-init` or contains `migrat` escapes this audit if it omits
`restart:`; `scripts/ops-surface-baseline.json` (enforced by
`compose-ops-surface-audit.py`) is what surfaces any new ephemeral name.

Usage:
    python3 scripts/compose-restart-policy-audit.py
Exit 0 when compliant, 1 on violations.
"""

from __future__ import annotations

import argparse
import importlib.util
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import (  # noqa: E402
    REPO_ROOT,
    ROOT_COMPOSE,
    iter_production_services,
)


def _load_ops_surface_classify():
    path = _SCRIPTS / "compose-ops-surface-audit.py"
    spec = importlib.util.spec_from_file_location("compose_ops_surface_audit", path)
    if spec is None or spec.loader is None:
        raise SystemExit(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.classify_services


classify_services = _load_ops_surface_classify()

# Profiled one-shot runners whose command runs to completion rather than
# running as a daemon. Map compose service name -> one-line reason from compose.
ONE_SHOT_ALLOWLIST: dict[str, str] = {
    "alt-perf": (
        "compose/perf.yaml: E2E performance measurement tool run manually to "
        "completion via `run --rm alt-perf scan`"
    ),
    "k6": (
        "compose/perf.yaml: grafana/k6 load-test runner executing "
        "`command: [run, /scripts/scenarios/smoke.js]` to completion"
    ),
}


def _display_path(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT.resolve()).as_posix()
    except ValueError:
        return path.as_posix()


def _unmodeled_key_violations(
    name: str, svc: dict, file_label: str
) -> list[str]:
    violations: list[str] = []
    if "extends" in svc and svc.get("extends") is not None:
        violations.append(
            f"{name} ({file_label}): uses 'extends:' which this audit does not model yet"
        )
    deploy = svc.get("deploy")
    if (
        isinstance(deploy, dict)
        and "restart_policy" in deploy
        and deploy.get("restart_policy") is not None
    ):
        violations.append(
            f"{name} ({file_label}): uses 'deploy.restart_policy' which this audit does not model yet"
        )
    return violations


def _restart_violation_reason(svc: dict) -> str | None:
    if "restart" not in svc or svc.get("restart") is None:
        return "missing restart policy (defaults to 'no')"
    restart = svc.get("restart")
    if restart is False:
        return "restart is 'no'"
    if not isinstance(restart, str):
        return f"invalid restart policy {restart!r}"
    if "${" in restart:
        return (
            f"unresolvable restart policy {restart!r}; declare a literal value "
            "(e.g. 'unless-stopped')"
        )
    val = restart.strip().strip("\"'").lower()
    if val == "":
        return "empty restart policy"
    if val in {"no", "false"}:
        return "restart is 'no'"
    return None


def _classify_ignoring_profiles(
    services: dict[str, dict],
) -> dict[str, list[str]]:
    """Reuse shared ephemeral/long_running classification without exempting profiles."""
    unprofiled = {
        name: {k: v for k, v in svc.items() if k != "profiles"}
        for name, svc in services.items()
    }
    return classify_services(unprofiled)


def audit_compose(
    root: Path | None = None,
    *,
    allowlist: dict[str, str] | None = None,
) -> list[str]:
    effective_allowlist = ONE_SHOT_ALLOWLIST if allowlist is None else allowlist
    services: dict[str, dict] = {}
    service_files: dict[str, Path] = {}
    for path, name, svc in iter_production_services(root):
        services[name] = svc
        service_files[name] = path

    violations: list[str] = []
    for name in sorted(services):
        file_label = _display_path(service_files[name])
        violations.extend(_unmodeled_key_violations(name, services[name], file_label))

    classified = _classify_ignoring_profiles(services)
    for name in classified["long_running"]:
        reason_allow = effective_allowlist.get(name)
        if isinstance(reason_allow, str) and reason_allow.strip():
            continue
        reason = _restart_violation_reason(services[name])
        if reason is not None:
            file_label = _display_path(service_files[name])
            violations.append(f"{name} ({file_label}): {reason}")
    return violations


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--compose",
        default=str(ROOT_COMPOSE),
        help="Root compose file (default: compose/compose.yaml)",
    )
    args = parser.parse_args()
    root = Path(args.compose)
    violations = audit_compose(root)
    if violations:
        print("compose-restart-policy-audit FAILED:")
        for v in violations:
            print(f"  - {v}")
        print(
            "\nDeclare a literal `restart: unless-stopped` (or `always` / `on-failure`) "
            "on each long-running service, or add genuine one-shot runners to "
            "ONE_SHOT_ALLOWLIST in scripts/compose-restart-policy-audit.py."
        )
        return 1
    services = {name: svc for _p, name, svc in iter_production_services(root)}
    classified = _classify_ignoring_profiles(services)
    allowlisted = [
        name
        for name in classified["long_running"]
        if isinstance(ONE_SHOT_ALLOWLIST.get(name), str)
        and ONE_SHOT_ALLOWLIST[name].strip()
    ]
    audited = len(classified["long_running"]) - len(allowlisted)
    print(
        f"OK: {audited} long-running service(s) declare a literal non-'no' restart policy "
        f"({len(classified['ephemeral'])} ephemeral, "
        f"{len(allowlisted)} allowlisted one-shot exempt)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
