#!/usr/bin/env python3
"""Unit tests for scripts/tests/safe_log.py (no Docker, stdlib only).

Captured stdout from check() must never contain password-file values,
step_ca_root_password, Docker secret paths, env values named
*PASSWORD*/*SECRET*, or arbitrary caller-supplied assertion names or details.
Output must contain only fixed literal PASS/FAIL and the assertion ordinal.
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
        self.assertIn("[assertion #1]", out)
        self.assertNotIn("bootstrap mapping", out)
        self.assertNotIn("password-file args", out)
        self.assertNotIn("/run/secrets/", out)
        self.assertNotIn("step_ca_root_password", out)
        self.assertEqual(safe_log.FAIL, 1)
        self.assertEqual(safe_log.PASS, 0)

    def test_fail_stdout_omits_password_env_values(self) -> None:
        out = self._capture("provisioner password file", False, PASSWORD_ENV_DETAIL)
        self.assertIn("FAIL", out)
        self.assertIn("[assertion #1]", out)
        self.assertNotIn("provisioner password file", out)
        self.assertNotIn("STEP_CA_PROVISIONER_PASSWORD_FILE", out)
        self.assertNotIn("/run/secrets/", out)
        self.assertEqual(safe_log.FAIL, 1)
        self.assertEqual(safe_log.PASS, 0)

    def test_pass_stdout_omits_secret_detail(self) -> None:
        out = self._capture("allowlist verification", True, SECRET_PATH_DETAIL)
        self.assertIn("PASS", out)
        self.assertIn("[assertion #1]", out)
        self.assertNotIn("allowlist verification", out)
        self.assertNotIn("/run/secrets/", out)
        self.assertNotIn("step_ca_root_password", out)
        self.assertEqual(safe_log.PASS, 1)
        self.assertEqual(safe_log.FAIL, 0)

    def test_condition_still_drives_pass_fail(self) -> None:
        out1 = self._capture("ok", True, PASSWORD_FILE_DETAIL)
        out2 = self._capture("bad", False, PASSWORD_FILE_DETAIL)
        self.assertIn("PASS", out1)
        self.assertNotIn("ok", out1)
        self.assertIn("FAIL", out2)
        self.assertNotIn("bad", out2)
        self.assertEqual(safe_log.PASS, 1)
        self.assertEqual(safe_log.FAIL, 1)
        self.assertEqual(safe_log.COUNT, 2)

    def test_sensitive_name_and_token_omitted_completely(self) -> None:
        out = self._capture("workload mounts secret_api_token_12345", True, "")
        self.assertIn("PASS", out)
        self.assertNotIn("workload", out)
        self.assertNotIn("secret_api_token_12345", out)
        self.assertNotIn("12345", out)
        self.assertEqual(out.strip(), "PASS  [assertion #1]")

    def test_sensitive_declaration_and_key_omitted_completely(self) -> None:
        out = self._capture("compose declares pki-agent-custom-jwk with key sk_live_99999", False, "")
        self.assertIn("FAIL", out)
        self.assertNotIn("compose", out)
        self.assertNotIn("pki-agent-custom-jwk", out)
        self.assertNotIn("sk_live_99999", out)
        self.assertEqual(out.strip(), "FAIL  [assertion #1]")

    def test_counters_and_numeric_assertion_ordinal(self) -> None:
        self.assertEqual(safe_log.COUNT, 0)
        out1 = self._capture("first benign check", True, "")
        self.assertEqual(safe_log.COUNT, 1)
        self.assertEqual(safe_log.PASS, 1)
        self.assertEqual(out1.strip(), "PASS  [assertion #1]")
        self.assertNotIn("first benign check", out1)

        out2 = self._capture("second check password protection", False, "")
        self.assertEqual(safe_log.COUNT, 2)
        self.assertEqual(safe_log.FAIL, 1)
        self.assertEqual(out2.strip(), "FAIL  [assertion #2]")
        self.assertNotIn("second check password protection", out2)

        safe_log.reset()
        self.assertEqual(safe_log.COUNT, 0)
        self.assertEqual(safe_log.PASS, 0)
        self.assertEqual(safe_log.FAIL, 0)

    def test_regression_arbitrary_keyword_free_name_never_leaks(self) -> None:
        cases = [
            ("xQ9vRm7wA2zC6dE8", "xQ9vRm7wA2zC6dE8"),
            ("sk_live_99999", "sk_live_99999"),
            ("safe_prefix\r\nINJECTED_LOG_LINE", "INJECTED_LOG_LINE"),
            ("/run/secrets/step_ca_root_password", "/run/secrets/"),
            ("/run/secrets/step_ca_root_password", "step_ca_root_password"),
            ("pki-agent-custom-jwk", "pki-agent-custom-jwk"),
        ]
        for name, leak_candidate in cases:
            for condition in (True, False):
                out = self._capture(name, condition, PASSWORD_FILE_DETAIL)
                status = "PASS" if condition else "FAIL"
                self.assertIn(status, out)
                self.assertNotIn(leak_candidate, out)
                self.assertNotIn("\r", out)
                self.assertNotIn("password-file", out)
                self.assertNotIn("/run/secrets/", out)
                self.assertRegex(out.strip(), r"^(PASS|FAIL)\s+\[assertion #\d+\]$")


if __name__ == "__main__":
    unittest.main()
