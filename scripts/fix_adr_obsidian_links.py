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
from pathlib import Path

ADR_DIR = Path(__file__).resolve().parent.parent / "docs" / "ADR"

# --- Step 1: Build title index ---

def parse_frontmatter(text: str) -> dict:
    """Extract frontmatter fields from ADR markdown."""
    if not text.startswith("---"):
        return {}
    end = text.find("---", 3)
    if end == -1:
        return {}
    fm = text[3:end]
    result = {}
    # title
    m = re.search(r'^title:\s*(.+)$', fm, re.MULTILINE)
    if m:
        result['title'] = m.group(1).strip().strip('"').strip("'")
    # aliases - handle both inline [A, B] and multiline - A\n- B
    m = re.search(r'^aliases:\s*\[([^\]]+)\]', fm, re.MULTILINE)
    if m:
        result['aliases'] = [a.strip() for a in m.group(1).split(',')]
    else:
        aliases = []
        in_aliases = False
        for line in fm.split('\n'):
            if re.match(r'^aliases:\s*$', line):
                in_aliases = True
                continue
            if in_aliases:
                m2 = re.match(r'^\s+-\s+(.+)$', line)
                if m2:
                    aliases.append(m2.group(1).strip())
                else:
                    break
        if aliases:
            result['aliases'] = aliases
    return result


def build_title_index() -> dict[int, str]:
    """Map ADR number -> title for all ADR files."""
    index = {}
    for f in sorted(ADR_DIR.glob("000*.md")):
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        fm = parse_frontmatter(text)
        title = fm.get('title', '')
        # Strip leading "ADR-000NNN: " or "ADR NNN/NNN " from title
        title = re.sub(r'^ADR[-\s]?\d+(/\d+)?[:\s]+', '', title).strip()
        index[num] = title
    return index


# --- Step 2: Convert inline ADR references to wikilinks ---

def pad6(n: int) -> str:
    return f"{n:06d}"


def find_related_section_start(lines: list[str]) -> int | None:
    """Find the line index of the Related ADRs / 関連 / APPENDIX section heading."""
    for i, line in enumerate(lines):
        if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
            return i
    return None


def is_in_frontmatter(text: str, match_start: int) -> bool:
    """Check if position is inside YAML frontmatter."""
    if not text.startswith("---"):
        return False
    end = text.find("---", 3)
    if end == -1:
        return False
    return match_start < end + 3


def convert_inline_refs(text: str, title_index: dict[int, str]) -> str:
    """Convert plain-text ADR references to wikilinks in body text.

    Handles:
    - ADR-98 -> [[000098|ADR-98]]
    - ADR 98 -> [[000098|ADR 98]]
    - ADR-000098 -> [[000098|ADR-000098]]
    - ADR-98/99 -> [[000098|ADR-98]]/[[000099|99]]
    - ADR 98/99 -> [[000098|ADR 98]]/[[000099|99]]
    """
    lines = text.split('\n')
    result_lines = []

    in_frontmatter = False
    frontmatter_count = 0
    in_code_block = False
    in_related = False

    for line in lines:
        # Track frontmatter
        if line.strip() == '---':
            frontmatter_count += 1
            if frontmatter_count == 1:
                in_frontmatter = True
            elif frontmatter_count == 2:
                in_frontmatter = False
            result_lines.append(line)
            continue

        if in_frontmatter:
            result_lines.append(line)
            continue

        # Track code blocks
        if line.strip().startswith('```'):
            in_code_block = not in_code_block
            result_lines.append(line)
            continue

        if in_code_block:
            result_lines.append(line)
            continue

        # Track Related/APPENDIX sections
        if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
            in_related = True
            result_lines.append(line)
            continue
        if in_related and re.match(r'^#{2,}\s', line):
            in_related = False

        # Skip heading lines like "# ADR-000NNN: ..."
        if re.match(r'^#{1,6}\s+ADR', line):
            result_lines.append(line)
            continue

        # Skip wikilink list items in Related sections (- [[000NNN]] description)
        if in_related and re.match(r'^-\s+\[\[\d{6}', line):
            result_lines.append(line)
            continue

        # Process the line for ADR references
        line = _convert_line_refs(line, title_index)
        result_lines.append(line)

    return '\n'.join(result_lines)


