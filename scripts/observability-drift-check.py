#!/usr/bin/env python3
"""Compare the observability config on disk with the config actually loaded.

Prometheus reads its configuration once, at start-up or on an explicit reload.
Nothing in the deploy pipeline performs that reload, so the file in the working
tree and the config the process is evaluating drift apart silently: rules that
were added are never evaluated, scrape jobs that were added are never scraped,
and every dashboard and alert built on them reads as "no data" rather than as
"broken".

This checks the two against each other and names the difference:

  * scrape job names          (disk prometheus.yml vs /api/v1/status/config)
  * alertmanager targets      (same pair — an unloaded `alerting:` block means
                               every firing rule notifies nobody)
  * global knobs declared on disk
  * alert/recording rules     (disk rule files vs /api/v1/rules), compared as
                               (file, group, name) so a partially loaded file
                               is reported per rule, not as a count mismatch
  * alertmanager.yml text     (disk vs /api/v2/status config.original)
  * last successful reload    (age, and whether the last attempt failed)

Exit codes:
  0  in sync
  1  drift detected (or the last reload attempt failed)
  2  could not reach Prometheus / could not read the tree

Usage:
    python3 scripts/observability-drift-check.py
    python3 scripts/observability-drift-check.py --max-reload-age 93600
    python3 scripts/observability-drift-check.py --quiet   # exit code only
"""

from __future__ import annotations

import argparse
import glob
import json
import os
import sys
import time
import urllib.error
import urllib.request

import yaml

PROMETHEUS_CONFIG = "observability/prometheus/prometheus.yml"
PROMETHEUS_RULES_DIR = "observability/prometheus/rules"
ALERTMANAGER_CONFIG = "observability/alertmanager/alertmanager.yml"

DEFAULT_PROMETHEUS_URL = "http://127.0.0.1:9090"
DEFAULT_ALERTMANAGER_URL = "http://127.0.0.1:9093"


class Unreachable(Exception):
    pass


def http_get(url: str, timeout: float) -> bytes:
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:  # noqa: S310
            return resp.read()
    except (urllib.error.URLError, urllib.error.HTTPError, OSError) as exc:
        raise Unreachable(f"{url}: {exc}") from exc


def get_json(url: str, timeout: float) -> dict:
    return json.loads(http_get(url, timeout))


def parse_metrics(text: str) -> dict[str, float]:
    """Parse the handful of unlabelled gauges this check needs from /metrics."""
    out: dict[str, float] = {}
    for line in text.splitlines():
        if not line or line.startswith("#"):
            continue
        name, _, value = line.partition(" ")
        if "{" in name:
            continue
        try:
            out[name] = float(value)
        except ValueError:
            continue
    return out


def disk_rule_index(root: str) -> set[tuple[str, str, str]]:
    index: set[tuple[str, str, str]] = set()
    for ext in ("*.yml", "*.yaml"):
        for path in glob.glob(os.path.join(root, PROMETHEUS_RULES_DIR, ext)):
            with open(path, encoding="utf-8") as handle:
                doc = yaml.safe_load(handle) or {}
            base = os.path.basename(path)
            for group in doc.get("groups") or []:
                gname = group.get("name", "<unnamed>")
                for rule in group.get("rules") or []:
                    name = rule.get("alert") or rule.get("record") or "<unnamed>"
                    index.add((base, gname, name))
    return index


def loaded_rule_index(api: dict) -> set[tuple[str, str, str]]:
    index: set[tuple[str, str, str]] = set()
    for group in api.get("data", {}).get("groups", []):
        base = os.path.basename(group.get("file", ""))
        gname = group.get("name", "<unnamed>")
        for rule in group.get("rules", []):
            index.add((base, gname, rule.get("name", "<unnamed>")))
    return index


def job_names(cfg: dict) -> list[str]:
    return [sc.get("job_name", "<unnamed>") for sc in cfg.get("scrape_configs") or []]


