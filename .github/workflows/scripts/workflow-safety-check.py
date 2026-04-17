#!/usr/bin/env python3
"""Structural PyYAML check behind workflow-safety-lint.yaml.

Kept in a separate file because the source contains literal GitHub Actions
template-expression sequences (the three-character sequence
dollar-openbrace-openbrace) that GitHub Actions would try to expand if this
code lived inline in a `run:` block.

Checks:
  1. No workflow in `.github/workflows/*.y{a,}ml` declares the forbidden
     top-level triggers `pull_request_target` or `workflow_run`.
  2. No workflow with a `pull_request:` trigger enlists a self-hosted
     runner (string, list, or object `runs-on:` forms). Expression-derived
     `runs-on:` values on such jobs are also rejected because static
     analysis can't resolve the label.
"""
from __future__ import annotations

import glob
import sys

import yaml

# Literal template-expression opener. Built from fragments so the script
# source never contains the exact sequence GitHub Actions scans for.
GHA_EXPR_OPEN = "$" + "{{"

FORBIDDEN_TRIGGERS = {"pull_request_target", "workflow_run"}


def extract_labels(runs_on):
    """Return (labels, had_expression) for any runs-on shape."""
    if runs_on is None:
        return [], False

    # String form: single label, possibly an expression ("${{ ... }}" etc).
    if isinstance(runs_on, str):
        return [runs_on], (GHA_EXPR_OPEN in runs_on)

    # List form: labels array, may include expression entries.
    if isinstance(runs_on, list):
        labels = [str(x) for x in runs_on]
        return labels, any(GHA_EXPR_OPEN in label for label in labels)

    # Object form: {group: ..., labels: [...]} — runner group + label pins.
    if isinstance(runs_on, dict):
        labels_raw = runs_on.get("labels", [])
        if isinstance(labels_raw, str):
            labels_list = [labels_raw]
        elif isinstance(labels_raw, list):
            labels_list = [str(x) for x in labels_raw]
        else:
            labels_list = []
        group = runs_on.get("group")
        had_expr = any(GHA_EXPR_OPEN in label for label in labels_list) or (
            isinstance(group, str) and GHA_EXPR_OPEN in group
        )
        return labels_list, had_expr

    return [], False


def main() -> int:
    files = sorted(
        glob.glob(".github/workflows/*.yml")
        + glob.glob(".github/workflows/*.yaml")
    )

    bad = 0
    for path in files:
        with open(path) as fh:
            try:
                doc = yaml.safe_load(fh)
            except yaml.YAMLError as e:
                print(
                    f"::error file={path}::failed to parse YAML: {e}",
                    file=sys.stderr,
                )
                bad += 1
                continue

        if not isinstance(doc, dict):
            continue

        # PyYAML parses the `on:` key as Python True (YAML 1.1 boolean quirk).
        triggers = doc.get(True)
        if triggers is None:
            triggers = doc.get("on", {})

        trigger_names: set[str] = set()
        if isinstance(triggers, dict):
            trigger_names = set(triggers.keys())
        elif isinstance(triggers, list):
            trigger_names = set(triggers)
        elif isinstance(triggers, str):
            trigger_names = {triggers}

        # Rule 1 (authoritative) — forbidden trigger names.
        used = trigger_names & FORBIDDEN_TRIGGERS
        if used:
            for t in sorted(used):
                print(
                    f"::error file={path}::workflow uses forbidden trigger "
                    f"{t!r} (ADR-000763 security hardening).",
                    file=sys.stderr,
                )
            bad += 1

        # Rule 2 — self-hosted on pull_request-triggered jobs.
        if "pull_request" not in trigger_names:
            continue
        for job_name, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue
            labels, had_expr = extract_labels(job.get("runs-on"))
            if any("self-hosted" in label for label in labels):
                print(
                    f"::error file={path}::job {job_name!r} has a pull_request "
                    f"trigger and runs on self-hosted. Route PR validation "
                    f"through ubuntu-latest only (ADR-000763 security "
                    f"hardening).",
                    file=sys.stderr,
                )
                bad += 1
            elif had_expr:
                print(
                    f"::error file={path}::job {job_name!r} has a pull_request "
                    f"trigger and an expression-derived runs-on value. Static "
                    f"analysis cannot confirm it is not self-hosted — either "
                    f"pin the runner literal or split PR and self-hosted into "
                    f"separate workflows.",
                    file=sys.stderr,
                )
                bad += 1

    if bad:
        return 1
    print("OK: no forbidden triggers and no self-hosted on pull_request.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
