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


if __name__ == "__main__":
    unittest.main()