def alertmanager_targets(cfg: dict) -> list[str]:
    return sorted(
        target
        for am in (cfg.get("alerting") or {}).get("alertmanagers") or []
        for sc in am.get("static_configs") or []
        for target in sc.get("targets") or []
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
    )
    parser.add_argument(
        "--prometheus-url",
        default=os.environ.get("PROMETHEUS_URL", DEFAULT_PROMETHEUS_URL),
    )
    parser.add_argument(
        "--alertmanager-url",
        default=os.environ.get("ALERTMANAGER_URL", DEFAULT_ALERTMANAGER_URL),
    )
    parser.add_argument("--timeout", type=float, default=5.0)
    parser.add_argument(
        "--max-reload-age",
        type=float,
        default=0.0,
        help=(
            "seconds; report drift when the last successful reload is older than "
            "this. 0 disables the check. Pair with the periodic reload unit so the "
            "metric doubles as a liveness signal for the reload path itself."
        ),
    )
    parser.add_argument("--quiet", action="store_true")
    args = parser.parse_args()

    root = os.path.abspath(args.repo_root)
    prom = args.prometheus_url.rstrip("/")
    findings: list[str] = []
    notes: list[str] = []

    def emit(line: str) -> None:
        if not args.quiet:
            print(line)

    disk_path = os.path.join(root, PROMETHEUS_CONFIG)
    if not os.path.isfile(disk_path):
        print(f"ERROR: {PROMETHEUS_CONFIG} not found under {root}", file=sys.stderr)
        return 2
    with open(disk_path, encoding="utf-8") as handle:
        disk_cfg = yaml.safe_load(handle) or {}

    try:
        status = get_json(f"{prom}/api/v1/status/config", args.timeout)
        loaded_cfg = yaml.safe_load(status["data"]["yaml"]) or {}
        rules_api = get_json(f"{prom}/api/v1/rules", args.timeout)
        metrics = parse_metrics(http_get(f"{prom}/metrics", args.timeout).decode())
    except Unreachable as exc:
        print(f"ERROR: Prometheus unreachable — {exc}", file=sys.stderr)
        return 2
    except (KeyError, ValueError) as exc:
        print(f"ERROR: unexpected Prometheus API response — {exc}", file=sys.stderr)
        return 2

    # --- reload health -------------------------------------------------------
    ok = metrics.get("prometheus_config_last_reload_successful")
    reload_ts = metrics.get("prometheus_config_last_reload_success_timestamp_seconds")
    start_ts = metrics.get("process_start_time_seconds")
    if ok is not None and ok == 0:
        findings.append(
            "last config reload FAILED — the process is still evaluating the "
            "previously loaded config. Check `docker logs` for the parse error."
        )
    if reload_ts:
        age = time.time() - reload_ts
        notes.append(
            f"last successful reload : {time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(reload_ts))} "
            f"({age / 3600:.1f}h ago)"
        )
        if args.max_reload_age and age > args.max_reload_age:
            findings.append(
                f"last successful reload is {age / 3600:.1f}h old, older than the "
                f"{args.max_reload_age / 3600:.1f}h budget — the reload path itself "
                f"is not running"
            )
    if start_ts:
        notes.append(
            f"process started        : "
            f"{time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(start_ts))}"
        )

    # --- scrape jobs ---------------------------------------------------------
    disk_jobs = job_names(disk_cfg)
    live_jobs = job_names(loaded_cfg)
    missing = [j for j in disk_jobs if j not in live_jobs]
    extra = [j for j in live_jobs if j not in disk_jobs]
    for job in missing:
        findings.append(f"scrape job '{job}' is on disk but not loaded — never scraped")
    for job in extra:
        findings.append(
            f"scrape job '{job}' is loaded but absent from disk — a reload removes it"
        )

    # --- alertmanager wiring -------------------------------------------------
    disk_ams = alertmanager_targets(disk_cfg)
    live_ams = alertmanager_targets(loaded_cfg)
    if disk_ams != live_ams:
        findings.append(
            f"alerting.alertmanagers differ — disk {disk_ams or '[]'} vs loaded "
            f"{live_ams or '[]'}. While the loaded set is empty every firing rule "
            f"notifies nobody."
        )

    # --- global knobs declared on disk --------------------------------------
    # Only keys the file actually sets: the loaded config is the fully
    # defaulted form, so absent keys would compare as spurious drift.
    disk_global = disk_cfg.get("global") or {}
    live_global = loaded_cfg.get("global") or {}
    for key, want in disk_global.items():
        got = live_global.get(key)
        if got != want:
            findings.append(f"global.{key}: disk {want!r} vs loaded {got!r}")

    # --- rule files ----------------------------------------------------------
    disk_rules = disk_rule_index(root)
    live_rules = loaded_rule_index(rules_api)
    for base, group, name in sorted(disk_rules - live_rules):
        findings.append(f"rule not loaded: {base} :: {group} :: {name}")
    for base, group, name in sorted(live_rules - disk_rules):
        findings.append(f"rule loaded but not on disk: {base} :: {group} :: {name}")
    notes.append(f"rules on disk / loaded  : {len(disk_rules)} / {len(live_rules)}")

    # --- alertmanager config text -------------------------------------------
    am_path = os.path.join(root, ALERTMANAGER_CONFIG)
    if os.path.isfile(am_path):
        am_url = args.alertmanager_url.rstrip("/")
        try:
            am_status = get_json(f"{am_url}/api/v2/status", args.timeout)
            original = (am_status.get("config") or {}).get("original", "")
            with open(am_path, encoding="utf-8") as handle:
                on_disk = handle.read()
            if original.strip() != on_disk.strip():
                findings.append(
                    "alertmanager.yml on disk differs from the config Alertmanager "
                    "loaded — reload it too"
                )
            else:
                notes.append("alertmanager config     : in sync")
        except Unreachable:
            notes.append(
                f"alertmanager config     : not checked ({am_url} unreachable — "
                f"the service may not be started yet)"
            )
        except (KeyError, ValueError) as exc:
            notes.append(f"alertmanager config     : not checked ({exc})")

    for note in notes:
        emit(note)

    if findings:
        emit("")
        emit(f"DRIFT: {len(findings)} difference(s) between disk and running config")
        for finding in findings:
            emit(f"  - {finding}")
        emit("")
        emit("Reconcile with: scripts/observability-reload.sh")
        return 1

    emit("")
    emit("OK: running config matches the tree on disk")
    return 0


if __name__ == "__main__":
    sys.exit(main())
