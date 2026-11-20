#!/usr/bin/env python3
"""Unit tests for pure helpers in scripts/fix_adr_obsidian_links.py.

Run:
    python3 scripts/tests/test-fix_adr_obsidian_links-helpers.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "fix_adr_obsidian_links.py"

spec = importlib.util.spec_from_file_location("fix_adr_obsidian_links", SCRIPT_PATH)
assert spec is not None, "script missing"
assert spec.loader is not None, "script loader missing"
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)

COUNTS = {"passed": 0, "failed": 0}


def check(name: str, condition: bool, detail: str = "") -> None:
    """Record a single assertion result."""
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        COUNTS["passed"] += 1
        return
    sys.stdout.write(f"  FAIL  {name}\n")
    if detail:
        sys.stdout.write(f"        {detail}\n")
    COUNTS["failed"] += 1


def test_add_backlinks_appends_new_section() -> None:
    """Appends a new Related ADRs section when none exists."""
    text = "# ADR-000010\n\nBody text.\n\n"
    titles = {5: "Fifth ADR", 20: "Twentieth ADR"}
    got = mod.add_backlinks(text, 10, {20, 5}, titles)
    expected = "# ADR-000010\n\nBody text.\n\n## Related ADRs\n\n- [[000005]] Fifth ADR\n- [[000020]] Twentieth ADR\n"
    check("add_backlinks appends Related ADRs section sorted", got == expected, repr(got))


def test_add_backlinks_replaces_nashi_and_inserts_into_existing() -> None:
    """Replaces '- なし' in existing section and inserts after last bullet otherwise."""
    titles = {7: "Seven", 8: "Eight", 9: "Nine"}

    with_nashi = "# ADR\n\n## Related ADRs\n\n- なし\n\n## Notes\n"
    got_nashi = mod.add_backlinks(with_nashi, 1, {7}, titles)
    check(
        "add_backlinks replaces '- なし' inside Related section",
        got_nashi == "# ADR\n\n## Related ADRs\n\n- [[000007]] Seven\n\n## Notes\n",
        repr(got_nashi),
    )

    with_existing = "# ADR\n\n## Related ADRs\n\n- [[000007]] Seven\n\n## Notes\n"
    got_existing = mod.add_backlinks(with_existing, 1, {7, 8}, titles)
    check(
        "add_backlinks inserts only missing backlinks after last content",
        got_existing
        == "# ADR\n\n## Related ADRs\n\n- [[000007]] Seven\n- [[000008]] Eight\n\n## Notes\n",
        repr(got_existing),
    )


def test_fix_aliases_inline_and_multiline() -> None:
    """Ensures both short and padded ADR aliases are added for inline and multiline forms."""
    inline = "---\ntitle: Test\naliases: [ADR-42]\n---\nBody\n"
    got_inline = mod.fix_aliases(inline, 42)
    check(
        "fix_aliases completes inline aliases",
        got_inline == "---\ntitle: Test\naliases: [ADR-42, ADR-000042]\n---\nBody\n",
        repr(got_inline),
    )

    already = "---\naliases: [ADR-42, ADR-000042]\n---\nBody\n"
    check(
        "fix_aliases leaves complete inline aliases unchanged",
        mod.fix_aliases(already, 42) == already,
    )

    multiline = "---\ntitle: Test\naliases:\n  - ADR-42\n---\nBody\n"
    got_multiline = mod.fix_aliases(multiline, 42)
    check(
        "fix_aliases completes multiline aliases at end of frontmatter",
        got_multiline == "---\ntitle: Test\naliases:\n  - ADR-42\n  - ADR-000042\n---\nBody\n",
        repr(got_multiline),
    )


def main() -> int:
    """Run all helper tests."""
    test_add_backlinks_appends_new_section()
    test_add_backlinks_replaces_nashi_and_inserts_into_existing()
    test_fix_aliases_inline_and_multiline()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
