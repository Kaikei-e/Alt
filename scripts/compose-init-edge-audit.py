#!/usr/bin/env python3
"""Fail when a one-shot `service_completed_successfully` edge is unlisted.

PM-2026-037: `up -d --no-deps` never starts a `restart: "no"` dependency
gated with `service_completed_successfully`. Wave 1 moves those hooks to
`pre_start` (Compose ≥ 5.4.0). Wave 1b moved the two single-consumer
chown-only inits onto `pre_start`; remaining one-shots (migrators,
bootstrap, oauth-token-init) stay on the allowlist in
scripts/ops-surface-baseline.json so a new edge cannot land silently.

Usage: python3 scripts/compose-init-edge-audit.py
Exit 0 when the production edges match the allowlist, 1 otherwise.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import REPO_ROOT, production_services  # noqa: E402


def restart_is_no(svc: dict) -> bool:
    restart = svc.get("restart")
    if restart is None:
        return True
    if isinstance(restart, str):
        return restart.strip().strip("\"'") == "no"
    return False


def edge_key(consumer: str, target: str) -> str:
    return f"{consumer} -> {target}"


def init_edges(services: dict[str, dict]) -> list[tuple[str, str]]:
    found: list[tuple[str, str]] = []
    for name, svc in sorted(services.items()):
        deps = svc.get("depends_on") or {}
        if not isinstance(deps, dict):
            continue
        for dep, cfg in deps.items():
            if not isinstance(cfg, dict):
                continue
            if cfg.get("condition") != "service_completed_successfully":
                continue
            target = services.get(dep) or {}
            if restart_is_no(target):
                found.append((name, dep))
    return found


def audit_init_edges(
    services: dict[str, dict],
    allowlist: set[str],
) -> list[str]:
    present = {edge_key(consumer, target) for consumer, target in init_edges(services)}
    violations: list[str] = []
    for edge in sorted(present - allowlist):
        violations.append(f"unlisted one-shot SCS edge {edge}")
    for edge in sorted(allowlist - present):
        violations.append(f"stale allowlist row {edge} (no such production edge)")
    return violations


def load_allowlist(path: Path) -> set[str]:
    if not path.is_file():
        raise SystemExit(f"missing ops-surface baseline: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    rows = data.get("init_edges_allowlist")
    if not isinstance(rows, list):
        raise SystemExit(f"init_edges_allowlist missing from {path}")
    return {row for row in rows if isinstance(row, str) and row.strip()}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--baseline",
        default=str(REPO_ROOT / "scripts" / "ops-surface-baseline.json"),
    )
    args = parser.parse_args()
    allowlist = load_allowlist(Path(args.baseline))
    found = audit_init_edges(production_services(), allowlist)
    if found:
        print("compose-init-edge-audit FAILED — one-shot SCS edges drifted:")
        for v in found:
            print(f"  - {v}")
        print(
            "\nAdd the edge to scripts/ops-surface-baseline.json "
            "init_edges_allowlist only with a reason, or (Wave 1) replace "
            "the init container with a pre_start hook and drop the row."
        )
        return 1
    print(
        f"OK: {len(allowlist)} one-shot service_completed_successfully "
        "edge(s) match the allowlist"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
