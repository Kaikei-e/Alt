#!/usr/bin/env python3
"""Structural audit of the observability configuration tree.

promtool and amtool validate a file against its own schema. They cannot see
the gap between a file and the container that mounts it, and that gap is
where observability config dies quietly:

  1. A rule file the `rule_files` glob will never match. It is valid YAML,
     `promtool check rules` is green, and it is dead on arrival in production.
  2. A promtool test file that landed *inside* the rules glob. That is a fatal
     config error at load time, not a skipped file — it takes every rule with
     it.
  3. A Grafana provisioning file that is not valid YAML or has no
     `apiVersion`. Grafana aborts the provisioning pass on the first bad file,
     so one typo removes every dashboard and every provisioned alert rule.
  4. A dashboard JSON that does not parse, or two dashboards sharing a `uid`
     (the second silently replaces the first, and the count looks fine).
  5. An alertmanager.yml that compose does not attach (bind mount or
     `configs:`), or whose source/target does not match the file on disk.
     Alertmanager then starts on its built-in default config, which routes
     every alert to a receiver that does nothing. The same source/target
     check applies to prometheus.yml and grafana.ini.

Exits 1 on any violation. Warnings do not fail the run. The trailing summary
prints the object counts, so any alert threshold that encodes them (e.g. an
expected rule-group count) can be maintained from this output.

Usage:
    python3 scripts/observability-config-audit.py [--repo-root PATH]
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

import yaml

SHORT_VOL_PARTS_MIN = 2

COMPOSE_FILE = "compose/observability.yaml"
PROMETHEUS_CONFIG = "observability/prometheus/prometheus.yml"
PROMETHEUS_RULES_DIR = "observability/prometheus/rules"
PROMETHEUS_TESTS_DIR = "observability/prometheus/tests"
ALERTMANAGER_CONFIG = "observability/alertmanager/alertmanager.yml"
GRAFANA_PROVISIONING_DIR = "observability/grafana/provisioning"
GRAFANA_DASHBOARDS_DIR = "observability/grafana/dashboards"
GRAFANA_INI = "observability/grafana/grafana.ini"

errors: list[str] = []
warnings: list[str] = []


def fail(msg: str) -> None:
    errors.append(msg)


def warn(msg: str) -> None:
    warnings.append(msg)


def load_yaml(path: str | Path) -> object:
    with Path(path).open(encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def _parse_compose_configs(cfg: dict, compose_dir: Path) -> dict[str, str]:
    config_files: dict[str, str] = {}
    for name, spec in (cfg.get("configs") or {}).items():
        if not isinstance(spec, dict):
            continue
        file_path = spec.get("file")
        if isinstance(file_path, str) and file_path:
            config_files[name] = os.path.normpath(compose_dir / file_path)
    return config_files


def _parse_service_mounts(
    svc: dict, compose_dir: Path, config_files: dict[str, str]
) -> list[tuple[str, str]]:
    entries: list[tuple[str, str]] = []
    for vol in svc.get("volumes") or []:
        if not isinstance(vol, str):
            continue
        parts = vol.split(":")
        if len(parts) < SHORT_VOL_PARTS_MIN:
            continue
        source, target = parts[0], parts[1]
        # Named volumes have no path separator and no leading dot.
        if not source.startswith((".", "/")):
            continue
        entries.append((os.path.normpath(compose_dir / source), target))
    for item in svc.get("configs") or []:
        if isinstance(item, str):
            source_name, target = item, f"/{item}"
        elif isinstance(item, dict):
            source_name = item.get("source")
            if not isinstance(source_name, str) or not source_name:
                continue
            raw_target = item.get("target")
            target = raw_target if isinstance(raw_target, str) and raw_target else f"/{source_name}"
        else:
            continue
        host = config_files.get(source_name)
        if host is not None:
            entries.append((host, target))
    return entries


def parse_mounts(compose_path: str | Path) -> dict[str, list[tuple[str, str]]]:
    """Map each service to its [(host_abs_path, container_path)] file attachments.

    Covers short-syntax bind mounts and Compose `configs:` with `file:`.
    Paths in a compose file are relative to the file's own directory, which is
    why the deploy pipeline can hand the same tree to two different working
    directories and get two different mount sources.
    """
    compose_dir = Path(os.path.normpath(Path(compose_path).absolute())).parent
    cfg = load_yaml(compose_path) or {}
    config_files = _parse_compose_configs(cfg, compose_dir)
    return {
        name: _parse_service_mounts(svc, compose_dir, config_files)
        for name, svc in (cfg.get("services") or {}).items()
        if isinstance(svc, dict)
    }


def container_to_host(container_path: str, service_mounts: list[tuple[str, str]]) -> str | None:
    """Translate a container path to its host path via the longest mount match."""
    best: tuple[int, str] | None = None
    for host, raw_target in service_mounts:
        target = raw_target.rstrip("/") or "/"
        if container_path == target:
            candidate = host
        elif container_path.startswith(target + "/"):
            candidate = str(Path(host) / container_path[len(target) + 1 :])
        else:
            continue
        if best is None or len(target) > best[0]:
            best = (len(target), candidate)
    return best[1] if best else None


def audit_attached_file(
    service: str,
    container_path: str,
    expected_rel: str,
    mounts: dict[str, list[tuple[str, str]]],
    root: str,
) -> None:
    """Fail unless `service` attaches `expected_rel` exactly at `container_path`."""
    expected = Path(root) / expected_rel
    host_cfg = container_to_host(container_path, mounts.get(service, []))
    if host_cfg is None or os.path.normpath(Path(host_cfg).absolute()) != os.path.normpath(
        expected.absolute()
    ):
        fail(
            f"{COMPOSE_FILE}: {service} does not attach {expected_rel} "
            f"at {container_path} (bind mount or configs:)"
        )


def _glob_path_pattern(pattern: str) -> list[Path]:
    p = Path(pattern)
    if p.is_absolute():
        anchor = Path(p.anchor)
        return list(anchor.glob(str(p.relative_to(anchor))))
    return list(Path().glob(pattern))


def audit_prometheus_rule_glob(root: str, mounts: dict[str, list[tuple[str, str]]]) -> None:
    prom_mounts = mounts.get("prometheus", [])
    if not prom_mounts:
        fail(f"{COMPOSE_FILE}: service 'prometheus' has no bind mounts")
        return

    cfg = load_yaml(Path(root) / PROMETHEUS_CONFIG) or {}
    rule_globs = cfg.get("rule_files") or []
    if not rule_globs:
        fail(f"{PROMETHEUS_CONFIG}: no rule_files configured — no rule ever loads")
        return

    matched: set[str] = set()
    for pattern in rule_globs:
        host_pattern = container_to_host(pattern, prom_mounts)
        if host_pattern is None:
            fail(
                f"{PROMETHEUS_CONFIG}: rule_files entry {pattern!r} is not covered by "
                f"any bind mount of the prometheus service in {COMPOSE_FILE} — the "
                f"glob matches nothing inside the container and every rule in it is "
                f"silently absent"
            )
            continue
        hits = _glob_path_pattern(host_pattern)
        if not hits:
            fail(
                f"{PROMETHEUS_CONFIG}: rule_files entry {pattern!r} resolves to host "
                f"pattern {os.path.relpath(host_pattern, root)!r} which matches no file"
            )
        matched.update(os.path.normpath(h.absolute()) for h in hits)

    rules_dir = Path(root) / PROMETHEUS_RULES_DIR
    on_disk = {
        os.path.normpath(p.absolute()) for ext in ("*.yml", "*.yaml") for p in rules_dir.glob(ext)
    }
    for orphan in sorted(on_disk - matched):
        fail(
            f"{os.path.relpath(orphan, root)}: present in {PROMETHEUS_RULES_DIR}/ but "
            f"not matched by any rule_files glob — it will never be evaluated"
        )

    tests_dir = os.path.normpath((Path(root) / PROMETHEUS_TESTS_DIR).absolute())
    for leaked in sorted(m for m in matched if m.startswith(tests_dir + os.sep)):
        fail(
            f"{os.path.relpath(leaked, root)}: a promtool test file is matched by a "
            f"rule_files glob. Prometheus parses it as a rule group and refuses the "
            f"whole config — every rule stops loading, not just this one"
        )


def audit_alertmanager(root: str, mounts: dict[str, list[tuple[str, str]]]) -> bool:
    config_path = Path(root) / ALERTMANAGER_CONFIG
    declared = "alertmanager" in mounts
    exists = config_path.is_file()

    if not exists and not declared:
        warn(
            f"{ALERTMANAGER_CONFIG} absent and no alertmanager service in "
            f"{COMPOSE_FILE} — every `severity: page` rule evaluates and notifies "
            f"nobody"
        )
        return False

    if exists and not declared:
        fail(
            f"{ALERTMANAGER_CONFIG} exists but {COMPOSE_FILE} declares no "
            f"alertmanager service to consume it"
        )
        return exists

    if declared and not exists:
        fail(
            f"{COMPOSE_FILE} declares an alertmanager service but "
            f"{ALERTMANAGER_CONFIG} is missing — the container starts on the "
            f"built-in default config, whose receiver drops every notification"
        )
        return False

    audit_attached_file(
        "alertmanager",
        "/etc/alertmanager/alertmanager.yml",
        ALERTMANAGER_CONFIG,
        mounts,
        root,
    )

    prom_cfg = load_yaml(Path(root) / PROMETHEUS_CONFIG) or {}
    targets = [
        target
        for am in (prom_cfg.get("alerting") or {}).get("alertmanagers") or []
        for sc in am.get("static_configs") or []
        for target in sc.get("targets") or []
    ]
    if not targets:
        fail(
            f"{PROMETHEUS_CONFIG}: an alertmanager is deployed but no `alerting:` "
            f"block points at it — rules fire into the Prometheus UI and nowhere else"
        )
    return True


def _collect_grafana_provider_paths(root: str, provisioning_files: list[Path]) -> list[str]:
    dashboard_provider_paths: list[str] = []
    for path in provisioning_files:
        rel = os.path.relpath(path, root)
        try:
            doc = load_yaml(path)
        except yaml.YAMLError as exc:
            fail(
                f"{rel}: not valid YAML ({exc.__class__.__name__}). Grafana aborts the "
                f"whole provisioning pass on this file, taking every dashboard and "
                f"provisioned alert rule with it"
            )
            continue
        if not isinstance(doc, dict):
            fail(f"{rel}: provisioning file must be a YAML mapping")
            continue
        if "apiVersion" not in doc:
            fail(f"{rel}: missing `apiVersion` — Grafana rejects the file")
        for provider in doc.get("providers") or []:
            path_opt = (provider.get("options") or {}).get("path")
            if path_opt:
                dashboard_provider_paths.append(path_opt)
    return dashboard_provider_paths


def _check_grafana_provider_mounts(
    root: str, provider_paths: list[str], grafana_mounts: list[tuple[str, str]]
) -> None:
    for container_path in provider_paths:
        host_path = container_to_host(container_path, grafana_mounts)
        if host_path is None:
            fail(
                f"{GRAFANA_PROVISIONING_DIR}/: dashboard provider path "
                f"{container_path!r} is not bind-mounted by the grafana service in "
                f"{COMPOSE_FILE} — the provider loads zero dashboards"
            )
        elif not Path(host_path).is_dir():
            fail(
                f"{GRAFANA_PROVISIONING_DIR}/: dashboard provider path "
                f"{container_path!r} maps to {os.path.relpath(host_path, root)!r}, "
                f"which is not a directory"
            )


def audit_grafana(root: str, mounts: dict[str, list[tuple[str, str]]]) -> tuple[int, int]:
    provisioning_root = Path(root) / GRAFANA_PROVISIONING_DIR
    provisioning_files = sorted(
        p for ext in ("*.yaml", "*.yml") for p in provisioning_root.rglob(ext)
    )
    if not provisioning_files:
        fail(f"{GRAFANA_PROVISIONING_DIR}/: no provisioning files found")

    provider_paths = _collect_grafana_provider_paths(root, provisioning_files)
    _check_grafana_provider_mounts(root, provider_paths, mounts.get("grafana", []))

    dashboards_dir = Path(root) / GRAFANA_DASHBOARDS_DIR
    dashboards = sorted(dashboards_dir.glob("*.json"))
    uids: dict[str, str] = {}
    for path in dashboards:
        rel = os.path.relpath(path, root)
        try:
            with path.open(encoding="utf-8") as handle:
                doc = json.load(handle)
        except json.JSONDecodeError as exc:
            fail(f"{rel}: not valid JSON (line {exc.lineno}, col {exc.colno}: {exc.msg})")
            continue
        uid = doc.get("uid")
        if not uid:
            warn(f"{rel}: no `uid` — Grafana assigns a random one on every provision")
            continue
        if uid in uids:
            fail(
                f"{rel}: dashboard uid {uid!r} already used by {uids[uid]} — the "
                f"second provisioned dashboard replaces the first"
            )
        else:
            uids[uid] = rel

    stray = sorted(
        os.path.relpath(p, root) for p in (Path(root) / "observability/grafana").glob("*.json")
    )
    for path in stray:
        warn(
            f"{path}: dashboard JSON outside {GRAFANA_DASHBOARDS_DIR}/ — no "
            f"provisioning provider serves it, so it is never loaded"
        )

    return len(provisioning_files), len(dashboards)


def count_prometheus_rules(root: str) -> tuple[int, int, list[str]]:
    groups = 0
    rules = 0
    names: list[str] = []
    for ext in ("*.yml", "*.yaml"):
        for path in sorted((Path(root) / PROMETHEUS_RULES_DIR).glob(ext)):
            doc = load_yaml(path) or {}
            for group in doc.get("groups") or []:
                groups += 1
                names.append(group.get("name", "<unnamed>"))
                rules += len(group.get("rules") or [])
    return groups, rules, names


PKI_INPROCESS_PARENTS = (
    "alt-backend",
    "alt-harvester",
    "alt-data-hub",
    "alt-notifier",
    "alt-butterfly-facade",
    "auth-hub",
    "search-indexer",
    "rag-orchestrator",
    "pre-processor",
    "tag-generator",
    "recap-worker",
    "acolyte-orchestrator",
    "recap-subworker",
    "news-creator",
    "knowledge-sovereign",
    "recap-evaluator",
)

# job_name of the dedicated :9110 scrape. Distinct from any existing app-port job.
PKI_OPS_JOBS = {
    "alt-backend": "alt-backend",
    "alt-harvester": "alt-harvester",
    "alt-data-hub": "alt-data-hub",
    "alt-notifier": "alt-notifier",
    "alt-butterfly-facade": "alt-butterfly-facade-ops",
    "auth-hub": "auth-hub",
    "search-indexer": "search-indexer",
    "rag-orchestrator": "rag-orchestrator-ops",
    "pre-processor": "pre-processor-ops",
    "tag-generator": "tag-generator",
    "recap-worker": "recap-worker-ops",
    "acolyte-orchestrator": "acolyte-orchestrator",
    "recap-subworker": "recap-subworker-ops",
    "news-creator": "news-creator-ops",
    "knowledge-sovereign": "knowledge-sovereign-ops",
    "recap-evaluator": "recap-evaluator",
}

ABSENT_SUBJECT_RE = re.compile(
    r"absent\(\s*pki_enrollment_healthy\s*\{\s*subject\s*=\s*\"([^\"]+)\"\s*\}\s*\)"
)
JOB_MATCHER_RE = re.compile(r"job\s*=~\s*\"([^\"]+)\"")


def _alert_exprs(rules_cfg: object) -> dict[str, str]:
    found: dict[str, str] = {}
    if not isinstance(rules_cfg, dict):
        return found
    for group in rules_cfg.get("groups") or []:
        if not isinstance(group, dict):
            continue
        for rule in group.get("rules") or []:
            if not isinstance(rule, dict):
                continue
            name = rule.get("alert")
            expr = rule.get("expr")
            if isinstance(name, str) and isinstance(expr, str):
                found[name] = expr
    return found


def _scrape_jobs(prom_cfg: object) -> list[dict]:
    if not isinstance(prom_cfg, dict):
        return []
    return [job for job in prom_cfg.get("scrape_configs") or [] if isinstance(job, dict)]


def _job_targets(job: dict) -> list[str]:
    targets: list[str] = []
    for block in job.get("static_configs") or []:
        if not isinstance(block, dict):
            continue
        targets.extend(target for target in block.get("targets") or [] if isinstance(target, str))
    return targets


def _collect_pki_ops_jobs(prom_cfg: object) -> tuple[dict[str, tuple[str, str]], list[str]]:
    violations: list[str] = []
    ops_by_host: dict[str, tuple[str, str]] = {}
    for job in _scrape_jobs(prom_cfg):
        name = job.get("job_name")
        if name == "pki-agent" or (isinstance(name, str) and name.startswith("pki-agent")):
            violations.append(f"scrape job {name!r} is a leftover pki-agent sidecar")
        path = job.get("metrics_path", "/metrics")
        for target in _job_targets(job):
            if target.startswith("pki-agent"):
                violations.append(f"scrape target {target!r} is a leftover pki-agent sidecar")
            if not target.endswith(":9110"):
                continue
            host = target[: -len(":9110")]
            if host in ops_by_host:
                violations.append(f"duplicate :9110 scrape for {host}")
                continue
            if not isinstance(name, str):
                violations.append(f":9110 target {target!r} has no job_name")
                continue
            if path != "/metrics":
                violations.append(f"{name} target {target} metrics_path is {path!r}, want /metrics")
            ops_by_host[host] = (name, path)
    return ops_by_host, violations


def _audit_pki_alert_exprs(
    exprs: dict[str, str], want_jobs: set[str], expected_hosts: set[str]
) -> list[str]:
    violations: list[str] = []
    absent_expr = exprs.get("PkiEnrollmentWorkloadMetricsAbsent", "")
    absent_subjects = ABSENT_SUBJECT_RE.findall(absent_expr)
    if set(absent_subjects) != expected_hosts:
        missing = sorted(expected_hosts - set(absent_subjects))
        extra = sorted(set(absent_subjects) - expected_hosts)
        violations.append(
            f"PkiEnrollmentWorkloadMetricsAbsent subjects missing={missing} extra={extra}"
        )
    if len(absent_subjects) != len(set(absent_subjects)):
        violations.append("PkiEnrollmentWorkloadMetricsAbsent repeats a subject")

    fleet = "".join(exprs.get("PkiEnrollmentFleetIncomplete", "").split())
    if "count(countby(subject)(pki_enrollment_healthy))" not in fleet:
        violations.append("PkiEnrollmentFleetIncomplete must count unique subjects, not raw series")
    if "orvector(0)" not in fleet:
        violations.append(
            "PkiEnrollmentFleetIncomplete must use `or vector(0)` so all-absent fires"
        )
    want_fleet = len(PKI_INPROCESS_PARENTS)
    if f"!={want_fleet}" not in fleet and f"=={want_fleet}" not in fleet:
        violations.append(
            f"PkiEnrollmentFleetIncomplete must pin exactly {want_fleet} unique subjects"
        )

    ops_down = exprs.get("PkiEnrollmentOpsDown", "")
    matcher = JOB_MATCHER_RE.search(ops_down)
    if matcher is None:
        violations.append('PkiEnrollmentOpsDown must select up{{job=~"..."}} for every PKI ops job')
    else:
        listed = [part for part in matcher.group(1).split("|") if part]
        if set(listed) != want_jobs:
            violations.append(
                "PkiEnrollmentOpsDown jobs "
                f"missing={sorted(want_jobs - set(listed))} extra={sorted(set(listed) - want_jobs)}"
            )
        if "==0" not in "".join(ops_down.split()):
            violations.append("PkiEnrollmentOpsDown must alert on up==0")

    compact = "".join(exprs.get("PkiEnrollmentRenewalFailing", "").split())
    if "unlesson(subject)" not in compact:
        violations.append(
            "PkiEnrollmentRenewalFailing must use `unless on(subject)` so absent/zero success still fires"
        )
    return violations


def _audit_retired_pki_agent_alerts(rules_cfg: object, exprs: dict[str, str]) -> list[str]:
    violations: list[str] = []
    sidecar_alerts = {
        "PkiAgentCertExpirySoon",
        "PkiAgentCertExpired",
        "PkiAgentRenewalFailing",
        "PkiAgentDown",
        "PkiAgentFleetIncomplete",
        "PkiAgentMetricsAbsent",
    }
    live_sidecar = sorted(name for name in sidecar_alerts if name in exprs)
    if live_sidecar:
        violations.append(
            "dead pki-agent sidecar alerts still defined: "
            + ",".join(live_sidecar)
            + " (leftover detection is the Docker-label gate)"
        )
    if isinstance(rules_cfg, dict):
        names = [g.get("name") for g in (rules_cfg.get("groups") or []) if isinstance(g, dict)]
        if "pki_agent_cert_lifecycle" in names:
            violations.append(
                "pki_agent_cert_lifecycle group must not exist; live alerts are in-process"
            )
    return violations


def audit_pki_ops_surface(prom_cfg: object, rules_cfg: object) -> list[str]:
    """Pin the always-on in-process PKI ops scrapes and absence/fleet/ops-down alerts.

    Parses loaded YAML documents only. Comments in the source file cannot
    satisfy these checks.
    """
    ops_by_host, violations = _collect_pki_ops_jobs(prom_cfg)

    expected_hosts = set(PKI_INPROCESS_PARENTS)
    got_hosts = set(ops_by_host)
    violations.extend(
        f"missing {host}:9110 scrape target" for host in sorted(expected_hosts - got_hosts)
    )
    violations.extend(
        f"unexpected :9110 scrape host {host}" for host in sorted(got_hosts - expected_hosts)
    )
    for host, (job_name, _) in sorted(ops_by_host.items()):
        want = PKI_OPS_JOBS.get(host)
        if want and job_name != want:
            violations.append(f"{host}:9110 job_name is {job_name!r}, want {want!r}")
    want_jobs = {PKI_OPS_JOBS[h] for h in expected_hosts if h in PKI_OPS_JOBS}
    got_jobs = {name for name, _ in ops_by_host.values()}
    violations.extend(
        f"missing PKI ops job_name {job_name!r}" for job_name in sorted(want_jobs - got_jobs)
    )

    exprs = _alert_exprs(rules_cfg)
    violations.extend(_audit_pki_alert_exprs(exprs, want_jobs, expected_hosts))
    violations.extend(_audit_retired_pki_agent_alerts(rules_cfg, exprs))
    return violations


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        default=str(Path(__file__).absolute().parent.parent),
        help="repository root (default: parent of this script's directory)",
    )
    args = parser.parse_args()
    root = os.path.normpath(Path(args.repo_root).absolute())

    compose_path = Path(root) / COMPOSE_FILE
    if not compose_path.is_file():
        sys.stdout.write(f"::error::{COMPOSE_FILE} not found under {root}\n")
        return 1

    mounts = parse_mounts(compose_path)
    audit_prometheus_rule_glob(root, mounts)
    audit_attached_file(
        "prometheus",
        "/etc/prometheus/prometheus.yml",
        PROMETHEUS_CONFIG,
        mounts,
        root,
    )
    alertmanager_present = audit_alertmanager(root, mounts)
    provisioning_count, dashboard_count = audit_grafana(root, mounts)
    audit_attached_file(
        "grafana",
        "/etc/grafana/grafana.ini",
        GRAFANA_INI,
        mounts,
        root,
    )
    group_count, rule_count, group_names = count_prometheus_rules(root)
    prom_cfg = load_yaml(Path(root) / PROMETHEUS_CONFIG)
    rules_cfg = load_yaml(Path(root) / PROMETHEUS_RULES_DIR / "pki-agent-alerts.yml")
    for msg in audit_pki_ops_surface(prom_cfg, rules_cfg):
        fail(msg)

    for msg in warnings:
        sys.stdout.write(f"::warning::{msg}\n")
    for msg in errors:
        sys.stdout.write(f"::error::{msg}\n")

    sys.stdout.write("\n")
    sys.stdout.write("observability config summary (on disk):\n")
    sys.stdout.write(f"  prometheus rule groups : {group_count}\n")
    sys.stdout.write(f"  prometheus rules       : {rule_count}\n")
    sys.stdout.write(f"  rule group names       : {', '.join(sorted(group_names)) or '(none)'}\n")
    sys.stdout.write(f"  grafana provisioning   : {provisioning_count} file(s)\n")
    sys.stdout.write(f"  grafana dashboards     : {dashboard_count} file(s)\n")
    sys.stdout.write(
        f"  alertmanager config    : {'present' if alertmanager_present else 'absent'}\n"
    )

    if errors:
        sys.stdout.write(f"\nFAIL: {len(errors)} structural violation(s)\n")
        return 1
    sys.stdout.write("\nOK: structural audit clean\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
