#!/usr/bin/env python3
"""Structural PyYAML check behind workflow-safety-lint.yaml.

Kept in a separate file because the source contains literal GitHub Actions
template-expression sequences (the three-character sequence
dollar-openbrace-openbrace) that GitHub Actions would try to expand if this
code lived inline in a `run:` block.

Checks:
  1. No workflow in `.github/workflows/*.y{a,}ml` declares the forbidden
     top-level triggers `pull_request_target` or `workflow_run`.
  2. Every job runs on a static GitHub-hosted label (`ubuntu-*`,
     `windows-*`, `macos-*`). This repo is public and registers no
     self-hosted runner at all (ADR-000763): the self-hosted pool lives in
     the private deploy repo. A `self-hosted` label, an unknown label, or
     an expression-derived `runs-on:` (which static analysis can't
     resolve) all fail — on every trigger, not just `pull_request`, because
     a reusable workflow's runner is chosen by its caller.
  3. Some job actually invokes alt-frontend-sv's `client` vitest project.
     (1) and (2) are ADR-000763 fork-PR hardening; (3) is a different
     failure mode with the same shape — a workflow that *looks* like it
     runs a suite while collecting zero of its files. The `client` project
     is only constructed when VITEST_BROWSER=true, so for a long time every
     src/**/*.svelte.{test,spec}.ts file was skipped while the job stayed
     green. Asserting it structurally means deleting the job, dropping
     `--project=client`, or gutting the npm script fails the lint instead
     of quietly re-hiding 63 files.
  4. Every workflow declares a top-level `permissions:` block, so the
     GITHUB_TOKEN scope is visible in the file rather than inherited from
     the repository default.
  5. Every `actions/checkout` step sets `persist-credentials: false`. No
     workflow here pushes, so the token has no business surviving in
     `.git/config` where a dependency's install hook can read it.
  6. No `run:` body interpolates `github.event.*` or `github.head_ref`
     through a template expression. Those are attacker-controlled on
     pull_request events (PR title, branch name, commit message) and expand
     before the shell sees them — route them through `env:` instead.
  7. Every `uses:` outside this repo is pinned to a 40-hex commit SHA (or,
     for `docker://` images, an `@sha256:` digest). Tags move; digests do
     not. The repository setting `sha_pinning_required` enforces the same
     at run time; this check surfaces it at PR time.
"""
from __future__ import annotations

import glob
import json
import os
import re
import sys

import yaml

# Literal template-expression opener. Built from fragments so the script
# source never contains the exact sequence GitHub Actions scans for.
GHA_EXPR_OPEN = "$" + "{{"

FORBIDDEN_TRIGGERS = {"pull_request_target", "workflow_run"}

GITHUB_HOSTED_LABEL_RE = re.compile(r"^(ubuntu|windows|macos)-[a-z0-9.-]+$")
SHA_PIN_RE = re.compile(r"@[0-9a-f]{40}$")
DOCKER_DIGEST_RE = re.compile(r"@sha256:[0-9a-f]{64}$")
UNTRUSTED_CONTEXT_RE = re.compile(r"github\.event\.|github\.head_ref")
TEMPLATE_EXPR_RE = re.compile(re.escape(GHA_EXPR_OPEN) + r"(.*?)}}", re.S)

# --- Check 3 constants -------------------------------------------------
# The project name and the gating env var are duplicated from
# alt-frontend-sv/vitest.config.ts on purpose: this check exists precisely
# to detect the two sides drifting apart, so it must not import them.
FRONTEND_PACKAGE_JSON = os.path.join("alt-frontend-sv", "package.json")
CLIENT_PROJECT_NAME = "client"
BROWSER_ENV_VAR = "VITEST_BROWSER"

# `bun run x`, `npm run x`, `pnpm run x`, `yarn run x` — the indirection a
# workflow step uses to reach a package.json script.
RUN_SCRIPT_RE = re.compile(
    r"\b(?:bun|bunx|npm|pnpm|yarn)\s+run\s+([A-Za-z0-9:_.-]+)"
)
# Leading `FOO=bar BAZ=qux cmd ...` inline environment assignments.
INLINE_ENV_RE = re.compile(r"\b([A-Z_][A-Z0-9_]*)=(\S+)")
# `--project=client` and `--project client` both select a project.
PROJECT_FLAG_RE = re.compile(r"--project(?:=|\s+)([A-Za-z0-9:_.-]+)")
# Shell separators that start a fresh command within one `run:` block.
COMMAND_SPLIT_RE = re.compile(r"&&|\|\||;|\n|\|")


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


