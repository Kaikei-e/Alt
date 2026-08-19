#!/usr/bin/env python3
"""Structural contract for SLO evaluators, inhibit labels, and CI wiring.

Pins H1/H2, M3/M4, L2 leftovers that promtool cannot see:
  - page inhibits ticket on stable SLI labels, not alertname
  - Grafana KH copies are not a second evaluator (empty groups)
  - noDataState: OK is forbidden on any remaining KH Grafana rule
  - dead pki-agent sidecar alert group is gone
  - counter guard and BFF paths are in observability validate/CI
"""

from __future__ import annotations

import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
PASS = 0
FAIL = 0

DEAD_SIDECAR_ALERTS = {
    "PkiAgentCertExpirySoon",
    "PkiAgentCertExpired",
    "PkiAgentRenewalFailing",
    "PkiAgentDown",
    "PkiAgentFleetIncomplete",
    "PkiAgentMetricsAbsent",
}


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


def load(path: pathlib.Path) -> object:
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def _comment_only_invocation(text: str, command: str) -> bool:
    """True when command appears only in comments, never as an executed line."""
    executed = False
    mentioned = False
    for raw in text.splitlines():
        stripped = raw.strip()
        if command not in stripped:
            continue
        mentioned = True
        if stripped.startswith("#"):
            continue
        executed = True
    return mentioned and not executed


print("SLO evaluator / inhibit / CI contract")

am = load(ROOT / "observability" / "alertmanager" / "alertmanager.yml")
inhibits = am.get("inhibit_rules") or []
page_ticket = [
    r
    for r in inhibits
    if isinstance(r, dict)
    and any("severity=~\"page" in str(m) or "severity=~'page" in str(m) for m in (r.get("source_matchers") or []))
]
check("page→ticket inhibit rule exists", len(page_ticket) == 1, f"got {len(page_ticket)}")
equal = page_ticket[0].get("equal") if page_ticket else []
check(
    "page inhibits ticket on stack/sli/service/journey, not alertname",
    equal == ["stack", "sli", "service", "journey"],
    f"equal={equal!r} — matching on alertname cannot collapse 14.4x page onto 6x ticket",
)

grafana_kh = load(
    ROOT / "observability" / "grafana" / "provisioning" / "alerting" / "knowledge-home-slo-rules.yaml"
)
g_groups = (grafana_kh or {}).get("groups") or []
g_rules = [rule for g in g_groups for rule in (g.get("rules") or [])]
check(
    "Grafana provisioned KH SLO groups have no rules (Prometheus is the evaluator)",
    g_rules == [],
    f"{len(g_rules)} Grafana KH rules would dual-evaluate with Prometheus",
)
for rule in g_rules:
    check(
        f"Grafana KH rule {rule.get('uid')!r} must not use noDataState OK",
        str(rule.get("noDataState", "")).upper() != "OK",
        "re-enabling a paused copy with noDataState: OK silently drops missing series",
    )

alerts_kh = ROOT / "observability" / "alerts" / "knowledge-home-slo-rules.yaml"
if alerts_kh.is_file():
    doc = load(alerts_kh) or {}
    extra = [rule for g in (doc.get("groups") or []) for rule in (g.get("rules") or [])]
    check(
        "observability/alerts KH copy has no live Grafana rules",
        extra == [],
        f"{len(extra)} rules in the non-provisioned copy can still be imported",
    )

pki = load(ROOT / "observability" / "prometheus" / "rules" / "pki-agent-alerts.yml")
group_names = [g.get("name") for g in (pki.get("groups") or [])]
check(
    "pki_agent_cert_lifecycle sidecar group is absent",
    "pki_agent_cert_lifecycle" not in group_names,
    f"groups={group_names!r}; leftover detection is the Docker-label gate, live alerts are in-process",
)
sidecar_alerts = {
    rule.get("alert")
    for g in (pki.get("groups") or [])
    for rule in (g.get("rules") or [])
    if rule.get("alert") in DEAD_SIDECAR_ALERTS
}
check(
    "no live PkiAgent* sidecar alerts",
    sidecar_alerts == set(),
    f"still defined: {sorted(sidecar_alerts)}",
)
inprocess = {
    rule.get("alert")
    for g in (pki.get("groups") or [])
    for rule in (g.get("rules") or [])
    if isinstance(rule.get("alert"), str) and str(rule.get("alert")).startswith("PkiEnrollment")
}
check(
    "in-process PkiEnrollment* alerts remain",
    "PkiEnrollmentCertExpirySoon" in inprocess and "PkiEnrollmentOpsDown" in inprocess,
    f"got {sorted(inprocess)}",
)

