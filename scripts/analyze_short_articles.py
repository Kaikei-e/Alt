#!/usr/bin/env python3
"""
短すぎる記事（100文字未満）を実際のDBを見て類型化するスクリプト
"""

import os
import re
import sys
import traceback
from collections import defaultdict
from collections.abc import Callable
from pathlib import Path

HTML_RATIO_THRESHOLD = 0.3
HTML_TAG_COUNT_MIN = 50
SHORT_ARTICLE_MAX_LEN = 100
REPEAT_PREFIX_LEN = 10
MIN_REPEAT_COUNT = 3
MIN_TECH_TAG_COUNT = 3
TAG_ONLY_MAX_LEN = 80
HEADING_ONLY_MAX_LEN = 50
MAX_EXAMPLES_PER_CATEGORY = 5

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    sys.stdout.write("psycopg2が必要です。インストールしてください: pip install psycopg2-binary\n")
    sys.exit(1)

try:
    import bleach
except ImportError:
    sys.stdout.write("bleachが必要です。インストールしてください: pip install bleach\n")
    sys.exit(1)


def load_env_vars() -> dict[str, str]:
    """環境変数または.envファイルからデータベース接続情報を読み込む"""
    env_file = Path(__file__).parent.parent / ".env"
    env_vars = {}

    if env_file.exists():
        with env_file.open(encoding="utf-8") as f:
            for raw_line in f:
                line = raw_line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    env_vars[key.strip()] = value.strip().strip('"').strip("'")

    # 環境変数で上書き
    for key in ["DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"]:
        if key in os.environ:
            env_vars[key] = os.environ[key]

    return env_vars


def get_db_connection() -> psycopg2.extensions.connection:
    """データベース接続を取得"""
    env_vars = load_env_vars()

    host = env_vars.get("DB_HOST", "localhost")
    if host == "db":
        host = "localhost"

    port = env_vars.get("DB_PORT", "5432")
    user = env_vars.get("DB_USER", "alt_appuser")
    password = env_vars.get("DB_PASSWORD", "")
    dbname = env_vars.get("DB_NAME", "alt")

    try:
        conn = psycopg2.connect(
            host=host, port=port, user=user, password=password, dbname=dbname, connect_timeout=10
        )
    except psycopg2.Error as e:
        sys.stdout.write(f"データベース接続エラー: {e}\n")
        sys.exit(1)
    else:
        return conn


def clean_html_content(content: str) -> tuple[str, bool]:
    """
    HTMLタグを除去してテキストを抽出（news-creatorと同じロジック）
    """
    if not content:
        return "", False

    # HTMLかどうかを判定
    html_indicators = [
        content.strip().startswith("<!doctype"),
        content.strip().startswith("<!DOCTYPE"),
        content.strip().startswith("<html"),
        content.strip().startswith("<HTML"),
    ]

    html_tags = re.findall(r"<[^>]+>", content)
    html_tag_count = len(html_tags)
    html_ratio = (len("".join(html_tags)) / len(content)) if content else 0.0

    is_html = any(html_indicators) or (
        html_ratio > HTML_RATIO_THRESHOLD and html_tag_count > HTML_TAG_COUNT_MIN
    )

    if not is_html:
        return content, False

    try:
        cleaned = bleach.clean(content, tags=[], strip=True)
        cleaned = bleach.clean(cleaned, tags=[], strip=True)
        cleaned = re.sub(r"\s+", " ", cleaned)
        cleaned = cleaned.strip()
        cleaned = re.sub(r"\b[a-zA-Z-]+:\s*[^;]+;?", " ", cleaned)
        cleaned = re.sub(r"https?://[^\s]+", " ", cleaned)
        cleaned = re.sub(r"[^\w\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]{3,}", " ", cleaned)
        cleaned = re.sub(r"\s+", " ", cleaned).strip()
    except (ValueError, TypeError, AttributeError) as e:
        sys.stderr.write(f"bleach clean failed, falling back to regex: {e}\n")
        cleaned = re.sub(r"<[^>]+>", " ", content)
        cleaned = re.sub(r"&[a-zA-Z0-9#]+;", " ", cleaned)
        cleaned = re.sub(r"\s+", " ", cleaned).strip()
        return cleaned, True
    else:
        return cleaned, True


_ERROR_PATTERNS = (
    r"404",
    r"403",
    r"500",
    r"Error",
    r"Forbidden",
    r"Not Found",
    r"Access Denied",
    r"Page not found",
)

_TECH_TAGS = (
    "Node.js",
    "PDF",
    "Puppeteer",
    "aws",
    "ses",
    "Nodemailer",
    "tech",
    "GitHub",
    "Linux",
    "LVM",
    "idea",
    "Discussion",
    "Property",
    "Photography",
    "Art",
)