def _convert_line_refs(line: str, title_index: dict[int, str]) -> str:
    """Convert ADR references in a single line, handling slash-separated refs."""

    # Pattern for ADR references with optional slash-separated additional numbers
    # Matches: ADR-98, ADR 98, ADR-000098, ADR-98/99, ADR 98/99
    # Does NOT match if already inside [[ ]]
    # Does NOT match markdown links [ADR-...](...)

    # First, protect existing wikilinks and markdown links by replacing them with placeholders
    placeholders = []

    def save_placeholder(m):
        placeholders.append(m.group(0))
        return f"\x00PH{len(placeholders)-1}\x00"

    # Protect wikilinks [[...]]
    protected = re.sub(r'\[\[[^\]]+\]\]', save_placeholder, line)
    # Protect markdown links [...](...)
    protected = re.sub(r'\[[^\]]*\]\([^\)]*\)', save_placeholder, protected)

    # Now convert ADR references
    # Pattern: ADR[-\s](\d{1,6})(/\d{1,6})*
    def replace_adr_ref(m):
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
            parts = slash_part.split('/')
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
    converted = re.sub(
        r'(ADR[-])(\d{1,6})((?:/\d{1,6})*)',
        replace_adr_ref,
        protected
    )
    converted = re.sub(
        r'(ADR )(\d{1,6})((?:/\d{1,6})*)',
        replace_adr_ref,
        converted
    )

    # Restore placeholders
    for i, ph in enumerate(placeholders):
        converted = converted.replace(f"\x00PH{i}\x00", ph)

    return converted


# --- Step 3: Markdown link -> wikilink conversion ---

def convert_markdown_links(text: str, title_index: dict[int, str]) -> str:
    """Convert [ADR-000NNN](./000NNN.md) to [[000NNN]] or [[000NNN]] Title."""

    def replace_md_link(m):
        full_match = m.group(0)
        link_num_str = m.group(1)  # "000235"
        num = int(link_num_str)
        if num not in title_index:
            return full_match
        return f"[[{pad6(num)}]]"

    # Pattern: [ADR-000NNN](./000NNN.md) or [ADR-NNN](./000NNN.md)
    text = re.sub(
        r'\[ADR-\d+\]\(\./(\d{6})\.md\)',
        replace_md_link,
        text
    )
    return text


def convert_related_section_md_links(text: str, title_index: dict[int, str]) -> str:
    """In Related/APPENDIX sections, convert full markdown link lines.

    - 関連 ADR: [ADR-000235](./000235.md) — Title
    becomes:
    - [[000235]] Title
    """
    lines = text.split('\n')
    result = []
    in_related = False

    for line in lines:
        if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
            in_related = True
            result.append(line)
            continue

        if in_related and re.match(r'^#{2,}\s', line):
            in_related = False

        if in_related:
            # Match patterns like "- 関連 ADR: [ADR-000235](./000235.md) — Title"
            m = re.match(
                r'^-\s+(?:関連\s*(?:ADR)?:\s*)?\[ADR-\d+\]\(\./(\d{6})\.md\)\s*(?:—\s*(.*))?$',
                line
            )
            if m:
                num = int(m.group(1))
                desc = m.group(2)
                if num in title_index:
                    title = desc.strip() if desc else title_index[num]
                    line = f"- [[{pad6(num)}]] {title}"

        result.append(line)

    return '\n'.join(result)


# --- Step 4: Fix "Related ADRs: なし" ---