def is_pinned(uses):
    """True for local actions, SHA-pinned repos, and digest-pinned images."""
    if uses.startswith("./"):
        return True
    if uses.startswith("docker://"):
        return bool(DOCKER_DIGEST_RE.search(uses))
    return bool(SHA_PIN_RE.search(uses))


def load_frontend_scripts():
    """Return alt-frontend-sv's package.json `scripts`, or None if missing."""
    try:
        with open(FRONTEND_PACKAGE_JSON) as fh:
            return json.load(fh).get("scripts", {}) or {}
    except (OSError, ValueError):
        return None


def expand_run_scripts(command, scripts, depth=0):
    """Inline `bun run <script>` bodies so the real vitest flags are visible.

    A workflow step never spells the vitest invocation out; it says
    `bun run test:client` and package.json holds the flags. Resolving the
    indirection is what makes the check notice a script being renamed or
    hollowed out, not just the job being deleted.
    """
    if depth > 3:
        return command
    expanded = command
    for name in RUN_SCRIPT_RE.findall(command):
        body = scripts.get(name)
        if body:
            expanded += " ;; " + expand_run_scripts(body, scripts, depth + 1)
    return expanded


def collects_client_project(command, inherited_env):
    """True if `command` runs vitest in a way that collects the client project.

    Mirrors vitest.config.ts: the project exists only when VITEST_BROWSER is
    "true", and it is collected when no `--project` filter is given or when
    the filter names it. `playwright test` also takes `--project`, so vitest
    has to be the binary being run — otherwise the e2e job's
    `--project=desktop-chromium` would satisfy this by accident.
    """
    for segment in COMMAND_SPLIT_RE.split(command):
        if not re.search(r"\bvitest\b", segment):
            continue
        if re.search(r"\bplaywright\b", segment):
            continue

        env = dict(inherited_env)
        env.update(INLINE_ENV_RE.findall(segment))
        if env.get(BROWSER_ENV_VAR) != "true":
            continue

        projects = PROJECT_FLAG_RE.findall(segment)
        if not projects or CLIENT_PROJECT_NAME in projects:
            return True
    return False


def is_disabled(node):
    """True if a job/step is switched off or cannot fail the build.

    `continue-on-error: true` and `if: false` are the two ways to keep a
    step in the file while making it toothless — the exact move check 3
    exists to stop, so such a step must not satisfy it.
    """
    if not isinstance(node, dict):
        return False
    if node.get("continue-on-error") in (True, "true"):
        return True
    return str(node.get("if", "")).strip().lower() in ("false", "${{ false }}")


def iter_run_steps(doc):
    """Yield (job_name, command, env, blocking) for every `run:` step."""
    workflow_env = doc.get("env") if isinstance(doc.get("env"), dict) else {}
    for job_name, job in (doc.get("jobs") or {}).items():
        if not isinstance(job, dict):
            continue
        job_env = job.get("env") if isinstance(job.get("env"), dict) else {}
        job_blocking = not is_disabled(job)
        for step in job.get("steps") or []:
            if not isinstance(step, dict) or not isinstance(
                step.get("run"), str
            ):
                continue
            step_env = (
                step.get("env") if isinstance(step.get("env"), dict) else {}
            )
            env = {
                str(k): str(v)
                for k, v in {**workflow_env, **job_env, **step_env}.items()
            }
            yield (
                job_name,
                step["run"],
                env,
                job_blocking and not is_disabled(step),
            )