def _is_mostly_urls(cleaned_content: str) -> bool:
    urls = re.findall(r"https?://[^\s]+", cleaned_content)
    return len(" ".join(urls)) > len(cleaned_content) * 0.8


def _has_repetition(cleaned_content: str) -> bool:
    if len(cleaned_content) <= REPEAT_PREFIX_LEN:
        return False
    first_10 = cleaned_content[:REPEAT_PREFIX_LEN]
    return cleaned_content.count(first_10) > MIN_REPEAT_COUNT


def _is_error_message(cleaned_content: str) -> bool:
    return any(re.search(pattern, cleaned_content, re.IGNORECASE) for pattern in _ERROR_PATTERNS)


def _is_tag_metadata_only(cleaned_content: str) -> bool:
    tag_count = sum(1 for tag in _TECH_TAGS if tag in cleaned_content)
    return tag_count >= MIN_TECH_TAG_COUNT and len(cleaned_content) < TAG_ONLY_MAX_LEN


def _is_mostly_symbols(cleaned_content: str) -> bool:
    text_only = re.sub(r"[^\w\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]", "", cleaned_content)
    return len(text_only) < len(cleaned_content) * 0.3


_CATEGORY_RULES: tuple[tuple[Callable[[str, int, int], bool], str], ...] = (
    (lambda c, _o, _cl: not c or not c.strip(), "空または空白のみ"),
    (
        lambda _c, o, cl: cl < o * 0.1 and o > SHORT_ARTICLE_MAX_LEN,
        "HTMLクリーニングで大幅に短縮",
    ),
    (lambda c, _o, _cl: _is_mostly_urls(c), "URLのみ"),
    (lambda c, _o, _cl: _has_repetition(c), "繰り返しパターン"),
    (lambda c, _o, _cl: _is_error_message(c), "エラーメッセージ"),
    (
        lambda c, _o, _cl: c.strip() == "Discussion" or c.strip().startswith("Discussion"),
        "Zenn Discussionのみ",
    ),
    (lambda c, _o, _cl: "Explore more on these topics" in c, "写真ギャラリー記事（メタ情報のみ）"),
    (lambda c, _o, _cl: _is_tag_metadata_only(c), "タグ・メタ情報のみ"),
    (
        lambda c, _o, _cl: len(c) < HEADING_ONLY_MAX_LEN and "\n" not in c,
        "タイトル/見出しのみ",
    ),
    (lambda c, _o, _cl: _is_mostly_symbols(c), "記号・特殊文字が多い"),
)


def categorize_short_article(
    content: str, cleaned_content: str, original_length: int, cleaned_length: int
) -> str:
    """短い記事をカテゴリに分類"""
    del content
    for predicate, label in _CATEGORY_RULES:
        if predicate(cleaned_content, original_length, cleaned_length):
            return label
    return "通常の短い記事"


def analyze_short_articles(cursor: RealDictCursor, limit: int = 100) -> list[dict]:
    """100文字未満の記事を取得して分析"""
    query = """
        SELECT
            a.id,
            a.title,
            a.content,
            a.url,
            a.created_at,
            LENGTH(a.content) as original_length
        FROM articles a
        WHERE LENGTH(a.content) < 200  -- 少し余裕を持たせて取得
        ORDER BY a.created_at DESC
        LIMIT %s
    """

    cursor.execute(query, (limit,))
    rows = cursor.fetchall()

    results = []
    for row in rows:
        article = dict(row)
        content = article.get("content", "")
        original_length = article.get("original_length", 0)

        # HTMLクリーニング
        cleaned_content, was_html = clean_html_content(content)
        cleaned_length = len(cleaned_content.strip())

        # 100文字未満かチェック（クリーニング後）
        if cleaned_length < SHORT_ARTICLE_MAX_LEN:
            category = categorize_short_article(
                content, cleaned_content, original_length, cleaned_length
            )

            article["cleaned_content"] = cleaned_content
            article["cleaned_length"] = cleaned_length
            article["was_html"] = was_html
            article["category"] = category
            article["reduction_ratio"] = (
                round((1 - (cleaned_length / original_length)) * 100, 2)
                if original_length > 0
                else 0
            )

            results.append(article)

    return results


def _group_articles_by_category(
    short_articles: list[dict],
) -> tuple[dict[str, int], dict[str, list[dict]]]:
    """短い記事をカテゴリ別に件数集計し、各カテゴリの代表例を収集"""
    category_counts: dict[str, int] = defaultdict(int)
    category_examples: dict[str, list[dict]] = defaultdict(list)

    for article in short_articles:
        category = article["category"]
        category_counts[category] += 1
        if len(category_examples[category]) < MAX_EXAMPLES_PER_CATEGORY:
            category_examples[category].append(article)

    return category_counts, category_examples


