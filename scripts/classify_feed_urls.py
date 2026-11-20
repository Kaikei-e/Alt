#!/usr/bin/env python3
"""
feed_linksテーブルのURLを解析してジャンルを分類するスクリプト。

Usage:
    python scripts/classify_feed_urls.py --dsn "postgresql://user:pass@host/db" [--output output.csv]
"""

import argparse
import csv
import os
import sys
from collections import Counter
from pathlib import Path
from urllib.parse import urlparse

import psycopg2
import psycopg2.extras

UNKNOWN_RATE_WARN_THRESHOLD = 0.05


def normalize_domain(url_str: str, *, verbose: bool = False) -> str:
    """URLからドメインを正規化（www.を除去、小文字化）"""
    try:
        parsed = urlparse(url_str)
        domain = parsed.netloc.lower()
        if domain.startswith("www."):
            domain = domain[4:]
    except ValueError as e:
        if verbose:
            sys.stderr.write(f"normalize_domain failed for {url_str!r}: {e}\n")
        return ""
    else:
        return domain


def _host_matches(host: str, *suffixes: str) -> bool:
    """Safe host-suffix match.

    ``"theverge.com" in host`` also matches ``evil-theverge.com`` and
    ``theverge.com.attacker.com`` — classic substring-sanitisation bug.
    This helper accepts the host only when it equals a suffix or ends
    with ``"." + suffix``.
    """
    if not host:
        return False
    host = host.lower()
    return any(host == s or host.endswith("." + s) for s in suffixes)


_PATH_GENRE_RULES: tuple[tuple[tuple[str, ...], str], ...] = (
    (("/artanddesign", "/arts", "/culture"), "art_culture"),
    (("/science",), "science"),
    (("/environment", "/climate"), "environment_policy"),
    (("/world", "/us-news", "/politics"), "global_politics"),
    (("/society", "/social"), "society_justice"),
    (("/crosswords", "/games", "/puzzles"), "games_puzzles"),
    (("/business", "/finance", "/economy"), "business_finance"),
    (("/technology", "/tech"), "consumer_tech"),
    (("/health", "/medical"), "health"),
    (("/travel",), "travel_lifestyle"),
)

_GUARDIAN_PATH_RULES: tuple[tuple[tuple[str, ...], str], ...] = (
    (("/artanddesign", "/culture"), "art_culture"),
    (("/science",), "science"),
    (("/environment",), "environment_policy"),
    (("/world", "/us-news"), "global_politics"),
    (("/society",), "society_justice"),
    (("/crosswords",), "games_puzzles"),
    (("/business",), "business_finance"),
    (("/technology",), "consumer_tech"),
)

_DOMAIN_GENRE_RULES: tuple[tuple[str, tuple[str, ...], str], ...] = (
    ("host", ("androidauthority.com", "9to5mac.com", "9to5google.com"), "consumer_tech"),
    ("host", ("theverge.com", "wired.com"), "consumer_tech"),
    ("host", ("zenn.dev", "qiita.com"), "developer_insights"),
    ("substr", ("techblog", "tech-blog", "engineering", "developers"), "developer_insights"),
    ("host", ("techno-edge.net", "impress.co.jp", "zdnet.com"), "pro_it_media"),
    ("host", ("travelvoice.jp", "flywheel.jp"), "travel_lifestyle"),
    ("host", ("io.cyberdefense.jp",), "security_policy"),
    ("substr", ("security",), "security_policy"),
    ("host", ("openai.com", "anthropic.com"), "ai_research"),
    ("substr", (".tech", "tech-", "-tech"), "pro_it_media"),
    (
        "substr",
        (
            "philosophy",
            "psyche.co",
            "aeon.co",
            "ethicsblog",
            "uehiro.ox.ac.uk",
            "sou-philosophia",
        ),
        "art_culture",
    ),
    (
        "substr",
        ("hyperallergic", "theart.co.jp", "architizer", "artnews", "dezeen", "aldaily"),
        "art_culture",
    ),
    ("substr", ("alistapart", "tympanus.net", "uxplanet", "nngroup", "codrops"), "design"),
    ("substr", ("photography", "lightstalking"), "art_culture"),
    (
        "substr",
        (
            "medicalxpress",
            "medscape",
            "mindhacks",
            "neural.it",
            "psychologicalscience",
            "nationalelfservice",
            "thetransmitter",
            "neuroscience",
        ),
        "health",
    ),
    ("host", ("sciencedaily.com",), "science"),
    ("substr", ("cnet.com", "logmi.jp", "publickey1.jp", "nhk.or.jp"), "tech"),
    ("substr", ("web.dev",), "developer_insights"),
)