def extract_body_refs(text: str, self_num: int, title_index: dict[int, str]) -> set[int]:
    """Extract all ADR numbers referenced in the body (wikilinks + inline)."""
    refs = set()
    # Wikilinks: [[000NNN...]]
    for m in re.finditer(r'\[\[(\d{6})', text):
        num = int(m.group(1))
        if num != self_num and num in title_index:
            refs.add(num)
    # Inline: ADR-NNN or ADR NNN (not in frontmatter)
    # We parse after frontmatter
    body = text
    if text.startswith("---"):
        end = text.find("---", 3)
        if end != -1:
            body = text[end+3:]
    for m in re.finditer(r'ADR[-\s](\d{1,6})', body):
        num = int(m.group(1))
        if num != self_num and num in title_index:
            refs.add(num)
    return refs


def _is_nashi_line(line: str) -> bool:
    """Check if line is a '- なし' entry (with or without parenthetical explanation)."""
    stripped = line.strip()
    return stripped == '- なし' or re.match(r'^- なし（.+）$', stripped) is not None


def _is_in_related_section(lines: list[str], idx: int) -> bool:
    """Check if the line at idx is inside a Related/APPENDIX subsection."""
    for j in range(idx - 1, -1, -1):
        if re.match(r'^#{2,3}\s+(Related ADRs|関連\s*ADR|関連ファイル|関連)\s*$', lines[j], re.IGNORECASE):
            return True
        if re.match(r'^#{1,}\s', lines[j]):
            return False
    return False


def fix_nashi_entries(text: str, _self_num: int, body_refs: set[int], title_index: dict[int, str]) -> str:
    """Replace '- なし' with actual references if body contains ADR refs."""
    if not body_refs:
        return text

    lines = text.split('\n')
    result = []
    for i, line in enumerate(lines):
        if _is_nashi_line(line) and _is_in_related_section(lines, i):
            # Replace with actual refs
            for ref_num in sorted(body_refs):
                title = title_index.get(ref_num, '')
                result.append(f"- [[{pad6(ref_num)}]] {title}")
            continue
        result.append(line)

    return '\n'.join(result)


# --- Step 5: Bidirectional backlinks ---

def build_reference_graph(title_index: dict[int, str]) -> dict[int, set[int]]:
    """Build a graph of who references whom from Related/APPENDIX sections."""
    graph: dict[int, set[int]] = {n: set() for n in title_index}

    for f in sorted(ADR_DIR.glob("000*.md")):
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        lines = text.split('\n')

        in_related = False
        for line in lines:
            if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
                in_related = True
                continue
            if in_related and re.match(r'^#{2,}\s', line):
                in_related = False
            if in_related:
                for m in re.finditer(r'\[\[(\d{6})', line):
                    ref_num = int(m.group(1))
                    if ref_num != num and ref_num in title_index:
                        graph[num].add(ref_num)

    return graph


def get_existing_related_refs(text: str) -> set[int]:
    """Get ADR numbers already listed in Related/APPENDIX section."""
    refs = set()
    lines = text.split('\n')
    in_related = False
    for line in lines:
        if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
            in_related = True
            continue
        if in_related and re.match(r'^#{2,}\s', line):
            in_related = False
        if in_related:
            for m in re.finditer(r'\[\[(\d{6})', line):
                refs.add(int(m.group(1)))
    return refs


