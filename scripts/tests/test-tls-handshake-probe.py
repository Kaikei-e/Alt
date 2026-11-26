#!/usr/bin/env python3
"""Contract tests for the externally observed mTLS handshake certificate probe.

Pins the pieces promtool cannot see: the scrape job's target list against the
compose listeners, the join label, the prober's no-client-identity module, the
prober's compose hardening, and the fleet pin in the alert rules. A listener
added to compose without a probe target (or an explicit exclusion with a
reason) fails here, so the probe cannot silently fall behind the fleet.

Run:
    python3 scripts/tests/test-tls-handshake-probe.py
"""

from __future__ import annotations

import importlib.util
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location(
    "compose_include", ROOT / "scripts" / "compose_include.py"
)
assert _spec is not None
assert _spec.loader is not None
_compose_include = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_compose_include)
production_services = _compose_include.production_services

PROM_CONFIG = ROOT / "observability" / "prometheus" / "prometheus.yml"
RULES = ROOT / "observability" / "prometheus" / "rules" / "tls-handshake-alerts.yml"
BLACKBOX = ROOT / "observability" / "blackbox" / "blackbox.yml"

JOB = "tls-handshake"
MODULE = "mtls_server_cert"

# target -> subject (the pki_enrollment_* `subject` label the alert joins on).
EXPECTED_TARGETS = {
    "alt-data-hub:9443": "alt-data-hub",
    "auth-hub:9443": "auth-hub",
    "auth-hub:8443": "auth-hub",
    "search-indexer:9443": "search-indexer",
    "pre-processor:9443": "pre-processor",
    "tag-generator:9443": "tag-generator",
    "recap-worker:9443": "recap-worker",
    "acolyte-orchestrator:9443": "acolyte-orchestrator",
    "recap-subworker:9443": "recap-subworker",
    "news-creator:9443": "news-creator",
    "rag-orchestrator:9010": "rag-orchestrator",
    "rag-orchestrator:9011": "rag-orchestrator",
}

# Environment variables that turn on an inbound TLS listener.
LISTENER_KEYS = (
    "MTLS_LISTEN",
    "MTLS_PORT",
    "INBOUND_MTLS",
    "INBOUND_TLS_ENABLED",
    "FRONTEND_TLS_LISTEN",
    "DATAHUB_LISTEN_ADDR",
)

# What each listener reads for its certificate and key, in precedence order.
# The variable names differ per service, so this is read from each service's
# listener code; enrollment writes CERT_PATH / KEY_PATH.
LISTENER_PATHS = {
    "alt-data-hub:9443": (("DATAHUB_TLS_CERT_FILE",), ("DATAHUB_TLS_KEY_FILE",)),
    "auth-hub:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "auth-hub:8443": (("FRONTEND_TLS_CERT_FILE",), ("FRONTEND_TLS_KEY_FILE",)),
    "search-indexer:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "pre-processor:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "tag-generator:9443": (
        ("INBOUND_TLS_CERT_FILE", "MTLS_CERT_FILE"),
        ("INBOUND_TLS_KEY_FILE", "MTLS_KEY_FILE"),
    ),
    "recap-worker:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "acolyte-orchestrator:9443": (
        ("INBOUND_TLS_CERT_FILE", "MTLS_CERT_FILE"),
        ("INBOUND_TLS_KEY_FILE", "MTLS_KEY_FILE"),
    ),
    "recap-subworker:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "news-creator:9443": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "rag-orchestrator:9010": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
    "rag-orchestrator:9011": (("MTLS_CERT_FILE",), ("MTLS_KEY_FILE",)),
}


def _default_of(value: str) -> str:
    m = re.fullmatch(r"\$\{[^:}]+:-([^}]*)\}", value)
    return m.group(1) if m else value


def _opens_no_listener(svc: dict) -> bool:
    env = env_map(svc)
    return not any(k in env for k in LISTENER_KEYS)


def _closed_by_default(svc: dict) -> bool:
    return _default_of(env_map(svc).get("MTLS_LISTEN", "true")).lower() == "false"


def _profile_gated(svc: dict) -> bool:
    return bool(svc.get("profiles"))


# Every compose service that enrolls a leaf (CERT_SUBJECT) and is not probed
# must be named here with its reason and the compose fact that makes it true.
UNPROBED_SUBJECTS = {
    "alt-backend": (
        "cmd/backend opens no TLS listener; the data plane is alt-data-hub",
        _opens_no_listener,
    ),
    "alt-harvester": ("outbound mTLS client only", _opens_no_listener),
    "alt-notifier": ("outbound mTLS client only", _opens_no_listener),
    "recap-evaluator": ("outbound mTLS client only", _opens_no_listener),
    "knowledge-sovereign": (
        "bearer-token plaintext on :9500; the leaf is client-side",
        _opens_no_listener,
    ),
    "alt-butterfly-facade": (
        "inbound mTLS listener is closed by default (MTLS_LISTEN)",
        _closed_by_default,
    ),
    "tts-speaker": ("opt-in compose profile, not part of the default stack", _profile_gated),
}

