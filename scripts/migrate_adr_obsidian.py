#!/usr/bin/env python3
"""Migrate ADR files for Obsidian compatibility.

Operations:
1. Add `aliases` to frontmatter of ADRs that lack them
2. Convert Related ADRs references to wikilink format
"""

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

ADR_DIR = Path(__file__).resolve().parent.parent / "docs" / "ADR"

# Section headers that contain ADR references (Related ADRs sections)
# Matches both ## and ### levels
RELATED_SECTION_RE = re.compile(
    r"^#{2,3}\s+(Related\s*ADRs?|関連\s*ADRs?|関連する\s*ADRs?|関連するアーキテクチャ決定|関連)$"
)

# Section headers to EXCLUDE (these contain file/commit references, not ADR links)
EXCLUDED_SECTION_RE = re.compile(
    r"^#{2,3}\s+(関連ファイル|関連リソース|関連コード|関連コンポーネント|関連する型定義"
    r"|関連マイグレーション|関連する将来的な作業|関連ログ|関連技術|関連パイプライン"
    r"|関連する既存設計|関連ファイル一覧|参考コミット|参考リンク)"
)

# Patterns for ADR references in Related sections
# (A) `ADR-000139 (タイトル)` or `ADR-139 (タイトル)`
PATTERN_A = re.compile(r"ADR[- ]0*(\d+)\s*\(([^)]+)\)")
# (B) `[ADR-029: タイトル](./000029.md)` or `[ADR 98: タイトル](000098.md)`
PATTERN_B = re.compile(r"\[ADR[- ]0*(\d+):?\s*([^\]]+)\]\(\.?/?0*\d+\.md\)")
# (C) `**ADR-265**: タイトル` or `**ADR-000265**: タイトル`
PATTERN_C = re.compile(r"\*\*ADR[- ]0*(\d+)\*\*:?\s*(.*)")
# (D) `ADR-167: タイトル` or `ADR 093: タイトル` (colon separator, no parens/bold/link)
PATTERN_D = re.compile(r"ADR[- ]0*(\d+):\s*(.*)")
# (E) `ADR-000139` or `ADR-139` standalone (no title after)
PATTERN_E = re.compile(r"ADR[- ]0*(\d+)$")


def add_aliases(content: str, adr_num: int) -> str | None:
    """Add aliases field to frontmatter if missing. Returns modified content or None."""
    lines = content.split("\n")
    if not lines or lines[0].strip() != "---":
        return None

    # Find end of frontmatter
    end_idx = None
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            end_idx = i
            break

    if end_idx is None:
        return None

    # Check if aliases already exists
    has_aliases = False
    insert_after = end_idx - 1  # Default: insert before closing ---

    for i in range(1, end_idx):
        line = lines[i]
        if line.startswith("aliases"):
            has_aliases = True
            break
        # Insert after affected_services if present
        if line.startswith("affected_services"):
            insert_after = i
            # Skip multi-line list items
            for j in range(i + 1, end_idx):
                if lines[j].startswith("  - ") or lines[j].startswith("  -"):
                    insert_after = j
                else:
                    break

    if has_aliases:
        return None

    short_num = str(adr_num)
    padded_num = f"{adr_num:06d}"
    alias_line = f"aliases: [ADR-{short_num}, ADR-{padded_num}]"

    lines.insert(insert_after + 1, alias_line)
    return "\n".join(lines)


def _update_related_section_state(
    stripped: str, in_related_section: bool, related_section_level: int
) -> tuple[bool, int, bool]:
    """Update Related section tracking state for a stripped line.

    Returns (in_related_section, related_section_level, is_header_consumed).
    """
    if stripped.startswith("### ") or stripped.startswith("## "):
        header_level = 3 if stripped.startswith("### ") else 2
        if EXCLUDED_SECTION_RE.match(stripped):
            return False, related_section_level, True
        if RELATED_SECTION_RE.match(stripped):
            return True, header_level, True
        if in_related_section and header_level <= related_section_level:
            return False, related_section_level, True
    return in_related_section, related_section_level, False


def _convert_rest_to_wikilink(rest: str) -> str | None:
    """Convert an ADR reference bullet body into [[000NNN]] wikilink format."""
    if m := PATTERN_B.search(rest):
        num = int(m.group(1))
        title = m.group(2).strip()
        after = rest[m.end() :].strip()
        return f"[[{num:06d}]] {title} {after}".rstrip() if after else f"[[{num:06d}]] {title}"

    if m := PATTERN_C.search(rest):
        num = int(m.group(1))
        title = m.group(2).strip().rstrip("*").strip()
        return f"[[{num:06d}]] {title}"

    if m := PATTERN_A.search(rest):
        num = int(m.group(1))
        title = m.group(2).strip()
        after = rest[m.end() :].strip()
        return f"[[{num:06d}]] {title} {after}".rstrip() if after else f"[[{num:06d}]] {title}"

    if m := PATTERN_D.search(rest):
        num = int(m.group(1))
        title = m.group(2).strip()
        return f"[[{num:06d}]] {title}"

    if m := PATTERN_E.search(rest.strip()):
        num = int(m.group(1))
        return f"[[{num:06d}]]"

    return None