def _match_path_rules(path: str, rules: tuple[tuple[tuple[str, ...], str], ...]) -> str | None:
    """Return the genre for the first matching path substring rule, or None."""
    for needles, genre in rules:
        if any(needle in path for needle in needles):
            return genre
    return None


def _classify_by_domain(domain: str, path: str) -> str | None:
    """Classify genre from normalized domain (with Guardian path fallback)."""
    if _host_matches(domain, "theguardian.com"):
        return _match_path_rules(path, _GUARDIAN_PATH_RULES) or "global_politics"

    for kind, patterns, genre in _DOMAIN_GENRE_RULES:
        matched = (
            _host_matches(domain, *patterns)
            if kind == "host"
            else any(pattern in domain for pattern in patterns)
        )
        if matched:
            return genre

    return None


def classify_by_domain_and_path(url_str: str) -> str | None:
    """
    URLのドメインとパスからジャンルを分類。

    優先順位:
    1. パスベースの分類（より具体的）
    2. ドメインベースの分類
    3. 不明な場合はNone
    """
    try:
        parsed = urlparse(url_str)
        domain = normalize_domain(url_str)
        path = parsed.path.lower()

        path_genre = _match_path_rules(path, _PATH_GENRE_RULES)
        if path_genre is not None:
            return path_genre

        return _classify_by_domain(domain, path)
    except ValueError as e:
        sys.stderr.write(f"classify_by_domain_and_path failed for {url_str!r}: {e}\n")
        return None


def fetch_feed_urls(dsn: str) -> list[tuple[str, str]]:
    """feed_linksテーブルから全URLを取得"""
    conn = psycopg2.connect(dsn, connect_timeout=10)
    try:
        with conn.cursor() as cur:
            cur.execute("SELECT id::text, url FROM feed_links ORDER BY url")
            return cur.fetchall()
    finally:
        conn.close()


def main() -> int:
    parser = argparse.ArgumentParser(description="Classify feed URLs by genre")
    parser.add_argument("--dsn", default=os.getenv("ALT_DB_DSN"), help="PostgreSQL DSN")
    parser.add_argument("--output", default="feed_urls_classified.csv", help="Output CSV file")
    parser.add_argument("--verbose", action="store_true", help="Verbose output")

    args = parser.parse_args()

    if not args.dsn:
        sys.stdout.write("Error: --dsn or ALT_DB_DSN environment variable required\n")
        return 1

    sys.stdout.write("Fetching URLs from feed_links...\n")
    urls = fetch_feed_urls(args.dsn)
    sys.stdout.write(f"Found {len(urls)} URLs\n")

    if not urls:
        sys.stdout.write("Error: No URLs found in feed_links\n")
        return 1

    classified = []
    genre_counts = Counter()
    unknown_count = 0

    for feed_id, url in urls:
        genre = classify_by_domain_and_path(url)
        if genre is None:
            genre = "unknown"
            unknown_count += 1

        classified.append((feed_id, url, genre))
        genre_counts[genre] += 1

        if args.verbose:
            sys.stdout.write(f"{genre:20s} {url}\n")

    # CSV出力
    with Path(args.output).open("w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(["feed_id", "url", "genre"])
        writer.writerows(classified)

    sys.stdout.write("\nClassification complete:\n")
    sys.stdout.write(f"  Total URLs: {len(urls)}\n")
    sys.stdout.write(f"  Unknown: {unknown_count} ({100.0 * unknown_count / len(urls):.1f}%)\n")
    sys.stdout.write("\nGenre distribution:\n")
    for genre, count in genre_counts.most_common():
        sys.stdout.write(f"  {genre:20s}: {count:4d} ({100.0 * count / len(urls):.1f}%)\n")

    if unknown_count / len(urls) > UNKNOWN_RATE_WARN_THRESHOLD:
        sys.stdout.write(
            f"\n⚠️  Warning: Unknown rate ({100.0 * unknown_count / len(urls):.1f}%) exceeds 5%\n"
        )
        sys.stdout.write("   Consider refining classification rules\n")
    else:
        sys.stdout.write(
            f"\n✓ Unknown rate ({100.0 * unknown_count / len(urls):.1f}%) is acceptable\n"
        )

    sys.stdout.write(f"\nResults saved to: {args.output}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
