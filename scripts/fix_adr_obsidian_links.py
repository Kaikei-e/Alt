#!/usr/bin/env python3
"""
Fix ADR Obsidian wikilinks: inline refs, markdown links, backlinks, aliases.

Steps:
1. Build title index from frontmatter
2. Convert inline ADR-NNN references to [[000NNN|ADR-NNN]] wikilinks
3. Convert markdown links [ADR-000NNN](./000NNN.md) to [[000NNN]]
4. Fix "Related ADRs: なし" where references exist
5. Add bidirectional backlinks
6. Verify aliases consistency
"""

import re
import sys
from pathlib import Path

ADR_DIR = Path(__file__).resolve().parent.parent / "docs" / "ADR"

# --- Step 1: Build title index ---


def parse_frontmatter(text: str) -> dict[str, object]:
    """Extract frontmatter fields from ADR markdown."""
    if not text.startswith("---"):
        return {}
    end = text.find("---", 3)
    if end == -1:
        return {}
    fm = text[3:end]
    result = {}
    # title
    m = re.search(r"^title:\s*(.+)$", fm, re.MULTILINE)
    if m:
        result["title"] = m.group(1).strip().strip('"').strip("'")
    # aliases - handle both inline [A, B] and multiline - A\n- B
    m = re.search(r"^aliases:\s*\[([^\]]+)\]", fm, re.MULTILINE)
    if m:
        result["aliases"] = [a.strip() for a in m.group(1).split(",")]
    else:
        aliases = []
        in_aliases = False
        for line in fm.split("\n"):
            if re.match(r"^aliases:\s*$", line):
                in_aliases = True
                continue
            if in_aliases:
                m2 = re.match(r"^\s+-\s+(.+)$", line)
                if m2:
                    aliases.append(m2.group(1).strip())
                else:
                    break
        if aliases:
            result["aliases"] = aliases
    return result


def build_title_index() -> dict[int, str]:
    """Map ADR number -> title for all ADR files."""
    index = {}
    for f in sorted(ADR_DIR.glob("000*.md")):
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        fm = parse_frontmatter(text)
        title = fm.get("title", "")
        # Strip leading "ADR-000NNN: " or "ADR NNN/NNN " from title
        title = re.sub(r"^ADR[-\s]?\d+(/\d+)?[:\s]+", "", title).strip()
        index[num] = title
    return index


# --- Step 2: Convert inline ADR references to wikilinks ---


def pad6(n: int) -> str:
    return f"{n:06d}"


FRONTMATTER_CLOSE_COUNT = 2


def convert_inline_refs(text: str, title_index: dict[int, str]) -> str:
    """Convert plain-text ADR references to wikilinks in body text.

    Handles:
    - ADR-98 -> [[000098|ADR-98]]
    - ADR 98 -> [[000098|ADR 98]]
    - ADR-000098 -> [[000098|ADR-000098]]
    - ADR-98/99 -> [[000098|ADR-98]]/[[000099|99]]
    - ADR 98/99 -> [[000098|ADR 98]]/[[000099|99]]
    """
    lines = text.split("\n")
    result_lines = []

    in_frontmatter = False
    frontmatter_count = 0
    in_code_block = False
    in_related = False

    for line in lines:
        # Track frontmatter
        if line.strip() == "---":
            frontmatter_count += 1
            if frontmatter_count == 1:
                in_frontmatter = True
            elif frontmatter_count == FRONTMATTER_CLOSE_COUNT:
                in_frontmatter = False
            result_lines.append(line)
            continue

        if in_frontmatter:
            result_lines.append(line)
            continue

        # Track code blocks
        if line.strip().startswith("```"):
            in_code_block = not in_code_block
            result_lines.append(line)
            continue

        if in_code_block:
            result_lines.append(line)
            continue

        # Track Related/APPENDIX sections
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$",
            line,
            re.IGNORECASE,
        ):
            in_related = True
            result_lines.append(line)
            continue
        if in_related and re.match(r"^#{2,}\s", line):
            in_related = False

        # Skip heading lines like "# ADR-000NNN: ..."
        if re.match(r"^#{1,6}\s+ADR", line):
            result_lines.append(line)
            continue

        # Skip wikilink list items in Related sections (- [[000NNN]] description)
        if in_related and re.match(r"^-\s+\[\[\d{6}", line):
            result_lines.append(line)
            continue

        # Process the line for ADR references
        result_lines.append(_convert_line_refs(line, title_index))

    return "\n".join(result_lines)


