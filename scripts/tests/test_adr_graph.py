#!/usr/bin/env python3
"""Unit tests for scripts/adr_graph.py (supersedes DAG check/resolve/graph)."""
from __future__ import annotations

import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import adr_graph  # noqa: E402


def write_adr(dir_path: Path, adr_id: str, status: str = "accepted", supersedes=None, inline=False):
    supersedes = supersedes or []
    supersedes_block = ""
    if supersedes and inline:
        supersedes_block = "supersedes: [" + ", ".join(f'"{s}"' for s in supersedes) + "]\n"
    elif supersedes:
        supersedes_block = "supersedes:\n" + "".join(f'  - "{s}"\n' for s in supersedes)
    content = (
        "---\n"
        f"title: Test ADR {adr_id}\n"
        "date: 2026-01-01\n"
        f"status: {status}\n"
        "tags: [test]\n"
        "affected_services: [test-service]\n"
        f"aliases: [ADR-{adr_id}]\n"
        f"{supersedes_block}"
        "---\n"
        f"# ADR-{adr_id}: Test\n"
    )
    (dir_path / f"{adr_id}.md").write_text(content, encoding="utf-8")


class FrontmatterAndLoadingTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmpdir, ignore_errors=True)

    def test_load_adrs_reads_supersedes_block_list(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        self.assertEqual(adrs["000020"]["supersedes"], ["000010"])
        self.assertEqual(adrs["000010"]["supersedes"], [])

    def test_load_adrs_reads_supersedes_inline_list(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"], inline=True)
        adrs = adr_graph.load_adrs(self.tmpdir)
        self.assertEqual(adrs["000020"]["supersedes"], ["000010"])

    def test_load_adrs_with_no_supersedes_field(self):
        write_adr(self.tmpdir, "000010")
        adrs = adr_graph.load_adrs(self.tmpdir)
        self.assertEqual(adrs["000010"]["supersedes"], [])
        self.assertEqual(adrs["000010"]["status"], "accepted")

    def test_normalize_adr_id_handles_various_formats(self):
        self.assertEqual(adr_graph.normalize_adr_id("339"), "000339")
        self.assertEqual(adr_graph.normalize_adr_id("000339"), "000339")
        self.assertEqual(adr_graph.normalize_adr_id("ADR-339"), "000339")


class GraphAlgorithmTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmpdir, ignore_errors=True)

    def test_build_graph_simple(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        graph = adr_graph.build_supersedes_graph(adrs)
        self.assertEqual(graph["000020"], ["000010"])
        self.assertEqual(graph["000010"], [])

    def test_resolve_no_supersessor_returns_self(self):
        write_adr(self.tmpdir, "000010")
        adrs = adr_graph.load_adrs(self.tmpdir)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(adr_graph.resolve("000010", reverse), ["000010"])

    def test_resolve_single_chain(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(adr_graph.resolve("000010", reverse), ["000020"])
        self.assertEqual(adr_graph.resolve("000020", reverse), ["000020"])

    def test_resolve_split_supersession(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        write_adr(self.tmpdir, "000030", supersedes=["000010"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(sorted(adr_graph.resolve("000010", reverse)), ["000020", "000030"])

    def test_resolve_multi_hop_chain(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        write_adr(self.tmpdir, "000030", supersedes=["000020"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(adr_graph.resolve("000010", reverse), ["000030"])

    def test_find_cycle_detects_cycle(self):
        graph = {"000010": ["000020"], "000020": ["000010"]}
        cycle = adr_graph.find_cycle(graph)
        assert cycle is not None
        self.assertEqual(cycle[0], cycle[-1])

    def test_find_cycle_returns_none_for_acyclic_graph(self):
        graph = {"000010": [], "000020": ["000010"], "000030": ["000010"]}
        self.assertIsNone(adr_graph.find_cycle(graph))

    def test_find_dangling_refs(self):
        write_adr(self.tmpdir, "000020", supersedes=["999999"])
        adrs = adr_graph.load_adrs(self.tmpdir)
        graph = adr_graph.build_supersedes_graph(adrs)
        dangling = adr_graph.find_dangling_refs(adrs, graph)
        self.assertEqual(dangling, [("000020", "999999")])

    def test_render_mermaid_contains_edges(self):
        graph = {"000020": ["000010"], "000010": []}
        rendered = adr_graph.render_mermaid(graph, {})
        self.assertIn("000010", rendered)
        self.assertIn("000020", rendered)
        self.assertIn("mermaid", rendered)


class CliCommandTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmpdir, ignore_errors=True)

    def test_cmd_check_returns_nonzero_on_cycle(self):
        write_adr(self.tmpdir, "000010", supersedes=["000020"])
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        self.assertEqual(adr_graph.cmd_check(self.tmpdir), 1)

    def test_cmd_check_returns_nonzero_on_dangling_ref(self):
        write_adr(self.tmpdir, "000010", supersedes=["999999"])
        self.assertEqual(adr_graph.cmd_check(self.tmpdir), 1)

    def test_cmd_check_returns_zero_when_clean(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        self.assertEqual(adr_graph.cmd_check(self.tmpdir), 0)

    def test_cmd_resolve_prints_effective_adr(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        self.assertEqual(adr_graph.cmd_resolve(self.tmpdir, "000010"), 0)

    def test_cmd_resolve_returns_nonzero_for_unknown_id(self):
        write_adr(self.tmpdir, "000010")
        self.assertEqual(adr_graph.cmd_resolve(self.tmpdir, "999999"), 1)

    def test_cmd_graph_writes_output_file(self):
        write_adr(self.tmpdir, "000010")
        write_adr(self.tmpdir, "000020", supersedes=["000010"])
        out_path = self.tmpdir / "_supersedes-graph.md"
        self.assertEqual(adr_graph.cmd_graph(self.tmpdir, out_path), 0)
        self.assertTrue(out_path.exists())
        self.assertIn("mermaid", out_path.read_text(encoding="utf-8"))


class RealAdrCorpusTests(unittest.TestCase):
    """Integration test against the actual docs/ADR/ corpus.

    Guards the full backfill so a future edit can't silently reintroduce a
    cycle or a dangling supersedes reference across the 946-ADR corpus.
    The first 6 files (000340, 000533, 000739, 000740, 000741, 000743)
    came from the original Status-section regex sweep; the rest came from
    a full-text read of every ADR (32 parallel agents), which caught
    prose-only supersede declarations the regex sweep couldn't see.
    """

    def test_real_corpus_has_no_cycles_or_dangling_refs(self):
        adrs = adr_graph.load_adrs(adr_graph.ADR_DIR)
        graph = adr_graph.build_supersedes_graph(adrs)
        self.assertIsNone(adr_graph.find_cycle(graph))
        self.assertEqual(adr_graph.find_dangling_refs(adrs, graph), [])

    def test_real_corpus_resolves_known_supersede_chains(self):
        adrs = adr_graph.load_adrs(adr_graph.ADR_DIR)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(adr_graph.resolve("000339", reverse), ["000340"])
        self.assertEqual(sorted(adr_graph.resolve("000486", reverse)), ["000533"])
        self.assertEqual(sorted(adr_graph.resolve("000488", reverse)), ["000533"])
        self.assertEqual(sorted(adr_graph.resolve("000736", reverse)), ["000739", "000740"])
        self.assertEqual(adr_graph.resolve("000737", reverse), ["000741"])

    def test_real_corpus_resolves_supersede_chains_from_full_text_audit(self):
        adrs = adr_graph.load_adrs(adr_graph.ADR_DIR)
        reverse = adr_graph.build_reverse_graph(adr_graph.build_supersedes_graph(adrs))
        self.assertEqual(sorted(adr_graph.resolve("000219", reverse)), ["000222"])
        self.assertEqual(sorted(adr_graph.resolve("000220", reverse)), ["000222"])
        self.assertEqual(sorted(adr_graph.resolve("000221", reverse)), ["000222"])
        self.assertEqual(adr_graph.resolve("000383", reverse), ["000384"])
        self.assertEqual(adr_graph.resolve("000527", reverse), ["000624"])
        self.assertEqual(adr_graph.resolve("000396", reverse), ["000627"])
        self.assertEqual(adr_graph.resolve("000784", reverse), ["000802"])
        self.assertEqual(adr_graph.resolve("000865", reverse), ["000867"])
        self.assertEqual(adr_graph.resolve("000232", reverse), ["000900"])
        self.assertEqual(adr_graph.resolve("000408", reverse), ["000923"])
        for old_id in ("000929", "000930", "000933", "000937", "000938", "000939"):
            self.assertEqual(adr_graph.resolve(old_id, reverse), ["000940"])


if __name__ == "__main__":
    unittest.main()
