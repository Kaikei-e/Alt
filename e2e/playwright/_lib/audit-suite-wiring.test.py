"""Offline test of the token-key rule in audit-suite-wiring.py."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("audit", Path(__file__).parent / "audit-suite-wiring.py")
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

LIB = """
suite_init() {
  mkdir -p x
%s
  trap suite_cleanup EXIT
}

other() {
  suite_token_keys
}
"""


class SuiteInitTokenKeysTest(unittest.TestCase):
    def test_accepts_a_suite_init_that_mints_keys(self):
        self.assertTrue(audit.suite_init_mints_token_keys(LIB % "  suite_token_keys"))

    def test_rejects_keys_minted_only_elsewhere(self):
        self.assertFalse(audit.suite_init_mints_token_keys(LIB % "  :"))

    def test_a_comment_does_not_count(self):
        self.assertFalse(audit.suite_init_mints_token_keys(LIB % "  # suite_token_keys"))

    def test_real_library_passes(self):
        self.assertTrue(audit.suite_init_mints_token_keys((audit.PW_ROOT / "_lib" / "suite.sh").read_text()))


if __name__ == "__main__":
    unittest.main()
