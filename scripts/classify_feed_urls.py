#!/usr/bin/env python3
"""
feed_linksテーブルのURLを解析してジャンルを分類するスクリプト。

Usage:
    python scripts/classify_feed_urls.py --dsn "postgresql://user:pass@host/db" [--output output.csv]
"""

import argparse
import csv
import os
import re
from collections import Counter
from typing import Optional
from urllib.parse import urlparse

import psycopg2
import psycopg2.extras


def normalize_domain(url_str: str) -> str:
    """URLからドメインを正規化（www.を除去、小文字化）"""
    try:
        parsed = urlparse(url_str)
        domain = parsed.netloc.lower()
        if domain.startswith("www."):
            domain = domain[4:]
        return domain
    except Exception:
        return ""


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


def classify_by_domain_and_path(url_str: str) -> Optional[str]:
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
        host = parsed.hostname.lower() if parsed.hostname else ""

        # パスベースの分類（優先）
        if "/artanddesign" in path or "/arts" in path or "/culture" in path:
            return "art_culture"
        if "/science" in path:
            return "science"
        if "/environment" in path or "/climate" in path:
            return "environment_policy"
        if "/world" in path or "/us-news" in path or "/politics" in path:
            return "global_politics"
        if "/society" in path or "/social" in path:
            return "society_justice"
        if "/crosswords" in path or "/games" in path or "/puzzles" in path:
            return "games_puzzles"
        if "/business" in path or "/finance" in path or "/economy" in path:
            return "business_finance"
        if "/technology" in path or "/tech" in path:
            return "consumer_tech"
        if "/health" in path or "/medical" in path:
            return "health"
        if "/travel" in path:
            return "travel_lifestyle"

        # ドメインベースの分類
        if _host_matches(domain, "theguardian.com"):
            if "/artanddesign" in path or "/culture" in path:
                return "art_culture"
            if "/science" in path:
                return "science"
            if "/environment" in path:
                return "environment_policy"
            if "/world" in path or "/us-news" in path:
                return "global_politics"
            if "/society" in path:
                return "society_justice"
            if "/crosswords" in path:
                return "games_puzzles"
            if "/business" in path:
                return "business_finance"
            if "/technology" in path:
                return "consumer_tech"
            return "global_politics"  # デフォルト

        if _host_matches(
            domain,
            "androidauthority.com",
            "9to5mac.com",
            "9to5google.com",
        ):
            return "consumer_tech"

        if _host_matches(domain, "theverge.com", "wired.com"):
            return "consumer_tech"

        if _host_matches(domain, "zenn.dev", "qiita.com"):
            return "developer_insights"

        if any(x in domain for x in ["techblog", "tech-blog", "engineering", "developers"]):
            return "developer_insights"

        if _host_matches(
            domain,
            "techno-edge.net",
            "impress.co.jp",
            "zdnet.com",
        ):
            return "pro_it_media"

        if _host_matches(domain, "travelvoice.jp", "flywheel.jp"):
            return "travel_lifestyle"

        if _host_matches(domain, "io.cyberdefense.jp") or "security" in domain:
            return "security_policy"

        # AI関連の特定ドメイン
        if _host_matches(domain, "openai.com", "anthropic.com"):
            return "ai_research"

        # その他の技術系
        if any(x in domain for x in [".tech", "tech-", "-tech"]):
            return "pro_it_media"

        # 哲学・思想系
        if any(x in domain for x in ["philosophy", "psyche.co", "aeon.co", "ethicsblog", "uehiro.ox.ac.uk", "sou-philosophia"]):
            return "art_culture"

        # アート・美術系
        if any(x in domain for x in ["hyperallergic", "theart.co.jp", "architizer", "artnews", "dezeen", "aldaily"]):
            return "art_culture"

        # デザイン・UX系
        if any(x in domain for x in ["alistapart", "tympanus.net", "uxplanet", "nngroup", "codrops"]):
            return "design"

        # 写真系
        if any(x in domain for x in ["photography", "lightstalking"]):
            return "art_culture"

        # 医療・健康・心理学系
        if any(x in domain for x in ["medicalxpress", "medscape", "mindhacks", "neural.it", "psychologicalscience", "nationalelfservice", "thetransmitter", "neuroscience"]):
            return "health"

        # 科学・研究系
        if _host_matches(domain, "sciencedaily.com"):
            return "science"

        # ニュース・メディア系
        if any(x in domain for x in ["cnet.com", "logmi.jp", "publickey1.jp", "nhk.or.jp"]):
            return "tech"  # 技術ニュース系

        # Web開発・技術系
        if "web.dev" in domain:
            return "developer_insights"

        # デフォルト: 不明
        return None

    except Exception:
        return None


def fetch_feed_urls(dsn: str) -> list[tuple[str, str]]:
    """feed_linksテーブルから全URLを取得"""
    conn = psycopg2.connect(dsn)
    try:
        with conn.cursor() as cur:
            cur.execute("SELECT id::text, url FROM feed_links ORDER BY url")
            return cur.fetchall()
    finally:
        conn.close()


def main():
    parser = argparse.ArgumentParser(description="Classify feed URLs by genre")
    parser.add_argument("--dsn", default=os.getenv("ALT_DB_DSN"), help="PostgreSQL DSN")
    parser.add_argument("--output", default="feed_urls_classified.csv", help="Output CSV file")
    parser.add_argument("--verbose", action="store_true", help="Verbose output")

    args = parser.parse_args()

    if not args.dsn:
        print("Error: --dsn or ALT_DB_DSN environment variable required")
        return 1

    print(f"Fetching URLs from feed_links...")
    urls = fetch_feed_urls(args.dsn)
    print(f"Found {len(urls)} URLs")

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
            print(f"{genre:20s} {url}")

    # CSV出力
    with open(args.output, "w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(["feed_id", "url", "genre"])
        writer.writerows(classified)

    print(f"\nClassification complete:")
    print(f"  Total URLs: {len(urls)}")
    print(f"  Unknown: {unknown_count} ({100.0 * unknown_count / len(urls):.1f}%)")
    print(f"\nGenre distribution:")
    for genre, count in genre_counts.most_common():
        print(f"  {genre:20s}: {count:4d} ({100.0 * count / len(urls):.1f}%)")

    if unknown_count / len(urls) > 0.05:
        print(f"\n⚠️  Warning: Unknown rate ({100.0 * unknown_count / len(urls):.1f}%) exceeds 5%")
        print("   Consider refining classification rules")
    else:
        print(f"\n✓ Unknown rate ({100.0 * unknown_count / len(urls):.1f}%) is acceptable")

    print(f"\nResults saved to: {args.output}")
    return 0


if __name__ == "__main__":
    exit(main())

