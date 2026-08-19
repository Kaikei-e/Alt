#!/usr/bin/env python3
"""Unit tests for scripts/tests/safe_log.py (no Docker, stdlib only).

Captured stdout from check() must never contain password-file values,
step_ca_root_password, Docker secret paths, or env values named
*PASSWORD*/*SECRET* when those strings are passed as detail.
"""

from __future__ import annotations

import contextlib
import io
import unittest

import safe_log
from safe_log import check

PASSWORD_FILE_DETAIL = (
    "password-file args: ['/run/secrets/step_ca_root_password']"
)
PASSWORD_ENV_DETAIL = (
    "STEP_CA_PROVISIONER_PASSWORD_FILE='/run/secrets/pki-agent-alt-backend-jwk'"
)
SECRET_PATH_DETAIL = "secrets=['step_ca_root_password'] password_file='/run/secrets/x'"


class CheckStdoutTests(unittest.TestCase):
    def setUp(self) -> None:
        safe_log.reset()

    def _capture(self, name: str, condition: bool, detail: str) -> str:
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            check(name, condition, detail)
        return buf.getvalue()

    def test_fail_stdout_omits_password_file_values(self) -> None:
        out = self._capture("bootstrap mapping", False, PASSWORD_FILE_DETAIL)
        self.assertIn("FAIL", out)
        self.assertIn("bootstrap mapping", out)
        self.assertNotIn("password-file args", out)
        self.assertNotIn("/run/secrets/", out)
        self.assertNotIn("step_ca_root_password", out)
        self.assertEqual(safe_log.FAIL, 1)
        self.assertEqual(safe_log.PASS, 0)

    def test_fail_stdout_omits_password_env_values(self) -> None:
        out = self._capture("provisioner password file", False, PASSWORD_ENV_DETAIL)
        self.assertIn("FAIL", out)
        self.assertNotIn("STEP_CA_PROVISIONER_PASSWORD_FILE", out)
        self.assertNotIn("/run/secrets/", out)

    def test_pass_stdout_omits_secret_detail(self) -> None:
        out = self._capture("no root password", True, SECRET_PATH_DETAIL)
        self.assertIn("PASS", out)
        self.assertIn("no root password", out)
        self.assertNotIn("/run/secrets/", out)
        self.assertNotIn("step_ca_root_password", out)
        self.assertEqual(safe_log.PASS, 1)
        self.assertEqual(safe_log.FAIL, 0)

    def test_condition_still_drives_pass_fail(self) -> None:
        self._capture("ok", True, PASSWORD_FILE_DETAIL)
        self._capture("bad", False, PASSWORD_FILE_DETAIL)
        self.assertEqual(safe_log.PASS, 1)
        self.assertEqual(safe_log.FAIL, 1)


if __name__ == "__main__":
    unittest.main()
