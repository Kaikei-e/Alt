#!/usr/bin/env python3
"""
問題のあるサマリーの元記事の実際の内容を確認するスクリプト
"""

import os
import sys
from pathlib import Path
from typing import Dict, List, Optional

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    print("psycopg2が必要です。インストールしてください: pip install psycopg2-binary")
    sys.exit(1)


def load_env_vars() -> Dict[str, str]:
    """環境変数または.envファイルからデータベース接続情報を読み込む"""
    env_file = Path(__file__).parent.parent / ".env"
    env_vars = {}

    if env_file.exists():
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
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
            host=host,
            port=port,
            user=user,
            password=password,
            dbname=dbname,
            connect_timeout=10
        )
        return conn
    except psycopg2.Error as e:
        print(f"データベース接続エラー: {e}")
        sys.exit(1)


def inspect_article_content(cursor: RealDictCursor, article_id: str) -> Optional[Dict]:
    """指定された記事IDの内容を取得"""
    query = """
        SELECT
            a.id,
            a.title,
            a.content,
            a.url,
            a.created_at,
            LENGTH(a.content) as content_length,
            -- HTMLタグの数をカウント
            (LENGTH(a.content) - LENGTH(REGEXP_REPLACE(a.content, '<[^>]+>', '', 'g'))) / 10 as estimated_html_tags
        FROM articles a
        WHERE a.id = %s
    """

    cursor.execute(query, (article_id,))
    row = cursor.fetchone()

    if row:
        return dict(row)
    return None


def inspect_problematic_summaries(cursor: RealDictCursor) -> List[Dict]:
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


def analyze_content(content: str) -> Dict:
    """記事の内容を分析"""
    if not content:
        return {
            "is_html": False,
            "html_tag_count": 0,
            "html_ratio": 0.0,
            "starts_with_html": False,
            "has_meaningful_text": False,
            "sample_start": "",
            "sample_end": ""
        }

    # HTMLタグの検出
    import re
    html_tags = re.findall(r'<[^>]+>', content)
    html_tag_count = len(html_tags)
    html_ratio = (len(''.join(html_tags)) / len(content)) if content else 0.0

    # 最初の部分を確認
    starts_with_html = content.strip().startswith('<!') or content.strip().startswith('<html')

    # 意味のあるテキストがあるか（HTMLタグ以外の文字が50%以上）
    text_only = re.sub(r'<[^>]+>', '', content)
    text_ratio = len(text_only.strip()) / len(content) if content else 0.0
    has_meaningful_text = text_ratio > 0.5

    return {
        "is_html": html_ratio > 0.3 or starts_with_html,
        "html_tag_count": html_tag_count,
        "html_ratio": round(html_ratio * 100, 2),
        "starts_with_html": starts_with_html,
        "has_meaningful_text": has_meaningful_text,
        "text_ratio": round(text_ratio * 100, 2),
        "sample_start": content[:200],
        "sample_end": content[-200:] if len(content) > 200 else ""
    }


