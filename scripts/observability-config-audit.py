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
  5. An alertmanager.yml that compose does not mount, or whose mount source
     does not exist. Alertmanager then starts on its built-in default config,
     which routes every alert to a receiver that does nothing.

Exits 1 on any violation. Warnings do not fail the run. The trailing summary
prints the object counts, so any alert threshold that encodes them (e.g. an
expected rule-group count) can be maintained from this output.

Usage:
    python3 scripts/observability-config-audit.py [--repo-root PATH]
"""

from __future__ import annotations

import argparse
import glob
import json
import os
import sys

import yaml

COMPOSE_FILE = "compose/observability.yaml"
PROMETHEUS_CONFIG = "observability/prometheus/prometheus.yml"
PROMETHEUS_RULES_DIR = "observability/prometheus/rules"
PROMETHEUS_TESTS_DIR = "observability/prometheus/tests"
ALERTMANAGER_CONFIG = "observability/alertmanager/alertmanager.yml"
GRAFANA_PROVISIONING_DIR = "observability/grafana/provisioning"
GRAFANA_DASHBOARDS_DIR = "observability/grafana/dashboards"

errors: list[str] = []
warnings: list[str] = []


def fail(msg: str) -> None:
    errors.append(msg)


def warn(msg: str) -> None:
    warnings.append(msg)


def load_yaml(path: str) -> object:
    with open(path, encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def parse_mounts(compose_path: str) -> dict[str, list[tuple[str, str]]]:
    """Map each service to its [(host_abs_path, container_path)] bind mounts.

    Paths in a compose file are relative to the file's own directory, which is
    why the deploy pipeline can hand the same tree to two different working
    directories and get two different mount sources.
    """
    compose_dir = os.path.dirname(os.path.abspath(compose_path))
    cfg = load_yaml(compose_path) or {}
    mounts: dict[str, list[tuple[str, str]]] = {}
    for name, svc in (cfg.get("services") or {}).items():
        entries: list[tuple[str, str]] = []
        for vol in svc.get("volumes") or []:
            if not isinstance(vol, str):
                continue
            parts = vol.split(":")
            if len(parts) < 2:
                continue
            source, target = parts[0], parts[1]
            # Named volumes have no path separator and no leading dot.
            if not source.startswith((".", "/")):
                continue
            entries.append(
                (os.path.normpath(os.path.join(compose_dir, source)), target)
            )
        mounts[name] = entries
    return mounts


def container_to_host(
    container_path: str, service_mounts: list[tuple[str, str]]
) -> str | None:
    """Translate a container path to its host path via the longest mount match."""
    best: tuple[int, str] | None = None
    for host, target in service_mounts:
        target = target.rstrip("/") or "/"
        if container_path == target:
            candidate = host
        elif container_path.startswith(target + "/"):
            candidate = os.path.join(host, container_path[len(target) + 1 :])
        else:
            continue
        if best is None or len(target) > best[0]:
            best = (len(target), candidate)
    return best[1] if best else None


def audit_prometheus_rule_glob(root: str, mounts: dict[str, list[tuple[str, str]]]) -> None:
    prom_mounts = mounts.get("prometheus", [])
    if not prom_mounts:
        fail(f"{COMPOSE_FILE}: service 'prometheus' has no bind mounts")
        return

    cfg = load_yaml(os.path.join(root, PROMETHEUS_CONFIG)) or {}
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
        hits = glob.glob(host_pattern)
        if not hits:
            fail(
                f"{PROMETHEUS_CONFIG}: rule_files entry {pattern!r} resolves to host "
                f"pattern {os.path.relpath(host_pattern, root)!r} which matches no file"
            )
        matched.update(os.path.abspath(h) for h in hits)

    rules_dir = os.path.join(root, PROMETHEUS_RULES_DIR)
    on_disk = {
        os.path.abspath(p)
        for ext in ("*.yml", "*.yaml")
        for p in glob.glob(os.path.join(rules_dir, ext))
    }
    for orphan in sorted(on_disk - matched):
        fail(
            f"{os.path.relpath(orphan, root)}: present in {PROMETHEUS_RULES_DIR}/ but "
            f"not matched by any rule_files glob — it will never be evaluated"
        )

    tests_dir = os.path.abspath(os.path.join(root, PROMETHEUS_TESTS_DIR))
    for leaked in sorted(m for m in matched if m.startswith(tests_dir + os.sep)):
        fail(
            f"{os.path.relpath(leaked, root)}: a promtool test file is matched by a "
            f"rule_files glob. Prometheus parses it as a rule group and refuses the "
            f"whole config — every rule stops loading, not just this one"
        )


def audit_alertmanager(root: str, mounts: dict[str, list[tuple[str, str]]]) -> bool:
    config_path = os.path.join(root, ALERTMANAGER_CONFIG)
    declared = "alertmanager" in mounts
    exists = os.path.isfile(config_path)

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

    am_mounts = mounts.get("alertmanager", [])
    host_cfg = container_to_host("/etc/alertmanager/alertmanager.yml", am_mounts)
    if host_cfg is None or os.path.abspath(host_cfg) != os.path.abspath(config_path):
        fail(
            f"{COMPOSE_FILE}: alertmanager does not bind-mount {ALERTMANAGER_CONFIG} "
            f"at /etc/alertmanager/alertmanager.yml"
        )

    prom_cfg = load_yaml(os.path.join(root, PROMETHEUS_CONFIG)) or {}
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


def audit_grafana(root: str, mounts: dict[str, list[tuple[str, str]]]) -> tuple[int, int]:
    provisioning_root = os.path.join(root, GRAFANA_PROVISIONING_DIR)
    provisioning_files = sorted(
        p
        for ext in ("*.yaml", "*.yml")
        for p in glob.glob(os.path.join(provisioning_root, "**", ext), recursive=True)
    )
    if not provisioning_files:
        fail(f"{GRAFANA_PROVISIONING_DIR}/: no provisioning files found")

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

    grafana_mounts = mounts.get("grafana", [])
    for container_path in dashboard_provider_paths:
        host_path = container_to_host(container_path, grafana_mounts)
        if host_path is None:
            fail(
                f"{GRAFANA_PROVISIONING_DIR}/: dashboard provider path "
                f"{container_path!r} is not bind-mounted by the grafana service in "
                f"{COMPOSE_FILE} — the provider loads zero dashboards"
            )
        elif not os.path.isdir(host_path):
            fail(
                f"{GRAFANA_PROVISIONING_DIR}/: dashboard provider path "
                f"{container_path!r} maps to {os.path.relpath(host_path, root)!r}, "
                f"which is not a directory"
            )

    dashboards_dir = os.path.join(root, GRAFANA_DASHBOARDS_DIR)
    dashboards = sorted(glob.glob(os.path.join(dashboards_dir, "*.json")))
    uids: dict[str, str] = {}
    for path in dashboards:
        rel = os.path.relpath(path, root)
        try:
            with open(path, encoding="utf-8") as handle:
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
        os.path.relpath(p, root)
        for p in glob.glob(os.path.join(root, "observability/grafana/*.json"))
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
        for path in sorted(glob.glob(os.path.join(root, PROMETHEUS_RULES_DIR, ext))):
            doc = load_yaml(path) or {}
            for group in doc.get("groups") or []:
                groups += 1
                names.append(group.get("name", "<unnamed>"))
                rules += len(group.get("rules") or [])
    return groups, rules, names


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
        help="repository root (default: parent of this script's directory)",
    )
    args = parser.parse_args()
    root = os.path.abspath(args.repo_root)

    compose_path = os.path.join(root, COMPOSE_FILE)
    if not os.path.isfile(compose_path):
        print(f"::error::{COMPOSE_FILE} not found under {root}")
        return 1

    mounts = parse_mounts(compose_path)
    audit_prometheus_rule_glob(root, mounts)
    alertmanager_present = audit_alertmanager(root, mounts)
    provisioning_count, dashboard_count = audit_grafana(root, mounts)
    group_count, rule_count, group_names = count_prometheus_rules(root)

    for msg in warnings:
        print(f"::warning::{msg}")
    for msg in errors:
        print(f"::error::{msg}")

    print("")
    print("observability config summary (on disk):")
    print(f"  prometheus rule groups : {group_count}")
    print(f"  prometheus rules       : {rule_count}")
    print(f"  rule group names       : {', '.join(sorted(group_names)) or '(none)'}")
    print(f"  grafana provisioning   : {provisioning_count} file(s)")
    print(f"  grafana dashboards     : {dashboard_count} file(s)")
    print(f"  alertmanager config    : {'present' if alertmanager_present else 'absent'}")

    if errors:
        print(f"\nFAIL: {len(errors)} structural violation(s)")
        return 1
    print("\nOK: structural audit clean")
    return 0


if __name__ == "__main__":
    sys.exit(main())
