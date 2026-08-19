#!/usr/bin/env python3
"""Fail when production compose operational surface exceeds the P2-14 budget.

The baseline is counted from compose/compose.yaml's include chain using
the `services:` mapping only. A grep of indented keys counts the
`backup-docker-proxy` *network* as a 94th service; that is not a service.

Mechanical classification (names, not essential vs accidental):

- profiled: has a non-empty `profiles:` list (opt-in, not in default `up`)
- ephemeral: not profiled, restart is `no` or omitted (compose default),
  and the name looks like a one-shot (`migrat`, `bootstrap`, `-init`)
  OR the service is a `service_completed_successfully` target
- long_running: everything else in the include chain

P2-14 budget (scripts/ops-surface-inventory.yaml metadata only):

- F1: every long-running production service appears in the inventory
- F2: accidental OSU is non-increasing vs the frozen cap unless excepted
- F3: a new long-running unit needs offset_of or an unexpired exception
- F4: expired / sunset-less exception fails
- F5: sidecar-* without a live parent in compose fails

Usage:
  python3 scripts/compose-ops-surface-audit.py
  python3 scripts/compose-ops-surface-audit.py --write-baseline
Exit 0 when counts, names, and the budget hold; 1 on drift or budget break.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import date, datetime
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import REPO_ROOT, load_yaml, production_services  # noqa: E402


def restart_is_no(svc: dict) -> bool:
    restart = svc.get("restart")
    if restart is None:
        return True
    if isinstance(restart, str):
        return restart.strip().strip("\"'") == "no"
    return False


def _init_edge_keys(services: dict[str, dict]) -> list[str]:
    keys: list[str] = []
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
                keys.append(f"{name} -> {dep}")
    return keys

ONESHOT_NAME_RE = re.compile(r"(migrat|bootstrap$|-init$)", re.IGNORECASE)

FALSE_94 = (
    "compose/backup.yaml top-level networks.backup-docker-proxy is a "
    "network, not a service. A grep of indented `name:` keys counts it "
    "and reports 94. Unique keys under services: across the include "
    "chain are the declared total."
)
CLASSIFICATION = (
    "profiled = non-empty profiles:; ephemeral = not profiled, "
    "restart is no/omitted, and (name matches migrat/bootstrap/-init "
    "OR is a service_completed_successfully target); "
    "long_running = the rest."
)
RENDER_COUNT_KEYS = (
    "compose_config_default_profiles",
    "compose_config_full_profiles",
)


def _scs_targets(services: dict[str, dict]) -> set[str]:
    found: set[str] = set()
    for svc in services.values():
        deps = svc.get("depends_on") or {}
        if not isinstance(deps, dict):
            continue
        for dep, cfg in deps.items():
            if isinstance(cfg, dict) and cfg.get("condition") == "service_completed_successfully":
                found.add(dep)
    return found


def classify_services(services: dict[str, dict]) -> dict[str, list[str]]:
    targets = _scs_targets(services)
    profiled: list[str] = []
    ephemeral: list[str] = []
    long_running: list[str] = []
    for name, svc in sorted(services.items()):
        profiles = svc.get("profiles") or []
        if profiles:
            profiled.append(name)
            continue
        oneshot = bool(ONESHOT_NAME_RE.search(name)) or name in targets
        if restart_is_no(svc) and oneshot:
            ephemeral.append(name)
            continue
        long_running.append(name)
    return {
        "profiled": profiled,
        "ephemeral": ephemeral,
        "long_running": long_running,
    }


def inventory(services: dict[str, dict]) -> dict:
    classified = classify_services(services)
    declared = len(services)
    profiled_n = len(classified["profiled"])
    return {
        "schema_version": 1,
        "generated_from": "compose/compose.yaml include chain",
        "counts": {
            "declared_total": declared,
            "long_running": len(classified["long_running"]),
            "ephemeral": len(classified["ephemeral"]),
            "profiled": profiled_n,
            "compose_config_default_profiles": declared - profiled_n,
            "compose_config_full_profiles": declared,
        },
        "long_running": classified["long_running"],
        "ephemeral": classified["ephemeral"],
        "profiled": classified["profiled"],
    }


def drift(current: dict, baseline: dict) -> list[str]:
    violations: list[str] = []
    count_keys = (
        "declared_total",
        "long_running",
        "ephemeral",
        "profiled",
        *RENDER_COUNT_KEYS,
    )
    for key in count_keys:
        got = current["counts"].get(key)
        want = (baseline.get("counts") or {}).get(key)
        if got != want:
            violations.append(f"counts.{key}: {got} != baseline {want}")
    for group in ("long_running", "ephemeral", "profiled"):
        got = set(current[group])
        want = set(baseline[group])
        for name in sorted(got - want):
            violations.append(f"{group}: added {name}")
        for name in sorted(want - got):
            violations.append(f"{group}: removed {name}")
    disc = baseline.get("discrepancy") or {}
    for key in RENDER_COUNT_KEYS:
        computed = current["counts"].get(key)
        if computed is None:
            continue
        if key in disc and disc.get(key) != computed:
            violations.append(
                f"discrepancy.{key}: {disc.get(key)} != computed {computed}"
            )
    return violations


ALLOWED_CLASSES = frozenset({"essential", "accidental", "platform", "ephemeral"})
ALLOWED_KINDS = frozenset(
    {
        "app",
        "store",
        "sidecar-netns",
        "sidecar-cert",
        "sidecar-logs",
        "platform",
        "ephemeral",
    }
)
EXCEPTION_CODES = frozenset(
    {
        "X-SECURITY-BOUNDARY",
        "X-REPLACE-IN-FLIGHT",
        "X-TRIAL",
        "X-PLATFORM-SHARED",
    }
)
NETNS_PARENT_RE = re.compile(r"^service:(.+)$")
INTERP_DEFAULT_RE = re.compile(r"^\$\{[^:}]+:-([^}]+)\}$")
ADR_EXCEPTION_RE = re.compile(r"^(?:ADR-)?\d{6}$")


def load_units_inventory(path: Path) -> dict:
    """Load classification metadata keyed by compose service name.

    The file holds only class / kind / exception / sunset / offset_of.
    Long-running names themselves come from compose, not from this file.
    """
    if not path.is_file():
        raise SystemExit(f"missing ops-surface inventory: {path}")
    data = load_yaml(path)
    raw_units = data.get("units")
    if not isinstance(raw_units, dict) or not raw_units:
        raise SystemExit(f"units mapping missing from {path}")
    units: dict[str, dict] = {}
    for name, meta in raw_units.items():
        if isinstance(name, str) and isinstance(meta, dict):
            units[name] = meta
    try:
        accidental_cap = int(data["accidental_osu_baseline"])
        long_running_cap = int(data["long_running_osu_baseline"])
    except (KeyError, TypeError, ValueError) as exc:
        raise SystemExit(
            f"accidental_osu_baseline / long_running_osu_baseline "
            f"missing or invalid in {path}"
        ) from exc
    return {
        "units": units,
        "accidental_osu_baseline": accidental_cap,
        "long_running_osu_baseline": long_running_cap,
    }


def _env_map(svc: dict) -> dict[str, str]:
    env = svc.get("environment")
    out: dict[str, str] = {}
    if isinstance(env, dict):
        for key, value in env.items():
            if isinstance(key, str) and value is not None:
                out[key] = str(value)
        return out
    if isinstance(env, list):
        for item in env:
            if not isinstance(item, str) or "=" not in item:
                continue
            key, _, value = item.partition("=")
            out[key] = value
    return out


def _resolve_interp(raw: str) -> str:
    text = raw.strip().strip("\"'")
    match = INTERP_DEFAULT_RE.match(text)
    if match:
        return match.group(1).strip().strip("\"'")
    return text


def sidecar_parent(name: str, svc: dict, meta: dict) -> str | None:
    """Derive the live parent of a sidecar-* unit from compose, not inventory."""
    mode = svc.get("network_mode")
    if isinstance(mode, str):
        match = NETNS_PARENT_RE.match(mode.strip().strip("\"'"))
        if match:
            parent = match.group(1).strip()
            if parent:
                return parent
    env = _env_map(svc)
    kind = str(meta.get("kind") or "")
    if kind == "sidecar-logs" or name.endswith("-logs"):
        target = env.get("TARGET_SERVICE")
        if target:
            resolved = _resolve_interp(target)
            if resolved and not resolved.startswith("${"):
                return resolved
        if name.endswith("-logs") and name != "-logs":
            return name[: -len("-logs")]
    subject = env.get("CERT_SUBJECT")
    if subject:
        resolved = _resolve_interp(subject)
        if resolved and not resolved.startswith("${"):
            return resolved
    if name.startswith("pki-agent-") and name != "pki-agent-":
        return name[len("pki-agent-") :]
    return None


def _as_date(value: object) -> date | None:
    if isinstance(value, datetime):
        return value.date()
    if isinstance(value, date):
        return value
    if isinstance(value, str) and value.strip():
        try:
            return date.fromisoformat(value.strip())
        except ValueError:
            return None
    return None


def _exception_code_ok(code: str) -> bool:
    return code in EXCEPTION_CODES or bool(ADR_EXCEPTION_RE.fullmatch(code))


def exception_is_live(meta: dict, today: date) -> bool:
    """True when exception + sunset are set, well-formed, and not yet expired."""
    return _exception_fault(meta, today) is None and bool(meta.get("exception"))


def _exception_fault(meta: dict, today: date) -> str | None:
    code = meta.get("exception")
    if code in (None, "", False):
        return None
    if not isinstance(code, str) or not _exception_code_ok(code):
        return f"unknown exception {code!r}"
    sunset = meta.get("sunset")
    if sunset in (None, ""):
        return "exception without sunset"
    end = _as_date(sunset)
    if end is None:
        return f"sunset {sunset!r} is not an ISO date"
    if end <= today:
        return f"exception expired on {end.isoformat()}"
    return None


def _must_be_accidental(name: str, kind: object) -> bool:
    kind_s = str(kind or "")
    return kind_s.startswith("sidecar-") or name.endswith("-logs")


def budget_violations(
    services: dict[str, dict],
    units: dict[str, dict],
    baseline: dict,
    *,
    accidental_osu_baseline: int,
    long_running_osu_baseline: int,
    today: date,
) -> list[str]:
    """P2-14 fitness functions F1–F5. Empty list means the budget holds."""
    classified = classify_services(services)
    long_running = classified["long_running"]
    baseline_lr = set(baseline.get("long_running") or [])
    violations: list[str] = []

    for name in sorted(set(units) - set(services)):
        violations.append(f"F1: stale inventory row {name} (not in production compose)")

    for name in long_running:
        meta = units.get(name)
        if meta is None:
            violations.append(f"F1: {name} missing from inventory")
            continue
        class_ = meta.get("class")
        kind = meta.get("kind")
        if class_ not in ALLOWED_CLASSES:
            violations.append(f"F1: {name} has invalid class {class_!r}")
        elif class_ == "ephemeral":
            violations.append(f"F1: {name} is long-running but class is ephemeral")
        if kind not in ALLOWED_KINDS:
            violations.append(f"F1: {name} has invalid kind {kind!r}")
        if _must_be_accidental(name, kind) and class_ != "accidental":
            violations.append(
                f"F1: {name} kind {kind!r} / *-logs must be class accidental, "
                f"got {class_!r}"
            )

        fault = _exception_fault(meta, today)
        if fault:
            violations.append(f"F4: {name} {fault}")

        if isinstance(kind, str) and kind.startswith("sidecar-"):
            parent = sidecar_parent(name, services.get(name) or {}, meta)
            if not parent or parent not in services:
                label = parent or "unresolved"
                violations.append(
                    f"F5: {name} kind {kind} has no live parent {label} in compose"
                )

    for name in sorted(services):
        if name in long_running:
            continue
        meta = units.get(name) or {}
        class_ = meta.get("class")
        kind = meta.get("kind")
        if _must_be_accidental(name, kind) and class_ != "accidental":
            violations.append(
                f"F1: {name} kind {kind!r} / *-logs must be class accidental, "
                f"got {class_!r}"
            )

    uncapped_accidental = [
        name
        for name in long_running
        if (units.get(name) or {}).get("class") == "accidental"
        and not exception_is_live(units.get(name) or {}, today)
    ]
    if len(uncapped_accidental) > accidental_osu_baseline:
        extras = [name for name in uncapped_accidental if name not in baseline_lr]
        msg = (
            f"F2: accidental OSU {len(uncapped_accidental)} exceeds baseline "
            f"{accidental_osu_baseline}"
        )
        if extras:
            msg += f"; new non-excepted: {', '.join(extras)}"
        violations.append(msg)

    added = [name for name in long_running if name not in baseline_lr]
    removed = {name for name in baseline_lr if name not in long_running}
    claimed: set[str] = set()
    for name in added:
        meta = units.get(name) or {}
        offset = meta.get("offset_of")
        if isinstance(offset, str) and offset.strip():
            target = offset.strip()
            if target not in removed:
                violations.append(
                    f"F3: {name} offset_of {target} is not a removed long-running unit"
                )
            elif target in claimed:
                violations.append(
                    f"F3: {name} offset_of {target} already claimed by another new unit"
                )
            else:
                claimed.add(target)
            continue
        if exception_is_live(meta, today):
            continue
        violations.append(
            f"F3: {name} is a new long-running unit; set offset_of or an "
            "unexpired exception with sunset"
        )

    counted = [
        name
        for name in long_running
        if not exception_is_live(units.get(name) or {}, today)
    ]
    if len(counted) > long_running_osu_baseline:
        violations.append(
            f"F3: long-running OSU {len(counted)} exceeds cap "
            f"{long_running_osu_baseline}"
        )
    return violations


def load_baseline(path: Path) -> dict:
    if not path.is_file():
        raise SystemExit(f"missing ops-surface baseline: {path}")
    return json.loads(path.read_text(encoding="utf-8"))


def init_edges_allowlist_update(
    existing_allowlist: object,
    live_keys: list[str],
) -> tuple[list[str], list[str]]:
    """Preserve the PM-037 allowlist; refuse silent growth."""
    if isinstance(existing_allowlist, list):
        preserved = [
            row for row in existing_allowlist if isinstance(row, str) and row.strip()
        ]
    else:
        preserved = []
    growth = sorted(set(live_keys) - set(preserved))
    if not growth:
        return preserved, []
    refusals = [
        (
            f"init_edges_allowlist grew: {edge}. --write-baseline will not "
            "expand the PM-037 allowlist; add the pair with an explicit "
            "reason in scripts/ops-surface-baseline.json"
        )
        for edge in growth
    ]
    return preserved, refusals


def discrepancy_for(current: dict) -> dict:
    counts = current["counts"]
    return {
        "false_94": FALSE_94,
        "compose_config_default_profiles": counts["compose_config_default_profiles"],
        "compose_config_full_profiles": counts["compose_config_full_profiles"],
        "compose_config_note": (
            "docker compose -f compose/compose.yaml config without --profile "
            "omits the 4 profiled services (backup + perf)."
        ),
        "yaml_declared": counts["declared_total"],
        "profiled_excluded_from_default_up": current["profiled"],
    }


def prepare_baseline_write(
    current: dict,
    existing: dict,
    live_init_edges: list[str],
    budget_found: list[str],
) -> tuple[dict | None, list[str]]:
    """Build a baseline payload, or refuse growth / F1–F5 failures."""
    preserved, growth = init_edges_allowlist_update(
        existing.get("init_edges_allowlist"),
        live_init_edges,
    )
    found = list(budget_found) + growth
    if found:
        return None, found
    payload = {
        **current,
        "measured_at": date.today().isoformat(),
        "discrepancy": discrepancy_for(current),
        "classification": CLASSIFICATION,
        "init_edges_allowlist": preserved,
    }
    return payload, []


def write_baseline(path: Path, current: dict) -> list[str]:
    services = production_services()
    existing = load_baseline(path) if path.is_file() else {}
    loaded = load_units_inventory(
        Path(REPO_ROOT / "scripts" / "ops-surface-inventory.yaml")
    )
    budget_found = budget_violations(
        services,
        loaded["units"],
        existing if existing else current,
        accidental_osu_baseline=loaded["accidental_osu_baseline"],
        long_running_osu_baseline=loaded["long_running_osu_baseline"],
        today=date.today(),
    )
    payload, found = prepare_baseline_write(
        current,
        existing,
        _init_edge_keys(services),
        budget_found,
    )
    if found or payload is None:
        return found
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    return []


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--baseline",
        default=str(REPO_ROOT / "scripts" / "ops-surface-baseline.json"),
    )
    parser.add_argument(
        "--inventory",
        default=str(REPO_ROOT / "scripts" / "ops-surface-inventory.yaml"),
    )
    parser.add_argument("--write-baseline", action="store_true")
    args = parser.parse_args()
    services = production_services()
    current = inventory(services)
    path = Path(args.baseline)
    loaded = load_units_inventory(Path(args.inventory))
    if not path.is_file() and not args.write_baseline:
        raise SystemExit(f"missing ops-surface baseline: {path}")
    existing = load_baseline(path) if path.is_file() else {}
    budget_found = budget_violations(
        services,
        loaded["units"],
        existing if existing else {"long_running": []},
        accidental_osu_baseline=loaded["accidental_osu_baseline"],
        long_running_osu_baseline=loaded["long_running_osu_baseline"],
        today=date.today(),
    )
    if args.write_baseline:
        payload, found = prepare_baseline_write(
            current,
            existing,
            _init_edge_keys(services),
            budget_found,
        )
        if found or payload is None:
            print("compose-ops-surface-audit --write-baseline FAILED:")
            for v in found:
                print(f"  - {v}")
            print(
                "\nFix F1–F5 first. --write-baseline does not expand "
                "init_edges_allowlist or skip budget violations. Add a new "
                "SCS edge with an explicit reason in "
                "scripts/ops-surface-baseline.json."
            )
            return 1
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        print(f"wrote {path}")
        return 0
    found = drift(current, existing)
    found.extend(budget_found)
    if found:
        print("compose-ops-surface-audit FAILED — inventory drifted:")
        for v in found:
            print(f"  - {v}")
        print(
            "\nIf the change is intentional, regenerate with "
            "--write-baseline and explain the offset in the PR. "
            "A new long-running unit is operational-surface debt: it needs "
            "offset_of or an unexpired exception with sunset in "
            "scripts/ops-surface-inventory.yaml. See "
            "docs/runbooks/ops-surface-budget.md."
        )
        return 1
    counts = current["counts"]
    accidental = sum(
        1
        for name in current["long_running"]
        if loaded["units"].get(name, {}).get("class") == "accidental"
    )
    print(
        f"OK: declared={counts['declared_total']} "
        f"long_running={counts['long_running']} "
        f"ephemeral={counts['ephemeral']} "
        f"profiled={counts['profiled']} "
        f"accidental_osu={accidental}/{loaded['accidental_osu_baseline']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
