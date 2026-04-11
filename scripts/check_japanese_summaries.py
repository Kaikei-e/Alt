#!/usr/bin/env python3
"""
データベースから直近30件の日本語サマリーを取得し、品質を評価するスクリプト
"""

from __future__ import annotations

import os
import sys
import re
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

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

    # Docker Compose環境では、DB_HOSTが"db"の場合、localhostに接続
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


def fetch_recent_summaries(cursor: RealDictCursor, limit: int = 30) -> List[Dict[str, Any]]:
    """直近の日本語サマリーを取得"""
    query = """
        SELECT
            a_s.id as summary_id,
            a_s.article_id,
            a_s.article_title,
            a_s.summary_japanese,
            a_s.created_at,
            a.content as article_content,
            a.url as article_url
        FROM article_summaries a_s
        LEFT JOIN articles a ON a_s.article_id = a.id
        WHERE a_s.summary_japanese IS NOT NULL
          AND a_s.summary_japanese != ''
        ORDER BY a_s.created_at DESC, a_s.id DESC
        LIMIT %s
    """

    cursor.execute(query, (limit,))
    rows = cursor.fetchall()

    return [dict(row) for row in rows]


def evaluate_summary_quality(
    summary: str,
    article_title: Optional[str],
    article_content: Optional[str]
) -> Dict[str, Any]:
    """サマリーの品質を評価"""
    issues = []
    score = 100.0

    # 文字数チェック
    summary_length = len(summary.strip())
    if summary_length < 50:
        issues.append(f"サマリーが短すぎます（{summary_length}文字）")
        score -= 30.0
    elif summary_length < 100:
        issues.append(f"サマリーがやや短いです（{summary_length}文字）")
        score -= 10.0
    elif summary_length > 2000:
        issues.append(f"サマリーが長すぎます（{summary_length}文字）")
        score -= 20.0

    # 誤字脱字・不自然な表現のチェック
    # JSONゴミが混入していないか
    if summary.strip().startswith("{") or summary.strip().startswith("["):
        issues.append("JSON形式のデータが混入している可能性があります")
        score -= 50.0

    # 引用符が多すぎる（JSONの可能性）
    quote_ratio = summary.count('"') / max(len(summary), 1)
    if quote_ratio > 0.15:
        issues.append("引用符が多すぎます（JSONデータの可能性）")
        score -= 30.0

    # 不自然な繰り返し
    words = summary.split()
    if len(words) > 0:
        unique_ratio = len(set(words)) / len(words)
        if unique_ratio < 0.3:
            issues.append("同じ単語の繰り返しが多すぎます")
            score -= 20.0

    # 元記事との整合性チェック（可能な場合）
    if article_title and article_content:
        title_words = set(re.findall(r'\w+', article_title.lower()))
        summary_words = set(re.findall(r'\w+', summary.lower()))

        # タイトルとサマリーの関連性が低い場合
        if len(title_words) > 0:
            overlap = len(title_words & summary_words) / len(title_words)
            if overlap < 0.1:
                issues.append("元記事のタイトルとサマリーの関連性が低い可能性があります")
                score -= 15.0

    # 不完全な文章（句読点で終わっていない）
    if summary.strip() and not summary.strip()[-1] in ['。', '！', '？', '.', '!', '?']:
        issues.append("サマリーが句読点で終わっていません")
        score -= 5.0

    # 空白や改行が多すぎる
    if summary.count('\n') > 10:
        issues.append("改行が多すぎます")
        score -= 10.0

    score = max(0.0, min(100.0, score))

    return {
        "score": round(score, 1),
        "issues": issues,
        "length": summary_length,
        "word_count": len(words) if 'words' in locals() else 0,
    }


def search_web_for_verification(title: Optional[str], url: Optional[str]) -> Optional[Dict[str, Any]]:
    """Web検索で情報を確認（必要時）"""
    if not title:
        return None

    try:
        # タイトルからキーワードを抽出（最初の50文字）
        search_query = title[:50] if len(title) > 50 else title

        # 実際のWeb検索は外部ツールを使用するため、ここでは検索クエリを返す
        # 実際の実装では、web_searchツールを使用して検索結果を取得
        return {
            "search_query": search_query,
            "note": "Web検索は手動で実行してください",
            "suggested_url": url
        }
    except Exception as e:
        print(f"Web検索エラー: {e}")
        return None


