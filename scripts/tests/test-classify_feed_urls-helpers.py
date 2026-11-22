#!/usr/bin/env python3
"""Unit tests for pure URL classification helpers in scripts/classify_feed_urls.py.

Run:
    python3 scripts/tests/test-classify_feed_urls-helpers.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import pathlib
import sys
import types
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / "scripts" / "classify_feed_urls.py"

spec = importlib.util.spec_from_file_location("classify_feed_urls", SCRIPT_PATH)
assert spec is not None, "script missing"
assert spec.loader is not None, "script loader missing"
mod = importlib.util.module_from_spec(spec)
with contextlib.ExitStack() as _stack:
    if importlib.util.find_spec("psycopg2") is None:
        psycopg2_stub = types.ModuleType("psycopg2")
        psycopg2_extras_stub = types.ModuleType("psycopg2.extras")
        _stack.enter_context(
            patch.dict(
                sys.modules,
                {
                    "psycopg2": psycopg2_stub,
                    "psycopg2.extras": psycopg2_extras_stub,
                },
            )
        )
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


def test_normalize_domain_and_host_matches() -> None:
    """Covers www stripping, case folding, and suffix boundary safety."""
    check(
        "normalize strips www", mod.normalize_domain("https://WWW.Example.COM/rss") == "example.com"
    )
    check("host_matches exact", mod._host_matches("theverge.com", "theverge.com"))
    check("host_matches subdomain", mod._host_matches("feeds.theverge.com", "theverge.com"))
    check(
        "host_matches rejects prefix spoof",
        not mod._host_matches("evil-theverge.com", "theverge.com"),
    )
    check(
        "host_matches rejects suffix spoof",
        not mod._host_matches("theverge.com.attacker.com", "theverge.com"),
    )


def test_classify_by_domain_and_path() -> None:
    """Covers path priority, Guardian fallback, domain rules, and unknown URLs."""
    cases = [
        ("https://example.com/culture/rss", "art_culture"),
        ("https://example.com/science/feed", "science"),
        ("https://example.com/climate/news", "environment_policy"),
        ("https://example.com/politics/rss", "global_politics"),
        ("https://example.com/social/feed", "society_justice"),
        ("https://example.com/puzzles/rss", "games_puzzles"),
        ("https://example.com/finance/rss", "business_finance"),
        ("https://example.com/tech/rss", "consumer_tech"),
        ("https://example.com/medical/rss", "health"),
        ("https://example.com/travel/rss", "travel_lifestyle"),
        ("https://www.theguardian.com/uk/rss", "global_politics"),
        ("https://www.theguardian.com/science/rss", "science"),
        ("https://www.theverge.com/rss/index.xml", "consumer_tech"),
        ("https://zenn.dev/feed", "developer_insights"),
        ("https://engineering.example.com/feed.xml", "developer_insights"),
        ("https://www.impress.co.jp/rss", "pro_it_media"),
        ("https://io.cyberdefense.jp/feed", "security_policy"),
        ("https://security-blog.example.org/rss", "security_policy"),
        ("https://openai.com/blog/rss.xml", "ai_research"),
        ("https://aeon.co/feed.rss", "art_culture"),
        ("https://nngroup.com/feed/rss/", "design"),
        ("https://www.sciencedaily.com/rss/all.xml", "science"),
        ("https://www.publickey1.jp/atom.xml", "tech"),
        ("https://web.dev/feed.xml", "developer_insights"),
        ("https://unknown-personal-blog.example.org/feed.xml", None),
        ("http://[::1", None),
    ]
    for url, expected in cases:
        got = mod.classify_by_domain_and_path(url)
        check(f"classify {url}", got == expected, f"got={got!r}, expected={expected!r}")


def main() -> int:
    """Run all helper tests."""
    test_normalize_domain_and_host_matches()
    test_classify_by_domain_and_path()
    sys.stdout.write(f"{COUNTS['passed']} passed, {COUNTS['failed']} failed\n")
    return 1 if COUNTS["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