def _convert_line_refs(line: str, title_index: dict[int, str]) -> str:
    """Convert ADR references in a single line, handling slash-separated refs."""

    # Pattern for ADR references with optional slash-separated additional numbers
    # Matches: ADR-98, ADR 98, ADR-000098, ADR-98/99, ADR 98/99
    # Does NOT match if already inside [[ ]]
    # Does NOT match markdown links [ADR-...](...)

    # First, protect existing wikilinks and markdown links by replacing them with placeholders
    placeholders = []

    def save_placeholder(m: re.Match[str]) -> str:
        placeholders.append(m.group(0))
        return f"\x00PH{len(placeholders) - 1}\x00"

    # Protect wikilinks [[...]]
    protected = re.sub(r"\[\[[^\]]+\]\]", save_placeholder, line)
    # Protect markdown links [...](...)
    protected = re.sub(r"\[[^\]]*\]\([^\)]*\)", save_placeholder, protected)

    # Now convert ADR references
    # Pattern: ADR[-\s](\d{1,6})(/\d{1,6})*
    def replace_adr_ref(m: re.Match[str]) -> str:
        prefix = m.group(1)  # "ADR-" or "ADR "
        first_num_str = m.group(2)  # "98" or "000098"
        slash_part = m.group(3) or ""  # "/99" or "/99/100" or ""

        first_num = int(first_num_str)
        if first_num not in title_index:
            return m.group(0)  # Unknown ADR, leave as-is

        # Determine display text for first ref
        display_first = f"{prefix}{first_num_str}"
        result = f"[[{pad6(first_num)}|{display_first}]]"

        # Handle slash-separated numbers
        if slash_part:
            parts = slash_part.split("/")
            for part in parts:
                if not part:
                    continue
                sub_num = int(part)
                if sub_num in title_index:
                    result += f"/[[{pad6(sub_num)}|{part}]]"
                else:
                    result += f"/{part}"

        return result

    # Match ADR- or ADR followed by space, then digits, optionally /digits
    converted = re.sub(r"(ADR[-])(\d{1,6})((?:/\d{1,6})*)", replace_adr_ref, protected)
    converted = re.sub(r"(ADR )(\d{1,6})((?:/\d{1,6})*)", replace_adr_ref, converted)

    # Restore placeholders
    for i, ph in enumerate(placeholders):
        converted = converted.replace(f"\x00PH{i}\x00", ph)

    return converted


# --- Step 3: Markdown link -> wikilink conversion ---


def convert_markdown_links(text: str, title_index: dict[int, str]) -> str:
    """Convert [ADR-000NNN](./000NNN.md) to [[000NNN]] or [[000NNN]] Title."""

    def replace_md_link(m: re.Match[str]) -> str:
        full_match = m.group(0)
        link_num_str = m.group(1)  # "000235"
        num = int(link_num_str)
        if num not in title_index:
            return full_match
        return f"[[{pad6(num)}]]"

    # Pattern: [ADR-000NNN](./000NNN.md) or [ADR-NNN](./000NNN.md)
    return re.sub(r"\[ADR-\d+\]\(\./(\d{6})\.md\)", replace_md_link, text)


def convert_related_section_md_links(text: str, title_index: dict[int, str]) -> str:
    """In Related/APPENDIX sections, convert full markdown link lines.

    - 関連 ADR: [ADR-000235](./000235.md) — Title
    becomes:
    - [[000235]] Title
    """
    lines = text.split("\n")
    result = []
    in_related = False

    for line in lines:
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$",
            line,
            re.IGNORECASE,
        ):
            in_related = True
            result.append(line)
            continue

        if in_related and re.match(r"^#{2,}\s", line):
            in_related = False

        out_line = line
        if in_related:
            # Match patterns like "- 関連 ADR: [ADR-000235](./000235.md) — Title"
            m = re.match(
                r"^-\s+(?:関連\s*(?:ADR)?:\s*)?\[ADR-\d+\]\(\./(\d{6})\.md\)\s*(?:—\s*(.*))?$", line
            )
            if m:
                num = int(m.group(1))
                desc = m.group(2)
                if num in title_index:
                    title = desc.strip() if desc else title_index[num]
                    out_line = f"- [[{pad6(num)}]] {title}"

        result.append(out_line)

    return "\n".join(result)


# --- Step 4: Fix "Related ADRs: なし" ---


