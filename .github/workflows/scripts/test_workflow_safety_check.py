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

        # 7) `|| echo`, `|| exit 0`, and `|| :` (with flexible whitespace) are detected.
        for snippet in (
            "      - run: curl -fsS http://localhost || echo '{}'\n",
            "      - run: some-cmd ||  exit 0\n",
            "      - run: some-cmd || :\n",
        ):
            with self.subTest(snippet=snippet):
                errs = self.mod.check_workflow_text("sample.yaml", snippet)
                self.assertEqual(len(errs), 1)
                self.assertIn("sample.yaml:1: swallowed failure without a reason.", errs[0])


if __name__ == "__main__":
    unittest.main()
