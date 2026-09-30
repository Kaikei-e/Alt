"""Tests for VRAM budget (effective peak) and RTF gate evaluation logic."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from bench_vram_rtf import evaluate_gates


class TestBudgetGate(unittest.TestCase):
    def test_both_pass(self):
        # budget = 1000, ratio = 0.9 -> limit = 900.0
        # effective_peak = 800 <= 900.0, rtf_p95 = 0.85 <= 1.0, failures = 0
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=800.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=0.85,
            rtf_max=1.0,
            failures=0,
            pid_hits=1,
        )
        self.assertTrue(b_pass)
        self.assertTrue(r_pass)
        self.assertTrue(all_pass)

    def test_budget_exceeded_by_effective_peak(self):
        # effective_peak = 950 > 900.0
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=950.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=0.85,
            rtf_max=1.0,
            failures=0,
            pid_hits=1,
        )
        self.assertFalse(b_pass)
        self.assertTrue(r_pass)
        self.assertFalse(all_pass)

    def test_rtf_exceeded(self):
        # rtf_p95 = 1.05 > 1.0
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=800.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=1.05,
            rtf_max=1.0,
            failures=0,
            pid_hits=1,
        )
        self.assertTrue(b_pass)
        self.assertFalse(r_pass)
        self.assertFalse(all_pass)

    def test_failures_disqualify_both_gates(self):
        # Even with good numbers, failures > 0 causes both gates to fail
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=800.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=0.5,
            rtf_max=1.0,
            failures=1,
            pid_hits=1,
        )
        self.assertFalse(b_pass)
        self.assertFalse(r_pass)
        self.assertFalse(all_pass)

    def test_rtf_none_fails_rtf_gate(self):
        # If no requests completed (e.g. initial failure), rtf_p95 is None
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=800.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=None,
            rtf_max=1.0,
            failures=0,
            pid_hits=1,
        )
        self.assertTrue(b_pass)
        self.assertFalse(r_pass)
        self.assertFalse(all_pass)

    def test_zero_pid_hits_fails_budget_gate(self):
        b_pass, r_pass, all_pass = evaluate_gates(
            effective_peak_mib=800.0,
            budget_mib=1000.0,
            budget_ratio=0.9,
            rtf_p95=0.5,
            rtf_max=1.0,
            failures=0,
            pid_hits=0,
        )
        self.assertFalse(b_pass)
        self.assertTrue(r_pass)
        self.assertFalse(all_pass)


if __name__ == "__main__":
    unittest.main()
