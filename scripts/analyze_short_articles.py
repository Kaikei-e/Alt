#!/usr/bin/env python3
"""
短すぎる記事（100文字未満）を実際のDBを見て類型化するスクリプト
"""

import os
import sys
import re
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from collections import defaultdict

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    print("psycopg2が必要です。インストールしてください: pip install psycopg2-binary")
    sys.exit(1)

try:
    import bleach
except ImportError:
    print("bleachが必要です。インストールしてください: pip install bleach")
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


def clean_html_content(content: str) -> Tuple[str, bool]:
    """
    HTMLタグを除去してテキストを抽出（news-creatorと同じロジック）
    """
    if not content:
        return "", False

    original_length = len(content)

    # HTMLかどうかを判定
    html_indicators = [
        content.strip().startswith('<!doctype'),
        content.strip().startswith('<!DOCTYPE'),
        content.strip().startswith('<html'),
        content.strip().startswith('<HTML'),
    ]

    html_tags = re.findall(r'<[^>]+>', content)
    html_tag_count = len(html_tags)
    html_ratio = (len(''.join(html_tags)) / len(content)) if content else 0.0

    is_html = any(html_indicators) or (html_ratio > 0.3 and html_tag_count > 50)

    if not is_html:
        return content, False

    try:
        cleaned = bleach.clean(content, tags=[], strip=True)
        cleaned = bleach.clean(cleaned, tags=[], strip=True)
        cleaned = re.sub(r'\s+', ' ', cleaned)
        cleaned = cleaned.strip()
        cleaned = re.sub(r'\b[a-zA-Z-]+:\s*[^;]+;?', ' ', cleaned)
        cleaned = re.sub(r'https?://[^\s]+', ' ', cleaned)
        cleaned = re.sub(r'[^\w\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]{3,}', ' ', cleaned)
        cleaned = re.sub(r'\s+', ' ', cleaned).strip()
        return cleaned, True
    except Exception:
        # Fallback
        cleaned = re.sub(r'<[^>]+>', ' ', content)
        cleaned = re.sub(r'&[a-zA-Z0-9#]+;', ' ', cleaned)
        cleaned = re.sub(r'\s+', ' ', cleaned).strip()
        return cleaned, True


def categorize_short_article(content: str, cleaned_content: str, original_length: int, cleaned_length: int) -> str:
    """短い記事をカテゴリに分類"""
    # 1. 完全に空または空白のみ
    if not cleaned_content or not cleaned_content.strip():
        return "空または空白のみ"

    # 2. HTMLクリーニングで大幅に短くなった（元の10%未満）
    if cleaned_length < original_length * 0.1 and original_length > 100:
        return "HTMLクリーニングで大幅に短縮"

    # 3. URLのみまたはリンクのみ
    url_pattern = r'https?://[^\s]+'
    urls = re.findall(url_pattern, cleaned_content)
    if len(' '.join(urls)) > len(cleaned_content) * 0.8:
        return "URLのみ"

    # 4. 繰り返しパターン
    if len(cleaned_content) > 10:
        first_10 = cleaned_content[:10]
        if cleaned_content.count(first_10) > 3:
            return "繰り返しパターン"

    # 5. エラーメッセージやシステムメッセージ
    error_patterns = [
        r'404',
        r'403',
        r'500',
        r'Error',
        r'Forbidden',
        r'Not Found',
        r'Access Denied',
        r'Page not found',
    ]
    for pattern in error_patterns:
        if re.search(pattern, cleaned_content, re.IGNORECASE):
            return "エラーメッセージ"

    # 6. Zennの「Discussion」のみ
    if cleaned_content.strip() == "Discussion" or cleaned_content.strip().startswith("Discussion"):
        return "Zenn Discussionのみ"

    # 7. The Guardianの写真ギャラリー記事（「Explore more on these topics」）
    if "Explore more on these topics" in cleaned_content:
        return "写真ギャラリー記事（メタ情報のみ）"

    # 8. タグやカテゴリ情報のみ（技術タグが多く含まれる）
    tech_tags = ['Node.js', 'PDF', 'Puppeteer', 'aws', 'ses', 'Nodemailer', 'tech', 'GitHub',
                 'Linux', 'LVM', 'idea', 'Discussion', 'Property', 'Photography', 'Art']
    tag_count = sum(1 for tag in tech_tags if tag in cleaned_content)
    if tag_count >= 3 and len(cleaned_content) < 80:
        return "タグ・メタ情報のみ"

    # 9. タイトルのみまたは見出しのみ（改行なし、短い）
    if len(cleaned_content) < 50 and '\n' not in cleaned_content:
        return "タイトル/見出しのみ"

    # 10. 記号や特殊文字のみ
    text_only = re.sub(r'[^\w\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]', '', cleaned_content)
    if len(text_only) < len(cleaned_content) * 0.3:
        return "記号・特殊文字が多い"

    # 11. 短いテキスト（通常の短い記事）
    return "通常の短い記事"