def extract_body_refs(text: str, self_num: int, title_index: dict[int, str]) -> set[int]:
    """Extract all ADR numbers referenced in the body (wikilinks + inline)."""
    refs = set()
    # Wikilinks: [[000NNN...]]
    for m in re.finditer(r"\[\[(\d{6})", text):
        num = int(m.group(1))
        if num != self_num and num in title_index:
            refs.add(num)
    # Inline: ADR-NNN or ADR NNN (not in frontmatter)
    # We parse after frontmatter
    body = text
    if text.startswith("---"):
        end = text.find("---", 3)
        if end != -1:
            body = text[end + 3 :]
    for m in re.finditer(r"ADR[-\s](\d{1,6})", body):
        num = int(m.group(1))
        if num != self_num and num in title_index:
            refs.add(num)
    return refs


def _is_nashi_line(line: str) -> bool:
    """Check if line is a '- なし' entry (with or without parenthetical explanation)."""
    stripped = line.strip()
    return stripped == "- なし" or re.match(r"^- なし（.+）$", stripped) is not None


def _is_in_related_section(lines: list[str], idx: int) -> bool:
    """Check if the line at idx is inside a Related/APPENDIX subsection."""
    for j in range(idx - 1, -1, -1):
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連\s*ADR|関連ファイル|関連)\s*$", lines[j], re.IGNORECASE
        ):
            return True
        if re.match(r"^#{1,}\s", lines[j]):
            return False
    return False


def fix_nashi_entries(
    text: str, _self_num: int, body_refs: set[int], title_index: dict[int, str]
) -> str:
    """Replace '- なし' with actual references if body contains ADR refs."""
    if not body_refs:
        return text

    lines = text.split("\n")
    result = []
    for i, line in enumerate(lines):
        if _is_nashi_line(line) and _is_in_related_section(lines, i):
            # Replace with actual refs
            for ref_num in sorted(body_refs):
                title = title_index.get(ref_num, "")
                result.append(f"- [[{pad6(ref_num)}]] {title}")
            continue
        result.append(line)

    return "\n".join(result)


# --- Step 5: Bidirectional backlinks ---


def build_reference_graph(title_index: dict[int, str]) -> dict[int, set[int]]:
    """Build a graph of who references whom from Related/APPENDIX sections."""
    graph: dict[int, set[int]] = {n: set() for n in title_index}

    for f in sorted(ADR_DIR.glob("000*.md")):
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        lines = text.split("\n")

        in_related = False
        for line in lines:
            if re.match(
                r"^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$",
                line,
                re.IGNORECASE,
            ):
                in_related = True
                continue
            if in_related and re.match(r"^#{2,}\s", line):
                in_related = False
            if in_related:
                for m in re.finditer(r"\[\[(\d{6})", line):
                    ref_num = int(m.group(1))
                    if ref_num != num and ref_num in title_index:
                        graph[num].add(ref_num)

    return graph


def get_existing_related_refs(text: str) -> set[int]:
    """Get ADR numbers already listed in Related/APPENDIX section."""
    refs = set()
    lines = text.split("\n")
    in_related = False
    for line in lines:
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$",
            line,
            re.IGNORECASE,
        ):
            in_related = True
            continue
        if in_related and re.match(r"^#{2,}\s", line):
            in_related = False
        if in_related:
            for m in re.finditer(r"\[\[(\d{6})", line):
                refs.add(int(m.group(1)))
    return refs


def _find_related_section_bounds(lines: list[str]) -> tuple[int | None, int | None]:
    """Return (start_idx, next_heading_idx) for the last Related/APPENDIX heading."""
    related_section_idx = None
    next_section_idx = None

    for i, line in enumerate(lines):
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$",
            line,
            re.IGNORECASE,
        ):
            related_section_idx = i
            next_section_idx = None  # Reset: find next heading after THIS match

    if related_section_idx is not None:
        for i in range(related_section_idx + 1, len(lines)):
            if re.match(r"^#{2,}\s", lines[i]):
                next_section_idx = i
                break

    return related_section_idx, next_section_idx


def _append_related_section(
    lines: list[str], to_add: list[int], title_index: dict[int, str]
) -> str:
    """Append a new '## Related ADRs' section with backlinks at the end of the document."""
    result = lines[:]
    while result and result[-1].strip() == "":
        result.pop()
    result.append("")
    result.append("## Related ADRs")
    result.append("")
    for num in to_add:
        title = title_index.get(num, "")
        result.append(f"- [[{pad6(num)}]] {title}")
    result.append("")
    return "\n".join(result)