def _print_category_breakdown(
    short_articles: list[dict],
    category_counts: dict[str, int],
    category_examples: dict[str, list[dict]],
) -> None:
    """カテゴリ別の集計と代表例を標準出力へ表示"""
    sys.stdout.write("\n【カテゴリ別集計】\n")
    sys.stdout.write(f"{'=' * 80}\n")
    for category, count in sorted(category_counts.items(), key=lambda x: x[1], reverse=True):
        sys.stdout.write(f"{category}: {count}件 ({count / len(short_articles) * 100:.1f}%)\n")

    sys.stdout.write("\n【カテゴリ別詳細例】\n")
    sys.stdout.write(f"{'=' * 80}\n")

    for category in sorted(category_counts.keys(), key=lambda x: category_counts[x], reverse=True):
        examples = category_examples[category]
        sys.stdout.write(f"\n{'=' * 80}\n")
        sys.stdout.write(f"【{category}】 ({category_counts[category]}件)\n")
        sys.stdout.write(f"{'=' * 80}\n")

        for idx, article in enumerate(examples, 1):
            sys.stdout.write(f"\n--- 例 #{idx} ---\n")
            sys.stdout.write(f"記事ID: {article['id']}\n")
            sys.stdout.write(f"タイトル: {article['title'][:100]}\n")
            sys.stdout.write(f"URL: {article['url'][:100]}\n")
            sys.stdout.write(f"作成日時: {article['created_at']}\n")
            sys.stdout.write(f"\n元の長さ: {article['original_length']}文字\n")
            sys.stdout.write(f"クリーニング後: {article['cleaned_length']}文字\n")
            sys.stdout.write(f"HTML削除: {'はい' if article['was_html'] else 'いいえ'}\n")
            if article["was_html"]:
                sys.stdout.write(f"削減率: {article['reduction_ratio']}%\n")
            sys.stdout.write("\n元の内容（最初の300文字）:\n")
            sys.stdout.write(f"  {article['content'][:300]}\n")
            sys.stdout.write("\nクリーニング後:\n")
            sys.stdout.write(f"  {article['cleaned_content'][:300]}\n")
            sys.stdout.write("\n")


def _print_overall_stats(short_articles: list[dict]) -> None:
    """全体統計情報を標準出力へ表示"""
    sys.stdout.write(f"\n{'=' * 80}\n")
    sys.stdout.write("【統計情報】\n")
    sys.stdout.write(f"{'=' * 80}\n")

    html_count = sum(1 for a in short_articles if a["was_html"])
    avg_original = sum(a["original_length"] for a in short_articles) / len(short_articles)
    avg_cleaned = sum(a["cleaned_length"] for a in short_articles) / len(short_articles)

    sys.stdout.write(f"総件数: {len(short_articles)}件\n")
    sys.stdout.write(
        f"HTMLが含まれていた記事: {html_count}件 ({html_count / len(short_articles) * 100:.1f}%)\n"
    )
    sys.stdout.write(f"平均元の長さ: {avg_original:.1f}文字\n")
    sys.stdout.write(f"平均クリーニング後: {avg_cleaned:.1f}文字\n")
    sys.stdout.write(f"平均削減率: {(1 - avg_cleaned / avg_original) * 100:.1f}%\n")

    sys.stdout.write(f"\n{'=' * 80}\n")
    sys.stdout.write("分析完了\n")
    sys.stdout.write(f"{'=' * 80}\n")


def main() -> None:
    """メイン処理"""
    sys.stdout.write("=" * 80 + "\n")
    sys.stdout.write("短すぎる記事（100文字未満）の類型化分析\n")
    sys.stdout.write("=" * 80 + "\n")

    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    try:
        sys.stdout.write("\n短い記事を取得中...\n")
        short_articles = analyze_short_articles(cursor, limit=200)

        sys.stdout.write(f"\n✓ {len(short_articles)}件の短い記事を取得しました\n\n")

        if not short_articles:
            sys.stdout.write("短い記事が見つかりませんでした。\n")
            return

        category_counts, category_examples = _group_articles_by_category(short_articles)
        _print_category_breakdown(short_articles, category_counts, category_examples)
        _print_overall_stats(short_articles)

    except Exception as e:  # noqa: BLE001 — top-level CLI boundary prints traceback before DB cleanup and exit
        sys.stdout.write(f"エラーが発生しました: {e}\n")
        traceback.print_exc()
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
