#!/usr/bin/env python3
"""
問題のあるサマリーの元記事の実際の内容を確認するスクリプト
"""

import os
import re
import sys
import traceback
from pathlib import Path

MEANINGFUL_TEXT_RATIO_MIN = 0.5
HTML_RATIO_MIN = 0.3
PREVIEW_LEN = 200
SUMMARY_MIN_LEN = 50
REPEAT_PREFIX_LEN = 10
MIN_REPEAT_COUNT = 3

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    sys.stdout.write("psycopg2が必要です。インストールしてください: pip install psycopg2-binary\n")
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


def inspect_problematic_summaries(cursor: RealDictCursor) -> list[dict]:
    """問題のあるサマリーとその元記事を取得"""
    query = """
        SELECT
            a_s.id as summary_id,
            a_s.article_id,
            a_s.article_title,
            a_s.summary_japanese,
            a_s.created_at as summary_created_at,
            a.content as article_content,
            a.title as article_title_in_db,
            a.url as article_url,
            LENGTH(a.content) as article_content_length,
            -- HTMLタグの数をカウント
            (LENGTH(a.content) - LENGTH(REGEXP_REPLACE(a.content, '<[^>]+>', '', 'g'))) / 10 as estimated_html_tags
        FROM article_summaries a_s
        LEFT JOIN articles a ON a_s.article_id = a.id
        WHERE a_s.summary_japanese IS NOT NULL
          AND a_s.summary_japanese != ''
        ORDER BY a_s.created_at DESC
        LIMIT 30
    """

    cursor.execute(query)
    rows = cursor.fetchall()

    return [dict(row) for row in rows]


def analyze_content(content: str) -> dict:
    """記事の内容を分析"""
    if not content:
        return {
            "is_html": False,
            "html_tag_count": 0,
            "html_ratio": 0.0,
            "starts_with_html": False,
            "has_meaningful_text": False,
            "sample_start": "",
            "sample_end": "",
        }

    # HTMLタグの検出
    html_tags = re.findall(r"<[^>]+>", content)
    html_tag_count = len(html_tags)
    html_ratio = (len("".join(html_tags)) / len(content)) if content else 0.0

    # 最初の部分を確認
    starts_with_html = content.strip().startswith("<!") or content.strip().startswith("<html")

    # 意味のあるテキストがあるか（HTMLタグ以外の文字が50%以上）
    text_only = re.sub(r"<[^>]+>", "", content)
    text_ratio = len(text_only.strip()) / len(content) if content else 0.0
    has_meaningful_text = text_ratio > MEANINGFUL_TEXT_RATIO_MIN

    return {
        "is_html": html_ratio > HTML_RATIO_MIN or starts_with_html,
        "html_tag_count": html_tag_count,
        "html_ratio": round(html_ratio * 100, 2),
        "starts_with_html": starts_with_html,
        "has_meaningful_text": has_meaningful_text,
        "text_ratio": round(text_ratio * 100, 2),
        "sample_start": content[:PREVIEW_LEN],
        "sample_end": content[-PREVIEW_LEN:] if len(content) > PREVIEW_LEN else "",
    }


def _categorize_summaries(
    summaries: list[dict],
) -> tuple[list[tuple], list[tuple], list[tuple], list[tuple]]:
    """サマリーをHTML問題・繰り返し問題・短文問題・正常に分類する"""
    html_issues: list[tuple] = []
    repetition_issues: list[tuple] = []
    short_issues: list[tuple] = []
    good_summaries: list[tuple] = []

    for idx, data in enumerate(summaries, 1):
        summary = data.get("summary_japanese", "")
        article_content = data.get("article_content", "")

        content_analysis = analyze_content(article_content)

        summary_has_repetition = len(summary) > SUMMARY_MIN_LEN and (
            summary.count(summary[:REPEAT_PREFIX_LEN]) > MIN_REPEAT_COUNT
            if len(summary) > REPEAT_PREFIX_LEN
            else False
        )
        summary_is_short = len(summary.strip()) < SUMMARY_MIN_LEN

        if content_analysis["is_html"]:
            html_issues.append((idx, data, content_analysis))
        elif summary_has_repetition:
            repetition_issues.append((idx, data, content_analysis))
        elif summary_is_short:
            short_issues.append((idx, data, content_analysis))
        else:
            good_summaries.append((idx, data, content_analysis))

    return html_issues, repetition_issues, short_issues, good_summaries