def convert_related_links(content: str) -> str | None:
    """Convert ADR references in Related sections to wikilinks. Returns modified content or None."""
    lines = content.split("\n")
    modified = False
    in_related_section = False
    related_section_level = 0  # 2 for ##, 3 for ###

    for i, line in enumerate(lines):
        stripped = line.strip()

        in_related_section, related_section_level, consumed = _update_related_section_state(
            stripped, in_related_section, related_section_level
        )
        if consumed:
            continue

        if not in_related_section:
            continue

        # Skip non-reference lines
        if not stripped.startswith("-") and not stripped.startswith("*"):
            continue

        # Skip "- なし" lines
        if stripped in ("- なし", "* なし", "- None", "- none"):
            continue

        # Extract leading whitespace and bullet
        bullet_match = re.match(r"^(\s*[-*]\s*)", line)
        if not bullet_match:
            continue
        bullet_prefix = bullet_match.group(1)
        rest = line[len(bullet_prefix) :]

        new_rest = _convert_rest_to_wikilink(rest)

        if new_rest is not None:
            new_line = bullet_prefix + new_rest
            if new_line != line:
                lines[i] = new_line
                modified = True

    if modified:
        return "\n".join(lines)
    return None


@dataclass(frozen=True)
class _MigrationOptions:
    """Options controlling ADR migration behavior."""

    dry_run: bool = False
    aliases_only: bool = False
    links_only: bool = False


@dataclass
class _MigrationSummary:
    """Counters and errors accumulated across processed ADR files."""

    alias_count: int = 0
    link_file_count: int = 0
    link_ref_count: int = 0
    errors: list[str] = field(default_factory=list)


def _apply_aliases_step(
    filename: str, content: str, adr_num: int, summary: _MigrationSummary, *, dry_run: bool
) -> str:
    """Apply frontmatter alias insertion and update summary counters."""
    result = add_aliases(content, adr_num)
    if result is None:
        return content
    summary.alias_count += 1
    if dry_run:
        sys.stdout.write(
            f"[aliases] {filename}: would add aliases [ADR-{adr_num}, ADR-{adr_num:06d}]\n"
        )
    return result


def _apply_links_step(
    filename: str, content: str, summary: _MigrationSummary, *, dry_run: bool
) -> str:
    """Apply Related-section wikilink conversion and update summary counters."""
    result = convert_related_links(content)
    if result is None:
        return content
    old_lines = content.split("\n")
    new_lines = result.split("\n")
    ref_count = sum(1 for a, b in zip(old_lines, new_lines, strict=True) if a != b)
    summary.link_file_count += 1
    summary.link_ref_count += ref_count
    if dry_run:
        sys.stdout.write(f"[links]   {filename}: would convert {ref_count} references\n")
        for a, b in zip(old_lines, new_lines, strict=True):
            if a != b:
                sys.stdout.write(f"          - {a.strip()}\n")
                sys.stdout.write(f"          + {b.strip()}\n")
    return result


def _process_single_file(
    path: Path, summary: _MigrationSummary, options: _MigrationOptions
) -> None:
    """Process one ADR file, updating summary counts and writing if changed."""
    filename = path.name
    num_match = re.match(r"^0*(\d+)\.md$", filename)
    if not num_match:
        return
    adr_num = int(num_match.group(1))

    try:
        content = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as e:
        summary.errors.append(f"ERROR reading {filename}: {e}")
        return

    original = content

    if not options.links_only:
        content = _apply_aliases_step(filename, content, adr_num, summary, dry_run=options.dry_run)

    if not options.aliases_only:
        content = _apply_links_step(filename, content, summary, dry_run=options.dry_run)

    if content != original and not options.dry_run:
        path.write_text(content, encoding="utf-8")


def _print_migration_summary(summary: _MigrationSummary, *, dry_run: bool) -> None:
    """Write migration totals and any read errors to stdout."""
    sys.stdout.write(f"\n{'[DRY RUN] ' if dry_run else ''}Summary:\n")
    sys.stdout.write(f"  Aliases added: {summary.alias_count} files\n")
    sys.stdout.write(
        f"  Links converted: {summary.link_ref_count} references in {summary.link_file_count} files\n"
    )
    if summary.errors:
        sys.stdout.write(f"  Errors: {len(summary.errors)}\n")
        for e in summary.errors:
            sys.stdout.write(f"    {e}\n")


def process_files(
    dry_run: bool = False, aliases_only: bool = False, links_only: bool = False
) -> None:
    """Process all ADR files."""
    adr_files = sorted(ADR_DIR.glob("0*.md"))
    options = _MigrationOptions(dry_run=dry_run, aliases_only=aliases_only, links_only=links_only)
    summary = _MigrationSummary()

    for path in adr_files:
        _process_single_file(path, summary, options)

    _print_migration_summary(summary, dry_run=dry_run)


def main() -> None:
    parser = argparse.ArgumentParser(description="Migrate ADR files for Obsidian compatibility")
    parser.add_argument("--dry-run", action="store_true", help="Preview changes without writing")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--aliases-only", action="store_true", help="Only add aliases")
    group.add_argument("--links-only", action="store_true", help="Only convert related links")
    args = parser.parse_args()

    sys.stdout.write(f"ADR directory: {ADR_DIR}\n")
    sys.stdout.write(f"Mode: {'dry-run' if args.dry_run else 'live'}\n")
    if args.aliases_only:
        sys.stdout.write("Operation: aliases only\n")
    elif args.links_only:
        sys.stdout.write("Operation: links only\n")
    else:
        sys.stdout.write("Operation: aliases + links\n")
    sys.stdout.write("\n")

    process_files(dry_run=args.dry_run, aliases_only=args.aliases_only, links_only=args.links_only)


if __name__ == "__main__":
    main()
