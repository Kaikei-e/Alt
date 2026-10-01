"""Tests for compare.py table formatting, sorting, and gate outcome marking."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from compare import format_comparison_table, load_result_file


class TestCompare(unittest.TestCase):
    def test_load_result_file_validation(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)

            # Valid result
            valid_file = tmp / "valid.json"
            valid_file.write_text(
                json.dumps(
                    {"label": "C1", "all_passed": True, "rtf": {"p50": 0.5, "p95": 0.8}}
                ),
                encoding="utf-8",
            )
            data = load_result_file(valid_file)
            self.assertEqual(data["label"], "C1")

            # Non-dict JSON raises TypeError
            array_file = tmp / "array.json"
            array_file.write_text(json.dumps([1, 2, 3]), encoding="utf-8")
            with self.assertRaises(TypeError):
                load_result_file(array_file)

            # Missing rtf
            bad_file = tmp / "bad.json"
            bad_file.write_text(json.dumps({"label": "C2"}), encoding="utf-8")
            with self.assertRaises(ValueError):
                load_result_file(bad_file)

    def test_format_comparison_table_sorting_and_gates(self):
        # Unsorted configs with varied pass/fail conditions using nested rtf
        results = [
            {
                "label": "C3",
                "budget_pass": False,
                "rtf_pass": True,
                "all_passed": False,
                "peak_process_mib": 2900.0,
                "peak_reserved_mib": 2700.0,
                "effective_peak_mib": 2950.0,
                "pid_hits": 10,
                "peak_allocated_mib": 2500.0,
                "rtf": {"p50": 0.35, "p95": 0.50},
                "failures": 0,
            },
            {
                "label": "C1",
                "budget_pass": True,
                "rtf_pass": True,
                "all_passed": True,
                "peak_process_mib": 2100.0,
                "peak_reserved_mib": 2000.0,
                "effective_peak_mib": 2150.0,
                "pid_hits": 12,
                "peak_allocated_mib": 1800.0,
                "rtf": {"p50": 0.40, "p95": 0.60},
                "failures": 0,
            },
            {
                "label": "C4",
                "budget_pass": True,
                "rtf_pass": False,
                "all_passed": False,
                "peak_process_mib": 2200.0,
                "peak_reserved_mib": 2100.0,
                "effective_peak_mib": 2250.0,
                "pid_hits": 8,
                "peak_allocated_mib": 1900.0,
                "rtf": {"p50": 0.80, "p95": 1.25},
                "failures": 0,
            },
            {
                "label": "C2",
                "budget_pass": True,
                "rtf_pass": True,
                "all_passed": True,
                "peak_process_mib": 2400.0,
                "peak_reserved_mib": 2300.0,
                "effective_peak_mib": 2450.0,
                "pid_hits": 15,
                "peak_allocated_mib": 2100.0,
                "rtf": {"p50": 0.30, "p95": 0.45},
                "failures": 0,
            },
        ]

        table = format_comparison_table(results)

        # 1. Column headers and footnote check
        self.assertIn("Effective Peak (MiB)*", table)
        self.assertIn("Peak Proc (MiB)", table)
        self.assertIn("Peak Reserved (MiB)", table)
        self.assertIn("PID Hits", table)
        self.assertIn("Budget gate evaluates Effective Peak (MiB)", table)

        # 2. Ordering check: C1 must appear before C2, C2 before C3, C3 before C4
        pos_c1 = table.find("C1")
        pos_c2 = table.find("C2")
        pos_c3 = table.find("C3")
        pos_c4 = table.find("C4")
        self.assertTrue(0 <= pos_c1 < pos_c2 < pos_c3 < pos_c4)

        # 3. Gate check: C1 and C2 meet both gates; C3 and C4 do not
        lines = [line for line in table.splitlines() if "|" in line]
        data_lines = [
            line
            for line in lines
            if any(lbl in line for lbl in ["C1", "C2", "C3", "C4"])
        ]
        self.assertEqual(len(data_lines), 4)

        c1_line = next(line for line in data_lines if "C1" in line)
        self.assertIn("PASS [OK]", c1_line)

        c2_line = next(line for line in data_lines if "C2" in line)
        self.assertIn("PASS [OK]", c2_line)

        c3_line = next(line for line in data_lines if "C3" in line)
        self.assertIn("FAIL", c3_line)

        c4_line = next(line for line in data_lines if "C4" in line)
        self.assertIn("FAIL", c4_line)

        self.assertIn("Configurations meeting both gates: C1, C2", table)

    def test_format_comparison_table_empty(self):
        table = format_comparison_table([])
        self.assertEqual(table, "No results to compare.")

    def test_format_comparison_table_speed_column_and_ordering(self):
        results = [
            {
                "label": "C1",
                "speed": 1.5,
                "budget_pass": True,
                "rtf_pass": True,
                "all_passed": True,
                "effective_peak_mib": 2150.0,
                "peak_process_mib": 2100.0,
                "peak_reserved_mib": 2000.0,
                "pid_hits": 12,
                "rtf": {"p50": 0.30, "p95": 0.45},
                "failures": 0,
            },
            {
                "label": "C1",
                "speed": 1.0,
                "budget_pass": True,
                "rtf_pass": True,
                "all_passed": True,
                "effective_peak_mib": 2150.0,
                "peak_process_mib": 2100.0,
                "peak_reserved_mib": 2000.0,
                "pid_hits": 12,
                "rtf": {"p50": 0.40, "p95": 0.60},
                "failures": 0,
            },
            {
                "label": "C2",
                "speed": 1.0,
                "budget_pass": True,
                "rtf_pass": True,
                "all_passed": True,
                "effective_peak_mib": 2450.0,
                "peak_process_mib": 2400.0,
                "peak_reserved_mib": 2300.0,
                "pid_hits": 15,
                "rtf": {"p50": 0.30, "p95": 0.45},
                "failures": 0,
            },
        ]

        table = format_comparison_table(results)

        # 1. Speed column header check
        self.assertIn("Speed", table)

        # 2. Ordering check: C1 at 1.0x must appear before C1 at 1.5x, before C2 at 1.0x
        lines = [
            line for line in table.splitlines() if "|" in line and "Label" not in line
        ]
        self.assertEqual(len(lines), 3)

        self.assertIn("C1", lines[0])
        self.assertIn("1.0x", lines[0])

        self.assertIn("C1", lines[1])
        self.assertIn("1.5x", lines[1])

        self.assertIn("C2", lines[2])
        self.assertIn("1.0x", lines[2])


if __name__ == "__main__":
    unittest.main()
