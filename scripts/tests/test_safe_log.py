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
import re
import unittest

import safe_log
from safe_log import check

PASSWORD_FILE_DETAIL = "password-file args: ['/run/secrets/step_ca_root_password']"  # noqa: S105 - test fixture value
PASSWORD_ENV_DETAIL = "STEP_CA_PROVISIONER_PASSWORD_FILE='/run/secrets/pki-agent-alt-backend-jwk'"  # noqa: S105 - test fixture value
SECRET_PATH_DETAIL = "secrets=['step_ca_root_password'] password_file='/run/secrets/x'"  # noqa: S105 - test fixture value
EXPECTED_TWO_ASSERTIONS = 2


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
        assert "FAIL" in out, out
        assert "[assertion #1]" in out, out
        assert "bootstrap mapping" not in out, out
        assert "password-file args" not in out, out
        assert "/run/secrets/" not in out, out
        assert "step_ca_root_password" not in out, out
        assert safe_log.FAIL == 1, safe_log.FAIL
        assert safe_log.PASS == 0, safe_log.PASS

    def test_fail_stdout_omits_password_env_values(self) -> None:
        out = self._capture("provisioner password file", False, PASSWORD_ENV_DETAIL)
        assert "FAIL" in out, out
        assert "[assertion #1]" in out, out
        assert "provisioner password file" not in out, out
        assert "STEP_CA_PROVISIONER_PASSWORD_FILE" not in out, out
        assert "/run/secrets/" not in out, out
        assert safe_log.FAIL == 1, safe_log.FAIL
        assert safe_log.PASS == 0, safe_log.PASS

    def test_pass_stdout_omits_secret_detail(self) -> None:
        out = self._capture("allowlist verification", True, SECRET_PATH_DETAIL)
        assert "PASS" in out, out
        assert "[assertion #1]" in out, out
        assert "allowlist verification" not in out, out
        assert "/run/secrets/" not in out, out
        assert "step_ca_root_password" not in out, out
        assert safe_log.PASS == 1, safe_log.PASS
        assert safe_log.FAIL == 0, safe_log.FAIL

    def test_condition_still_drives_pass_fail(self) -> None:
        out1 = self._capture("ok", True, PASSWORD_FILE_DETAIL)
        out2 = self._capture("bad", False, PASSWORD_FILE_DETAIL)
        assert "PASS" in out1, out1
        assert "ok" not in out1, out1
        assert "FAIL" in out2, out2
        assert "bad" not in out2, out2
        assert safe_log.PASS == 1, safe_log.PASS
        assert safe_log.FAIL == 1, safe_log.FAIL
        assert safe_log.COUNT == EXPECTED_TWO_ASSERTIONS, safe_log.COUNT

    def test_sensitive_name_and_token_omitted_completely(self) -> None:
        out = self._capture("workload mounts secret_api_token_12345", True, "")
        assert "PASS" in out, out
        assert "workload" not in out, out
        assert "secret_api_token_12345" not in out, out
        assert "12345" not in out, out
        assert out.strip() == "PASS  [assertion #1]", out

    def test_sensitive_declaration_and_key_omitted_completely(self) -> None:
        out = self._capture(
            "compose declares pki-agent-custom-jwk with key sk_live_99999", False, ""
        )
        assert "FAIL" in out, out
        assert "compose" not in out, out
        assert "pki-agent-custom-jwk" not in out, out
        assert "sk_live_99999" not in out, out
        assert out.strip() == "FAIL  [assertion #1]", out

    def test_counters_and_numeric_assertion_ordinal(self) -> None:
        assert safe_log.COUNT == 0, safe_log.COUNT
        out1 = self._capture("first benign check", True, "")
        assert safe_log.COUNT == 1, safe_log.COUNT
        assert safe_log.PASS == 1, safe_log.PASS
        assert out1.strip() == "PASS  [assertion #1]", out1
        assert "first benign check" not in out1, out1

        out2 = self._capture("second check password protection", False, "")
        assert safe_log.COUNT == EXPECTED_TWO_ASSERTIONS, safe_log.COUNT
        assert safe_log.FAIL == 1, safe_log.FAIL
        assert out2.strip() == "FAIL  [assertion #2]", out2
        assert "second check password protection" not in out2, out2

        safe_log.reset()
        assert safe_log.COUNT == 0, safe_log.COUNT
        assert safe_log.PASS == 0, safe_log.PASS
        assert safe_log.FAIL == 0, safe_log.FAIL

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
                assert status in out, out
                assert leak_candidate not in out, out
                assert "\r" not in out, out
                assert "password-file" not in out, out
                assert "/run/secrets/" not in out, out
                assert re.search(r"^(PASS|FAIL)\s+\[assertion #\d+\]$", out.strip()), out


if __name__ == "__main__":
    unittest.main()
