"""Offline test of the token-key rule in audit-suite-wiring.py."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("audit", Path(__file__).parent / "audit-suite-wiring.py")
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

SERVICES = {
    "auth-hub": {"secrets": ["backend_token_signing_key", "other"]},
    "auth-introspection": {"secrets": [{"source": "backend_token_verify_key"}]},
    "redis": {"secrets": ["redis_password"]},
    "plain": {},
}


class TokenKeyRuleTest(unittest.TestCase):
    def test_finds_services_that_mount_a_token_key(self):
        self.assertEqual(audit.token_key_services(SERVICES), {"auth-hub", "auth-introspection"})

    def test_suite_starting_such_a_service_must_mint_keys(self):
        problems = audit.token_key_problems("s", ["auth-introspection", "redis"], {"auth-introspection"}, "suite_up x\n")
        self.assertEqual(len(problems), 1)
        self.assertIn("suite_token_keys", problems[0])

    def test_suite_that_mints_keys_passes(self):
        code = "suite_token_keys\nsuite_up x\n"
        self.assertEqual(audit.token_key_problems("s", ["auth-hub"], {"auth-hub"}, code), [])

    def test_a_commented_call_does_not_count(self):
        code = "# suite_token_keys\nsuite_up x\n"
        self.assertEqual(len(audit.token_key_problems("s", ["auth-hub"], {"auth-hub"}, code)), 1)

    def test_suite_without_token_services_needs_nothing(self):
        self.assertEqual(audit.token_key_problems("s", ["redis"], {"auth-hub"}, "suite_up x\n"), [])


if __name__ == "__main__":
    unittest.main()
