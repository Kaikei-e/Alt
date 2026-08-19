#!/usr/bin/env python3
"""Structural audit of observability/synthetic/probes.yaml.

Validates every probe URL/method/header/body against the live Plecto edge,
SvelteKit routes, and Connect-RPC procedures. Rejects SPA HTML-200 masquerade,
unknown endpoints, Connect clients missing protocol headers, redirect+cookie
credential leaks, too-aggressive intervals, and committed secrets.

Executed from scripts/observability-validate.sh (and CI) together with
the unit tests in scripts/tests/test-synthetic-probes-audit.py.

Run:
    python3 scripts/synthetic-probes-audit.py
    python3 scripts/tests/test-synthetic-probes-audit.py
Exit 0 when the spec is clean, non-zero on violations.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys
from dataclasses import dataclass, field

try:
    import yaml
except ImportError as exc:  # pragma: no cover - CI installs PyYAML
    raise SystemExit("PyYAML is required: pip install 'pyyaml==6.*'") from exc

try:
    import tomllib
except ImportError:  # pragma: no cover - Python < 3.11
    tomllib = None  # type: ignore[assignment]


REPO_ROOT = pathlib.Path(__file__).resolve().parents[1]
PROBES_PATH = REPO_ROOT / "observability" / "synthetic" / "probes.yaml"

PLACEHOLDER_RE = re.compile(r"^\{\{[A-Z][A-Z0-9_]*\}\}$")
PROVIDER_SECRET_RE = re.compile(r"^provider_secret:[A-Z][A-Z0-9_]*$")
SECRETISH_RE = re.compile(
    r"(?i)(sk-|api[_-]?key|bearer\s+[A-Za-z0-9._\-]{8,}|password\s*[:=]\s*(?!\{\{)(?!provider_secret:)\S+)"
)
CONNECT_PATH_RE = re.compile(
    r"^/api/v2/([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)*)/([A-Za-z][A-Za-z0-9_]*)$"
)
SVELTE_SERVER_RE = re.compile(r"export const (GET|POST|PUT|PATCH|DELETE|fallback)\b")
PROTO_PACKAGE_RE = re.compile(r"^package\s+([\w.]+)\s*;", re.M)
PUBLIC_SERVICE_RE = re.compile(r'"((?:alt|services)\.[^"]+)"')
MIN_INTERVAL_S = 60
MIN_EXTERNAL_GAP_S = 5
KRATOS_WHOAMI = "/ory/sessions/whoami"
HEARTBEAT_METRIC = "alt_synthetic_probe_result"
HEARTBEAT_FRESHNESS = frozenset({"15m", "15m0s"})
BFF_OBSERVATION_JOURNEYS = frozenset({"feeds", "search"})
LOGIN_JOURNEYS = frozenset({"login"})
HEARTBEAT_LABELS = frozenset({"journey", "probe"})


@dataclass(frozen=True)
class PlectoRoute:
    prefix: str
    upstream: str
    strip_prefix: str = ""


@dataclass
class RouteCatalog:
    plecto_routes: list[PlectoRoute] = field(default_factory=list)
    svelte_methods: dict[str, set[str]] = field(default_factory=dict)
    public_services: set[str] = field(default_factory=set)
    rpcs: dict[str, set[str]] = field(default_factory=dict)
    connect_proxy: bool = False

    def longest_plecto(self, path: str) -> PlectoRoute | None:
        matches = [r for r in self.plecto_routes if path.startswith(r.prefix)]
        if not matches:
            return None
        return max(matches, key=lambda r: len(r.prefix))


def _read(path: pathlib.Path) -> str:
    return path.read_text(encoding="utf-8")


def parse_plecto_routes(manifest_text: str) -> list[PlectoRoute]:
    if tomllib is None:
        raise RuntimeError("tomllib is required to parse plecto/manifest.toml")
    data = tomllib.loads(manifest_text)
    routes: list[PlectoRoute] = []
    for item in data.get("route") or []:
        match = item.get("match") or {}
        prefix = match.get("path_prefix")
        upstream = item.get("upstream")
        if not prefix or not upstream:
            continue
        routes.append(
            PlectoRoute(
                prefix=prefix,
                upstream=upstream,
                strip_prefix=str(item.get("strip_prefix") or ""),
            )
        )
    return routes


def parse_public_services(allowlist_text: str) -> set[str]:
    return set(PUBLIC_SERVICE_RE.findall(allowlist_text))


def parse_proto_rpcs(proto_text: str) -> dict[str, set[str]]:
    package_match = PROTO_PACKAGE_RE.search(proto_text)
    package = package_match.group(1) if package_match else ""
    rpcs: dict[str, set[str]] = {}
    service: str | None = None
    depth = 0
    in_service = False
    for raw in proto_text.splitlines():
        line = raw.split("//", 1)[0].strip()
        if not line:
            continue
        svc = re.match(r"service\s+(\w+)\s*\{?", line)
        if svc and not in_service:
            service = f"{package}.{svc.group(1)}" if package else svc.group(1)
            rpcs.setdefault(service, set())
            in_service = True
            depth = line.count("{") - line.count("}")
            if depth <= 0 and "{" in line and "}" in line:
                in_service = False
                service = None
            continue
        if in_service:
            rpc = re.match(r"rpc\s+(\w+)\s*\(", line)
            if rpc and service:
                rpcs[service].add(rpc.group(1))
            depth += line.count("{") - line.count("}")
            if depth <= 0:
                in_service = False
                service = None
    return rpcs


def svelte_url_from_server_path(rel: pathlib.Path) -> str:
    parts: list[str] = []
    for part in rel.parts[:-1]:
        if part.startswith("(") and part.endswith(")"):
            continue
        if part.startswith("[...") and part.endswith("]"):
            parts.append("*")
            continue
        if part.startswith("[") and part.endswith("]"):
            parts.append("{" + part[1:-1] + "}")
            continue
        parts.append(part)
    return "/" + "/".join(parts)


def parse_svelte_methods(server_text: str) -> set[str]:
    found = set(SVELTE_SERVER_RE.findall(server_text))
    if "fallback" in found:
        return {"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}
    return found


def load_catalog(repo_root: pathlib.Path) -> RouteCatalog:
    repo = pathlib.Path(repo_root)
    plecto = parse_plecto_routes(_read(repo / "plecto" / "manifest.toml"))
    allowlist = parse_public_services(
        _read(repo / "alt-frontend-sv" / "src" / "lib" / "gen" / "allowlist.ts")
    )
    rpcs: dict[str, set[str]] = {}
    proto_root = repo / "proto"
    if proto_root.is_dir():
        for proto in proto_root.rglob("*.proto"):
            for service, methods in parse_proto_rpcs(_read(proto)).items():
                rpcs.setdefault(service, set()).update(methods)
    svelte_methods: dict[str, set[str]] = {}
    connect_proxy = False
    routes_root = repo / "alt-frontend-sv" / "src" / "routes"
    for server in routes_root.rglob("+server.ts"):
        rel = server.relative_to(routes_root)
        url = svelte_url_from_server_path(rel)
        methods = parse_svelte_methods(_read(server))
        if url.rstrip("/") == "/api/v2" or url == "/api/v2/*":
            connect_proxy = True
            svelte_methods["/api/v2/*"] = methods
            continue
        svelte_methods[url] = methods
    return RouteCatalog(
        plecto_routes=plecto,
        svelte_methods=svelte_methods,
        public_services=allowlist,
        rpcs=rpcs,
        connect_proxy=connect_proxy,
    )


def load_probes(path: pathlib.Path) -> object:
    with pathlib.Path(path).open(encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def _merged_headers(defaults: dict, probe: dict) -> dict[str, str]:
    headers: dict[str, str] = {}
    for src in (defaults.get("headers") or {}, probe.get("headers") or {}):
        for key, value in src.items():
            headers[str(key)] = str(value)
    return headers


def _header(headers: dict[str, str], name: str) -> str:
    target = name.lower()
    for key, value in headers.items():
        if key.lower() == target:
            return value
    return ""


def _body_json(probe: dict) -> dict | None:
    body = probe.get("body")
    if not isinstance(body, dict):
        return None
    payload = body.get("json")
    return payload if isinstance(payload, dict) else None


def _json_field(payload: dict, *names: str) -> object:
    for name in names:
        if name in payload:
            return payload[name]
    lower = {str(k).lower(): v for k, v in payload.items()}
    for name in names:
        key = name.lower().replace("_", "")
        for existing, value in lower.items():
            if existing.replace("_", "") == key:
                return value
    return None


def _edge_path(url: str) -> str | None:
    prefix = "{{EDGE_BASE_URL}}"
    if not url.startswith(prefix):
        return None
    path = url[len(prefix) :] or "/"
    if not path.startswith("/"):
        path = "/" + path
    return path.split("?", 1)[0]


def _as_float(value: object) -> float | None:
    if isinstance(value, bool) or value is None:
        return None
    if isinstance(value, (int, float)):
        return float(value)
    if isinstance(value, str):
        try:
            return float(value)
        except ValueError:
            return None
    return None


def _audit_secrets(node: object, path: str, violations: list[str]) -> None:
    if isinstance(node, dict):
        for key, value in node.items():
            child = f"{path}.{key}" if path else str(key)
            lowered = str(key).lower()
            if lowered in {"password", "token", "secret", "authorization", "api_key", "apikey"}:
                text = str(value)
                if not PLACEHOLDER_RE.match(text) and not PROVIDER_SECRET_RE.match(text):
                    violations.append(
                        f"secret at {child} must be a {{{{PLACEHOLDER}}}} or provider_secret:NAME"
                    )
            if lowered == "password_ref" and isinstance(value, str):
                if not PROVIDER_SECRET_RE.match(value):
                    violations.append(
                        f"{child} must be provider_secret:NAME, got {value!r}"
                    )
            _audit_secrets(value, child, violations)
        return
    if isinstance(node, list):
        for index, item in enumerate(node):
            _audit_secrets(item, f"{path}[{index}]", violations)
        return
    if isinstance(node, str):
        if node.startswith(("http://", "https://")):
            violations.append(f"{path} commits a concrete URL instead of {{{{EDGE_BASE_URL}}}}")
        if SECRETISH_RE.search(node):
            violations.append(f"secret-like value at {path}")


def audit(doc: object, catalog: RouteCatalog) -> list[str]:
    """Return human-readable violations. Empty means the spec is clean."""
    if not isinstance(doc, dict):
        return ["probes.yaml root must be a mapping"]

    violations: list[str] = []
    defaults = doc.get("defaults") if isinstance(doc.get("defaults"), dict) else {}
    paging = doc.get("paging") if isinstance(doc.get("paging"), dict) else {}
    scheduling = doc.get("scheduling") if isinstance(doc.get("scheduling"), dict) else {}

    interval_s = _as_float(defaults.get("interval_s"))
    if interval_s is None:
        violations.append("defaults.interval_s is required (>= 60s)")
    elif interval_s < MIN_INTERVAL_S:
        violations.append(
            f"defaults.interval_s must be >= {MIN_INTERVAL_S}s, got {interval_s:g}"
        )

    gap_s = _as_float(defaults.get("min_external_interval_s"))
    if gap_s is None:
        gap_s = _as_float(scheduling.get("min_gap_s"))
    if gap_s is None:
        violations.append(
            "defaults.min_external_interval_s (or scheduling.min_gap_s) is required (>= 5s)"
        )
    elif gap_s < MIN_EXTERNAL_GAP_S:
        violations.append(
            f"external probe gap must be >= {MIN_EXTERNAL_GAP_S}s, got {gap_s:g}"
        )

    timeout_ms = _as_float(defaults.get("timeout_ms"))
    if timeout_ms is None:
        violations.append("defaults.timeout_ms is required")
    elif timeout_ms <= 0:
        violations.append("defaults.timeout_ms must be > 0")

    consec = paging.get("consecutive_failures")
    consec_n = _as_float(consec)
    if consec_n is None or consec_n < 3:
        violations.append("paging.consecutive_failures must be >= 3")
    on_first = str(paging.get("on_first_failure") or "").strip().lower()
    if on_first and on_first != "observe":
        violations.append(
            "paging.on_first_failure must be observe (page on consecutive failures, not the first)"
        )

    follow = defaults.get("follow_redirects")
    if follow is True:
        violations.append(
            "follow_redirects: true forwards session cookies across hosts; set false"
        )

    cookies = defaults.get("cookies") if isinstance(defaults.get("cookies"), dict) else {}
    credentials = (
        defaults.get("credentials") if isinstance(defaults.get("credentials"), dict) else {}
    )
    if cookies.get("host_only") is not True:
        violations.append(
            "defaults.cookies.host_only must be true (session cookie host-only semantics)"
        )
    if cookies.get("forward_on_redirect") is True:
        violations.append(
            "defaults.cookies.forward_on_redirect must be false (no cross-origin credential leak)"
        )
    same_site = str(cookies.get("same_site") or "").strip().lower()
    if same_site in {"", "none"}:
        violations.append(
            "defaults.cookies.same_site must be Lax or Strict (None leaks credentials cross-origin)"
        )
    cred_mode = str(credentials.get("mode") or "").strip().lower().replace("_", "-")
    if cred_mode and cred_mode not in {"same-origin"}:
        violations.append("defaults.credentials.mode must be same-origin")

    _audit_secrets(doc, "", violations)
    _audit_heartbeat(doc, violations)

    probes = doc.get("probes")
    if not isinstance(probes, list) or not probes:
        violations.append("probes must be a non-empty list")
        return violations

    for probe in probes:
        if not isinstance(probe, dict):
            violations.append("probe entry must be a mapping")
            continue
        pid = str(probe.get("id") or "<unknown>")
        journey = str(probe.get("journey") or "")
        _audit_bff_observation(probe, pid, journey, violations)
        method = str(probe.get("method") or "GET").upper()
        url = str(probe.get("url") or "")
        if not url.startswith("{{EDGE_BASE_URL}}"):
            violations.append(f"{pid}: url must start with {{{{EDGE_BASE_URL}}}}")
        path = _edge_path(url)
        if path is None:
            violations.append(f"{pid}: could not parse edge path from {url!r}")
            continue

        probe_interval = probe.get("interval_s")
        if probe_interval is not None:
            value = _as_float(probe_interval)
            if value is None or value < MIN_INTERVAL_S:
                violations.append(f"{pid}: interval_s must be >= {MIN_INTERVAL_S}s")

        auth = probe.get("auth") if isinstance(probe.get("auth"), dict) else {}
        follow_probe = probe.get("follow_redirects", follow)
        if follow_probe is True and auth.get("type") == "session_cookie":
            violations.append(
                f"{pid}: follow_redirects true with session_cookie forwards credentials"
            )

        route = catalog.longest_plecto(path)
        if route is None:
            violations.append(f"{pid}: {path} matches no Plecto route")
            continue
        if route.prefix == "/" and route.upstream == "alt-frontend-sv":
            violations.append(
                f"{pid}: {path} is served by the Plecto SPA catch-all "
                "(HTML 200 masquerade); use /ory/ or /api/"
            )
            continue

        if route.upstream == "kratos":
            if path != KRATOS_WHOAMI or method != "GET":
                violations.append(
                    f"{pid}: login whoami must be GET {KRATOS_WHOAMI}, got {method} {path}"
                )
            if probe.get("bff_observation") is True:
                violations.append(
                    f"{pid}: login whoami is Kratos and cannot set bff_observation true"
                )
            continue

        headers = _merged_headers(defaults, probe)
        body = _body_json(probe)
        connect = CONNECT_PATH_RE.match(path)
        if connect:
            service, rpc = connect.group(1), connect.group(2)
            if not catalog.connect_proxy:
                violations.append(f"{pid}: SvelteKit /api/v2 Connect proxy is missing")
            if service not in catalog.public_services:
                violations.append(f"{pid}: {service} is not a public Connect service")
            known = catalog.rpcs.get(service, set())
            if rpc not in known:
                listed = ", ".join(sorted(known)) or "(none)"
                violations.append(
                    f"{pid}: unknown Connect RPC {service}/{rpc}; known: {listed}"
                )
            if method != "POST":
                violations.append(f"{pid}: Connect unary {rpc} must be POST, got {method}")
            if str(_header(headers, "Connect-Protocol-Version")).strip() != "1":
                violations.append(f"{pid}: Connect-Protocol-Version: 1 is required")
            content_type = _header(headers, "Content-Type").lower()
            if "application/json" not in content_type:
                violations.append(
                    f"{pid}: Connect JSON requires Content-Type: application/json"
                )
            if rpc == "SearchArticles":
                query = _json_field(body or {}, "query")
                if not isinstance(query, str) or not query.strip():
                    violations.append(f"{pid}: SearchArticles body.json.query is required")
            if rpc == "GetKnowledgeHome" and body is None:
                violations.append(f"{pid}: GetKnowledgeHome requires a Connect JSON body")
            continue

        allowed = catalog.svelte_methods.get(path)
        if allowed is None:
            violations.append(
                f"{pid}: unknown endpoint {method} {path} (no SvelteKit +server.ts)"
            )
            continue
        if method not in {item.upper() for item in allowed}:
            violations.append(f"{pid}: {path} does not allow {method}")

    return violations


def _audit_heartbeat(doc: dict, violations: list[str]) -> None:
    heartbeat = doc.get("heartbeat")
    if not isinstance(heartbeat, dict):
        violations.append(
            "heartbeat.metric alt_synthetic_probe_result is required "
            "(provider-independent per-journey heartbeat)"
        )
        return
    metric = str(heartbeat.get("metric") or "").strip()
    if metric != HEARTBEAT_METRIC:
        violations.append(
            f"heartbeat.metric must be {HEARTBEAT_METRIC}, got {metric!r}"
        )
    freshness = str(heartbeat.get("freshness") or "").strip()
    if freshness not in HEARTBEAT_FRESHNESS:
        violations.append("heartbeat.freshness must be 15m (UserJourneyNoObservation window)")
    labels = heartbeat.get("labels") if isinstance(heartbeat.get("labels"), dict) else {}
    required = labels.get("required") or []
    required_set = {str(item) for item in required} if isinstance(required, list) else set()
    if not HEARTBEAT_LABELS.issubset(required_set):
        violations.append(
            "heartbeat.labels.required must include journey and probe"
        )
    listed = heartbeat.get("bff_observation_journeys") or []
    listed_set = {str(item) for item in listed} if isinstance(listed, list) else set()
    if listed_set != BFF_OBSERVATION_JOURNEYS:
        violations.append(
            "heartbeat.bff_observation_journeys must be [feeds, search] "
            "(login whoami is Kratos and cannot increment the BFF login counter)"
        )


def _audit_bff_observation(
    probe: dict, pid: str, journey: str, violations: list[str]
) -> None:
    obs = probe.get("bff_observation")
    if obs not in (True, False):
        violations.append(f"{pid}: bff_observation must be true or false")
        return
    if journey in LOGIN_JOURNEYS and obs is True:
        violations.append(
            f"{pid}: login whoami is Kratos and cannot set bff_observation true"
        )
    if journey in BFF_OBSERVATION_JOURNEYS and obs is False:
        violations.append(
            f"{pid}: {journey} synthetic traverses BFF; bff_observation must be true"
        )
    if (
        journey
        and journey not in LOGIN_JOURNEYS
        and journey not in BFF_OBSERVATION_JOURNEYS
        and obs is True
    ):
        violations.append(
            f"{pid}: {journey} is not a BFF user-journey SLO probe; "
            "bff_observation must be false"
        )


def audit_no_observation_rules(rules_text: str) -> list[str]:
    """Pin UserJourneyNoObservation to per-journey synthetic heartbeats."""
    violations: list[str] = []
    compact = re.sub(r"\s+", "", rules_text)
    if "up{job" in compact or "up{job" in rules_text:
        violations.append(
            "UserJourneyNoObservation must not use up{job=synthetic} "
            "(that arms every journey including login)"
        )
    if "alt:synthetic:probes:activated" in rules_text:
        violations.append(
            "generic alt:synthetic:probes:activated arms every journey; "
            "use present_over_time(alt_synthetic_probe_result) per journey"
        )
    if "present_over_time(alt_synthetic_probe_result" not in compact:
        violations.append(
            "UserJourneyNoObservation must use "
            "present_over_time(alt_synthetic_probe_result...) so a failing "
            "probe (result=0) is still a fresh heartbeat"
        )
    if 'journey=~"feeds|search"' not in rules_text and 'journey!="login"' not in rules_text:
        violations.append(
            "no-observation must not select journey=login "
            "(login whoami cannot increment the BFF login counter)"
        )
    if re.search(r"and\s+on\(\s*\)", rules_text):
        violations.append(
            "no-observation must join on(journey), not on() broadcast"
        )
    return violations


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo-root", type=pathlib.Path, default=REPO_ROOT)
    parser.add_argument("--probes", type=pathlib.Path, default=None)
    args = parser.parse_args(argv)
    repo = args.repo_root.resolve()
    probes_path = args.probes or (repo / "observability" / "synthetic" / "probes.yaml")
    catalog = load_catalog(repo)
    doc = load_probes(probes_path)
    violations = audit(doc, catalog)
    if violations:
        print(f"FAIL  {probes_path} ({len(violations)} violation(s))")
        for item in violations:
            print(f"  - {item}")
        return 1
    print(f"OK    {probes_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
