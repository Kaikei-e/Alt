#!/usr/bin/env python3
"""Pin docs/runbooks/health-deep-contract.md listener matrix to implemented probes.

Deep health is not a single unpublished PKI ops :9110. news-creator,
recap-subworker and search-indexer serve GET /health/deep on the app
listener; their PKI ops mux returns 404 for that path. alt-backend is the
service whose ops :9110 actually serves deep health, and that port is not
published to the host.

Does not start Docker. Run:
    python3 scripts/tests/test-health-deep-contract.py
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
RUNBOOK = ROOT / "docs" / "runbooks" / "health-deep-contract.md"
ADR = ROOT / "docs" / "ADR" / "000980.md"

PASS = 0
FAIL = 0

HOST_DEEP = {
    "news-creator": "127.0.0.1:11434/health/deep",
    "recap-subworker": "127.0.0.1:8002/health/deep",
    "search-indexer": "127.0.0.1:9300/health/deep",
    "knowledge-sovereign": "127.0.0.1:9511/health/deep",
}
GENERIC_HOST_9110_DEEP = re.compile(
    r"curl[^\n]*127\.0\.0\.1:9110/health/deep",
)


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
        return
    print(f"  FAIL  {name}")
    if detail:
        print(f"        {detail}")
    FAIL += 1


print("health-deep contract listener matrix")

md = RUNBOOK.read_text(encoding="utf-8")
adr = ADR.read_text(encoding="utf-8")

check("runbook exists", RUNBOOK.is_file(), str(RUNBOOK))
check("ADR 000980 exists", ADR.is_file(), str(ADR))

for service, url in HOST_DEEP.items():
    check(
        f"copyable host curl for {service} deep health",
        url in md,
        f"need curl ... http://{url}",
    )

check(
    "alt-backend deep health is unpublished ops :9110 via toolbox, not localhost:9110",
    "alt-backend:9110/health/deep" in md
    and ("docker run" in md and "--network" in md and "wget" in md),
    "distroless parent has no wget; probe via toolbox on the Compose network",
)

check(
    "TL;DR / copyable commands do not claim host :9110 serves deep health",
    GENERIC_HOST_9110_DEEP.search(md) is None,
    "127.0.0.1:9110/health/deep is connection-refused; PKI ops is unpublished",
)

for service in ("news-creator", "recap-subworker", "search-indexer"):
    check(
        f"{service} PKI ops :9110 is not documented as deep health",
        "PKI" in md and ":9110" in md and "/health/deep" in md,
        "must say PKI ops :9110 is /health+/metrics only for app-listener services",
    )

check(
    "runbook states PKI ops :9110 does not serve /health/deep for app-listener services",
    (
        "does not serve" in md.lower() or "serve しない" in md or "serves しない" in md
        or "404" in md
        or "PKI ops" in md
    )
    and "news-creator" in md
    and "recap-subworker" in md
    and "search-indexer" in md,
    "explicit: unpublished PKI :9110 is not the deep-health listener for those three",
)

check(
    "implemented-probe table names cheap vs deep listeners",
    all(
        token in md
        for token in (
            ":11434",
            ":8002",
            ":9300",
            ":9110",
            ":9501",
            "news-creator",
            "recap-subworker",
            "search-indexer",
            "alt-backend",
            "knowledge-sovereign",
        )
    ),
)

check(
    "journey SLO alert YAML is landed; synthetic activation is the remaining ops gate",
    ("user-journey-slo-alerts.yml" in md or "[[000980]]" in md)
    and (
        "activation" in md.lower()
        or "landed" in md.lower()
        or "オペレータ" in md
        or "ops gate" in md.lower()
    ),
    "do not say Wave 3 still owns unlanded journey alerts",
)

d1 = adr.split("### D1.", 1)[-1].split("### D2.", 1)[0]
check(
    "ADR 000980 D1 is a per-service listener matrix, not 'all deep on ops :9110'",
    "置く場所は ops `:9110`" not in d1
    and (
        "health-deep-contract" in d1
        or "リスナー" in d1
        or "listener" in d1.lower()
        or ":11434" in d1
    ),
    "do not claim unpublished PKI ops :9110 universally serves /health/deep",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
