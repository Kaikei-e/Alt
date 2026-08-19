#!/usr/bin/env python3
"""Tests for scripts/compose-init-edge-audit.py.

A `service_completed_successfully` edge onto a `restart: "no"` (or
omitted, which Compose defaults to `no`) service is the PM-2026-037
shape: rolling `up --no-deps` never starts the one-shot. The allowlist
is the only thing that keeps a new edge from landing unreviewed.

YAML merge (`<<: *anchor`) plus an overriding `depends_on:` must be
visible to the parser — pki-agent sidecars use both.

Run:
    python3 scripts/tests/test-compose-init-edge-audit.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location(
    "init_edge_audit", SCRIPTS / "compose-init-edge-audit.py"
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


print("restart_is_no")
check("explicit no", audit.restart_is_no({"restart": "no"}))
check("quoted no", audit.restart_is_no({"restart": '"no"'}))
check("omitted defaults to no", audit.restart_is_no({}))
check("unless-stopped is not no", not audit.restart_is_no({"restart": "unless-stopped"}))
check("always is not no", not audit.restart_is_no({"restart": "always"}))

print("init_edges")
SERVICES = {
    "migrate": {"restart": "no"},
    "alt-data-hub": {
        "depends_on": {
            "migrate": {"condition": "service_completed_successfully"},
            "pgbouncer": {"condition": "service_healthy"},
        }
    },
    "pgbouncer": {"restart": "always"},
    "kratos": {
        "depends_on": {
            "kratos-migrate": {"condition": "service_completed_successfully"},
        }
    },
    "kratos-migrate": {"restart": "no"},
    "running": {
        "restart": "unless-stopped",
        "depends_on": {"pgbouncer": {"condition": "service_healthy"}},
    },
}
edges = audit.init_edges(SERVICES)
check(
    "only service_completed_successfully edges onto restart-no targets",
    edges
    == [
        ("alt-data-hub", "migrate"),
        ("kratos", "kratos-migrate"),
    ],
)

print("audit_init_edges")
allow = {"alt-data-hub -> migrate", "kratos -> kratos-migrate"}
check(
    "matching allowlist is clean",
    audit.audit_init_edges(SERVICES, allow) == [],
)
extra = audit.audit_init_edges(SERVICES, {"alt-data-hub -> migrate"})
check(
    "an unlisted edge is a violation",
    any("kratos -> kratos-migrate" in v and "unlisted" in v for v in extra),
)
stale = audit.audit_init_edges(SERVICES, allow | {"ghost -> gone"})
check(
    "a stale allowlist row is a violation",
    any("ghost -> gone" in v and "stale" in v for v in stale),
)

NO_RESTART_TARGET = {
    "migrate": {},  # omitted restart = no
    "alt-data-hub": {
        "depends_on": {"migrate": {"condition": "service_completed_successfully"}}
    },
}
check(
    "omitted restart on the SCS target still counts as a one-shot edge",
    audit.init_edges(NO_RESTART_TARGET) == [("alt-data-hub", "migrate")],
)

print("production allowlist pin")
from compose_include import production_services  # noqa: E402
import json  # noqa: E402

PROD = production_services()
PROD_EDGES = [audit.edge_key(c, t) for c, t in audit.init_edges(PROD)]
BASELINE = json.loads(
    (SCRIPTS / "ops-surface-baseline.json").read_text(encoding="utf-8")
)
ALLOW = BASELINE["init_edges_allowlist"]
EXPECTED_22 = [
    "acolyte-orchestrator -> acolyte-db-migrator",
    "acolyte-orchestrator -> step-ca-bootstrap",
    "alt-backend -> step-ca-bootstrap",
    "alt-butterfly-facade -> step-ca-bootstrap",
    "alt-data-hub -> migrate",
    "alt-data-hub -> step-ca-bootstrap",
    "alt-harvester -> step-ca-bootstrap",
    "alt-notifier -> step-ca-bootstrap",
    "auth-hub -> step-ca-bootstrap",
    "auth-token-manager -> oauth-token-init",
    "knowledge-sovereign -> knowledge-sovereign-db-migrator",
    "kratos -> kratos-migrate",
    "news-creator -> step-ca-bootstrap",
    "pre-processor -> pre-processor-db-migrator",
    "pre-processor -> step-ca-bootstrap",
    "pre-processor-sidecar -> oauth-token-init",
    "pre-processor-sidecar -> pre-processor-db-migrator",
    "rag-orchestrator -> step-ca-bootstrap",
    "recap-subworker -> step-ca-bootstrap",
    "recap-worker -> step-ca-bootstrap",
    "search-indexer -> step-ca-bootstrap",
    "tag-generator -> step-ca-bootstrap",
]
check(
    "production has exactly 22 one-shot SCS edges",
    len(PROD_EDGES) == 22 and len(ALLOW) == 22,
)
check(
    "production edges match the exact 22 allowlist pairs",
    PROD_EDGES == EXPECTED_22 and ALLOW == EXPECTED_22,
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