_counts = {"passed": 0, "failed": 0}


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        _counts["passed"] += 1
        return
    sys.stdout.write(f"  FAIL  {name}\n")
    if detail:
        sys.stdout.write(f"        {detail}\n")
    _counts["failed"] += 1


def load(path: Path) -> dict:
    try:
        doc = yaml.safe_load(path.read_text(encoding="utf-8"))
    except OSError as exc:
        sys.stdout.write(f"  FAIL  cannot read {path.relative_to(ROOT)}: {exc}\n")
        _counts["failed"] += 1
        return {}
    return doc if isinstance(doc, dict) else {}


def scrape_job(prom: dict, name: str) -> dict:
    for job in prom.get("scrape_configs") or []:
        if job.get("job_name") == name:
            return job
    return {}


def env_map(svc: dict) -> dict[str, str]:
    env = svc.get("environment") or {}
    if isinstance(env, dict):
        return {str(k): str(v) for k, v in env.items()}
    out = {}
    for item in env:
        key, _, value = str(item).partition("=")
        out[key] = value
    return out


def parse_duration_s(value: str) -> float:
    m = re.fullmatch(r"(\d+)(ms|s|m|h)", str(value))
    if not m:
        return float("nan")
    return int(m.group(1)) * {"ms": 0.001, "s": 1, "m": 60, "h": 3600}[m.group(2)]


sys.stdout.write("scrape job\n")
prom = load(PROM_CONFIG)
job = scrape_job(prom, JOB)
check("prometheus.yml has the tls-handshake job", bool(job))
check("job probes through /probe", job.get("metrics_path") == "/probe")
check(
    "job selects the client-identity-free module",
    (job.get("params") or {}).get("module") == [MODULE],
)

targets: dict[str, str] = {}
for sc in job.get("static_configs") or []:
    subject = (sc.get("labels") or {}).get("subject", "")
    for t in sc.get("targets") or []:
        targets[t] = subject
check(
    "targets are exactly the TLS listeners",
    targets.keys() == EXPECTED_TARGETS.keys(),
    f"missing={sorted(EXPECTED_TARGETS.keys() - targets.keys())} "
    f"extra={sorted(targets.keys() - EXPECTED_TARGETS.keys())}",
)
check(
    "every target carries the pki_enrollment subject as its join label",
    targets == EXPECTED_TARGETS,
)

relabels = job.get("relabel_configs") or []
check(
    "relabeling moves the listener into ?target= and keeps it as instance",
    any(r.get("target_label") == "__param_target" for r in relabels)
    and any(r.get("target_label") == "instance" for r in relabels),
)
check(
    "relabeling sends the scrape to blackbox-exporter:9115",
    any(
        r.get("target_label") == "__address__" and r.get("replacement") == "blackbox-exporter:9115"
        for r in relabels
    ),
)
check("scrape interval is 60s", job.get("scrape_interval") == "60s")
own = scrape_job(prom, "blackbox-exporter")
check(
    "the prober's own /metrics is scraped",
    (own.get("static_configs") or [{}])[0].get("targets") == ["blackbox-exporter:9115"],
)

sys.stdout.write("listener inventory vs compose\n")
services = production_services()
for target, subject in EXPECTED_TARGETS.items():
    host = target.split(":")[0]
    svc = services.get(host)
    check(
        f"{target}: compose service exists and enrolls subject {subject}",
        svc is not None and env_map(svc).get("CERT_SUBJECT") == subject,
    )
    check(
        f"{target}: compose service is not profile-gated",
        svc is not None and not svc.get("profiles"),
    )

enrolled = {env_map(svc).get("CERT_SUBJECT") for svc in services.values()} - {None}
probed_subjects = set(EXPECTED_TARGETS.values())
unaccounted = enrolled - probed_subjects - UNPROBED_SUBJECTS.keys()
check(
    "every enrolling service is probed or excluded with a reason",
    not unaccounted,
    f"unaccounted subjects: {sorted(unaccounted)}",
)
stale = (probed_subjects | UNPROBED_SUBJECTS.keys()) - enrolled
check("no stale subject in the inventory", not stale, f"stale: {sorted(stale)}")

by_subject = {env_map(svc).get("CERT_SUBJECT"): svc for svc in services.values()}
for subject, (reason, holds) in UNPROBED_SUBJECTS.items():
    svc = by_subject.get(subject)
    check(f"{subject}: exclusion holds in compose ({reason})", svc is not None and holds(svc))