validate = (ROOT / "scripts" / "observability-validate.sh").read_text(encoding="utf-8")
check(
    "observability-validate.sh runs the BFF counter guard",
    "scripts/tests/test-user-journey-slo-counters.py" in validate,
    "counter guard must run in the same validate path as promtool",
)
workflow = (ROOT / ".github" / "workflows" / "observability-validate.yaml").read_text(
    encoding="utf-8"
)
check(
    "observability-validate workflow watches alt-butterfly-facade",
    "alt-butterfly-facade/**" in workflow,
    "BFF Record/mux wiring changes must retrigger the counter guard",
)
check(
    "observability-validate workflow watches the counter-guard script",
    "scripts/tests/test-user-journey-slo-counters.py" in workflow,
    "editing the guard without a workflow path would skip CI",
)

uj = (ROOT / "observability" / "prometheus" / "rules" / "user-journey-slo-alerts.yml").read_text(
    encoding="utf-8"
)
check(
    "14.4x annotation says ~50 hours, not ~2 hours",
    "~50 hours" in uj and "~2 hours" not in uj,
    "14.4× of a 30-day 99.5% budget is ~50 hours",
)
kh = (ROOT / "observability" / "prometheus" / "rules" / "knowledge-home-slo-alerts.yml").read_text(
    encoding="utf-8"
)
check(
    "KH 14.4x annotation says ~50 hours, not ~2 hours",
    "~50 hours" in kh and "~2 hours" not in kh,
    "same budget math as user-journey",
)
check(
    "user-journey burn uses increase1h >= 1, not > 50",
    "increase1h >= 1" in uj and "increase1h > 50" not in uj,
    "solo-ops traffic never reaches 50 events/h",
)
check(
    "user-journey 6x burn uses increase6h >= 1, not > 50",
    "increase6h >= 1" in uj and "increase6h > 50" not in uj,
)
check(
    "KH 14.4x burn has an observation gate",
    "increase(alt_home_requests_total[1h])" in kh and ">= 1" in kh,
    "0/0 must not page; an observed Home error at 14.4x must",
)

check(
    "observability-validate.sh runs the synthetic probes audit",
    "python3 scripts/tests/test-synthetic-probes-audit.py" in validate
    and not _comment_only_invocation(
        validate, "python3 scripts/tests/test-synthetic-probes-audit.py"
    ),
    "must be an executed stage, not a comment",
)
check(
    "observability-validate workflow watches synthetic probes spec",
    "observability/synthetic/**" in workflow,
)
check(
    "observability-validate workflow watches synthetic probes auditor",
    "scripts/synthetic-probes-audit.py" in workflow,
)
check(
    "observability-validate workflow watches synthetic probes tests",
    "scripts/tests/test-synthetic-probes-audit.py" in workflow,
)
check(
    "observability-validate workflow watches Plecto routes",
    "plecto/**" in workflow,
)
check(
    "user-journey no-observation is not armed by generic up{job=synthetic}",
    'up{job=~"synthetic' not in uj and 'up{job="synthetic"}' not in uj,
    "login whoami is Kratos and cannot increment the BFF login counter",
)
check(
    "no-observation uses per-journey synthetic heartbeat present_over_time",
    "present_over_time(alt_synthetic_probe_result" in uj.replace(" ", "").replace("\n", "")
    or "present_over_time(alt_synthetic_probe_result" in uj,
    "arm only the journey whose exact heartbeat is fresh",
)
check(
    "no-observation does not select journey=login",
    'journey=~"feeds|search"' in uj or 'journey!="login"' in uj,
    "login heartbeat must not false-page BFF login no-observation",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