def add_backlinks(text: str, _self_num: int, backlinks: set[int], title_index: dict[int, str]) -> str:
    """Add missing backlinks to the Related/APPENDIX section."""
    if not backlinks:
        return text

    existing = get_existing_related_refs(text)
    to_add = sorted(backlinks - existing)
    if not to_add:
        return text

    lines = text.split('\n')
    result = []
    related_section_idx = None
    next_section_idx = None

    # Find the LAST matching Related/APPENDIX section heading
    for i, line in enumerate(lines):
        if re.match(r'^#{2,3}\s+(Related ADRs|関連(する)?\s*ADR|関連ファイル|関連|APPENDIX|付録)\s*$', line, re.IGNORECASE):
            related_section_idx = i
            next_section_idx = None  # Reset: find next heading after THIS match

    # Find the next heading after the last related section
    if related_section_idx is not None:
        for i in range(related_section_idx + 1, len(lines)):
            if re.match(r'^#{2,}\s', lines[i]):
                next_section_idx = i
                break

    if related_section_idx is None:
        # No related section found - append one at the end
        result = lines[:]
        # Remove trailing empty lines
        while result and result[-1].strip() == '':
            result.pop()
        result.append('')
        result.append('## Related ADRs')
        result.append('')
        for num in to_add:
            title = title_index.get(num, '')
            result.append(f"- [[{pad6(num)}]] {title}")
        result.append('')
        return '\n'.join(result)

    # Find the last non-empty line in the related section
    insert_at = None
    end = next_section_idx if next_section_idx else len(lines)
    # Find last bullet or content line in related section
    last_content = related_section_idx
    for i in range(related_section_idx + 1, end):
        if lines[i].strip() and not lines[i].startswith('##'):
            last_content = i
        # Skip comment lines
        if lines[i].strip().startswith('<!--'):
            continue

    # Check if "- なし" (with or without explanation) remains — replace it with backlinks
    nashi_idx = None
    for i in range(related_section_idx + 1, end):
        if _is_nashi_line(lines[i]):
            nashi_idx = i

    if nashi_idx is not None:
        # Replace "- なし" line with backlinks
        result = lines[:nashi_idx]
        for num in to_add:
            title = title_index.get(num, '')
            result.append(f"- [[{pad6(num)}]] {title}")
        result.extend(lines[nashi_idx + 1:])
        return '\n'.join(result)

    insert_at = last_content + 1

    result = lines[:insert_at]
    for num in to_add:
        title = title_index.get(num, '')
        result.append(f"- [[{pad6(num)}]] {title}")
    result.extend(lines[insert_at:])

    return '\n'.join(result)


# --- Step 6: Aliases consistency ---

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

    # Check current aliases
    m = re.search(r'^aliases:\s*\[([^\]]*)\]', fm, re.MULTILINE)
    if m:
        current = [a.strip() for a in m.group(1).split(',')]
        needed = []
        if short not in current:
            needed.append(short)
        if padded not in current:
            needed.append(padded)
        if not needed:
            return text
        new_aliases = current + needed
        new_line = f"aliases: [{', '.join(new_aliases)}]"
        new_fm = fm[:m.start()] + new_line + fm[m.end():]
        return f"---{new_fm}---{text[end+3:]}"
    else:
        # Multiline aliases
        lines = fm.split('\n')
        new_lines = []
        in_aliases = False
        alias_values = []

        for line in lines:
            if re.match(r'^aliases:\s*$', line):
                in_aliases = True
                new_lines.append(line)
                continue
            if in_aliases:
                m2 = re.match(r'^(\s+)-\s+(.+)$', line)
                if m2:
                    alias_values.append(m2.group(2).strip())
                    new_lines.append(line)
                    continue
                else:
                    in_aliases = False
                    # Add missing aliases before moving on
                    if short not in alias_values:
                        new_lines.append(f"  - {short}")
                    if padded not in alias_values:
                        new_lines.append(f"  - {padded}")
            new_lines.append(line)

        # If aliases was the last field
        if in_aliases:
            if short not in alias_values:
                new_lines.append(f"  - {short}")
            if padded not in alias_values:
                new_lines.append(f"  - {padded}")

        new_fm = '\n'.join(new_lines)
        return f"---{new_fm}---{text[end+3:]}"


def remove_nashi_if_siblings_exist(text: str) -> str:
    """Remove '- なし(...)' from Related sections when wikilink entries exist alongside."""
    lines = text.split('\n')
    result = []
    in_related = False
    related_start = -1

    # Two-pass: first identify Related sections with both なし and wikilinks
    sections: list[tuple[int, int]] = []  # (start, end) of related sections
    for i, line in enumerate(lines):
        if re.match(r'^#{2,3}\s+(Related ADRs|関連\s*ADR|関連ファイル|関連)\s*$', line, re.IGNORECASE):
            in_related = True
            related_start = i
            continue
        if in_related and re.match(r'^#{1,}\s', line):
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
            if re.search(r'\[\[\d{6}', lines[i]):
                has_wikilink = True
            if _is_nashi_line(lines[i]):
                nashi_lines.append(i)
        if has_wikilink and nashi_lines:
            remove_lines.update(nashi_lines)

    if not remove_lines:
        return text

    return '\n'.join(line for i, line in enumerate(lines) if i not in remove_lines)


