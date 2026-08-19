#!/usr/bin/env python3
"""RED/GREEN tests for scripts/synthetic-probes-audit.py (H3 / M8 / M9).

The auditor must parse probes.yaml and reject:
  H3  login/search URLs that are not the live edge (SPA 200 masquerade,
      /sessions/whoami without /ory, GET /api/v1/search)
  M8  Knowledge Home (and search) Connect calls missing protocol headers/body
  M9  follow_redirects + session cookie, missing >=60s interval / >=5s gap,
      host-only cookie semantics, committed secrets

Live probes.yaml is asserted too: after GREEN it must itself be a clean spec.

Run:
    python3 scripts/tests/test-synthetic-probes-audit.py
Exit 0 on green, non-zero on red.

Executed by scripts/observability-validate.sh (not a comment-only reminder).
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"

spec = importlib.util.spec_from_file_location(
    "synthetic_probes_audit", SCRIPTS / "synthetic-probes-audit.py"
)
assert spec is not None and spec.loader is not None
audit = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = audit
spec.loader.exec_module(audit)

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


def has_violation(violations: list[str], *needles: str) -> bool:
    blob = "\n".join(violations).lower()
    return all(needle.lower() in blob for needle in needles)


def catalog() -> audit.RouteCatalog:
    return audit.RouteCatalog(
        plecto_routes=[
            audit.PlectoRoute("/ory/", "kratos", strip_prefix="/ory"),
            audit.PlectoRoute("/api/", "alt-frontend-sv"),
            audit.PlectoRoute("/", "alt-frontend-sv"),
        ],
        svelte_methods={
            "/api/v1/feeds/fetch/cursor": {"GET"},
            "/api/v2/*": {"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"},
        },
        public_services={
            "alt.search.v2.SearchService",
            "alt.knowledge_home.v1.KnowledgeHomeService",
        },
        rpcs={
            "alt.search.v2.SearchService": {"SearchArticles"},
            "alt.knowledge_home.v1.KnowledgeHomeService": {"GetKnowledgeHome"},
        },
        connect_proxy=True,
    )


def good_spec() -> dict:
    return {
        "version": 1,
        "status": "spec_only",
        "paging": {
            "consecutive_failures": 3,
            "on_first_failure": "observe",
            "credential_class": "min_privilege",
        },
        "defaults": {
            "timeout_ms": 8000,
            "interval_s": 60,
            "min_external_interval_s": 5,
            "follow_redirects": False,
            "headers": {
                "Accept": "application/json",
                "User-Agent": "alt-synthetic/p2-11",
            },
            "cookies": {
                "host_only": True,
                "same_site": "Lax",
                "forward_on_redirect": False,
            },
            "credentials": {"mode": "same-origin"},
            "tls": {"verify": True},
        },
        "scheduling": {"min_gap_s": 5},
        "heartbeat": {
            "metric": "alt_synthetic_probe_result",
            "freshness": "15m",
            "labels": {"required": ["journey", "probe"]},
            "bff_observation_journeys": ["feeds", "search"],
        },
        "probes": [
            {
                "id": "login_session",
                "journey": "login",
                "bff_observation": False,
                "method": "GET",
                "url": "{{EDGE_BASE_URL}}/ory/sessions/whoami",
                "auth": {
                    "type": "session_cookie",
                    "user": "{{SYNTHETIC_USER}}",
                    "password_ref": "provider_secret:SYNTHETIC_PASSWORD",
                },
                "expect": {"status": 200},
                "page_on": "consecutive_failures",
            },
            {
                "id": "feeds_cursor",
                "journey": "feeds",
                "bff_observation": True,
                "method": "GET",
                "url": "{{EDGE_BASE_URL}}/api/v1/feeds/fetch/cursor",
                "auth": {
                    "type": "session_cookie",
                    "user": "{{SYNTHETIC_USER}}",
                    "password_ref": "provider_secret:SYNTHETIC_PASSWORD",
                },
                "expect": {"status": 200},
                "page_on": "consecutive_failures",
            },
            {
                "id": "knowledge_home",
                "journey": "knowledge_home",
                "bff_observation": False,
                "method": "POST",
                "url": "{{EDGE_BASE_URL}}/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
                "auth": {
                    "type": "session_cookie",
                    "user": "{{SYNTHETIC_USER}}",
                    "password_ref": "provider_secret:SYNTHETIC_PASSWORD",
                },
                "headers": {
                    "Content-Type": "application/json",
                    "Connect-Protocol-Version": "1",
                },
                "body": {"json": {"limit": 20}},
                "expect": {"status": 200},
                "page_on": "consecutive_failures",
            },
            {
                "id": "knowledge_search",
                "journey": "search",
                "bff_observation": True,
                "method": "POST",
                "url": "{{EDGE_BASE_URL}}/api/v2/alt.search.v2.SearchService/SearchArticles",
                "auth": {
                    "type": "session_cookie",
                    "user": "{{SYNTHETIC_USER}}",
                    "password_ref": "provider_secret:SYNTHETIC_PASSWORD",
                },
                "headers": {
                    "Content-Type": "application/json",
                    "Connect-Protocol-Version": "1",
                },
                "body": {"json": {"query": "alt", "limit": 5}},
                "expect": {"status": 200},
                "page_on": "consecutive_failures",
            },
        ],
    }


print("catalog from the live tree")
live_catalog = audit.load_catalog(ROOT)
check(
    "Plecto exposes Kratos under /ory/",
    any(r.prefix == "/ory/" and r.upstream == "kratos" for r in live_catalog.plecto_routes),
)
check(
    "Plecto SPA catch-all is path_prefix / to alt-frontend-sv",
    any(r.prefix == "/" and r.upstream == "alt-frontend-sv" for r in live_catalog.plecto_routes),
)
check(
    "SvelteKit has GET /api/v1/feeds/fetch/cursor",
    "GET" in live_catalog.svelte_methods.get("/api/v1/feeds/fetch/cursor", set()),
)
check(
    "SvelteKit has no /api/v1/search route",
    "/api/v1/search" not in live_catalog.svelte_methods,
)
check(
    "Connect proxy is mounted at /api/v2",
    live_catalog.connect_proxy,
)
check(
    "SearchService.SearchArticles is a public Connect RPC",
    "alt.search.v2.SearchService" in live_catalog.public_services
    and "SearchArticles" in live_catalog.rpcs.get("alt.search.v2.SearchService", set()),
)
check(
    "SearchService has no RPC named Search",
    "Search" not in live_catalog.rpcs.get("alt.search.v2.SearchService", set()),
)
check(
    "GetKnowledgeHome is a public Connect RPC",
    "alt.knowledge_home.v1.KnowledgeHomeService" in live_catalog.public_services
    and "GetKnowledgeHome"
    in live_catalog.rpcs.get("alt.knowledge_home.v1.KnowledgeHomeService", set()),
)

print("auditor rejects the H3/M8/M9 defect classes")
cat = catalog()

whoami = good_spec()
whoami["probes"][0]["url"] = "{{EDGE_BASE_URL}}/sessions/whoami"
v = audit.audit(whoami, cat)
check(
    "H3 login /sessions/whoami is SPA masquerade (need /ory/sessions/whoami)",
    has_violation(v, "whoami") or has_violation(v, "spa") or has_violation(v, "/ory/"),
    "\n".join(v),
)

rest_search = good_spec()
rest_search["probes"][3]["method"] = "GET"
rest_search["probes"][3]["url"] = "{{EDGE_BASE_URL}}/api/v1/search"
rest_search["probes"][3]["query"] = {"q": "alt"}
rest_search["probes"][3].pop("headers", None)
rest_search["probes"][3].pop("body", None)
v = audit.audit(rest_search, cat)
check(
    "H3 GET /api/v1/search is an unknown endpoint",
    has_violation(v, "/api/v1/search") or has_violation(v, "unknown"),
    "\n".join(v),
)

unknown_rpc = good_spec()
unknown_rpc["probes"][3]["url"] = (
    "{{EDGE_BASE_URL}}/api/v2/alt.search.v2.SearchService/Search"
)
v = audit.audit(unknown_rpc, cat)
check(
    "H3 SearchService/Search is unknown (live RPC is SearchArticles)",
    has_violation(v, "Search") and (
        has_violation(v, "unknown") or has_violation(v, "SearchArticles")
    ),
    "\n".join(v),
)

home_no_connect = good_spec()
home_no_connect["probes"][2]["headers"] = {"Content-Type": "application/json"}
home_no_connect["probes"][2]["body"] = {"json": {}}
v = audit.audit(home_no_connect, cat)
check(
    "M8 Knowledge Home without Connect-Protocol-Version is rejected",
    has_violation(v, "Connect-Protocol-Version") or has_violation(v, "connect"),
    "\n".join(v),
)

search_no_body = good_spec()
search_no_body["probes"][3]["body"] = {"json": {}}
v = audit.audit(search_no_body, cat)
check(
    "H3 search Connect body must include query",
    has_violation(v, "query"),
    "\n".join(v),
)

redirects = good_spec()
redirects["defaults"]["follow_redirects"] = True
v = audit.audit(redirects, cat)
check(
    "M9 follow_redirects true with session cookies is rejected",
    has_violation(v, "redirect"),
    "\n".join(v),
)

no_interval = good_spec()
no_interval["defaults"].pop("interval_s")
v = audit.audit(no_interval, cat)
check(
    "M9 missing interval_s is rejected",
    has_violation(v, "interval"),
    "\n".join(v),
)

fast_interval = good_spec()
fast_interval["defaults"]["interval_s"] = 15
v = audit.audit(fast_interval, cat)
check(
    "M9 interval_s < 60 is rejected",
    has_violation(v, "60") or has_violation(v, "interval"),
    "\n".join(v),
)

no_gap = good_spec()
no_gap["defaults"].pop("min_external_interval_s")
no_gap["scheduling"] = {"min_gap_s": 1}
v = audit.audit(no_gap, cat)
check(
    "M9 external gap < 5s is rejected",
    has_violation(v, "5") or has_violation(v, "gap") or has_violation(v, "external"),
    "\n".join(v),
)

cookie_cross = good_spec()
cookie_cross["defaults"]["cookies"] = {
    "host_only": False,
    "same_site": "None",
    "forward_on_redirect": True,
}
v = audit.audit(cookie_cross, cat)
check(
    "M9 host-only / no cross-origin cookie forwarding is required",
    has_violation(v, "host") or has_violation(v, "cookie") or has_violation(v, "same-origin"),
    "\n".join(v),
)

secret = good_spec()
secret["probes"][0]["auth"]["password"] = "hunter2-not-a-placeholder"
v = audit.audit(secret, cat)
check(
    "placeholders contain no secrets",
    has_violation(v, "secret") or has_violation(v, "password") or has_violation(v, "hunter"),
    "\n".join(v),
)

timeouts = good_spec()
timeouts["defaults"].pop("timeout_ms")
timeouts["paging"]["consecutive_failures"] = 1
timeouts["paging"]["on_first_failure"] = "page"
v = audit.audit(timeouts, cat)
check(
    "timeout and consecutive-failure paging semantics are required",
    has_violation(v, "timeout") or has_violation(v, "consecutive"),
    "\n".join(v),
)

print("canonical spec is accepted")
v = audit.audit(good_spec(), cat)
check("good spec has no violations", v == [], "\n".join(v))

print("live observability/synthetic/probes.yaml")
live_doc = audit.load_probes(ROOT / "observability" / "synthetic" / "probes.yaml")
check("live spec parses as a mapping", isinstance(live_doc, dict))
live_v = audit.audit(live_doc, live_catalog)
check(
    "live probes.yaml matches Plecto/Svelte/Connect and cookie/interval rules",
    live_v == [],
    "\n".join(live_v),
)

print("per-journey heartbeat / BFF observation contract")
heartbeat = (live_doc or {}).get("heartbeat") if isinstance(live_doc, dict) else None
check(
    "live spec declares provider-independent heartbeat metric",
    isinstance(heartbeat, dict)
    and heartbeat.get("metric") == "alt_synthetic_probe_result",
    f"got {heartbeat!r}",
)
check(
    "heartbeat freshness is 15m (matches UserJourneyNoObservation)",
    isinstance(heartbeat, dict) and str(heartbeat.get("freshness") or "") in {"15m", "15m0s"},
    f"got {heartbeat!r}",
)
bff_journeys = set()
if isinstance(heartbeat, dict):
    listed = heartbeat.get("bff_observation_journeys") or []
    if isinstance(listed, list):
        bff_journeys = {str(item) for item in listed}
check(
    "BFF observation journeys are feeds and search only (not login)",
    bff_journeys == {"feeds", "search"},
    f"got {sorted(bff_journeys)}; login whoami is Kratos and cannot increment BFF login",
)

probes_by_id = {
    str(p.get("id")): p
    for p in ((live_doc or {}).get("probes") or [])
    if isinstance(p, dict)
}
check(
    "login_session sets bff_observation false",
    probes_by_id.get("login_session", {}).get("bff_observation") is False,
)
check(
    "feeds_cursor sets bff_observation true",
    probes_by_id.get("feeds_cursor", {}).get("bff_observation") is True,
)
check(
    "knowledge_search sets bff_observation true",
    probes_by_id.get("knowledge_search", {}).get("bff_observation") is True,
)

login_bff = good_spec()
login_bff["probes"][0]["bff_observation"] = True
v = audit.audit(login_bff, cat)
check(
    "auditor rejects login bff_observation true (Kratos whoami skips BFF)",
    has_violation(v, "login") and (
        has_violation(v, "bff") or has_violation(v, "kratos") or has_violation(v, "whoami")
    ),
    "\n".join(v),
)

no_hb = good_spec()
no_hb.pop("heartbeat", None)
v = audit.audit(no_hb, cat)
check(
    "auditor rejects a spec without heartbeat.metric",
    has_violation(v, "heartbeat") or has_violation(v, "alt_synthetic_probe_result"),
    "\n".join(v),
)

rules = (
    ROOT / "observability" / "prometheus" / "rules" / "user-journey-slo-alerts.yml"
).read_text(encoding="utf-8")
rules_v = audit.audit_no_observation_rules(rules)
check(
    "SLO rules no-observation contract is clean",
    rules_v == [],
    "\n".join(rules_v),
)
generic_up = (
    "up{job=~\"synthetic|blackbox-synthetic\"} or vector(0)\n"
    "and on() (alt:synthetic:probes:activated == 1)\n"
)
check(
    "auditor rejects generic up{job=synthetic} no-observation arming",
    has_violation(audit.audit_no_observation_rules(generic_up), "up{job"),
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