def generate_report(
    summaries: List[Dict[str, Any]],
    output_file: Optional[str] = None
) -> str:
    """品質評価レポートを生成"""
    report_lines = []
    report_lines.append("# 日本語サマリー品質評価レポート\n\n")
    report_lines.append(f"**生成日時**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n")
    report_lines.append(f"**評価対象**: {len(summaries)}件\n\n")
    report_lines.append("---\n\n")

    # 統計情報
    total_score = 0.0
    low_quality_count = 0
    medium_quality_count = 0
    high_quality_count = 0

    for summary_data in summaries:
        quality = summary_data.get("quality", {})
        score = quality.get("score", 0.0)
        total_score += score

        if score < 50:
            low_quality_count += 1
        elif score < 70:
            medium_quality_count += 1
        else:
            high_quality_count += 1

    avg_score = total_score / len(summaries) if summaries else 0.0

    report_lines.append("## 統計情報\n\n")
    report_lines.append(f"- **平均品質スコア**: {avg_score:.1f}/100\n")
    report_lines.append(f"- **高品質（70点以上）**: {high_quality_count}件\n")
    report_lines.append(f"- **中品質（50-69点）**: {medium_quality_count}件\n")
    report_lines.append(f"- **低品質（50点未満）**: {low_quality_count}件\n\n")
    report_lines.append("---\n\n")

    # 各サマリーの詳細
    report_lines.append("## サマリー詳細\n\n")

    for idx, summary_data in enumerate(summaries, 1):
        report_lines.append(f"### {idx}. サマリー #{summary_data.get('summary_id', 'N/A')}\n\n")

        report_lines.append(f"**記事ID**: `{summary_data.get('article_id', 'N/A')}`\n\n")
        report_lines.append(f"**記事タイトル**: {summary_data.get('article_title', 'N/A')}\n\n")
        report_lines.append(f"**作成日時**: {summary_data.get('created_at', 'N/A')}\n\n")

        if summary_data.get('article_url'):
            report_lines.append(f"**記事URL**: {summary_data.get('article_url')}\n\n")

        quality = summary_data.get("quality", {})
        score = quality.get("score", 0.0)
        issues = quality.get("issues", [])

        # 品質スコア
        score_emoji = "🟢" if score >= 70 else "🟡" if score >= 50 else "🔴"
        report_lines.append(f"**品質スコア**: {score_emoji} {score}/100\n\n")

        if issues:
            report_lines.append("**問題点**:\n\n")
            for issue in issues:
                report_lines.append(f"- ⚠️ {issue}\n")
            report_lines.append("\n")

        # サマリー本文
        summary_text = summary_data.get("summary_japanese", "")
        report_lines.append("**サマリー本文**:\n\n")
        report_lines.append("```\n")
        report_lines.append(summary_text)
        report_lines.append("\n```\n\n")

        # 元記事の内容（最初の500文字）
        if summary_data.get("article_content"):
            content = summary_data.get("article_content", "")
            preview = content[:500] + "..." if len(content) > 500 else content
            report_lines.append("**元記事の内容（プレビュー）**:\n\n")
            report_lines.append("```\n")
            report_lines.append(preview)
            report_lines.append("\n```\n\n")

        report_lines.append("---\n\n")

    report_text = "".join(report_lines)

    # ファイルに保存
    if output_file:
        output_path = Path(__file__).parent / "reports" / output_file
        output_path.parent.mkdir(parents=True, exist_ok=True)
        with open(output_path, "w", encoding="utf-8") as f:
            f.write(report_text)
        print(f"✓ レポートを保存しました: {output_path}")

    return report_text


def main():
    """メイン処理"""
    print("日本語サマリー品質確認を開始します...")

    # データベース接続
    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    try:
        # 直近30件のサマリーを取得
        print("データベースからサマリーを取得中...")
        summaries = fetch_recent_summaries(cursor, limit=30)

        if not summaries:
            print("サマリーが見つかりませんでした。")
            return

        print(f"✓ {len(summaries)}件のサマリーを取得しました。")

        # 品質評価
        print("品質評価を実行中...")
        for summary_data in summaries:
            quality = evaluate_summary_quality(
                summary=summary_data.get("summary_japanese", ""),
                article_title=summary_data.get("article_title"),
                article_content=summary_data.get("article_content")
            )
            summary_data["quality"] = quality

        # 品質が低いサマリーについてWeb検索（必要時）
        low_quality_summaries = [
            s for s in summaries
            if s.get("quality", {}).get("score", 100) < 50
        ]

        if low_quality_summaries:
            print(f"\n⚠️ 品質が低いサマリーが{len(low_quality_summaries)}件見つかりました。")
            print("Web検索による情報収集を実行中...")

            for summary_data in low_quality_summaries:
                # Web検索は後で実装（必要に応じて）
                web_info = search_web_for_verification(
                    title=summary_data.get("article_title"),
                    url=summary_data.get("article_url")
                )
                if web_info:
                    summary_data["web_verification"] = web_info

        # レポート生成
        print("\nレポートを生成中...")
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        report_file = f"japanese_summaries_quality_report_{timestamp}.md"
        report_text = generate_report(summaries, output_file=report_file)

        # コンソールにも表示
        print("\n" + "="*80)
        print(report_text)
        print("="*80)

        print(f"\n✓ 処理が完了しました。")

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