def _print_html_issues(html_issues: list[tuple]) -> None:
    """HTMLが含まれている記事の詳細を表示"""
    if not html_issues:
        return
    sys.stdout.write(f"\n{'=' * 80}\n")
    sys.stdout.write("【HTMLが含まれている記事】\n")
    sys.stdout.write(f"{'=' * 80}\n")
    for idx, data, analysis in html_issues[:5]:
        sys.stdout.write(f"\n--- サマリー #{idx} ---\n")
        sys.stdout.write(f"記事ID: {data.get('article_id')}\n")
        sys.stdout.write(f"記事タイトル: {data.get('article_title', 'N/A')}\n")
        sys.stdout.write(f"記事URL: {data.get('article_url', 'N/A')}\n")
        sys.stdout.write("\n記事の内容分析:\n")
        sys.stdout.write(f"  - HTMLタグ数: {analysis['html_tag_count']}\n")
        sys.stdout.write(f"  - HTML比率: {analysis['html_ratio']}%\n")
        sys.stdout.write(f"  - テキスト比率: {analysis['text_ratio']}%\n")
        sys.stdout.write(f"  - HTMLで始まる: {analysis['starts_with_html']}\n")
        sys.stdout.write(f"  - 意味のあるテキスト: {analysis['has_meaningful_text']}\n")
        sys.stdout.write("\n記事の最初の200文字:\n")
        sys.stdout.write(f"  {analysis['sample_start']}\n")
        sys.stdout.write("\n生成されたサマリー（最初の200文字）:\n")
        summary = data.get("summary_japanese", "")[:200]
        sys.stdout.write(f"  {summary}\n")
        sys.stdout.write("\n")


def _print_repetition_issues(repetition_issues: list[tuple]) -> None:
    """繰り返しパターンのサマリー詳細を表示"""
    if not repetition_issues:
        return
    sys.stdout.write(f"\n{'=' * 80}\n")
    sys.stdout.write("【繰り返しパターンのサマリー】\n")
    sys.stdout.write(f"{'=' * 80}\n")
    for idx, data, analysis in repetition_issues[:3]:
        sys.stdout.write(f"\n--- サマリー #{idx} ---\n")
        sys.stdout.write(f"記事ID: {data.get('article_id')}\n")
        sys.stdout.write(f"記事タイトル: {data.get('article_title', 'N/A')}\n")
        sys.stdout.write("\n記事の内容分析:\n")
        sys.stdout.write(f"  - HTMLタグ数: {analysis['html_tag_count']}\n")
        sys.stdout.write(f"  - HTML比率: {analysis['html_ratio']}%\n")
        sys.stdout.write(f"  - テキスト比率: {analysis['text_ratio']}%\n")
        sys.stdout.write(f"  - 意味のあるテキスト: {analysis['has_meaningful_text']}\n")
        sys.stdout.write("\n記事の最初の200文字:\n")
        sys.stdout.write(f"  {analysis['sample_start']}\n")
        sys.stdout.write("\n生成されたサマリー（最初の300文字）:\n")
        summary = data.get("summary_japanese", "")[:300]
        sys.stdout.write(f"  {summary}\n")
        sys.stdout.write("\n")


def _print_good_summaries(good_summaries: list[tuple]) -> None:
    """正常なサマリーの例を表示"""
    if not good_summaries:
        return
    sys.stdout.write(f"\n{'=' * 80}\n")
    sys.stdout.write("【正常なサマリーの例】\n")
    sys.stdout.write(f"{'=' * 80}\n")
    for idx, data, analysis in good_summaries[:2]:
        sys.stdout.write(f"\n--- サマリー #{idx} ---\n")
        sys.stdout.write(f"記事タイトル: {data.get('article_title', 'N/A')}\n")
        sys.stdout.write("\n記事の内容分析:\n")
        sys.stdout.write(f"  - HTMLタグ数: {analysis['html_tag_count']}\n")
        sys.stdout.write(f"  - HTML比率: {analysis['html_ratio']}%\n")
        sys.stdout.write(f"  - テキスト比率: {analysis['text_ratio']}%\n")
        sys.stdout.write("\n生成されたサマリー（最初の200文字）:\n")
        summary = data.get("summary_japanese", "")[:200]
        sys.stdout.write(f"  {summary}\n")
        sys.stdout.write("\n")


def main() -> None:
    """メイン処理"""
    sys.stdout.write("=" * 80 + "\n")
    sys.stdout.write("問題のあるサマリーの元記事の内容を確認します\n")
    sys.stdout.write("=" * 80 + "\n")

    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    try:
        sys.stdout.write("\n直近30件のサマリーと元記事を取得中...\n")
        summaries = inspect_problematic_summaries(cursor)

        sys.stdout.write(f"\n✓ {len(summaries)}件のデータを取得しました\n\n")

        html_issues, repetition_issues, short_issues, good_summaries = _categorize_summaries(
            summaries
        )

        sys.stdout.write("\n【分析結果】\n")
        sys.stdout.write(f"- HTMLが含まれている記事: {len(html_issues)}件\n")
        sys.stdout.write(f"- 繰り返しパターンのサマリー: {len(repetition_issues)}件\n")
        sys.stdout.write(f"- 短すぎるサマリー: {len(short_issues)}件\n")
        sys.stdout.write(f"- 正常なサマリー: {len(good_summaries)}件\n")

        _print_html_issues(html_issues)
        _print_repetition_issues(repetition_issues)
        _print_good_summaries(good_summaries)

        sys.stdout.write(f"\n{'=' * 80}\n")
        sys.stdout.write("分析完了\n")
        sys.stdout.write(f"{'=' * 80}\n")

    except Exception as e:  # noqa: BLE001 — top-level CLI boundary prints traceback before DB cleanup and exit
        sys.stdout.write(f"エラーが発生しました: {e}\n")
        traceback.print_exc()
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()