def _insert_backlinks_in_section(
    lines: list[str],
    related_section_idx: int,
    next_section_idx: int | None,
    to_add: list[int],
    title_index: dict[int, str],
) -> str:
    """Insert backlink bullets into an existing Related/APPENDIX section."""
    end = next_section_idx if next_section_idx else len(lines)
    last_content = related_section_idx
    for i in range(related_section_idx + 1, end):
        if lines[i].strip().startswith("<!--"):
            continue
        if lines[i].strip() and not lines[i].startswith("##"):
            last_content = i

    nashi_idx = None
    for i in range(related_section_idx + 1, end):
        if _is_nashi_line(lines[i]):
            nashi_idx = i

    backlink_lines = [f"- [[{pad6(num)}]] {title_index.get(num, '')}" for num in to_add]
    if nashi_idx is not None:
        result = lines[:nashi_idx]
        result.extend(backlink_lines)
        result.extend(lines[nashi_idx + 1 :])
        return "\n".join(result)

    insert_at = last_content + 1
    result = lines[:insert_at]
    result.extend(backlink_lines)
    result.extend(lines[insert_at:])
    return "\n".join(result)


def add_backlinks(
    text: str, _self_num: int, backlinks: set[int], title_index: dict[int, str]
) -> str:
    """Add missing backlinks to the Related/APPENDIX section."""
    if not backlinks:
        return text

    existing = get_existing_related_refs(text)
    to_add = sorted(backlinks - existing)
    if not to_add:
        return text

    lines = text.split("\n")
    related_section_idx, next_section_idx = _find_related_section_bounds(lines)

    if related_section_idx is None:
        return _append_related_section(lines, to_add, title_index)

    return _insert_backlinks_in_section(
        lines, related_section_idx, next_section_idx, to_add, title_index
    )


# --- Step 6: Aliases consistency ---


def _fix_inline_aliases(fm: str, match: re.Match[str], short: str, padded: str) -> str | None:
    """Update inline `aliases: [...]` in frontmatter, or return None if unchanged."""
    current = [a.strip() for a in match.group(1).split(",")]
    needed = []
    if short not in current:
        needed.append(short)
    if padded not in current:
        needed.append(padded)
    if not needed:
        return None
    new_aliases = current + needed
    new_line = f"aliases: [{', '.join(new_aliases)}]"
    return fm[: match.start()] + new_line + fm[match.end() :]


def _fix_multiline_aliases(fm: str, short: str, padded: str) -> str:
    """Ensure multiline `aliases:` block in frontmatter includes short and padded forms."""
    lines = fm.split("\n")
    new_lines = []
    in_aliases = False
    alias_values: list[str] = []

    for line in lines:
        if re.match(r"^aliases:\s*$", line):
            in_aliases = True
            new_lines.append(line)
            continue
        if in_aliases:
            m2 = re.match(r"^(\s+)-\s+(.+)$", line)
            if m2:
                alias_values.append(m2.group(2).strip())
                new_lines.append(line)
                continue
            in_aliases = False
            if short not in alias_values:
                new_lines.append(f"  - {short}")
            if padded not in alias_values:
                new_lines.append(f"  - {padded}")
        new_lines.append(line)

    if in_aliases:
        if short not in alias_values:
            new_lines.append(f"  - {short}")
        if padded not in alias_values:
            new_lines.append(f"  - {padded}")

    return "\n".join(new_lines)


def fix_aliases(text: str, num: int) -> str:
    """Ensure aliases contain both ADR-N and ADR-000NNN forms."""
    if not text.startswith("---"):
        return text
    end = text.find("---", 3)
    if end == -1:
        return text

    fm = text[3:end]
    short = f"ADR-{num}"
    padded = f"ADR-{num:06d}"

    m = re.search(r"^aliases:\s*\[([^\]]*)\]", fm, re.MULTILINE)
    if m:
        new_fm = _fix_inline_aliases(fm, m, short, padded)
        if new_fm is None:
            return text
        return f"---{new_fm}---{text[end + 3 :]}"

    new_fm = _fix_multiline_aliases(fm, short, padded)
    return f"---{new_fm}---{text[end + 3 :]}"


def remove_nashi_if_siblings_exist(text: str) -> str:
    """Remove '- なし(...)' from Related sections when wikilink entries exist alongside."""
    lines = text.split("\n")
    in_related = False
    related_start = -1

    # Two-pass: first identify Related sections with both なし and wikilinks
    sections: list[tuple[int, int]] = []  # (start, end) of related sections
    for i, line in enumerate(lines):
        if re.match(
            r"^#{2,3}\s+(Related ADRs|関連\s*ADR|関連ファイル|関連)\s*$", line, re.IGNORECASE
        ):
            in_related = True
            related_start = i
            continue
        if in_related and re.match(r"^#{1,}\s", line):
            sections.append((related_start, i))
            in_related = False
    if in_related:
        sections.append((related_start, len(lines)))

    # Find lines to remove
    remove_lines: set[int] = set()
    for start, end in sections:
        has_wikilink = False
        nashi_lines = []
        for i in range(start + 1, end):
            if re.search(r"\[\[\d{6}", lines[i]):
                has_wikilink = True
            if _is_nashi_line(lines[i]):
                nashi_lines.append(i)
        if has_wikilink and nashi_lines:
            remove_lines.update(nashi_lines)

    if not remove_lines:
        return text

    return "\n".join(line for i, line in enumerate(lines) if i not in remove_lines)