check(
    "every probe target has a listener path entry", LISTENER_PATHS.keys() == EXPECTED_TARGETS.keys()
)
for target, (cert_vars, key_vars) in LISTENER_PATHS.items():
    env = env_map(services.get(target.split(":")[0]) or {})
    for kind, names, enrolled_var in (
        ("cert", cert_vars, "CERT_PATH"),
        ("key", key_vars, "KEY_PATH"),
    ):
        effective = next((env[n] for n in names if env.get(n)), None)
        check(
            f"{target}: listener {kind} path ({'/'.join(names)}) equals {enrolled_var}",
            effective is not None and effective == env.get(enrolled_var),
            f"listener={effective!r} enrollment={env.get(enrolled_var)!r}",
        )

sys.stdout.write("prober module\n")
module = (load(BLACKBOX).get("modules") or {}).get(MODULE) or {}
tcp = module.get("tcp") or {}
tls_cfg = tcp.get("tls_config") or {}
check(
    "module uses the tcp prober with TLS", module.get("prober") == "tcp" and tcp.get("tls") is True
)
check(
    "module verifies against the published trust bundle",
    tls_cfg.get("ca_file") == "/trust/ca-bundle.pem",
)
check("module never skips verification", not tls_cfg.get("insecure_skip_verify"))
check(
    "module carries no client certificate or key",
    not any(k in tls_cfg for k in ("cert_file", "key_file", "cert", "key")),
)
check(
    "module timeout leaves room inside the scrape timeout",
    parse_duration_s(module.get("timeout", "")) < parse_duration_s(job.get("scrape_timeout", "0s")),
)

sys.stdout.write("prober compose service\n")
bb = services.get("blackbox-exporter") or {}
image = str(bb.get("image", ""))
check(
    "image is pinned to an exact version",
    re.fullmatch(r"prom/blackbox-exporter:v\d+\.\d+\.\d+", image) is not None,
    image,
)
check("all capabilities dropped", bb.get("cap_drop") == ["ALL"])
check("no-new-privileges", "no-new-privileges:true" in (bb.get("security_opt") or []))
check("read-only root filesystem", bb.get("read_only") is True)
check("runs as a non-root user", str(bb.get("user", "")).split(":")[0] not in ("", "0", "root"))
check("memory limit set", bb.get("mem_limit") is not None)
hc = " ".join(str(x) for x in (bb.get("healthcheck") or {}).get("test", []))
check(
    "healthcheck fails while the trust bundle is missing",
    "/trust/ca-bundle.pem" in hc and "9115" in hc,
    hc,
)
check("publishes no host port", not bb.get("ports"))
check("sits on alt-network", "alt-network" in (bb.get("networks") or []))
volumes = [str(v) for v in bb.get("volumes") or []]
check(
    "mounts only the public trust bundle, read-only",
    volumes == ["pki_trust_bundle:/trust:ro"],
    str(volumes),
)

sys.stdout.write("alert rules\n")
rules_doc = load(RULES)
exprs: dict[str, str] = {}
sev: dict[str, str] = {}
for group in rules_doc.get("groups") or []:
    for rule in group.get("rules") or []:
        exprs[rule["alert"]] = " ".join(str(rule["expr"]).split())
        sev[rule["alert"]] = (rule.get("labels") or {}).get("severity", "")
EXPECTED_SEVERITY = {
    "TlsHandshakeCertExpirySoon": "page",
    "TlsHandshakeProbeFailing": "page",
    "TlsHandshakeCertDivergesFromReported": "ticket",
    "TlsHandshakeProbeMassFailure": "ticket",
    "TlsHandshakeProberDown": "ticket",
    "TlsHandshakeProbeScrapeFailing": "ticket",
    "TlsHandshakeProbeFleetIncomplete": "ticket",
    "TlsHandshakeProbeNoExpiry": "ticket",
    "TlsHandshakeReportedCertMissing": "ticket",
}
check("all expected alerts exist", exprs.keys() == EXPECTED_SEVERITY.keys(), str(sorted(exprs)))
check("severities match the decision table", sev == EXPECTED_SEVERITY, str(sev))
pin = re.search(r"!=\s*(\d+)\s*$", exprs.get("TlsHandshakeProbeFleetIncomplete", ""))
check(
    "fleet pin equals the number of probe targets",
    pin is not None and int(pin.group(1)) == len(EXPECTED_TARGETS),
    exprs.get("TlsHandshakeProbeFleetIncomplete", ""),
)
check(
    "divergence joins on the subject label",
    "on(subject)" in exprs.get("TlsHandshakeCertDivergesFromReported", "").replace(" ", ""),
)

sys.stdout.write(f"\n{_counts['passed']} passed, {_counts['failed']} failed\n")
sys.exit(1 if _counts["failed"] else 0)
