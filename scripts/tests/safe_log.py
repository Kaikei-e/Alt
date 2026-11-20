"""Stdout helpers for PKI contract scripts.

check() prints PASS/FAIL with a numeric assertion ordinal.
Caller-supplied name and detail are deleted immediately upon function
entry to guarantee no arbitrary text or sensitive data can reach stdout.
"""

from __future__ import annotations

import sys


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0
        self.count = 0

    def reset(self) -> None:
        self.passed = 0
        self.failed = 0
        self.count = 0


_RESULTS = _Results()


def __getattr__(name: str) -> int:
    if name == "PASS":
        return _RESULTS.passed
    if name == "FAIL":
        return _RESULTS.failed
    if name == "COUNT":
        return _RESULTS.count
    msg = f"module {__name__!r} has no attribute {name!r}"
    raise AttributeError(msg)


def reset() -> None:
    _RESULTS.reset()


def check(name: str, condition: bool, detail: str = "") -> None:
    """Print fixed literal PASS/FAIL + assertion ordinal. Never print label/detail."""
    del name, detail
    _RESULTS.count += 1
    if condition:
        sys.stdout.write(f"  PASS  [assertion #{_RESULTS.count}]\n")
        _RESULTS.passed += 1
        return
    sys.stdout.write(f"  FAIL  [assertion #{_RESULTS.count}]\n")
    _RESULTS.failed += 1
