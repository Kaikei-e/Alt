"""Tests for seeds argument parsing in design_voices.py."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from design_voices import parse_seeds


class TestSeeds(unittest.TestCase):
    def test_single_seed(self):
        self.assertEqual(parse_seeds("7"), [7])

    def test_comma_separated(self):
        self.assertEqual(parse_seeds("1,5,9"), [1, 5, 9])

    def test_range(self):
        self.assertEqual(parse_seeds("1-5"), [1, 2, 3, 4, 5])

    def test_mixed_expression(self):
        self.assertEqual(parse_seeds("1-3,5,8-10"), [1, 2, 3, 5, 8, 9, 10])

    def test_whitespace_and_deduplication(self):
        self.assertEqual(parse_seeds(" 1 - 3 , 2 , 5 , 4 - 5 "), [1, 2, 3, 4, 5])

    def test_empty_string(self):
        with self.assertRaises(ValueError):
            parse_seeds("")
        with self.assertRaises(ValueError):
            parse_seeds("   ")

    def test_invalid_range_order(self):
        with self.assertRaises(ValueError):
            parse_seeds("5-2")

    def test_non_integer_tokens(self):
        with self.assertRaises(ValueError):
            parse_seeds("1,foo,3")
        with self.assertRaises(ValueError):
            parse_seeds("a-b")

    def test_malformed_ranges(self):
        with self.assertRaises(ValueError):
            parse_seeds("1-2-3")
        with self.assertRaises(ValueError):
            parse_seeds("-5")


if __name__ == "__main__":
    unittest.main()
