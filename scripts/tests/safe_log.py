"""Stdout helpers for PKI contract scripts.

check() prints only PASS/FAIL and the assertion name. Caller-supplied
detail is accepted for call-site compatibility but never written
(CodeQL py/clear-text-logging-sensitive-data).
"""

from __future__ import annotations

PASS = 0
FAIL = 0


def reset() -> None:
    global PASS, FAIL
    PASS = 0
    FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    """Print PASS/FAIL + assertion name. Never print detail."""
    global PASS, FAIL
    del detail
    if condition:
        # codeql[py/clear-text-logging-sensitive-data] -- name is a literal assertion label; the
        # only "secret"-shaped values reaching it are Docker secret ids and mount paths, never bytes
        print(f"  PASS  {name}")
        PASS += 1
        return
    # codeql[py/clear-text-logging-sensitive-data] -- see the PASS branch
    print(f"  FAIL  {name}")
    FAIL += 1
