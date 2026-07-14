#!/usr/bin/env python3
"""Validate and query the ADR supersedes DAG.

Authors write `supersedes:` only on the new ADR (the one doing the replacing).
The reverse direction (which ADR currently supersedes a given one) is always
computed, never authored, so there is nothing to keep in sync by hand.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ADR_DIR = Path(__file__).resolve().parent.parent / "docs" / "ADR"
ADR_FILENAME_RE = re.compile(r"^\d{6}$")


FRONTMATTER_RE = re.compile(r"^---\n(.*?)\n---\n", re.DOTALL)
FIELD_RE = re.compile(r"^([A-Za-z_]+):\s*(.*)$")
BLOCK_ITEM_RE = re.compile(r"^\s+-\s*(.*)$")


def parse_frontmatter(text: str) -> dict:
    """Parse the flat YAML frontmatter block of an ADR file into a dict.

    Supports scalar strings, inline flow lists (`key: [a, b]`), and block
    lists (`key:\\n  - a\\n  - b`) -- the only shapes Alt's ADR frontmatter uses.
    """
    match = FRONTMATTER_RE.match(text)
    if not match:
        return {}
    lines = match.group(1).split("\n")
    fields: dict = {}
    i = 0
    while i < len(lines):
        field_match = FIELD_RE.match(lines[i])
        if not field_match:
            i += 1
            continue
        key, value = field_match.group(1), field_match.group(2).strip()
        if value.startswith("[") and value.endswith("]"):
            inner = value[1:-1]
            fields[key] = [v.strip().strip('"').strip("'") for v in inner.split(",") if v.strip()]
            i += 1
        elif value == "":
            items = []
            j = i + 1
            while j < len(lines):
                item_match = BLOCK_ITEM_RE.match(lines[j])
                if not item_match:
                    break
                item = item_match.group(1).strip().strip('"').strip("'")
                if item:
                    items.append(item)
                j += 1
            fields[key] = items
            i = j
        else:
            fields[key] = value.strip('"').strip("'")
            i += 1
    return fields


def normalize_adr_id(raw: str) -> str:
    """Normalize any ADR id spelling (`339`, `ADR-339`, `000339`) to `000339`."""
    digits = re.sub(r"\D", "", raw)
    return digits.zfill(6)


def load_adrs(adr_dir: Path) -> dict:
    """Load every ADR under adr_dir into id -> {status, supersedes, title}."""
    adrs: dict = {}
    for path in sorted(adr_dir.glob("*.md")):
        if not ADR_FILENAME_RE.match(path.stem):
            continue
        fm = parse_frontmatter(path.read_text(encoding="utf-8"))
        supersedes = [normalize_adr_id(s) for s in fm.get("supersedes", [])]
        adrs[path.stem] = {
            "status": fm.get("status"),
            "supersedes": supersedes,
            "title": fm.get("title"),
        }
    return adrs


def build_supersedes_graph(adrs: dict) -> dict:
    """new_id -> [old_id, ...] adjacency for the supersedes DAG."""
    return {adr_id: list(data["supersedes"]) for adr_id, data in adrs.items()}


def build_reverse_graph(graph: dict) -> dict:
    """old_id -> [new_id, ...]: which ADRs currently supersede this one."""
    reverse: dict = {node: [] for node in graph}
    for new_id, targets in graph.items():
        for old_id in targets:
            reverse.setdefault(old_id, []).append(new_id)
    return reverse


def find_dangling_refs(adrs: dict, graph: dict) -> list:
    """Return (new_id, old_id) pairs where old_id is not a known ADR."""
    return [
        (new_id, old_id)
        for new_id, targets in graph.items()
        for old_id in targets
        if old_id not in adrs
    ]


def find_cycle(graph: dict):
    """Three-color DFS cycle detection. Returns the closed cycle path or None.

    Recursive on purpose: the supersedes graph is a handful of edges across
    ~1000 ADRs, nowhere near Python's recursion limit, so the simpler
    recursive form beats an iterative rewrite (cf. Airflow's 10k-task DAGs).
    """
    WHITE, GRAY, BLACK = 0, 1, 2
    color: dict = {}
    path_stack: list = []

    def visit(node):
        color[node] = GRAY
        path_stack.append(node)
        for neighbor in graph.get(node, []):
            state = color.get(neighbor, WHITE)
            if state == WHITE:
                found = visit(neighbor)
                if found:
                    return found
            elif state == GRAY:
                cycle_start = path_stack.index(neighbor)
                return path_stack[cycle_start:] + [neighbor]
        path_stack.pop()
        color[node] = BLACK
        return None

    for node in graph:
        if color.get(node, WHITE) == WHITE:
            found = visit(node)
            if found:
                return found
    return None


def resolve(adr_id: str, reverse_graph: dict) -> list:
    """Walk the supersedes chain forward to all currently-effective ADRs."""
    successors = reverse_graph.get(adr_id, [])
    if not successors:
        return [adr_id]
    terminal: list = []
    seen: set = set()
    for successor in successors:
        for leaf in resolve(successor, reverse_graph):
            if leaf not in seen:
                seen.add(leaf)
                terminal.append(leaf)
    return terminal


def _mermaid_label(adr_id: str, adrs: dict) -> str:
    title = adrs.get(adr_id, {}).get("title")
    if not title:
        return adr_id
    truncated = title if len(title) <= 40 else title[:37] + "..."
    escaped = truncated.replace('"', "'")
    return f"{adr_id}: {escaped}"


def render_mermaid(graph: dict, adrs: dict) -> str:
    """Render the supersedes DAG as a mermaid graph block."""
    lines = ["```mermaid", "graph LR"]
    for new_id, targets in sorted(graph.items()):
        for old_id in sorted(targets):
            old_label = _mermaid_label(old_id, adrs)
            new_label = _mermaid_label(new_id, adrs)
            lines.append(f'    {old_id}["{old_label}"] -->|superseded by| {new_id}["{new_label}"]')
    lines.append("```")
    return "\n".join(lines)


def cmd_check(adr_dir: Path) -> int:
    adrs = load_adrs(adr_dir)
    graph = build_supersedes_graph(adrs)

    dangling = find_dangling_refs(adrs, graph)
    if dangling:
        for new_id, old_id in dangling:
            print(f"ERROR: {new_id} supersedes unknown ADR {old_id}", file=sys.stderr)
        return 1

    cycle = find_cycle(graph)
    if cycle:
        print("ERROR: cycle detected in supersedes graph: " + " --> ".join(cycle), file=sys.stderr)
        return 1

    edge_count = sum(len(targets) for targets in graph.values())
    print(f"OK: {len(adrs)} ADRs, {edge_count} supersedes edges, no cycles")
    return 0


def cmd_resolve(adr_dir: Path, adr_id: str) -> int:
    adrs = load_adrs(adr_dir)
    normalized = normalize_adr_id(adr_id)
    if normalized not in adrs:
        print(f"ERROR: unknown ADR {normalized}", file=sys.stderr)
        return 1
    reverse_graph = build_reverse_graph(build_supersedes_graph(adrs))
    for leaf in resolve(normalized, reverse_graph):
        print(leaf)
    return 0


def cmd_graph(adr_dir: Path, out_path: Path) -> int:
    adrs = load_adrs(adr_dir)
    graph = build_supersedes_graph(adrs)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(render_mermaid(graph, adrs) + "\n", encoding="utf-8")
    print(f"wrote {out_path}")
    return 0


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description="Manage the ADR supersedes DAG.")
    parser.add_argument("--adr-dir", type=Path, default=ADR_DIR)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("check", help="Validate the supersedes graph (cycles, dangling refs).")

    resolve_parser = sub.add_parser(
        "resolve", help="Resolve an ADR id to its currently effective successor(s)."
    )
    resolve_parser.add_argument("adr_id")

    graph_parser = sub.add_parser("graph", help="Render the supersedes DAG as mermaid.")
    graph_parser.add_argument(
        "--out", type=Path, default=Path("docs/wiki/decisions/_supersedes-graph.md")
    )

    args = parser.parse_args(argv)

    if args.command == "check":
        return cmd_check(args.adr_dir)
    if args.command == "resolve":
        return cmd_resolve(args.adr_dir, args.adr_id)
    if args.command == "graph":
        return cmd_graph(args.adr_dir, args.out)
    return 1


if __name__ == "__main__":
    sys.exit(main())
