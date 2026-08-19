#!/usr/bin/env python3
"""Pin P2 documentation contracts that are not already covered by recovery/wave4.

Does not start Docker. Run:
    python3 scripts/tests/test-p2-doc-contracts.py
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]

PASS = 0
FAIL = 0


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


def read(rel: str) -> str:
    path = ROOT / rel
    check(f"{rel} exists", path.is_file(), str(path))
    return path.read_text(encoding="utf-8") if path.is_file() else ""


print("P2 documentation contracts")

claude = read("pki-agent/CLAUDE.md")
check(
    "pki-agent/CLAUDE.md is tooling-only: 14 parents, 0 compose workloads",
    "14" in claude
    and ("0" in claude)
    and ("tooling" in claude.lower() or "tooling" in claude)
    and "11 cert-only" not in claude,
    "rewrite as tooling-only; remaining fleet is not 11 sidecars",
)
check(
    "pki-agent/CLAUDE.md does not present :9510 as the current Prometheus scrape",
    "pki-agent-<svc>:9510/metrics" not in claude
    and (
        "pki_enrollment_" in claude
        or ":9110" in claude
        or "historical" in claude.lower()
        or "sidecar surface" in claude.lower()
    ),
    "live scrape is parent :9110 pki_enrollment_*; :9510 is historical sidecar surface",
)
check(
    "pki-agent leftovers are dual writers, not a current compose fleet",
    "dual writer" in claude.lower() or "dual-writer" in claude.lower(),
)
check(
    "pki-agent provisioner is subject-scoped; shared pki-agent is not a current compose override",
    "until cutover" not in claude
    and "may still override to the shared" not in claude,
)

adr978 = read("docs/ADR/000978.md")
status978 = adr978.split("## Date", 1)[0]
check(
    "000978 has no supersedes frontmatter (000774/000969 are not fully obsolete)",
    re.search(r"(?m)^supersedes:", adr978) is None,
)
check(
    "000978 Status names 000774 clause boundary without superseding the ADR",
    "[[000774]]" in status978
    and ("supersede しない" in status978 or "置換しない" in status978)
    and ("PROXY_" in status978 or "reverse-proxy" in status978)
    and (
        "validate_mtls_url_schemes" in status978
        or "MTLS_ALLOWED_PEERS" in status978
        or "allowed-peers" in status978.lower()
    ),
)
check(
    "000978 Status names 000969 clause boundary without superseding the ADR",
    "[[000969]]" in status978
    and (":9510" in status978)
    and ("Alertmanager" in status978 or "Pushover" in status978),
)
check(
    "000978 D1 documents transitional unset→disabled only before atomic compose cutover",
    "未設定から disabled を推論しない" not in adr978
    and ("過渡" in adr978 or "image-first" in adr978 or "mixed-mode" in adr978)
    and "disabled" in adr978
    and ("最終" in adr978 or "final compose" in adr978.lower())
    and "enabled" in adr978
    and ("空" in adr978 or 'PKI_ENROLLMENT=""' in adr978 or "empty" in adr978.lower()),
    "unset means disabled only before cutover; final compose is explicit enabled; empty string fail-fast",
)

adr979 = read("docs/ADR/000979.md")
status979 = adr979.split("## Date", 1)[0]
check(
    "000979 does not claim 000825 already says directory-bind",
    "000825]] の rolling 修正後の経路" not in adr979
    and "already says directory" not in adr979.lower(),
)
check(
    "000979 Status amends 000825 compose-init, keeps validator/is_file",
    "[[000825]]" in status979
    and ("named-volume" in status979 or "SCS" in status979 or "init-container" in status979)
    and ("is_file" in status979 or "validator" in status979 or "Pydantic" in status979)
    and ("置換しない" in status979 or "supersede しない" in status979),
)

gameday = read("docs/runbooks/user-journey-slo-gameday.md")
check(
    "GameDay requires six alt_user_journey_requests_total series, not missing /metrics",
    "does not yet expose `/metrics`" not in gameday
    and "does not yet expose /metrics" not in gameday
    and ("six" in gameday.lower() or "6 本" in gameday or "6 series" in gameday.lower())
    and "alt_user_journey_requests_total" in gameday,
)

catalog = read("docs/services/MICROSERVICES.md")
check(
    "MICROSERVICES does not call AI/Recap/RAG/logging optional compose profiles",
    "複数のプロファイルでオプションの AI、Recap、RAG" not in catalog
    and "include:" in catalog,
    "those stacks are production include:; profiled-only are alt-perf / docker-socket-proxy / k6 / restic-backup",
)

inventory = read("scripts/ops-surface-inventory.yaml")
check(
    "ops-surface inventory measured_at is 2026-08-19 (sidecar-0 recount)",
    'measured_at: "2026-08-19"' in inventory,
)

readme = read("docs/runbooks/README.md")
cert_row = next(
    (line for line in readme.splitlines() if "cert" in line.lower() and "pki-agent-recovery" in line),
    "",
)
check(
    "symptom index sends cert incidents to pki-agent-recovery, not live mtls-cutover",
    "[[pki-agent-recovery]]" in cert_row
    and "[[mtls-cutover]]" not in cert_row,
    f"row={cert_row!r}",
)

cutover = read("docs/runbooks/mtls-cutover.md")
check(
    "mtls-cutover has a historical banner pointing at current PKI contract",
    (
        "historical" in cutover.lower()
        or "歴史" in cutover
        or "陳腐" in cutover
        or "sidecar" in cutover.lower()
    )
    and ("[[000978]]" in cutover or "[[pki-agent-recovery]]" in cutover),
)

bind = read("docs/runbooks/compose-bind-mount-policy.md")
osu = read("docs/runbooks/ops-surface-budget.md")
pre_start_docs = bind + "\n" + osu
check(
    "pre_start counts are 2 Wave 1b model chowns + 14 Wave 4 cert chowns (16 total)",
    "Wave 4" in pre_start_docs
    and "14" in pre_start_docs
    and "Wave 1b" in bind
    and "the two chown-only `pre_start` hooks" not in osu,
    "do not stop at 'the two chown-only pre_start hooks'",
)
check(
    "accidental OSU is 16 *-logs; leftover pki-agent is dual-writer incident not budget",
    "*-logs" in osu
    and ("dual writer" in osu.lower() or "dual-writer" in osu.lower() or "incident" in osu.lower())
    and "largest accidental pile" not in osu,
)

synthetic = read("docs/runbooks/synthetic-monitoring.md")
check(
    "synthetic distinguishes landed journey alert YAML from provider activation",
    "Wave 3 owns the alert YAML" not in synthetic
    and "Wave 3 may add Prometheus-side journey SLO alerts" not in synthetic
    and (
        "user-journey-slo-alerts.yml" in synthetic
        or "[[000980]]" in synthetic
        or "landed" in synthetic.lower()
    ),
)

acolyte = read("docs/services/acolyte-orchestrator.md")
check(
    "acolyte mTLS stale-cert recovery recreates the parent, not a TLS sidecar",
    "acolyte-orchestrator" in acolyte
    and "Restart the sidecar" not in acolyte,
    "inbound TLS is in the parent; recreate acolyte-orchestrator",
)

altctl = read("altctl/CLAUDE.md")
check(
    "altctl aggregate rationale is cross-stack depends_on / step-ca-bootstrap, not pki-agent sidecars",
    "pki-agent sidecars `depends_on`" not in altctl
    and ("step-ca-bootstrap" in altctl or "cross-stack" in altctl or "cross-file" in altctl),
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
