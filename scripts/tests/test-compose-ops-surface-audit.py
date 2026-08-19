#!/usr/bin/env python3
"""Tests for scripts/compose-ops-surface-audit.py.

The 93 vs 94 discrepancy was a network (`backup-docker-proxy`) counted as
a service by a naive indented-key grep. Mechanical classification is
profiles vs one-shot names/SCS targets vs everything else. P2-14 budget
fitness functions F1–F5 sit on top of that: inventory metadata, accidental
OSU cap, offset/exception, sunset, sidecar parent.

Run:
    python3 scripts/tests/test-compose-ops-surface-audit.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

from compose_include import OverrideLoader  # noqa: E402
import yaml  # noqa: E402

spec = importlib.util.spec_from_file_location(
    "ops_surface_audit", SCRIPTS / "compose-ops-surface-audit.py"
)
assert spec is not None and spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

PASS = 0
FAIL = 0


def check(name, condition):
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}")
        FAIL += 1


print("networks are not services")
FRAGMENT = """
networks:
  backup-docker-proxy:
    internal: true
services:
  restic-backup:
    image: alpine
    profiles: [backup]
  migrate:
    restart: "no"
  alt-backend:
    restart: always
    depends_on:
      migrate:
        condition: service_completed_successfully
"""
data = yaml.load(FRAGMENT, Loader=OverrideLoader)
services = data["services"]
check(
    "backup-docker-proxy is a network, not a service key",
    "backup-docker-proxy" not in services and set(services) == {"restic-backup", "migrate", "alt-backend"},
)

print("classify_services")
classified = audit.classify_services(services)
check(
    "profiled backup is not long-running",
    classified["profiled"] == ["restic-backup"]
    and "restic-backup" not in classified["long_running"],
)
check(
    "restart-no migrator is ephemeral",
    classified["ephemeral"] == ["migrate"],
)
check(
    "always-restart app is long-running",
    classified["long_running"] == ["alt-backend"],
)

PACT = {
    "pact-broker": {},  # omitted restart, not a one-shot name, not an SCS target
    "pact-db": {},
}
pact = audit.classify_services(PACT)
check(
    "pact-broker/pact-db with omitted restart are long-running (not one-shots)",
    pact["long_running"] == ["pact-broker", "pact-db"] and pact["ephemeral"] == [],
)

OMITTED_MIGRATOR = {"rag-db-migrator": {}}
check(
    "omitted-restart *migrator name is ephemeral even without an SCS edge",
    audit.classify_services(OMITTED_MIGRATOR)["ephemeral"] == ["rag-db-migrator"],
)

print("drift")
current = audit.inventory(services)
baseline = {
    "counts": {
        "declared_total": 3,
        "long_running": 1,
        "ephemeral": 1,
        "profiled": 1,
        "compose_config_default_profiles": 2,
        "compose_config_full_profiles": 3,
    },
    "long_running": ["alt-backend"],
    "ephemeral": ["migrate"],
    "profiled": ["restic-backup"],
}
check("matching baseline is clean", audit.drift(current, baseline) == [])

grown = dict(services)
grown["new-sidecar"] = {"restart": "unless-stopped"}
check(
    "a new long-running service is drift",
    any("new-sidecar" in v for v in audit.drift(audit.inventory(grown), baseline)),
)

print("P2-14 budget fitness functions (F1–F5)")
TODAY = __import__("datetime").date(2026, 8, 18)


def _meta(class_, kind, **extra):
    row = {"class": class_, "kind": kind}
    row.update(extra)
    return row


# Fixture: one essential app, one accidental sidecar-cert, one platform unit.
BUDGET_SERVICES = {
    "alt-backend": {"restart": "always"},
    "pki-agent-alt-backend": {
        "restart": "unless-stopped",
        "environment": ["CERT_SUBJECT=alt-backend"],
    },
    "prometheus": {"restart": "unless-stopped"},
    "migrate": {"restart": "no"},
}
BUDGET_UNITS = {
    "alt-backend": _meta("essential", "app"),
    "pki-agent-alt-backend": _meta("accidental", "sidecar-cert"),
    "prometheus": _meta("platform", "platform"),
    "migrate": _meta("ephemeral", "ephemeral"),
}
BUDGET_BASELINE = {
    "counts": {
        "declared_total": 4,
        "long_running": 3,
        "ephemeral": 1,
        "profiled": 0,
    },
    "long_running": ["alt-backend", "pki-agent-alt-backend", "prometheus"],
    "ephemeral": ["migrate"],
    "profiled": [],
}


def _budget(services=None, units=None, baseline=None, accidental=1, long_running=3):
    return audit.budget_violations(
        services if services is not None else BUDGET_SERVICES,
        units if units is not None else BUDGET_UNITS,
        baseline if baseline is not None else BUDGET_BASELINE,
        accidental_osu_baseline=accidental,
        long_running_osu_baseline=long_running,
        today=TODAY,
    )


def _has(prefix, name, found):
    return any(v.startswith(prefix) and name in v for v in found)


clean = _budget()
check("seeded fixture holds the budget", clean == [])

print("F1 inventory closed")
missing_units = dict(BUDGET_UNITS)
del missing_units["pki-agent-alt-backend"]
f1 = _budget(units=missing_units)
check(
    "F1: long-running service absent from inventory fails",
    _has("F1:", "pki-agent-alt-backend", f1),
)
stale_units = dict(BUDGET_UNITS)
stale_units["ghost"] = _meta("essential", "app")
check(
    "F1: stale inventory row not in compose fails",
    _has("F1:", "ghost", _budget(units=stale_units)),
)

print("F2 accidental OSU non-increasing")
example_svc = dict(BUDGET_SERVICES)
example_svc["example"] = {"restart": "always"}
example_svc["pki-agent-example"] = {
    "restart": "unless-stopped",
    "environment": ["CERT_SUBJECT=example"],
}
example_units = dict(BUDGET_UNITS)
example_units["example"] = _meta("essential", "app")
example_units["pki-agent-example"] = _meta("accidental", "sidecar-cert")
example_baseline = {
    "counts": {
        "declared_total": 5,
        "long_running": 4,
        "ephemeral": 1,
        "profiled": 0,
    },
    "long_running": [
        "alt-backend",
        "pki-agent-alt-backend",
        "prometheus",
        "example",
    ],
    "ephemeral": ["migrate"],
    "profiled": [],
}
f2 = _budget(
    services=example_svc,
    units=example_units,
    baseline=example_baseline,
    accidental=1,
    long_running=5,
)
check(
    "F2: new accidental pki-agent-example without exception fails",
    _has("F2:", "pki-agent-example", f2) or any(v.startswith("F2:") for v in f2),
)
example_exc = dict(example_units)
example_exc["pki-agent-example"] = _meta(
    "accidental",
    "sidecar-cert",
    exception="X-SECURITY-BOUNDARY",
    sunset="2026-11-18",
)
f2_exc = _budget(
    services=example_svc,
    units=example_exc,
    baseline=example_baseline,
    accidental=1,
    long_running=5,
)
check(
    "F2: unexpired exception allows temporary accidental overage",
    not any(v.startswith("F2:") for v in f2_exc),
)

print("F3 new long-running unit needs offset or exception")
check(
    "F3: pki-agent-example without offset_of or exception fails",
    _has("F3:", "pki-agent-example", f2),
)
offset_services = dict(BUDGET_SERVICES)
del offset_services["prometheus"]
offset_services["pki-agent-example"] = {
    "restart": "unless-stopped",
    "environment": ["CERT_SUBJECT=alt-backend"],
}
offset_units = {
    "alt-backend": _meta("essential", "app"),
    "pki-agent-alt-backend": _meta("accidental", "sidecar-cert"),
    "pki-agent-example": _meta(
        "accidental", "sidecar-cert", offset_of="prometheus"
    ),
    "migrate": _meta("ephemeral", "ephemeral"),
}
f3_offset = _budget(
    services=offset_services,
    units=offset_units,
    accidental=2,
    long_running=3,
)
check(
    "F3: offset_of a removed long-running unit is accepted",
    not any(v.startswith("F3:") for v in f3_offset),
)
still_there = dict(example_units)
still_there["pki-agent-example"] = _meta(
    "accidental", "sidecar-cert", offset_of="prometheus"
)
f3_live_offset = _budget(
    services=example_svc,
    units=still_there,
    baseline=example_baseline,
    accidental=2,
    long_running=5,
)
check(
    "F3: offset_of a unit that is still long-running fails",
    _has("F3:", "prometheus", f3_live_offset),
)
check(
    "F3: unexpired exception satisfies the new-unit rule",
    not any(v.startswith("F3:") for v in f2_exc),
)

print("F4 expired exception")
expired = dict(BUDGET_UNITS)
expired["pki-agent-alt-backend"] = _meta(
    "accidental",
    "sidecar-cert",
    exception="X-REPLACE-IN-FLIGHT",
    sunset="2026-08-01",
)
check(
    "F4: sunset on or before today fails",
    _has("F4:", "pki-agent-alt-backend", _budget(units=expired)),
)
no_sunset = dict(BUDGET_UNITS)
no_sunset["pki-agent-alt-backend"] = _meta(
    "accidental",
    "sidecar-cert",
    exception="X-TRIAL",
)
check(
    "F4: exception without sunset fails",
    _has("F4:", "pki-agent-alt-backend", _budget(units=no_sunset)),
)
live_exc = dict(BUDGET_UNITS)
live_exc["pki-agent-alt-backend"] = _meta(
    "accidental",
    "sidecar-cert",
    exception="X-PLATFORM-SHARED",
    sunset="2026-11-18",
)
check(
    "F4: future sunset is live",
    not any(v.startswith("F4:") for v in _budget(units=live_exc)),
)

print("F5 sidecar parent")
orphan_logs = dict(BUDGET_SERVICES)
orphan_logs["ghost-logs"] = {
    "restart": "unless-stopped",
    "environment": {"TARGET_SERVICE": "does-not-exist"},
}
orphan_units = dict(BUDGET_UNITS)
orphan_units["ghost-logs"] = _meta("accidental", "sidecar-logs")
check(
    "F5: sidecar-logs without a live parent fails",
    _has("F5:", "ghost-logs", _budget(services=orphan_logs, units=orphan_units, long_running=5)),
)
netns_orphan = dict(BUDGET_SERVICES)
netns_orphan["pki-agent-missing"] = {
    "restart": "unless-stopped",
    "network_mode": "service:gone",
}
netns_units = dict(BUDGET_UNITS)
netns_units["pki-agent-missing"] = _meta("accidental", "sidecar-netns")
check(
    "F5: sidecar-netns whose network_mode parent is gone fails",
    _has(
        "F5:",
        "pki-agent-missing",
        _budget(services=netns_orphan, units=netns_units, long_running=5),
    ),
)
check(
    "F5: sidecar-cert with CERT_SUBJECT parent present is clean",
    not any(v.startswith("F5:") for v in clean),
)

print("sidecar_parent")
check(
    "CERT_SUBJECT list env yields the cert parent",
    audit.sidecar_parent(
        "pki-agent-alt-backend",
        BUDGET_SERVICES["pki-agent-alt-backend"],
        BUDGET_UNITS["pki-agent-alt-backend"],
    )
    == "alt-backend",
)
check(
    "network_mode service:X wins for sidecar-netns",
    audit.sidecar_parent(
        "pki-agent-tag-generator",
        {"network_mode": "service:tag-generator"},
        _meta("accidental", "sidecar-netns"),
    )
    == "tag-generator",
)
check(
    "TARGET_SERVICE interpolation default is the logs parent",
    audit.sidecar_parent(
        "nginx-logs",
        {"environment": {"TARGET_SERVICE": "${NGINX_TARGET:-plecto-proxy}"}},
        _meta("accidental", "sidecar-logs"),
    )
    == "plecto-proxy",
)

print("pki-agent-example PR shape")
# Compose-only add, no inventory row, no baseline row: F1 + F3 (and F2
# once classified). This is the merge-blocker the budget exists to be.
pr_services = dict(BUDGET_SERVICES)
pr_services["pki-agent-example"] = {
    "restart": "unless-stopped",
    "environment": ["CERT_SUBJECT=example"],
}
pr_found = _budget(services=pr_services, long_running=4)
check(
    "PR adding pki-agent-example without inventory fails F1",
    _has("F1:", "pki-agent-example", pr_found),
)
check(
    "PR adding pki-agent-example without offset/exception fails F3",
    _has("F3:", "pki-agent-example", pr_found),
)

print("production inventory pin")
PROD_INV = audit.load_units_inventory(SCRIPTS / "ops-surface-inventory.yaml")
from compose_include import production_services  # noqa: E402

prod_services = production_services()
prod_baseline = audit.load_baseline(SCRIPTS / "ops-surface-baseline.json")
prod_found = audit.budget_violations(
    prod_services,
    PROD_INV["units"],
    prod_baseline,
    accidental_osu_baseline=PROD_INV["accidental_osu_baseline"],
    long_running_osu_baseline=PROD_INV["long_running_osu_baseline"],
    today=TODAY,
)
check("production compose holds F1–F5", prod_found == [])
prod_classified = audit.classify_services(prod_services)
osu_class = {}
for name in prod_classified["long_running"]:
    cls = PROD_INV["units"][name]["class"]
    osu_class[cls] = osu_class.get(cls, 0) + 1
check(
    "production accidental OSU is 16 (0 pki + 16 logs)",
    osu_class.get("accidental") == 16 and PROD_INV["accidental_osu_baseline"] == 16,
)
check(
    "production long-running OSU cap is 63",
    len(prod_classified["long_running"]) == 63
    and PROD_INV["long_running_osu_baseline"] == 63,
)
check(
    "final PKI cutover reduced declared total to 77",
    len(prod_services) == 77 and prod_baseline["counts"]["declared_total"] == 77,
)
check(
    "Wave 1b reduced ephemeral oneshots to 10",
    len(prod_classified["ephemeral"]) == 10
    and prod_baseline["counts"]["ephemeral"] == 10,
)
prod_inv = audit.inventory(prod_services)
check(
    "production default render count is declared_total - profiled (73)",
    prod_inv["counts"].get("compose_config_default_profiles") == 73
    and prod_inv["counts"]["declared_total"]
    - len(prod_inv["profiled"])
    == 73,
)
check(
    "production full render count is declared_total (77)",
    prod_inv["counts"].get("compose_config_full_profiles") == 77
    and prod_inv["counts"]["declared_total"] == 77,
)
check(
    "baseline discrepancy default/full match the computed render counts (not a stale 87)",
    prod_baseline.get("discrepancy", {}).get("compose_config_default_profiles")
    == prod_inv["counts"].get("compose_config_default_profiles")
    and prod_baseline.get("discrepancy", {}).get("compose_config_full_profiles")
    == prod_inv["counts"].get("compose_config_full_profiles")
    and prod_baseline.get("discrepancy", {}).get("compose_config_default_profiles")
    != 87,
)
stale_render = dict(prod_baseline)
stale_render["discrepancy"] = dict(prod_baseline.get("discrepancy") or {})
stale_render["discrepancy"]["compose_config_default_profiles"] = 87
stale_render["counts"] = dict(prod_inv["counts"])
stale_render["counts"]["compose_config_default_profiles"] = 87
check(
    "drift fails a stale hardcoded default render count of 87",
    any(
        "87" in v and "compose_config_default_profiles" in v
        for v in audit.drift(prod_inv, stale_render)
    ),
)

print("*-logs / sidecar-* reclassification cannot drop accidental OSU")
reclass_svc = dict(BUDGET_SERVICES)
reclass_svc["alt-backend-logs"] = {
    "restart": "unless-stopped",
    "environment": {"TARGET_SERVICE": "alt-backend"},
}
reclass_units = dict(BUDGET_UNITS)
reclass_units["alt-backend-logs"] = _meta("essential", "app")
reclass_baseline = {
    "counts": {
        "declared_total": 5,
        "long_running": 4,
        "ephemeral": 1,
        "profiled": 0,
    },
    "long_running": [
        "alt-backend",
        "pki-agent-alt-backend",
        "prometheus",
        "alt-backend-logs",
    ],
    "ephemeral": ["migrate"],
    "profiled": [],
}
reclass_found = _budget(
    services=reclass_svc,
    units=reclass_units,
    baseline=reclass_baseline,
    accidental=2,
    long_running=4,
)
check(
    "F1: *-logs reclassified as essential/app is refused",
    any(
        v.startswith("F1:") and "alt-backend-logs" in v and "accidental" in v
        for v in reclass_found
    ),
)
sidecar_ess = dict(BUDGET_UNITS)
sidecar_ess["pki-agent-alt-backend"] = _meta("essential", "sidecar-cert")
check(
    "F1: sidecar-* kind with class essential is refused",
    any(
        v.startswith("F1:")
        and "pki-agent-alt-backend" in v
        and "accidental" in v
        for v in _budget(units=sidecar_ess)
    ),
)

print("--write-baseline must not expand the init-edge allowlist or skip F1–F5")
existing_allow = ["alt-backend -> migrate"]
grown_live = ["alt-backend -> migrate", "new-app -> new-init"]
preserved, growth = audit.init_edges_allowlist_update(existing_allow, grown_live)
check(
    "allowlist update refuses growth and preserves the existing rows",
    preserved == existing_allow
    and any("new-app -> new-init" in v for v in growth)
    and any("reason" in v for v in growth),
)
kept, no_growth = audit.init_edges_allowlist_update(existing_allow, existing_allow)
check(
    "allowlist update preserves an unchanged set",
    kept == existing_allow and no_growth == [],
)
payload, refused = audit.prepare_baseline_write(
    current=prod_inv,
    existing={"init_edges_allowlist": existing_allow},
    live_init_edges=grown_live,
    budget_found=[],
)
check(
    "--write-baseline refuses when the init-edge set grew",
    payload is None and any("new-app -> new-init" in v for v in refused),
)
payload_budget, refused_budget = audit.prepare_baseline_write(
    current=prod_inv,
    existing={"init_edges_allowlist": list(prod_baseline.get("init_edges_allowlist") or [])},
    live_init_edges=list(prod_baseline.get("init_edges_allowlist") or []),
    budget_found=["F2: accidental OSU 17 exceeds baseline 16"],
)
check(
    "--write-baseline refuses when F1–F5 still fail",
    payload_budget is None and any(v.startswith("F2:") for v in refused_budget),
)
ok_payload, ok_found = audit.prepare_baseline_write(
    current=prod_inv,
    existing=prod_baseline,
    live_init_edges=list(prod_baseline.get("init_edges_allowlist") or []),
    budget_found=[],
)
check(
    "--write-baseline preserves the 22-edge allowlist and writes computed 73/77",
    ok_found == []
    and ok_payload is not None
    and ok_payload["init_edges_allowlist"]
    == prod_baseline["init_edges_allowlist"]
    and (ok_payload.get("discrepancy") or {}).get("compose_config_default_profiles")
    == 73
    and (ok_payload.get("discrepancy") or {}).get("compose_config_full_profiles")
    == 77
    and (ok_payload.get("counts") or {}).get("compose_config_default_profiles")
    == 73,
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
