"""Tests verifying worst_case_ja.txt character length and content requirements."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
INPUTS_FILE = BENCH_DIR / "inputs" / "worst_case_ja.txt"


class TestWorstCaseJaInputs(unittest.TestCase):
    def test_file_exists_and_readable(self):
        self.assertTrue(
            INPUTS_FILE.is_file(), f"Inputs file not found at {INPUTS_FILE}"
        )

    def test_line_count_and_character_lengths(self):
        content = INPUTS_FILE.read_text(encoding="utf-8")
        lines = [line.strip() for line in content.splitlines() if line.strip()]

        # About 20 synthetic Japanese lines
        self.assertGreaterEqual(len(lines), 18)
        self.assertLessEqual(len(lines), 25)

        # Every line must be between 55 and 60 characters (inclusive)
        for i, line in enumerate(lines, 1):
            char_count = len(line)
            self.assertTrue(
                55 <= char_count <= 60,
                f"Line {i} character count {char_count} not in range [55, 60]: '{line}'",
            )

    def test_content_diversity(self):
        """Verify mixture of kanji, kana, numbers, dates, units, katakana, and ASCII."""
        content = INPUTS_FILE.read_text(encoding="utf-8")

        # Numbers
        self.assertTrue(re.search(r"[0-9０-９]", content), "Must contain digits")
        # Dates / Times
        self.assertTrue(
            re.search(r"(月|日|年|時|分)", content), "Must contain dates/times"
        )
        # Units
        self.assertTrue(
            re.search(r"(％|ｍ|Ｗ|℃|ｋｍ|ｄＢ|ｇ|円)", content), "Must contain units"
        )
        # Katakana
        self.assertTrue(
            re.search(r"[\u30A0-\u30FF]", content), "Must contain katakana loanwords"
        )
        # Kanji
        self.assertTrue(re.search(r"[\u4E00-\u9FFF]", content), "Must contain kanji")
        # ASCII / Fullwidth ASCII
        self.assertTrue(
            re.search(r"[A-Za-zＡ-Ｚａ-ｚ]", content),
            "Must contain ASCII words/acronyms",
        )


if __name__ == "__main__":
    unittest.main()
