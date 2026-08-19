#!/usr/bin/env python3
"""Tests for scripts/observability-config-audit.py file attachments.

Wave 1 moved prometheus.yml / alertmanager.yml / grafana.ini from short-syntax
bind mounts onto Compose `configs:`. The structural audit must treat a
configs: attachment as a real source→target mapping and still fail when the
source name, file: path, or container target is wrong — not merely when the
service exists.

Run:
    python3 scripts/tests/test-observability-config-audit.py
"""

from __future__ import annotations

import copy
import importlib.util
import os
import pathlib
import sys
import tempfile
import textwrap

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location(
    "obs_audit", SCRIPTS / "observability-config-audit.py"
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


def reset_audit():
    audit.errors.clear()
    audit.warnings.clear()


def write_tree(host: pathlib.Path, compose_body: str, *, am: bool = True, prom: bool = True) -> None:
    (host / "compose").mkdir(parents=True, exist_ok=True)
    (host / "observability" / "alertmanager").mkdir(parents=True, exist_ok=True)
    (host / "observability" / "prometheus").mkdir(parents=True, exist_ok=True)
    (host / "observability" / "grafana").mkdir(parents=True, exist_ok=True)
    (host / "compose" / "observability.yaml").write_text(
        textwrap.dedent(compose_body), encoding="utf-8"
    )
    if am:
        (host / "observability" / "alertmanager" / "alertmanager.yml").write_text(
            "route:\n  receiver: null\nreceivers:\n  - name: null\n",
            encoding="utf-8",
        )
    if prom:
        (host / "observability" / "prometheus" / "prometheus.yml").write_text(
            textwrap.dedent(
                """\
                alerting:
                  alertmanagers:
                    - static_configs:
                        - targets: ["alertmanager:9093"]
                """
            ),
            encoding="utf-8",
        )


CONFIGS_COMPOSE = """\
services:
  alertmanager:
    image: prom/alertmanager:v0.28.1
    volumes:
      - alertmanager_data:/alertmanager
    configs:
      - source: alertmanager_yml
        target: /etc/alertmanager/alertmanager.yml
configs:
  alertmanager_yml:
    file: ../observability/alertmanager/alertmanager.yml
volumes:
  alertmanager_data:
"""

BIND_COMPOSE = """\
services:
  alertmanager:
    image: prom/alertmanager:v0.28.1
    volumes:
      - ../observability/alertmanager/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro
"""

print("parse_file_attachments")

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(host, CONFIGS_COMPOSE)
    compose = str(host / "compose" / "observability.yaml")
    mounts = audit.parse_mounts(compose)
    am = mounts.get("alertmanager") or []
    check(
        "configs: attachment is collected as a host→container mapping",
        any(
            pathlib.Path(src).name == "alertmanager.yml"
            and dest == "/etc/alertmanager/alertmanager.yml"
            for src, dest in am
        ),
    )
    check(
        "named volume is not treated as a file attachment",
        all("alertmanager_data" not in src for src, _ in am),
    )
    host_cfg = audit.container_to_host("/etc/alertmanager/alertmanager.yml", am)
    check(
        "container_to_host resolves configs: target to the file: source",
        host_cfg is not None
        and os.path.abspath(host_cfg)
        == os.path.abspath(str(host / "observability/alertmanager/alertmanager.yml")),
    )

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(host, BIND_COMPOSE)
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    host_cfg = audit.container_to_host(
        "/etc/alertmanager/alertmanager.yml", mounts.get("alertmanager") or []
    )
    check(
        "short-syntax bind mount is still a valid attachment",
        host_cfg is not None
        and os.path.abspath(host_cfg)
        == os.path.abspath(str(host / "observability/alertmanager/alertmanager.yml")),
    )

print("audit_alertmanager")

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(host, CONFIGS_COMPOSE)
    reset_audit()
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    present = audit.audit_alertmanager(str(host), mounts)
    check("configs: with matching source and target is clean", present and not audit.errors)

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(
        host,
        """\
        services:
          alertmanager:
            configs:
              - source: alertmanager_yml
                target: /etc/alertmanager/wrong.yml
        configs:
          alertmanager_yml:
            file: ../observability/alertmanager/alertmanager.yml
        """,
    )
    reset_audit()
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    audit.audit_alertmanager(str(host), mounts)
    check(
        "wrong configs: target is a violation",
        any("alertmanager.yml" in e and "/etc/alertmanager/alertmanager.yml" in e for e in audit.errors),
    )

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(
        host,
        """\
        services:
          alertmanager:
            configs:
              - source: alertmanager_yml
                target: /etc/alertmanager/alertmanager.yml
        configs:
          alertmanager_yml:
            file: ../observability/alertmanager/other.yml
        """,
    )
    (host / "observability" / "alertmanager" / "other.yml").write_text("x\n", encoding="utf-8")
    reset_audit()
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    audit.audit_alertmanager(str(host), mounts)
    check(
        "configs: file: pointing at a different path is a violation",
        any("alertmanager.yml" in e for e in audit.errors),
    )

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(
        host,
        """\
        services:
          alertmanager:
            volumes:
              - alertmanager_data:/alertmanager
        volumes:
          alertmanager_data:
        """,
    )
    reset_audit()
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    audit.audit_alertmanager(str(host), mounts)
    check(
        "service without configs: or bind of the yml is a violation",
        any("alertmanager.yml" in e for e in audit.errors),
    )

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    write_tree(host, BIND_COMPOSE)
    reset_audit()
    mounts = audit.parse_mounts(str(host / "compose" / "observability.yaml"))
    present = audit.audit_alertmanager(str(host), mounts)
    check("legacy short-syntax bind of the yml is still clean", present and not audit.errors)

print("production")
reset_audit()
prod_compose = str(ROOT / "compose" / "observability.yaml")
prod_mounts = audit.parse_mounts(prod_compose)
am_host = audit.container_to_host(
    "/etc/alertmanager/alertmanager.yml", prod_mounts.get("alertmanager") or []
)
prom_host = audit.container_to_host(
    "/etc/prometheus/prometheus.yml", prod_mounts.get("prometheus") or []
)
grafana_host = audit.container_to_host(
    "/etc/grafana/grafana.ini", prod_mounts.get("grafana") or []
)
check(
    "production alertmanager attaches observability/alertmanager/alertmanager.yml",
    am_host is not None
    and os.path.abspath(am_host)
    == os.path.abspath(str(ROOT / "observability/alertmanager/alertmanager.yml")),
)
check(
    "production prometheus attaches observability/prometheus/prometheus.yml",
    prom_host is not None
    and os.path.abspath(prom_host)
    == os.path.abspath(str(ROOT / "observability/prometheus/prometheus.yml")),
)
check(
    "production grafana attaches observability/grafana/grafana.ini",
    grafana_host is not None
    and os.path.abspath(grafana_host)
    == os.path.abspath(str(ROOT / "observability/grafana/grafana.ini")),
)
reset_audit()
audit.audit_alertmanager(str(ROOT), prod_mounts)
check("production audit_alertmanager is clean", not audit.errors)

print("pki ops YAML surface")

prom = audit.load_yaml(str(ROOT / "observability/prometheus/prometheus.yml"))
rules = audit.load_yaml(str(ROOT / "observability/prometheus/rules/pki-agent-alerts.yml"))
check(
    "production PKI ops surface is clean",
    audit.audit_pki_ops_surface(prom, rules) == [],
)

omitted = copy.deepcopy(prom)
removed = False
for job in omitted.get("scrape_configs") or []:
    for block in job.get("static_configs") or []:
        targets = block.get("targets") or []
        if "news-creator:9110" in targets:
            targets.remove("news-creator:9110")
            removed = True
omit_v = audit.audit_pki_ops_surface(omitted, rules)
check("omitting one :9110 target is a violation", removed)
check(
    "omitting news-creator:9110 is detected",
    any("news-creator:9110" in v for v in omit_v),
)

sidecar = copy.deepcopy(prom)
scrape = sidecar.setdefault("scrape_configs", [])
scrape.append(
    {
        "job_name": "pki-agent",
        "metrics_path": "/metrics",
        "static_configs": [{"targets": ["pki-agent-news-creator:9510"]}],
    }
)
sidecar_v = audit.audit_pki_ops_surface(sidecar, rules)
check(
    "a leftover pki-agent scrape job is detected",
    any("pki-agent" in v for v in sidecar_v),
)

wrong_path = copy.deepcopy(prom)
for job in wrong_path.get("scrape_configs") or []:
    for block in job.get("static_configs") or []:
        if "auth-hub:9110" in (block.get("targets") or []):
            job["metrics_path"] = "/metrics/prometheus"
path_v = audit.audit_pki_ops_surface(wrong_path, rules)
check(
    "wrong metrics_path on a :9110 job is detected",
    any("metrics_path" in v for v in path_v),
)

no_absent = copy.deepcopy(rules)
for group in no_absent.get("groups") or []:
    for rule in group.get("rules") or []:
        if rule.get("alert") == "PkiEnrollmentWorkloadMetricsAbsent":
            rule["expr"] = 'absent(pki_enrollment_healthy{subject="alt-backend"})'
absent_v = audit.audit_pki_ops_surface(prom, no_absent)
check(
    "dropping an absent() subject is detected",
    any("missing=" in v and "news-creator" in v for v in absent_v),
)

comment_cfg = {
    "scrape_configs": [
        {
            "job_name": "prometheus",
            "static_configs": [{"targets": ["prometheus:9090"]}],
        }
    ]
}
comment_v = audit.audit_pki_ops_surface(comment_cfg, rules)
check(
    "a scrape graph without :9110 jobs cannot satisfy the 14-parent pin",
    any("missing alt-backend:9110" in v for v in comment_v),
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
