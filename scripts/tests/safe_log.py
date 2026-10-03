"""Stdout helpers for PKI contract scripts.

check() prints PASS/FAIL with a numeric assertion ordinal.
Caller-supplied name and detail are deleted immediately upon function
entry to guarantee no arbitrary text or sensitive data can reach stdout.
"""

from __future__ import annotations

PASS = 0
FAIL = 0
COUNT = 0


def reset() -> None:
    global PASS, FAIL, COUNT
    PASS = 0
    FAIL = 0
    COUNT = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    """Print fixed literal PASS/FAIL + assertion ordinal. Never print label/detail."""
    global PASS, FAIL, COUNT
    del name, detail
    COUNT += 1
    if condition:
        print(f"  PASS  [assertion #{COUNT}]")
        PASS += 1
        return
    print(f"  FAIL  [assertion #{COUNT}]")
    FAIL += 1
