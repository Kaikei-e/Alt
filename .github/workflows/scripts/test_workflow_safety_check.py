#!/usr/bin/env python3
"""Unit tests for .github/workflows/scripts/workflow-safety-check.py."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import unittest
import yaml

SCRIPT_PATH = Path(__file__).resolve().parent / "workflow-safety-check.py"
DIGEST_A = "a" * 64
DIGEST_B = "b" * 64
GHA_OPEN = "$" + "{{"


def load_checker():
    spec = importlib.util.spec_from_file_location("workflow_safety_check", SCRIPT_PATH)
    assert spec is not None and spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def base_workflow(job_spec: dict) -> dict:
    return yaml.safe_load(
        """
name: sample
on:
  pull_request:
permissions:
  contents: read
jobs: {}
"""
    ) | {"jobs": {"test": job_spec}}


class TestWorkflowSafetyCheckHardening(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_checker()

    def test_container_string_requires_sha256_digest(self) -> None:
        unpinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "container": "mcr.microsoft.com/playwright:v1.62.1-jammy",
                "steps": [{"run": "true"}],
            }
        )
        errors = self.mod.check_workflow_doc("sample.yaml", unpinned)
        self.assertTrue(
            any("container" in err and "@sha256:" in err for err in errors),
            f"Expected container digest error, got: {errors}",
        )

        pinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "container": f"mcr.microsoft.com/playwright:v1.62.1-jammy@sha256:{DIGEST_A}",
                "steps": [{"run": "true"}],
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", pinned), [])

    def test_container_mapping_image_requires_sha256_digest(self) -> None:
        unpinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "container": {"image": "mcr.microsoft.com/playwright:v1.62.1-jammy"},
                "steps": [{"run": "true"}],
            }
        )
        errors = self.mod.check_workflow_doc("sample.yaml", unpinned)
        self.assertTrue(
            any("container" in err and "@sha256:" in err for err in errors),
            f"Expected container.image digest error, got: {errors}",
        )

        pinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "container": {
                    "image": f"mcr.microsoft.com/playwright:v1.62.1-jammy@sha256:{DIGEST_A}"
                },
                "steps": [{"run": "true"}],
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", pinned), [])

    def test_services_image_requires_sha256_digest(self) -> None:
        unpinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "services": {
                    "pact-db": {"image": "postgres:16-alpine"},
                },
                "steps": [{"run": "true"}],
            }
        )
        errors = self.mod.check_workflow_doc("sample.yaml", unpinned)
        self.assertTrue(
            any("pact-db" in err and "@sha256:" in err for err in errors),
            f"Expected service image digest error, got: {errors}",
        )

        pinned = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "services": {
                    "pact-db": {"image": f"postgres:16-alpine@sha256:{DIGEST_B}"},
                },
                "steps": [{"run": "true"}],
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", pinned), [])

    def test_non_reusable_job_requires_numeric_or_expression_timeout_minutes(self) -> None:
        missing_timeout = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "steps": [{"run": "true"}],
            }
        )
        errors = self.mod.check_workflow_doc("sample.yaml", missing_timeout)
        self.assertTrue(
            any("timeout-minutes" in err for err in errors),
            f"Expected timeout-minutes error, got: {errors}",
        )

        zero_timeout = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 0,
                "steps": [{"run": "true"}],
            }
        )
        errors_zero = self.mod.check_workflow_doc("sample.yaml", zero_timeout)
        self.assertTrue(
            any("timeout-minutes" in err for err in errors_zero),
            f"Expected timeout-minutes error for 0, got: {errors_zero}",
        )

        literal_str_timeout = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": "ten",
                "steps": [{"run": "true"}],
            }
        )
        errors_str = self.mod.check_workflow_doc("sample.yaml", literal_str_timeout)
        self.assertTrue(
            any("timeout-minutes" in err for err in errors_str),
            f"Expected timeout-minutes error for literal string, got: {errors_str}",
        )

        expr_timeout = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": f"{GHA_OPEN} inputs.timeout-minutes }}",
                "steps": [{"run": "true"}],
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", expr_timeout), [])

    def test_reusable_workflow_caller_exempt_from_timeout_minutes(self) -> None:
        reusable_caller = base_workflow(
            {
                "uses": "./.github/workflows/reusable-test-go.yaml",
                "with": {"working-directory": "alt-backend/app"},
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", reusable_caller), [])

    def test_paths_filter_must_match_own_workflow_and_called_reusable_workflows(
        self,
    ) -> None:
        wf_path = ".github/workflows/backend-go.yaml"
        reusable_go = "./.github/workflows/reusable-test-go.yaml"
        reusable_gates = "./.github/workflows/reusable-go-quality-gates.yaml"

        # 1) Missing own file and called reusable workflows in on.push.paths / on.pull_request.paths
        missing_all = {
            "name": "backend-go",
            "on": {
                "push": {"paths": ["alt-backend/**"]},
                "pull_request": {"paths": ["alt-backend/**"]},
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
                "gates": {"uses": reusable_gates},
            },
        }
        errors_missing = self.mod.check_workflow_doc(wf_path, missing_all)
        self.assertTrue(
            any("backend-go.yaml" in err and "on.push.paths" in err for err in errors_missing),
            f"Expected missing own path error on push, got: {errors_missing}",
        )
        self.assertTrue(
            any("reusable-test-go.yaml" in err and "on.push.paths" in err for err in errors_missing),
            f"Expected missing reusable-test-go.yaml error on push, got: {errors_missing}",
        )
        self.assertTrue(
            any(
                "reusable-go-quality-gates.yaml" in err and "on.pull_request.paths" in err
                for err in errors_missing
            ),
            f"Expected missing reusable-go-quality-gates.yaml error on pull_request, got: {errors_missing}",
        )

        # 2) Explicit literal entries before '!' negations satisfy the rule
        valid_explicit = {
            "name": "backend-go",
            "on": {
                "push": {
                    "paths": [
                        "alt-backend/**",
                        ".github/workflows/backend-go.yaml",
                        ".github/workflows/reusable-test-go.yaml",
                        ".github/workflows/reusable-go-quality-gates.yaml",
                        "!**/CLAUDE.md",
                    ]
                },
                "pull_request": {
                    "paths": [
                        "alt-backend/**",
                        ".github/workflows/backend-go.yaml",
                        ".github/workflows/reusable-test-go.yaml",
                        ".github/workflows/reusable-go-quality-gates.yaml",
                        "!**/CLAUDE.md",
                    ]
                },
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
                "gates": {"uses": reusable_gates},
            },
        }
        self.assertEqual(self.mod.check_workflow_doc(wf_path, valid_explicit), [])

        # 3) Glob patterns (** and *) also satisfy the rule
        valid_glob = {
            "name": "backend-go",
            "on": {
                "push": {"paths": ["**", "!docs/**"]},
                "pull_request": {"paths": [".github/workflows/*.yaml", "alt-backend/**"]},
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
            },
        }
        self.assertEqual(self.mod.check_workflow_doc(wf_path, valid_glob), [])

        # 4) Later '!' negation that excludes own or reusable workflow fails
        excluded_by_negation = {
            "name": "backend-go",
            "on": {
                "push": {
                    "paths": [
                        "**",
                        "!.github/workflows/**",
                    ]
                },
                "pull_request": {
                    "paths": [
                        "alt-backend/**",
                        ".github/workflows/backend-go.yaml",
                        ".github/workflows/reusable-test-go.yaml",
                        "!.github/workflows/reusable-*.yaml",
                    ]
                },
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
            },
        }
        errors_negated = self.mod.check_workflow_doc(wf_path, excluded_by_negation)
        self.assertTrue(
            any("backend-go.yaml" in err and "on.push.paths" in err for err in errors_negated),
            f"Expected negated own path error on push, got: {errors_negated}",
        )
        self.assertTrue(
            any(
                "reusable-test-go.yaml" in err and "on.pull_request.paths" in err
                for err in errors_negated
            ),
            f"Expected negated reusable path error on pull_request, got: {errors_negated}",
        )

        # 5) paths-ignore and unfiltered triggers are not subject to paths rule
        paths_ignore_wf = {
            "name": "backend-go",
            "on": {
                "push": {"paths-ignore": ["docs/**"]},
                "pull_request": {"branches": ["main"]},
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
            },
        }
        self.assertEqual(self.mod.check_workflow_doc(wf_path, paths_ignore_wf), [])

        # 6) Leading dot in .github/workflows/... must not be stripped (removeprefix("./"), not lstrip("./"))
        missing_leading_dot = {
            "name": "backend-go",
            "on": {
                "push": {
                    "paths": [
                        "alt-backend/**",
                        "github/workflows/backend-go.yaml",
                        "github/workflows/reusable-test-go.yaml",
                    ]
                },
            },
            "permissions": {"contents": "read"},
            "jobs": {
                "unit": {"uses": reusable_go},
            },
        }
        errors_dot = self.mod.check_workflow_doc(wf_path, missing_leading_dot)
        self.assertTrue(
            any("on.push.paths does not match '.github/workflows/backend-go.yaml'" in err for err in errors_dot),
            f"Expected missing leading dot to fail self-trigger match, got: {errors_dot}",
        )

        # 7) Patterns containing '?', '+', or '[' are rejected with a clear error
        for bad_pat in (
            ".github/workflows/backend-go.y?ml",
            ".github/workflows/backend-go.+",
            ".github/workflows/[b]ackend-go.yaml",
        ):
            with self.subTest(bad_pat=bad_pat):
                unsupported_glob_wf = {
                    "name": "backend-go",
                    "on": {
                        "push": {
                            "paths": [
                                bad_pat,
                                ".github/workflows/backend-go.yaml",
                                ".github/workflows/reusable-test-go.yaml",
                            ]
                        }
                    },
                    "permissions": {"contents": "read"},
                    "jobs": {
                        "unit": {"uses": reusable_go},
                    },
                }
                errs = self.mod.check_workflow_doc(wf_path, unsupported_glob_wf)
                self.assertTrue(
                    any(bad_pat in err and "unsupported" in err.lower() for err in errs),
                    f"Expected unsupported glob error for {bad_pat!r}, got: {errs}",
                )

    def test_swallowed_failure_requires_reason(self) -> None:
        # 1) Unannotated `|| true` fails with file:line and guidance.
        unannotated_true = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: grep -q foo bar || true\n"
        )
        errors = self.mod.check_workflow_text("sample.yaml", unannotated_true)
        self.assertEqual(len(errors), 1)
        self.assertIn("sample.yaml:4: swallowed failure without a reason.", errors[0])
        self.assertIn("# swallow-ok:", errors[0])

        # 2) Same-line annotation passes.
        same_line_ok = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: grep -q foo bar || true  # swallow-ok: grep exits 1 when no match\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", same_line_ok), [])

        # 3) Previous-line annotation passes (even across blank lines).
        prev_line_ok = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: |\n"
            "          # swallow-ok: grep exits 1 when nothing matches\n"
            "\n"
            "          matches=$(grep foo bar || true)\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", prev_line_ok), [])

        # 4) Reason shorter than 10 characters after the colon fails.
        short_same_line = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: grep -q foo bar || true  # swallow-ok: too short\n"
        )
        self.assertEqual(len(self.mod.check_workflow_text("sample.yaml", short_same_line)), 1)

        short_prev_line = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: |\n"
            "          # swallow-ok: short\n"
            "          grep -q foo bar || true\n"
        )
        self.assertEqual(len(self.mod.check_workflow_text("sample.yaml", short_prev_line)), 1)

        # 5) `continue-on-error: true` without annotation fails and with annotation passes.
        coe_unannotated = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - name: Upload report\n"
            "        continue-on-error:  true\n"
            "        run: echo upload\n"
        )
        coe_errors = self.mod.check_workflow_text("sample.yaml", coe_unannotated)
        self.assertEqual(len(coe_errors), 1)
        self.assertIn("sample.yaml:5: swallowed failure without a reason.", coe_errors[0])

        coe_annotated = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - name: Upload report\n"
            "        # swallow-ok: optional telemetry upload must not block CI\n"
            "        continue-on-error: true\n"
            "        run: echo upload\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", coe_annotated), [])

        # `continue-on-error: false` is not a swallow.
        coe_false = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - continue-on-error: false\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", coe_false), [])

        # 6) Commented-out line containing `|| true` is ignored.
        commented_out = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      # - run: grep foo bar || true\n"
            "      - run: |\n"
            "          # old command: foo || true\n"
            "          echo ok\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", commented_out), [])

        # 7) `|| echo`, `|| exit 0`, `|| :`, and `set +e` (with flexible whitespace) are detected.
        for snippet in (
            "      - run: curl -fsS http://localhost || echo '{}'\n",
            "      - run: some-cmd ||  exit 0\n",
            "      - run: some-cmd || :\n",
            "      - run: |\n          set +e\n          some-cmd\n",
            "      - run: set +e; some-cmd\n",
        ):
            with self.subTest(snippet=snippet):
                errs = self.mod.check_workflow_text("sample.yaml", snippet)
                self.assertEqual(len(errs), 1)
                self.assertIn("swallowed failure without a reason.", errs[0])

        # 8) `set +e` in a non-lint step passes when annotated with `# swallow-ok:`.
        set_plus_e_ok = (
            "jobs:\n"
            "  test:\n"
            "    steps:\n"
            "      - run: |\n"
            "          # swallow-ok: capture diagnostic exit code from probe command\n"
            "          set +e\n"
            "          probe-cmd\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", set_plus_e_ok), [])

    def test_lint_jobs_and_steps_unconditionally_forbid_swallowed_failures(self) -> None:
        # 1) Lint jobs (`lint`, `quality-gates`, `static-checks`) forbid all swallows
        # even when `# swallow-ok:` is provided.
        for job_name in ("lint", "quality-gates", "static-checks"):
            for case_name, body in (
                (
                    "job continue-on-error",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    # swallow-ok: attempting to bypass lint failure\n"
                    "    continue-on-error: true\n"
                    "    steps:\n      - run: ./scripts/lint/run.sh repo\n",
                ),
                (
                    "step continue-on-error",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - name: Lint\n"
                    "        # swallow-ok: attempting to bypass lint failure\n"
                    "        continue-on-error: true\n"
                    "        run: ./scripts/lint/run.sh repo\n",
                ),
                (
                    "or-true",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - run: ./scripts/lint/run.sh repo || true  # swallow-ok: trying to ignore lint error\n",
                ),
                (
                    "or-colon",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - run: |\n          # swallow-ok: trying to ignore lint error\n          ./scripts/lint/run.sh repo || :\n",
                ),
                (
                    "or-echo",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - run: ./scripts/lint/run.sh repo || echo ok  # swallow-ok: trying to ignore lint error\n",
                ),
                (
                    "or-exit-0",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - run: ./scripts/lint/run.sh repo || exit 0  # swallow-ok: trying to ignore lint error\n",
                ),
                (
                    "set-plus-e",
                    f"jobs:\n  {job_name}:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n"
                    "    steps:\n      - run: |\n"
                    "          # swallow-ok: trying to disable errexit in lint job\n"
                    "          set +e\n"
                    "          ./scripts/lint/run.sh repo\n",
                ),
            ):
                with self.subTest(job=job_name, case=case_name):
                    wf_text = (
                        "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\n"
                        + body
                    )
                    text_errs = self.mod.check_workflow_text("sample.yaml", wf_text)
                    doc_errs = self.mod.check_workflow_doc("sample.yaml", yaml.safe_load(wf_text))
                    self.assertTrue(
                        text_errs or doc_errs,
                        f"Expected lint swallow rejection for {job_name}/{case_name}",
                    )
                    self.assertTrue(
                        len(text_errs) >= 1,
                        f"Expected check_workflow_text to reject {job_name}/{case_name}, got {text_errs}",
                    )
                    self.assertTrue(
                        len(doc_errs) >= 1,
                        f"Expected check_workflow_doc to reject {job_name}/{case_name}, got {doc_errs}",
                    )

        # 2) Lint steps (`./scripts/lint/run.sh` or `make lint`) inside a non-lint job
        # unconditionally forbid swallows, while sibling non-lint diagnostic steps keep `# swallow-ok:`.
        mixed_job_ok = (
            "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\n"
            "jobs:\n"
            "  python-job:\n"
            "    runs-on: ubuntu-latest\n"
            "    timeout-minutes: 10\n"
            "    steps:\n"
            "      - name: Diagnostics\n"
            "        run: |\n"
            "          # swallow-ok: diagnostic directory listing must not fail the job\n"
            "          ls -la . || true\n"
            "      - name: Run lint\n"
            "        run: ./scripts/lint/run.sh scripts\n"
        )
        self.assertEqual(self.mod.check_workflow_text("sample.yaml", mixed_job_ok), [])
        self.assertEqual(
            self.mod.check_workflow_doc("sample.yaml", yaml.safe_load(mixed_job_ok)),
            [],
        )

        for lint_cmd in ("./scripts/lint/run.sh scripts", "make lint TARGET=scripts"):
            for swallow_variant in (
                f"      - name: Run lint\n        # swallow-ok: should still be forbidden\n        continue-on-error: true\n        run: {lint_cmd}\n",
                f"      - name: Run lint\n        run: {lint_cmd} || true  # swallow-ok: should still be forbidden\n",
                f"      - name: Run lint\n        run: |\n          # swallow-ok: should still be forbidden\n          set +e\n          {lint_cmd}\n",
            ):
                with self.subTest(lint_cmd=lint_cmd, variant=swallow_variant.splitlines()[1]):
                    wf_text = (
                        "name: sample\non:\n  pull_request:\npermissions:\n  contents: read\n"
                        "jobs:\n"
                        "  python-job:\n"
                        "    runs-on: ubuntu-latest\n"
                        "    timeout-minutes: 10\n"
                        "    steps:\n"
                        + swallow_variant
                    )
                    self.assertTrue(
                        len(self.mod.check_workflow_text("sample.yaml", wf_text)) >= 1,
                        f"Expected check_workflow_text to reject lint step swallow for {lint_cmd}",
                    )
                    self.assertTrue(
                        len(self.mod.check_workflow_doc("sample.yaml", yaml.safe_load(wf_text))) >= 1,
                        f"Expected check_workflow_doc to reject lint step swallow for {lint_cmd}",
                    )

    def test_direct_lint_tool_invocations_in_run_are_rejected(self) -> None:
        forbidden_commands = [
            "golangci-lint run ./...",
            "gofmt -l .",
            "go vet ./...",
            "gosec ./...",
            "govulncheck ./...",
            "ruff check .",
            "bandit -r .",
            "pyrefly check",
            "cargo fmt --check",
            "cargo clippy -- -D warnings",
            "biome check .",
            "svelte-check",
            "deno fmt --check",
            "deno lint",
            "actionlint -shellcheck=",
            "buf lint",
            "buf breaking --against origin/main",
            "uv run --no-sync ruff check .",
            "python3 -m bandit -r .",
            "bunx svelte-check",
            "files=$(gofmt -l .)",
            '"$GOPATH/bin/govulncheck" ./...',
            "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12",
            "env FOO=1 cargo clippy",
            "docdag validate --format github --immutable-since origin/main",
            "docdag lint --strict --format github",
            "promtool check rules observability/prometheus/rules/*.yml",
            "amtool check-config observability/alertmanager/alertmanager.yml",
            "lkal check --json deploy/desired-env.lkal",
            "python3 scripts/compose-port-audit.py --overlays",
            "python3 scripts/compose-netns-cascade-audit.py",
            "python3 scripts/compose-feature-gate.py --gate-report",
            "python3 scripts/compose-image-audit.py",
            "./scripts/check-compose-variables.sh",
            "bash scripts/tests/test_compose_wiring.sh",
            "python3 scripts/kisage-env.py verify",
            "python3 scripts/regen-service-registry.py --check",
            "scripts/observability-validate.sh",
            "python3 scripts/agent-harness/harness_selfcheck.py",
            "python3 scripts/agent-harness/postmortem_ledger.py --check --base origin/main",
            "python3 scripts/agent-harness/migration_destructive_gate.py --base origin/main",
            "python3 scripts/agent-harness/adr_supersede_gate.py --base origin/main",
            "bash e2e/playwright/_lib/check-suites.sh",
            "python3 .github/workflows/scripts/workflow-safety-check.py",
        ]
        for cmd in forbidden_commands:
            with self.subTest(cmd=cmd):
                wf = base_workflow(
                    {
                        "runs-on": "ubuntu-latest",
                        "timeout-minutes": 10,
                        "steps": [{"run": cmd}],
                    }
                )
                errs = self.mod.check_workflow_doc("sample.yaml", wf)
                self.assertTrue(
                    any("scripts/lint/run.sh" in err for err in errs),
                    f"Expected direct-lint error for {cmd!r}, got: {errs}",
                )

    def test_allowed_run_commands_and_mentions_pass(self) -> None:
        allowed_steps = [
            {"run": './scripts/lint/run.sh "$TARGET"'},
            {
                "run": (
                    "# comment: golangci-lint gofmt go vet gosec govulncheck "
                    "ruff bandit pyrefly cargo fmt cargo clippy biome "
                    "svelte-check deno fmt deno lint actionlint buf lint buf breaking\n"
                    'echo "Run gofmt -w or ruff check or golangci-lint"\n'
                    "printf '::error title=gofmt::Run gofmt -w\\n'\n"
                )
            },
            {"run": "go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11"},
            {"run": "go install golang.org/x/vuln/cmd/govulncheck@latest"},
            {"run": 'go install "github.com/Kaikei-e/DocDag/cmd/docdag@v0.6.0"'},
            {"run": "make install-lkal install-kisage"},
            {"run": "python3 scripts/tests/test-compose-image-audit.py"},
            {"run": "python3 scripts/tests/test-kisage-env.py"},
            {"run": "pip install --quiet ruff bandit pyrefly"},
            {"run": "uv tool install ruff"},
            {"run": "mkdir -p /tmp/ruff-cache\nCACHE_KEY=golangci-lint-v2"},
            {"run": "go test ./...\ngo build ./...\ngo run . -desc out.binpb"},
            {"run": "cargo test --locked\ncargo check"},
            {"run": "deno test --allow-all tests/"},
            {"run": "buf build --exclude-source-info -o out.binpb"},
            {
                "uses": f"actions/cache@{'a' * 40}",
                "with": {"path": "~/.cache", "key": "golangci-lint-ruff-biome"},
            },
            {
                "uses": f"bufbuild/buf-action@{'b' * 40}",
                "with": {"setup_only": True},
            },
        ]
        wf = base_workflow(
            {
                "runs-on": "ubuntu-latest",
                "timeout-minutes": 10,
                "steps": allowed_steps,
            }
        )
        self.assertEqual(self.mod.check_workflow_doc("sample.yaml", wf), [])

    def test_lint_actions_and_docker_images_in_uses_are_rejected(self) -> None:
        forbidden_uses = [
            {"uses": f"golangci/golangci-lint-action@{'c' * 40}"},
            {"uses": f"docker://rhysd/actionlint:1.7.12@sha256:{DIGEST_A}"},
            {"uses": f"astral-sh/ruff-action@{'d' * 40}"},
            {"uses": f"bufbuild/buf-lint-action@{'e' * 40}"},
            {"uses": f"bufbuild/buf-breaking-action@{'f' * 40}"},
            {"uses": f"bufbuild/buf-action@{'1' * 40}"},
        ]
        for step in forbidden_uses:
            with self.subTest(uses=step["uses"]):
                wf = base_workflow(
                    {
                        "runs-on": "ubuntu-latest",
                        "timeout-minutes": 10,
                        "steps": [step],
                    }
                )
                errs = self.mod.check_workflow_doc("sample.yaml", wf)
                self.assertTrue(
                    any("scripts/lint/run.sh" in err for err in errs),
                    f"Expected lint-action error for {step['uses']!r}, got: {errs}",
                )


if __name__ == "__main__":
    unittest.main()
