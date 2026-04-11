#!/usr/bin/env python3
"""Migrate ADR files for Obsidian compatibility.

Operations:
1. Add `aliases` to frontmatter of ADRs that lack them
2. Convert Related ADRs references to wikilink format
"""

import re
import sys
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
PATTERN_A = re.compile(
    r"ADR[- ]0*(\d+)\s*\(([^)]+)\)"
)
# (B) `[ADR-029: タイトル](./000029.md)` or `[ADR 98: タイトル](000098.md)`
PATTERN_B = re.compile(
    r"\[ADR[- ]0*(\d+):?\s*([^\]]+)\]\(\.?/?0*\d+\.md\)"
)
# (C) `**ADR-265**: タイトル` or `**ADR-000265**: タイトル`
PATTERN_C = re.compile(
    r"\*\*ADR[- ]0*(\d+)\*\*:?\s*(.*)"
)
# (D) `ADR-167: タイトル` or `ADR 093: タイトル` (colon separator, no parens/bold/link)
PATTERN_D = re.compile(
    r"ADR[- ]0*(\d+):\s*(.*)"
)
# (E) `ADR-000139` or `ADR-139` standalone (no title after)
PATTERN_E = re.compile(
    r"ADR[- ]0*(\d+)$"
)


def parse_frontmatter(content: str) -> tuple[dict[str, str], int, int]:
    """Parse YAML frontmatter, return (fields_dict, start_pos, end_pos).

    Returns simple key->raw_value mapping. Only handles flat YAML fields
    and list fields on the same line (e.g., `tags: [a, b]`).
    """
    lines = content.split("\n")
    if not lines or lines[0].strip() != "---":
        return {}, 0, 0

    end_idx = None
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            end_idx = i
            break

    if end_idx is None:
        return {}, 0, 0

    fields = {}
    for i in range(1, end_idx):
        line = lines[i]
        if ":" in line and not line.startswith(" ") and not line.startswith("\t"):
            key, _, val = line.partition(":")
            fields[key.strip()] = val.strip()

    return fields, 0, end_idx


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


def convert_related_links(content: str) -> str | None:
    """Convert ADR references in Related sections to wikilinks. Returns modified content or None."""
    lines = content.split("\n")
    modified = False
    in_related_section = False
    related_section_level = 0  # 2 for ##, 3 for ###

    for i, line in enumerate(lines):
        stripped = line.strip()

        # Check for section headers (## or ###)
        if stripped.startswith("### ") or stripped.startswith("## "):
            header_level = 3 if stripped.startswith("### ") else 2
            if EXCLUDED_SECTION_RE.match(stripped):
                in_related_section = False
                continue
            if RELATED_SECTION_RE.match(stripped):
                in_related_section = True
                related_section_level = header_level
                continue
            # Any header at same or higher level ends the related section
            if in_related_section and header_level <= related_section_level:
                in_related_section = False
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
        rest = line[len(bullet_prefix):]

        new_rest = None

        # Try Pattern B first (markdown links) - most specific
        m = PATTERN_B.search(rest)
        if m:
            num = int(m.group(1))
            title = m.group(2).strip()
            padded = f"{num:06d}"
            after = rest[m.end():].strip()
            new_rest = f"[[{padded}]] {title} {after}".rstrip() if after else f"[[{padded}]] {title}"

        # Try Pattern C (bold ADR)
        if new_rest is None:
            m = PATTERN_C.search(rest)
            if m:
                num = int(m.group(1))
                title = m.group(2).strip()
                padded = f"{num:06d}"
                # Remove trailing bold markers if present
                title = title.rstrip("*").strip()
                new_rest = f"[[{padded}]] {title}"

        # Try Pattern A (parenthetical)
        if new_rest is None:
            m = PATTERN_A.search(rest)
            if m:
                num = int(m.group(1))
                title = m.group(2).strip()
                padded = f"{num:06d}"
                after = rest[m.end():].strip()
                new_rest = f"[[{padded}]] {title} {after}".rstrip() if after else f"[[{padded}]] {title}"

        # Try Pattern D (colon separator)
        if new_rest is None:
            m = PATTERN_D.search(rest)
            if m:
                num = int(m.group(1))
                title = m.group(2).strip()
                padded = f"{num:06d}"
                new_rest = f"[[{padded}]] {title}"

        # Try Pattern E (standalone ADR number, no title)
        if new_rest is None:
            m = PATTERN_E.search(rest.strip())
            if m:
                num = int(m.group(1))
                padded = f"{num:06d}"
                new_rest = f"[[{padded}]]"

        if new_rest is not None:
            new_line = bullet_prefix + new_rest
            if new_line != line:
                lines[i] = new_line
                modified = True

    if modified:
        return "\n".join(lines)
    return None


def process_files(dry_run: bool = False, aliases_only: bool = False, links_only: bool = False):
    """Process all ADR files."""
    adr_files = sorted(ADR_DIR.glob("0*.md"))

    alias_count = 0
    link_file_count = 0
    link_ref_count = 0
    errors = []

    for path in adr_files:
        filename = path.name
        # Extract ADR number from filename (e.g., 000139.md -> 139)
        num_match = re.match(r"^0*(\d+)\.md$", filename)
        if not num_match:
            continue
        adr_num = int(num_match.group(1))

        try:
            content = path.read_text(encoding="utf-8")
        except Exception as e:
            errors.append(f"ERROR reading {filename}: {e}")
            continue

        original = content

        # Step 1: Add aliases
        if not links_only:
            result = add_aliases(content, adr_num)
            if result is not None:
                content = result
                alias_count += 1
                if dry_run:
                    print(f"[aliases] {filename}: would add aliases [ADR-{adr_num}, ADR-{adr_num:06d}]")

        # Step 2: Convert related links
        if not aliases_only:
            result = convert_related_links(content)
            if result is not None:
                # Count converted references
                old_lines = content.split("\n")
                new_lines = result.split("\n")
                ref_count = sum(1 for a, b in zip(old_lines, new_lines) if a != b)
                link_file_count += 1
                link_ref_count += ref_count
                content = result
                if dry_run:
                    print(f"[links]   {filename}: would convert {ref_count} references")
                    for a, b in zip(old_lines, new_lines):
                        if a != b:
                            print(f"          - {a.strip()}")
                            print(f"          + {b.strip()}")

        # Write if changed
        if content != original and not dry_run:
            path.write_text(content, encoding="utf-8")

    # Summary
    print(f"\n{'[DRY RUN] ' if dry_run else ''}Summary:")
    print(f"  Aliases added: {alias_count} files")
    print(f"  Links converted: {link_ref_count} references in {link_file_count} files")
    if errors:
        print(f"  Errors: {len(errors)}")
        for e in errors:
            print(f"    {e}")


def main():
    args = sys.argv[1:]
    dry_run = "--dry-run" in args
    aliases_only = "--aliases-only" in args
    links_only = "--links-only" in args

    if aliases_only and links_only:
        print("Cannot use --aliases-only and --links-only together")
        sys.exit(1)

    print(f"ADR directory: {ADR_DIR}")
    print(f"Mode: {'dry-run' if dry_run else 'live'}")
    if aliases_only:
        print("Operation: aliases only")
    elif links_only:
        print("Operation: links only")
    else:
        print("Operation: aliases + links")
    print()

    process_files(dry_run=dry_run, aliases_only=aliases_only, links_only=links_only)


if __name__ == "__main__":
    main()