def main() -> int:
    files = sorted(
        glob.glob(".github/workflows/*.yml")
        + glob.glob(".github/workflows/*.yaml")
    )

    bad = 0
    # Check 3 is repo-wide, not per-file: one job anywhere has to run it.
    frontend_scripts = load_frontend_scripts()
    client_project_runners: list[str] = []

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

        # Rule 3 (collection pass) — record jobs that run the client vitest
        # project. Only automatically-triggered workflows count: a job that
        # exists but has to be launched by hand protects nobody, which is
        # the same class of bug as a project that is never collected.
        if frontend_scripts is not None and (
            trigger_names & {"push", "pull_request"}
        ):
            for job_name, command, env, blocking in iter_run_steps(doc):
                if not blocking:
                    continue
                resolved = expand_run_scripts(command, frontend_scripts)
                if collects_client_project(resolved, env):
                    client_project_runners.append(f"{path}::{job_name}")

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

        # Rule 2 — every job on a static GitHub-hosted label. Applies to all
        # triggers: a reusable workflow has no pull_request trigger of its
        # own, yet its jobs run in the caller's (possibly PR) context.
        for job_name, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue
            if "uses" in job and "runs-on" not in job:
                continue  # caller of a reusable workflow; checked there
            labels, had_expr = extract_labels(job.get("runs-on"))
            if had_expr:
                print(
                    f"::error file={path}::job {job_name!r} has an "
                    f"expression-derived runs-on value. Static analysis "
                    f"cannot confirm it is GitHub-hosted — pin the runner "
                    f"literal (ADR-000763: this repo registers no "
                    f"self-hosted runner).",
                    file=sys.stderr,
                )
                bad += 1
            elif not labels or not all(
                GITHUB_HOSTED_LABEL_RE.match(label) for label in labels
            ):
                print(
                    f"::error file={path}::job {job_name!r} runs on "
                    f"{labels!r}. Only static GitHub-hosted labels are "
                    f"allowed here; self-hosted runners live in the private "
                    f"deploy repo (ADR-000763 security hardening).",
                    file=sys.stderr,
                )
                bad += 1

        # Rule 4 — GITHUB_TOKEN scope declared in the file.
        if "permissions" not in doc:
            print(
                f"::error file={path}::workflow has no top-level "
                f"`permissions:` block. Declare it (`contents: read` unless "
                f"a job needs more) so the token scope does not depend on "
                f"the repository default.",
                file=sys.stderr,
            )
            bad += 1

        for job_name, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue
            for step in job.get("steps") or []:
                if not isinstance(step, dict):
                    continue
                uses = step.get("uses")
                if isinstance(uses, str):
                    # Rule 5 — checkout must not leave the token behind.
                    if uses.startswith("actions/checkout@"):
                        with_ = step.get("with")
                        persist = (
                            with_.get("persist-credentials")
                            if isinstance(with_, dict)
                            else None
                        )
                        if persist is not False:
                            print(
                                f"::error file={path}::job {job_name!r} checks "
                                f"out without `persist-credentials: false`. "
                                f"Nothing in this repo pushes from CI, so the "
                                f"GITHUB_TOKEN must not be written into "
                                f".git/config.",
                                file=sys.stderr,
                            )
                            bad += 1
                    # Rule 7 — immutable action references.
                    if not is_pinned(uses):
                        print(
                            f"::error file={path}::job {job_name!r} uses "
                            f"{uses!r}, which is not pinned to a commit SHA "
                            f"(or @sha256 digest for docker://). Tags and "
                            f"branches are mutable supply-chain inputs.",
                            file=sys.stderr,
                        )
                        bad += 1
                # Rule 6 — no untrusted context expanded into a shell body.
                run = step.get("run")
                if isinstance(run, str):
                    for expr in TEMPLATE_EXPR_RE.findall(run):
                        if UNTRUSTED_CONTEXT_RE.search(expr):
                            print(
                                f"::error file={path}::job {job_name!r} "
                                f"interpolates {expr.strip()!r} into a `run:` "
                                f"body. github.event.* and github.head_ref are "
                                f"attacker-controlled on pull_request; pass "
                                f"them through `env:` and read the variable "
                                f"in the shell.",
                                file=sys.stderr,
                            )
                            bad += 1

    # Rule 3 (assertion pass) — repo-wide, so it runs after every file.
    if frontend_scripts is None:
        print(
            f"::error::cannot read {FRONTEND_PACKAGE_JSON}, so the check that "
            f"alt-frontend-sv's {CLIENT_PROJECT_NAME!r} vitest project is "
            f"actually run cannot be evaluated. If the frontend moved or was "
            f"removed, update this script — do not let the check pass "
            f"vacuously.",
            file=sys.stderr,
        )
        bad += 1
    elif not client_project_runners:
        print(
            f"::error::no push/pull_request-triggered job runs alt-frontend-sv's "
            f"{CLIENT_PROJECT_NAME!r} vitest project. Those are the "
            f"src/**/*.svelte.{{test,spec}}.ts component specs; without a job "
            f"that sets {BROWSER_ENV_VAR}=true and runs vitest without a "
            f"--project filter or with --project={CLIENT_PROJECT_NAME}, they "
            f"are collected zero times while CI still reports green. Restore "
            f"the job (see alt-frontend-sv-unit-test.yaml) rather than "
            f"deleting this check. `continue-on-error` and `if: false` steps "
            f"deliberately do not count.",
            file=sys.stderr,
        )
        bad += 1

    if bad:
        return 1
    print(
        "OK: no forbidden triggers, GitHub-hosted labels only, permissions "
        "declared, checkouts credential-free, no untrusted context in run "
        "bodies, every action SHA-pinned."
    )
    print(
        "OK: client vitest project is run by "
        + ", ".join(sorted(client_project_runners))
        + "."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