def main():
    """メイン処理"""
    print("=" * 80)
    print("問題のあるサマリーの元記事の内容を確認します")
    print("=" * 80)

    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    try:
        # 直近30件のサマリーとその元記事を取得
        print("\n直近30件のサマリーと元記事を取得中...")
        summaries = inspect_problematic_summaries(cursor)

        print(f"\n✓ {len(summaries)}件のデータを取得しました\n")

        # 各サマリーを分析
        html_issues = []
        repetition_issues = []
        short_issues = []
        good_summaries = []

        for idx, data in enumerate(summaries, 1):
            summary = data.get("summary_japanese", "")
            article_content = data.get("article_content", "")

            # 記事の内容を分析
            content_analysis = analyze_content(article_content)

            # サマリーの問題を判定
            summary_has_repetition = (
                len(summary) > 50 and
                (summary.count(summary[:10]) > 3 if len(summary) > 10 else False)
            )
            summary_is_short = len(summary.strip()) < 50

            # カテゴリ分け
            if content_analysis["is_html"]:
                html_issues.append((idx, data, content_analysis))
            elif summary_has_repetition:
                repetition_issues.append((idx, data, content_analysis))
            elif summary_is_short:
                short_issues.append((idx, data, content_analysis))
            else:
                good_summaries.append((idx, data, content_analysis))

        # 結果を表示
        print(f"\n【分析結果】")
        print(f"- HTMLが含まれている記事: {len(html_issues)}件")
        print(f"- 繰り返しパターンのサマリー: {len(repetition_issues)}件")
        print(f"- 短すぎるサマリー: {len(short_issues)}件")
        print(f"- 正常なサマリー: {len(good_summaries)}件")

        # HTML問題の詳細
        if html_issues:
            print(f"\n{'='*80}")
            print("【HTMLが含まれている記事】")
            print(f"{'='*80}")
            for idx, data, analysis in html_issues[:5]:  # 最初の5件のみ表示
                print(f"\n--- サマリー #{idx} ---")
                print(f"記事ID: {data.get('article_id')}")
                print(f"記事タイトル: {data.get('article_title', 'N/A')}")
                print(f"記事URL: {data.get('article_url', 'N/A')}")
                print(f"\n記事の内容分析:")
                print(f"  - HTMLタグ数: {analysis['html_tag_count']}")
                print(f"  - HTML比率: {analysis['html_ratio']}%")
                print(f"  - テキスト比率: {analysis['text_ratio']}%")
                print(f"  - HTMLで始まる: {analysis['starts_with_html']}")
                print(f"  - 意味のあるテキスト: {analysis['has_meaningful_text']}")
                print(f"\n記事の最初の200文字:")
                print(f"  {analysis['sample_start']}")
                print(f"\n生成されたサマリー（最初の200文字）:")
                summary = data.get('summary_japanese', '')[:200]
                print(f"  {summary}")
                print()

        # 繰り返し問題の詳細
        if repetition_issues:
            print(f"\n{'='*80}")
            print("【繰り返しパターンのサマリー】")
            print(f"{'='*80}")
            for idx, data, analysis in repetition_issues[:3]:  # 最初の3件のみ表示
                print(f"\n--- サマリー #{idx} ---")
                print(f"記事ID: {data.get('article_id')}")
                print(f"記事タイトル: {data.get('article_title', 'N/A')}")
                print(f"\n記事の内容分析:")
                print(f"  - HTMLタグ数: {analysis['html_tag_count']}")
                print(f"  - HTML比率: {analysis['html_ratio']}%")
                print(f"  - テキスト比率: {analysis['text_ratio']}%")
                print(f"  - 意味のあるテキスト: {analysis['has_meaningful_text']}")
                print(f"\n記事の最初の200文字:")
                print(f"  {analysis['sample_start']}")
                print(f"\n生成されたサマリー（最初の300文字）:")
                summary = data.get('summary_japanese', '')[:300]
                print(f"  {summary}")
                print()

        # 正常なサマリーの例
        if good_summaries:
            print(f"\n{'='*80}")
            print("【正常なサマリーの例】")
            print(f"{'='*80}")
            for idx, data, analysis in good_summaries[:2]:  # 最初の2件のみ表示
                print(f"\n--- サマリー #{idx} ---")
                print(f"記事タイトル: {data.get('article_title', 'N/A')}")
                print(f"\n記事の内容分析:")
                print(f"  - HTMLタグ数: {analysis['html_tag_count']}")
                print(f"  - HTML比率: {analysis['html_ratio']}%")
                print(f"  - テキスト比率: {analysis['text_ratio']}%")
                print(f"\n生成されたサマリー（最初の200文字）:")
                summary = data.get('summary_japanese', '')[:200]
                print(f"  {summary}")
                print()

        print(f"\n{'='*80}")
        print("分析完了")
        print(f"{'='*80}")

    except Exception as e:
        print(f"エラーが発生しました: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    main()

