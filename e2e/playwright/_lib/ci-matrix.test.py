#!/usr/bin/env python3
"""Unit tests for e2e/playwright/_lib/ci-matrix.py."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import yaml

SCRIPT_PATH = Path(__file__).with_name("ci-matrix.py")
spec = importlib.util.spec_from_file_location("ci_matrix", SCRIPT_PATH)
assert spec is not None and spec.loader is not None
ci_matrix = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci_matrix)


def parse_kv_lines(text: str) -> dict[str, object]:
    result: dict[str, object] = {}
    for line in text.strip().splitlines():
        key, value = line.split("=", 1)
        result[key] = json.loads(value)
    return result


class CiMatrixTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory(prefix="ci-matrix-test-")
        self.tmp_dir = Path(self._tmp.name)

    def tearDown(self) -> None:
        self._tmp.cleanup()

    def _run_with_suites(
        self,
        suites_doc: dict[str, object],
        argv: list[str] | None = None,
        env: dict[str, str] | None = None,
    ) -> tuple[int, str, str]:
        suites_file = self.tmp_dir / "suites.yaml"
        suites_file.write_text(yaml.safe_dump(suites_doc), encoding="utf-8")
        stdout = io.StringIO()
        stderr = io.StringIO()
        with (
            mock.patch.object(ci_matrix, "SUITES_YAML", suites_file),
            mock.patch("sys.argv", ["ci-matrix.py", *(argv or [])]),
            mock.patch.dict("os.environ", env or {}, clear=False),
            contextlib.redirect_stdout(stdout),
            contextlib.redirect_stderr(stderr),
        ):
            rc = ci_matrix.main()
        return rc, stdout.getvalue(), stderr.getvalue()

    def test_excludes_suites_owned_by_another_workflow(self) -> None:
        rc, out, err = self._run_with_suites(
            {
                "suites": {
                    "mq-hub": {"images": ["mq-hub"]},
                    "auth-hub": {
                        "images": ["auth-hub"],
                        "workflow": "e2e-playwright.yml",
                    },
                    "alt-backend": {"images": ["alt-backend"], "shards": 2},
                    "alt-frontend-sv": {
                        "images": ["alt-frontend-sv"],
                        "shards": 3,
                        "workflow": "alt-frontend-sv.yml",
                    },
                    "custom-single": {
                        "images": ["custom-single"],
                        "workflow": "custom.yml",
                    },
                }
            }
        )
        self.assertEqual(rc, 0, msg=err)
        outputs = parse_kv_lines(out)
        self.assertEqual(outputs["single"], ["auth-hub", "mq-hub"])
        self.assertEqual(outputs["shardedBuild"], ["alt-backend"])
        self.assertEqual(
            outputs["shardedRun"],
            [
                {"suite": "alt-backend", "shard": "1/2", "index": "1"},
                {"suite": "alt-backend", "shard": "2/2", "index": "2"},
            ],
        )
        self.assertIs(outputs["hasSingle"], True)
        self.assertIs(outputs["hasSharded"], True)

    def test_has_sharded_false_when_only_sharded_suite_has_external_workflow(self) -> None:
        rc, out, err = self._run_with_suites(
            {
                "suites": {
                    "mq-hub": {"images": ["mq-hub"]},
                    "alt-frontend-sv": {
                        "images": ["alt-frontend-sv"],
                        "shards": 3,
                        "workflow": "alt-frontend-sv.yml",
                    },
                }
            }
        )
        self.assertEqual(rc, 0, msg=err)
        outputs = parse_kv_lines(out)
        self.assertEqual(outputs["single"], ["mq-hub"])
        self.assertEqual(outputs["shardedBuild"], [])
        self.assertEqual(outputs["shardedRun"], [])
        self.assertIs(outputs["hasSingle"], True)
        self.assertIs(outputs["hasSharded"], False)

    def test_github_flag_writes_filtered_matrices_to_github_output(self) -> None:
        gh_out = self.tmp_dir / "github_output.txt"
        rc, _, err = self._run_with_suites(
            {
                "suites": {
                    "mq-hub": {"images": ["mq-hub"]},
                    "alt-frontend-sv": {
                        "images": ["alt-frontend-sv"],
                        "shards": 3,
                        "workflow": "alt-frontend-sv.yml",
                    },
                }
            },
            argv=["--github"],
            env={"GITHUB_OUTPUT": str(gh_out)},
        )
        self.assertEqual(rc, 0, msg=err)
        outputs = parse_kv_lines(gh_out.read_text(encoding="utf-8"))
        self.assertEqual(outputs["single"], ["mq-hub"])
        self.assertNotIn("alt-frontend-sv", outputs["shardedBuild"])


if __name__ == "__main__":
    unittest.main()