def analyze_short_articles(cursor: RealDictCursor, limit: int = 100) -> List[Dict]:
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
        content = article.get('content', '')
        original_length = article.get('original_length', 0)

        # HTMLクリーニング
        cleaned_content, was_html = clean_html_content(content)
        cleaned_length = len(cleaned_content.strip())

        # 100文字未満かチェック（クリーニング後）
        if cleaned_length < 100:
            category = categorize_short_article(
                content, cleaned_content, original_length, cleaned_length
            )

            article['cleaned_content'] = cleaned_content
            article['cleaned_length'] = cleaned_length
            article['was_html'] = was_html
            article['category'] = category
            article['reduction_ratio'] = round((1 - (cleaned_length / original_length)) * 100, 2) if original_length > 0 else 0

            results.append(article)

    return results


def main():
    """メイン処理"""
    print("=" * 80)
    print("短すぎる記事（100文字未満）の類型化分析")
    print("=" * 80)

    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    try:
        print("\n短い記事を取得中...")
        short_articles = analyze_short_articles(cursor, limit=200)

        print(f"\n✓ {len(short_articles)}件の短い記事を取得しました\n")

        if not short_articles:
            print("短い記事が見つかりませんでした。")
            return

        # カテゴリごとに集計
        category_counts = defaultdict(int)
        category_examples = defaultdict(list)

        for article in short_articles:
            category = article['category']
            category_counts[category] += 1
            if len(category_examples[category]) < 5:  # 各カテゴリ最大5件
                category_examples[category].append(article)

        # 結果を表示
        print(f"\n【カテゴリ別集計】")
        print(f"{'='*80}")
        for category, count in sorted(category_counts.items(), key=lambda x: x[1], reverse=True):
            print(f"{category}: {count}件 ({count/len(short_articles)*100:.1f}%)")

        # 各カテゴリの詳細例
        print(f"\n【カテゴリ別詳細例】")
        print(f"{'='*80}")

        for category in sorted(category_counts.keys(), key=lambda x: category_counts[x], reverse=True):
            examples = category_examples[category]
            print(f"\n{'='*80}")
            print(f"【{category}】 ({category_counts[category]}件)")
            print(f"{'='*80}")

            for idx, article in enumerate(examples, 1):
                print(f"\n--- 例 #{idx} ---")
                print(f"記事ID: {article['id']}")
                print(f"タイトル: {article['title'][:100]}")
                print(f"URL: {article['url'][:100]}")
                print(f"作成日時: {article['created_at']}")
                print(f"\n元の長さ: {article['original_length']}文字")
                print(f"クリーニング後: {article['cleaned_length']}文字")
                print(f"HTML削除: {'はい' if article['was_html'] else 'いいえ'}")
                if article['was_html']:
                    print(f"削減率: {article['reduction_ratio']}%")
                print(f"\n元の内容（最初の300文字）:")
                print(f"  {article['content'][:300]}")
                print(f"\nクリーニング後:")
                print(f"  {article['cleaned_content'][:300]}")
                print()

        # 統計情報
        print(f"\n{'='*80}")
        print("【統計情報】")
        print(f"{'='*80}")

        html_count = sum(1 for a in short_articles if a['was_html'])
        avg_original = sum(a['original_length'] for a in short_articles) / len(short_articles)
        avg_cleaned = sum(a['cleaned_length'] for a in short_articles) / len(short_articles)

        print(f"総件数: {len(short_articles)}件")
        print(f"HTMLが含まれていた記事: {html_count}件 ({html_count/len(short_articles)*100:.1f}%)")
        print(f"平均元の長さ: {avg_original:.1f}文字")
        print(f"平均クリーニング後: {avg_cleaned:.1f}文字")
        print(f"平均削減率: {(1 - avg_cleaned/avg_original)*100:.1f}%")

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