# --- Main orchestration ---

def main():
    print("=== ADR Obsidian Link Fixer ===\n")

    # Step 1: Build index
    print("Step 1: Building title index...")
    title_index = build_title_index()
    print(f"  Found {len(title_index)} ADRs\n")

    # Step 2-4: Process each file
    print("Step 2-4: Converting inline refs, markdown links, fixing なし...")
    changed_files = {}
    body_refs_map: dict[int, set[int]] = {}

    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        original = f.read_text(encoding="utf-8")
        text = original

        # Step 6: Fix aliases first (frontmatter change)
        text = fix_aliases(text, num)

        # Step 3: Markdown links -> wikilinks (do before inline conversion)
        text = convert_related_section_md_links(text, title_index)
        text = convert_markdown_links(text, title_index)

        # Step 2: Inline ADR refs -> wikilinks
        text = convert_inline_refs(text, title_index)

        # Collect body refs for Step 4
        body_refs = extract_body_refs(text, num, title_index)
        body_refs_map[num] = body_refs

        # Step 4: Fix なし
        text = fix_nashi_entries(text, num, body_refs, title_index)

        # Cleanup: remove "- なし(...)" if wikilink entries exist in same Related section
        text = remove_nashi_if_siblings_exist(text)

        if text != original:
            changed_files[f] = text

    # Write intermediate changes so Step 5 can read updated files
    for f, text in changed_files.items():
        f.write_text(text, encoding="utf-8")

    step2_4_count = len(changed_files)
    print(f"  Changed {step2_4_count} files\n")

    # Step 5: Bidirectional backlinks
    print("Step 5: Building reference graph and adding backlinks...")
    ref_graph = build_reference_graph(title_index)

    # Compute reverse graph
    reverse_graph: dict[int, set[int]] = {n: set() for n in title_index}
    for src, targets in ref_graph.items():
        for tgt in targets:
            reverse_graph[tgt].add(src)

    # Find missing backlinks
    backlink_count = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")

        # Who references me?
        referrers = reverse_graph.get(num, set())
        # What do I already list?
        existing = get_existing_related_refs(text)
        missing = referrers - existing - {num}

        if missing:
            new_text = add_backlinks(text, num, missing, title_index)
            if new_text != text:
                f.write_text(new_text, encoding="utf-8")
                backlink_count += 1
                if f not in changed_files:
                    changed_files[f] = new_text

    print(f"  Added backlinks to {backlink_count} files\n")

    # Summary
    print("=== Summary ===")
    print(f"Total files changed: {len(changed_files)}")

    # Alias check
    print("\nStep 6: Aliases consistency check...")
    alias_issues = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        num = int(f.stem)
        text = f.read_text(encoding="utf-8")
        fm = parse_frontmatter(text)
        aliases = fm.get('aliases', [])
        short = f"ADR-{num}"
        padded = f"ADR-{num:06d}"
        if short not in aliases or padded not in aliases:
            alias_issues += 1
            print(f"  WARNING: {f.name} missing alias (has: {aliases})")
    if alias_issues == 0:
        print("  All aliases OK")

    # Check for broken wikilinks
    print("\nBroken wikilink check...")
    broken = 0
    for f in sorted(ADR_DIR.glob("000*.md")):
        if f.name == "template.md":
            continue
        text = f.read_text(encoding="utf-8")
        for m in re.finditer(r'\[\[(\d{6})', text):
            ref = int(m.group(1))
            if ref not in title_index:
                print(f"  BROKEN: {f.name} -> [[{m.group(1)}]]")
                broken += 1
    if broken == 0:
        print("  No broken wikilinks")

    print(f"\nDone. {len(changed_files)} files modified total.")


if __name__ == "__main__":
    main()
