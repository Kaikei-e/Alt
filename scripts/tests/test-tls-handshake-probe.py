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

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

from compose_include import production_services  # noqa: E402 - needs the sys.path entry above

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

# Every compose service that enrolls a leaf (CERT_SUBJECT) and is not probed
# must be named here with the reason it has no TLS server listener to probe.
UNPROBED_SUBJECTS = {
    "alt-backend": "cmd/backend opens no TLS listener; the data plane is alt-data-hub",
    "alt-harvester": "outbound mTLS client only",
    "alt-notifier": "outbound mTLS client only",
    "recap-evaluator": "outbound mTLS client only",
    "knowledge-sovereign": "serves bearer-token plaintext on :9500; the leaf is client-side",
    "alt-butterfly-facade": "inbound mTLS listener is closed by default (MTLS_LISTEN=false)",
    "tts-speaker": "opt-in compose profile `tts`, not part of the default stack",
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
check("job selects the client-identity-free module", (job.get("params") or {}).get("module") == [MODULE])

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

sys.stdout.write("prober module\n")
module = (load(BLACKBOX).get("modules") or {}).get(MODULE) or {}
tcp = module.get("tcp") or {}
tls_cfg = tcp.get("tls_config") or {}
check("module uses the tcp prober with TLS", module.get("prober") == "tcp" and tcp.get("tls") is True)
check("module verifies against the published trust bundle", tls_cfg.get("ca_file") == "/trust/ca-bundle.pem")
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
check("has a healthcheck", bool(bb.get("healthcheck")))
check("publishes no host port", not bb.get("ports"))
check("sits on alt-network", "alt-network" in (bb.get("networks") or []))
volumes = [str(v) for v in bb.get("volumes") or []]
check(
    "mounts only the public trust bundle, read-only",
    volumes == ["pki_trust_bundle:/trust:ro"],
    str(volumes),
)
check(
    "waits for the trust bundle to be published",
    "step-ca-bootstrap" in (bb.get("depends_on") or {}),
)

sys.stdout.write("alert rules\n")
rules_doc = load(RULES)
exprs: dict[str, str] = {}
sev: dict[str, str] = {}
for group in rules_doc.get("groups") or []:
    for rule in group.get("rules") or []:
        exprs[rule["alert"]] = " ".join(str(rule["expr"]).split())
        sev[rule["alert"]] = (rule.get("labels") or {}).get("severity", "")
EXPECTED_ALERTS = {
    "TlsHandshakeCertDivergesFromReported",
    "TlsHandshakeCertExpirySoon",
    "TlsHandshakeProbeFailing",
    "TlsHandshakeProbeScrapeDown",
    "TlsHandshakeProbeFleetIncomplete",
    "TlsHandshakeProbeNoExpiry",
    "TlsHandshakeReportedCertMissing",
}
check("all expected alerts exist", exprs.keys() == EXPECTED_ALERTS, str(sorted(exprs)))
check("every alert pages", all(v == "page" for v in sev.values()), str(sev))
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