# --- Main orchestration ---


def _process_adr_files(title_index: dict[int, str]) -> dict[Path, str]:
    """Run Steps 2-4 and alias fixes across all ADR files, writing changed files."""
    changed_files: dict[Path, str] = {}
    body_refs_map: dict[int, set[int]] = {}

    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        original = f.read_text(encoding="utf-8")
        text = original

        text = fix_aliases(text, num)
        text = convert_related_section_md_links(text, title_index)
        text = convert_markdown_links(text, title_index)
        text = convert_inline_refs(text, title_index)

        body_refs = extract_body_refs(text, num, title_index)
        body_refs_map[num] = body_refs

        text = fix_nashi_entries(text, num, body_refs, title_index)
        text = remove_nashi_if_siblings_exist(text)

        if text != original:
            changed_files[f] = text

    for f, text in changed_files.items():
        f.write_text(text, encoding="utf-8")

    sys.stdout.write(f"  Changed {len(changed_files)} files\n\n")
    return changed_files


def _add_all_backlinks(title_index: dict[int, str], changed_files: dict[Path, str]) -> None:
    """Compute reverse reference graph and write missing backlinks to ADR files."""
    ref_graph = build_reference_graph(title_index)

    reverse_graph: dict[int, set[int]] = {n: set() for n in title_index}
    for src, targets in ref_graph.items():
        for tgt in targets:
            reverse_graph[tgt].add(src)

    backlink_count = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")

        referrers = reverse_graph.get(num, set())
        existing = get_existing_related_refs(text)
        missing = referrers - existing - {num}

        if missing:
            new_text = add_backlinks(text, num, missing, title_index)
            if new_text != text:
                f.write_text(new_text, encoding="utf-8")
                backlink_count += 1
                if f not in changed_files:
                    changed_files[f] = new_text

    sys.stdout.write(f"  Added backlinks to {backlink_count} files\n\n")


def _check_aliases() -> None:
    """Verify every ADR frontmatter has both short and padded alias entries."""
    alias_issues = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        fm = parse_frontmatter(text)
        aliases = fm.get("aliases", [])
        short = f"ADR-{num}"
        padded = f"ADR-{num:06d}"
        if short not in aliases or padded not in aliases:
            alias_issues += 1
            sys.stdout.write(f"  WARNING: {f.name} missing alias (has: {aliases})\n")
    if alias_issues == 0:
        sys.stdout.write("  All aliases OK\n")


def _check_broken_wikilinks(title_index: dict[int, str]) -> None:
    """Report any [[000NNN]] wikilink pointing to an unknown ADR number."""
    broken = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        text = f.read_text(encoding="utf-8")
        for m in re.finditer(r"\[\[(\d{6})", text):
            ref = int(m.group(1))
            if ref not in title_index:
                sys.stdout.write(f"  BROKEN: {f.name} -> [[{m.group(1)}]]\n")
                broken += 1
    if broken == 0:
        sys.stdout.write("  No broken wikilinks\n")


def main() -> None:
    sys.stdout.write("=== ADR Obsidian Link Fixer ===\n\n")

    # Step 1: Build index
    sys.stdout.write("Step 1: Building title index...\n")
    title_index = build_title_index()
    sys.stdout.write(f"  Found {len(title_index)} ADRs\n\n")

    # Step 2-4: Process each file
    sys.stdout.write("Step 2-4: Converting inline refs, markdown links, fixing なし...\n")
    changed_files = _process_adr_files(title_index)

    # Step 5: Bidirectional backlinks
    sys.stdout.write("Step 5: Building reference graph and adding backlinks...\n")
    _add_all_backlinks(title_index, changed_files)

    # Summary
    sys.stdout.write("=== Summary ===\n")
    sys.stdout.write(f"Total files changed: {len(changed_files)}\n")

    # Alias check
    sys.stdout.write("\nStep 6: Aliases consistency check...\n")
    _check_aliases()

    # Check for broken wikilinks
    sys.stdout.write("\nBroken wikilink check...\n")
    _check_broken_wikilinks(title_index)

    sys.stdout.write(f"\nDone. {len(changed_files)} files modified total.\n")


if __name__ == "__main__":
    main()
