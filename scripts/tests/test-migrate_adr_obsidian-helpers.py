#!/usr/bin/env python3
"""Unit tests for pure helpers in scripts/migrate_adr_obsidian.py.

Run:
    python3 scripts/tests/test-migrate_adr_obsidian-helpers.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import pathlib
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "migrate_adr_obsidian.py"

spec = importlib.util.spec_from_file_location("migrate_adr_obsidian", SCRIPT_PATH)
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


def test_convert_rest_to_wikilink_patterns() -> None:
    """Covers patterns A through E in priority order."""
    check(
        "pattern B markdown link",
        mod._convert_rest_to_wikilink("[ADR-029: Auth Flow](./000029.md) extra")
        == "[[000029]] Auth Flow extra",
    )
    check(
        "pattern C bold ADR",
        mod._convert_rest_to_wikilink("**ADR-265**: Cache Strategy**")
        == "[[000265]] Cache Strategy",
    )
    check(
        "pattern A parenthetical",
        mod._convert_rest_to_wikilink("ADR-139 (Recap Flow)") == "[[000139]] Recap Flow",
    )
    check(
        "pattern D colon separator",
        mod._convert_rest_to_wikilink("ADR-93: Feed Poller") == "[[000093]] Feed Poller",
    )
    check(
        "pattern E standalone",
        mod._convert_rest_to_wikilink("ADR-7") == "[[000007]]",
    )


def test_convert_related_links_respects_sections() -> None:
    """Converts only bullets in Related sections and stops at excluded or peer headings."""
    src = (
        "## Context\n"
        "- ADR-10: Should stay untouched\n"
        "\n"
        "## Related ADRs\n"
        "- なし\n"
        "- [ADR-029: Auth Flow](./000029.md)\n"
        "  * ADR-139 (Recap Flow)\n"
        "\n"
        "## 関連ファイル\n"
        "- ADR-99: Excluded section\n"
    )
    got = mod.convert_related_links(src)
    expected = (
        "## Context\n"
        "- ADR-10: Should stay untouched\n"
        "\n"
        "## Related ADRs\n"
        "- なし\n"
        "- [[000029]] Auth Flow\n"
        "  * [[000139]] Recap Flow\n"
        "\n"
        "## 関連ファイル\n"
        "- ADR-99: Excluded section\n"
    )
    check("convert_related_links converts only Related section items", got == expected, repr(got))
    check(
        "convert_related_links returns None when unchanged",
        mod.convert_related_links("## Context\n- ADR-10: Plain\n") is None,
    )


def test_process_files_in_temp_dir() -> None:
    """Verifies process_files dry-run, aliases_only, links_only, and live updates in a temp dir."""
    sample = "---\ntitle: Sample\n---\n## Related ADRs\n- ADR-12: Sample Ref\n"
    with tempfile.TemporaryDirectory() as tmp:
        tmp_dir = pathlib.Path(tmp)
        adr_file = tmp_dir / "000042.md"
        adr_file.write_text(sample, encoding="utf-8")
        ignored = tmp_dir / "000readme.md"
        ignored.write_text(sample, encoding="utf-8")

        orig_dir = mod.ADR_DIR
        mod.ADR_DIR = tmp_dir
        try:
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                mod.process_files(dry_run=True)
            out = buf.getvalue()
            check("dry-run leaves file unchanged", adr_file.read_text(encoding="utf-8") == sample)
            check(
                "dry-run reports aliases and links",
                "[aliases] 000042.md" in out and "[links]   000042.md" in out,
                out,
            )

            buf2 = io.StringIO()
            with contextlib.redirect_stdout(buf2):
                mod.process_files(dry_run=False, aliases_only=True)
            after_aliases = adr_file.read_text(encoding="utf-8")
            check(
                "aliases_only adds aliases without converting links",
                "aliases: [ADR-42, ADR-000042]" in after_aliases
                and "- ADR-12: Sample Ref" in after_aliases,
                after_aliases,
            )

            buf3 = io.StringIO()
            with contextlib.redirect_stdout(buf3):
                mod.process_files(dry_run=False, links_only=True)
            after_links = adr_file.read_text(encoding="utf-8")
            check(
                "links_only converts related links",
                "[[000012]] Sample Ref" in after_links,
                after_links,
            )
            check(
                "non-numeric filename skipped",
                ignored.read_text(encoding="utf-8") == sample,
            )
        finally:
            mod.ADR_DIR = orig_dir


def main() -> int:
    """Run all helper tests."""
    test_convert_rest_to_wikilink_patterns()
    test_convert_related_links_respects_sections()
    test_process_files_in_temp_dir()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
