"""Stdout helpers for PKI contract scripts.

check() prints PASS/FAIL with a safe assertion label categorized into a
finite explicit set of trusted static literals with a numeric assertion
ordinal. Caller-supplied detail and raw arbitrary assertion names are
never written to stdout (CodeQL py/clear-text-logging-sensitive-data).
"""

from __future__ import annotations

PASS = 0
FAIL = 0
COUNT = 0

# Trusted static literal labels (benign explicit equality branches):
LITERAL_BOOTSTRAP_MAPPING = "bootstrap mapping"
LITERAL_ALLOWLIST_VERIFICATION = "allowlist verification"

# Finite explicit constant label categories (trusted static literals):
CATEGORY_SECRET_MOUNT = "secret mount verification"
CATEGORY_SECRET_DECLARATION = "compose secret declaration verification"
CATEGORY_PROVISIONER_CREDENTIAL = "provisioner credential verification"
CATEGORY_PASSWORD_PROTECTION = "password protection verification"
CATEGORY_KEY_CERT_CONFIG = "key and cert configuration verification"
CATEGORY_SENSITIVE_ASSERTION = "sensitive contract assertion"
CATEGORY_CONTRACT_ASSERTION = "contract assertion"

_SENSITIVE_SUBSTRINGS = (
    "secret",
    "password",
    "passwd",
    "jwk",
    "token",
    "key",
    "credential",
    "cred",
    "/run/",
    "private",
)


def reset() -> None:
    global PASS, FAIL, COUNT
    PASS = 0
    FAIL = 0
    COUNT = 0


def _is_sensitive(name: str) -> bool:
    """Detect if assertion name contains sensitive keywords, paths, or credentials."""
    if not isinstance(name, str):
        return False
    lower = name.lower()
    return any(ind in lower for ind in _SENSITIVE_SUBSTRINGS)


def _safe_label(name: str, ordinal: int) -> str:
    """Map assertion name to finite safe constant category or trusted literal with ordinal.

    Taint invariant: NO bytes from caller-supplied name or detail may ever reach stdout.
    Only trusted static literals and the integer ordinal are emitted.
    """
    if not isinstance(name, str):
        return f"[assertion #{ordinal}] {CATEGORY_CONTRACT_ASSERTION}"

    if name == LITERAL_BOOTSTRAP_MAPPING:
        cat = LITERAL_BOOTSTRAP_MAPPING
    elif name == LITERAL_ALLOWLIST_VERIFICATION:
        cat = LITERAL_ALLOWLIST_VERIFICATION
    else:
        lower = name.lower()
        if "mount" in lower:
            cat = CATEGORY_SECRET_MOUNT
        elif "declare" in lower or "compose" in lower or "file" in lower or "repo" in lower:
            cat = CATEGORY_SECRET_DECLARATION
        elif "provisioner" in lower or "jwk" in lower:
            cat = CATEGORY_PROVISIONER_CREDENTIAL
        elif "password" in lower or "passwd" in lower:
            cat = CATEGORY_PASSWORD_PROTECTION
        elif "key" in lower or "cert" in lower:
            cat = CATEGORY_KEY_CERT_CONFIG
        elif _is_sensitive(name):
            cat = CATEGORY_SENSITIVE_ASSERTION
        else:
            cat = CATEGORY_CONTRACT_ASSERTION

    return f"[assertion #{ordinal}] {cat}"


def check(name: str, condition: bool, detail: str = "") -> None:
    """Print PASS/FAIL + safe assertion label. Never print detail or secret-derived text."""
    global PASS, FAIL, COUNT
    del detail
    COUNT += 1
    label = _safe_label(name, COUNT)
    if condition:
        print(f"  PASS  {label}")
        PASS += 1
        return
    print(f"  FAIL  {label}")
    FAIL += 1
