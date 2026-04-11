#!/usr/bin/env python3
"""
統計的にジャンル分類器の重み付けを計算するスクリプト。

recap_genre_learning_resultsテーブルから実際の分類結果を取得し、
alt-dbのarticlesテーブルから記事のタイトルと本文を取得して、
TF-IDFとEmbedding重み付けを統計的に計算します。

Usage:
    python scripts/compute_genre_weights_statistically.py \
        --recap-dsn "postgresql://user:pass@host/recap_db" \
        --alt-dsn "postgresql://user:pass@host/alt_db" \
        --output genre_classifier_weights.json \
        [--min-samples 10] \
        [--days 30]
"""

from __future__ import annotations

import argparse
import json
import os
import re
from collections import Counter
from pathlib import Path
from typing import Optional

import psycopg2
import psycopg2.extras
import math

# 既存の特徴量語彙（拡張セット）
FEATURE_VOCAB = [
    "人工知能",
    "自動運転",
    "資金調達",
    "投資",
    "決算",
    "政策",
    "政府",
    "遺伝子",
    "医療",
    "量子",
    "サッカー",
    "音楽",
    "confidential computing",
    "cybersecurity",
    "transformer",
    "diplomacy",
    "treaty",
    "economy",
    "business",
    # Art & Culture
    "art", "arts", "artistic", "painting", "sculpture", "museum", "gallery",
    "philosophy", "philosophical", "aesthetics", "heritage", "literature",
    "アート", "美術", "芸術", "絵画", "彫刻", "美術館", "哲学", "美学", "文学",
    # Developer Insights
    "developer", "engineering", "programming", "coding", "backend", "frontend",
    "devops", "infrastructure", "architecture", "api design",
    "開発者", "エンジニア", "プログラミング", "コーディング", "インフラ", "アーキテクチャ",
    # Pro IT Media
    "enterprise", "enterprise software", "it infrastructure", "data center",
    "cloud computing", "saas", "paas", "iaas", "cio", "cto",
    "エンタープライズ", "ITインフラ", "データセンター", "クラウドコンピューティング",
    # Consumer Tech
    "smartphone", "tablet", "laptop", "gadget", "device", "consumer electronics",
    "mobile", "ios", "android", "apple", "google", "samsung", "review",
    "スマートフォン", "タブレット", "ガジェット", "デバイス", "モバイル",
    # Global Politics
    "international", "global", "world news", "diplomacy", "foreign policy",
    "election", "government", "parliament", "congress", "senate", "president",
    "prime minister", "united nations", "nato", "eu",
    "国際", "世界", "外交", "選挙", "政府", "国会", "大統領", "首相", "国連",
    # Environment Policy
    "environmental policy", "climate policy", "carbon neutral", "net zero",
    "renewable energy policy", "green policy", "sustainability policy",
    "environmental regulation", "climate action",
    "環境政策", "気候政策", "カーボンニュートラル", "再生可能エネルギー政策", "環境規制",
    # Society & Justice
    "social justice", "human rights", "civil rights", "equality", "discrimination",
    "justice", "court", "law", "legal", "lawsuit", "trial", "sentence", "prison",
    "社会正義", "人権", "市民権", "平等", "差別", "司法", "裁判", "法", "法律", "訴訟",
    # Travel & Lifestyle
    "travel", "tourism", "vacation", "trip", "journey", "destination", "hotel",
    "resort", "airline", "flight", "cruise", "adventure",
    "旅行", "観光", "バケーション", "旅", "目的地", "ホテル", "リゾート", "フライト",
    # Security Policy
    "security policy", "cybersecurity policy", "data protection", "privacy policy",
    "gdpr", "compliance", "security regulation", "cyber defense", "threat intelligence",
    "セキュリティ政策", "サイバーセキュリティ政策", "データ保護", "プライバシー政策",
    # Business & Finance
    "finance", "financial", "banking", "investment", "stock market", "trading",
    "gdp", "inflation", "monetary policy", "fiscal policy",
    "金融", "財務", "銀行", "投資", "株式市場", "取引", "経済", "GDP", "インフレ",
    # AI Research
    "ai research", "machine learning research", "deep learning research",
    "neural network research", "nlp research", "computer vision research",
    "ai paper", "arxiv", "conference", "academic", "research paper",
    "AI研究", "機械学習研究", "深層学習研究", "ニューラルネットワーク研究", "研究論文", "学術",
    # AI Policy
    "ai policy", "ai regulation", "ai ethics", "ai governance", "ai safety",
    "ai alignment", "responsible ai", "ai legislation",
    "AI政策", "AI規制", "AI倫理", "AIガバナンス", "AI安全性", "責任あるAI",
    # Games & Puzzles
    "game", "gaming", "puzzle", "crossword", "sudoku", "video game", "console",
    "playstation", "xbox", "nintendo", "esports", "gaming industry",
    "ゲーム", "パズル", "クロスワード", "数独", "ビデオゲーム", "コンソール", "eスポーツ",
]
EMBEDDING_DIM = 6

# Embedding lookup（簡易版：実際にはより洗練された埋め込みが必要）
EMBED_LOOKUP = {
    # AI/Tech (dim 0)
    "人工知能": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "自動運転": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "transformer": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "ai research": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "machine learning research": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "ai policy": [0.8, 0.0, 0.2, 0.0, 0.0, 0.0],
    # Business/Finance (dim 1)
    "資金調達": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "投資": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "決算": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "economy": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "business": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "finance": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "banking": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "stock market": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    # Politics (dim 2)
    "政策": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "政府": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "diplomacy": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "treaty": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "election": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "global politics": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    # Health/Science (dim 3)
    "遺伝子": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "医療": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "量子": [0.4, 0.1, 0.0, 0.9, 0.0, 0.0],
    "science": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    # Sports/Entertainment (dim 4)
    "サッカー": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    "game": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    "gaming": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    # Culture/Art (dim 5)
    "音楽": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "art": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "philosophy": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "literature": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    # Security (dim 0 + dim 2)
    "confidential computing": [0.8, 0.3, 0.0, 0.0, 0.0, 0.0],
    "cybersecurity": [0.8, 0.2, 0.0, 0.0, 0.0, 0.0],
    "security policy": [0.6, 0.0, 0.4, 0.0, 0.0, 0.0],
}

# 新ジャンルリスト（既存15 + 新規13 + other）
GENRES = [
    "ai",
    "tech",
    "business",
    "politics",
    "health",
    "sports",
    "science",
    "entertainment",
    "world",
    "security",
    "product",
    "design",
    "culture",
    "environment",
    "lifestyle",
    "art_culture",
    "developer_insights",
    "pro_it_media",
    "consumer_tech",
    "global_politics",
    "environment_policy",
    "society_justice",
    "travel_lifestyle",
    "security_policy",
    "business_finance",
    "ai_research",
    "ai_policy",
    "games_puzzles",
    "other",
]


def tokenize_text(text: str) -> list[str]:
    """テキストをトークン化（簡易版）"""
    if not text:
        return []
    # 日本語と英語の両方に対応
    text_lower = text.lower()
    # 空白、句読点で分割
    tokens = re.findall(r'\b\w+\b|[ぁ-んァ-ヶー一-龯]+', text_lower)
    return tokens


def expand_tokens(tokens: list[str]) -> list[str]:
    """トークンを拡張（複数形の処理など）"""
    expanded = []
    for token in tokens:
        expanded.append(token)
        # 英語の複数形処理
        if token.endswith("s") and len(token) > 3:
            expanded.append(token[:-1])
    return expanded


def fetch_learning_results(recap_dsn: str, days: int = 30) -> list[dict]:
    """recap_genre_learning_resultsテーブルから学習結果を取得"""
    conn = psycopg2.connect(recap_dsn)
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute(
                """
                SELECT
                    article_id,
                    COALESCE(refine_decision->>'final_genre', refine_decision->>'genre') as genre,
                    refine_decision->>'confidence' as confidence,
                    created_at
                FROM recap_genre_learning_results
                WHERE created_at > NOW() - INTERVAL '%s days'
                  AND (refine_decision->>'final_genre' IS NOT NULL
                       OR refine_decision->>'genre' IS NOT NULL)
                  AND COALESCE(refine_decision->>'final_genre', refine_decision->>'genre') != ''
                  AND (refine_decision->>'confidence')::float IS NOT NULL
                ORDER BY created_at DESC
                """,
                (days,),
            )
            return [dict(row) for row in cur.fetchall()]
    finally:
        conn.close()


def fetch_article_content(alt_dsn: str, article_id: str) -> Optional[dict]:
    """alt-dbのarticlesテーブルから記事のタイトルと本文を取得"""
    conn = psycopg2.connect(alt_dsn)
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute(
                """
                SELECT id, title, content, url
                FROM articles
                WHERE id::text = %s
                """,
                (article_id,),
            )
            row = cur.fetchone()
            if row:
                return dict(row)
            return None
    finally:
        conn.close()


def build_feature_counts(samples: list[dict]) -> tuple[dict[str, Counter], Counter]:
    """各ジャンルに対する特徴量の出現回数を集計"""
    feature_counts: dict[str, Counter] = {genre: Counter() for genre in GENRES}
    genre_totals: Counter = Counter()

    for sample in samples:
        genre = sample.get("genre", "").lower()
        if genre not in feature_counts:
            continue

        tokens = sample.get("tokens", [])
        expanded_tokens = expand_tokens(tokens)

        genre_totals[genre] += 1
        for token in expanded_tokens:
            # 特徴量語彙に含まれるトークンのみカウント
            if token in FEATURE_VOCAB:
                feature_counts[genre][token] += 1

    return feature_counts, genre_totals


def compute_tfidf_weights(
    feature_counts: dict[str, Counter],
    genre_totals: Counter,
) -> tuple[list[list[float]], list[float]]:
    """TF-IDF重み付けを計算"""
    tfidf_weights: list[list[float]] = []
    total_docs = sum(genre_totals.values()) or 1

    # termごとのidfを先に計算してキャッシュ
    idf_values: list[float] = []
    for term in FEATURE_VOCAB:
        docs_with_term = sum(1 for g in GENRES if feature_counts[g][term] > 0)
        if docs_with_term > 0:
            idf = 1.0 + math.log((total_docs + 1) / (docs_with_term + 1))
        else:
            idf = 1.0
        idf_values.append(idf)

    for genre in GENRES:
        total = max(1, genre_totals[genre])
        row: list[float] = []
        for idx, term in enumerate(FEATURE_VOCAB):
            tf = feature_counts[genre][term] / total if total > 0 else 0.0
            weight = round(tf * idf_values[idx] * 1.5, 3)
            row.append(weight)
        tfidf_weights.append(row)

    return tfidf_weights, idf_values


def compute_embedding_weights(feature_counts: dict[str, Counter]) -> list[list[float]]:
    """Embedding重み付けを計算"""
    embedding_weights = []

    for genre in GENRES:
        agg = [0.0] * EMBEDDING_DIM
        hits = 0

        for term in FEATURE_VOCAB:
            vec = EMBED_LOOKUP.get(term)
            if vec and feature_counts[genre][term] > 0:
                hits += 1
                for idx, value in enumerate(vec):
                    agg[idx] += value

        if hits > 0:
            agg = [round(v / hits, 3) for v in agg]
        else:
            # デフォルト値（均等分布）
            agg = [round(1.0 / EMBEDDING_DIM, 3)] * EMBEDDING_DIM

        embedding_weights.append(agg)

    return embedding_weights


def compute_bias(genre_totals: Counter) -> list[float]:
    """バイアスを計算（ジャンルの出現頻度に基づく）"""
    total = sum(genre_totals.values())
    if total == 0:
        return [-0.2] * len(GENRES)

    bias = []
    for genre in GENRES:
        count = genre_totals[genre]
        # 出現頻度に基づくバイアス（-0.3から-0.1の範囲）
        freq = count / total if total > 0 else 0.0
        bias_val = -0.3 + (freq * 0.2)  # 頻度が高いほどバイアスが小さく（正に近く）なる
        bias.append(round(bias_val, 3))

    return bias


def main() -> None:
    parser = argparse.ArgumentParser(
        description="統計的にジャンル分類器の重み付けを計算"
    )
    parser.add_argument(
        "--recap-dsn",
        default=os.getenv("RECAP_DB_DSN"),
        help="Recap DB DSN (環境変数 RECAP_DB_DSN も使用可能)",
    )
    parser.add_argument(
        "--alt-dsn",
        default=os.getenv("ALT_DB_DSN"),
        help="Alt DB DSN (環境変数 ALT_DB_DSN も使用可能)",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("genre_classifier_weights.json"),
        help="出力JSONファイルのパス",
    )
    parser.add_argument(
        "--min-samples",
        type=int,
        default=10,
        help="各ジャンルに必要な最小サンプル数",
    )
    parser.add_argument(
        "--days",
        type=int,
        default=30,
        help="取得するデータの日数（デフォルト: 30日）",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="詳細なログを出力",
    )

    args = parser.parse_args()

    if not args.recap_dsn:
        print("Error: --recap-dsn または RECAP_DB_DSN 環境変数が必要です")
        return

    if not args.alt_dsn:
        print("Error: --alt-dsn または ALT_DB_DSN 環境変数が必要です")
        return

    print(f"学習結果を取得中（過去{args.days}日）...")
    learning_results = fetch_learning_results(args.recap_dsn, args.days)
    print(f"取得した学習結果: {len(learning_results)}件")

    if not learning_results:
        print("Error: 学習結果が取得できませんでした")
        return

    print("記事のタイトルと本文を取得中...")
    samples = []
    # サンプル数を制限（処理時間を短縮）
    max_samples = min(10000, len(learning_results))  # 最大10,000件まで処理
    if len(learning_results) > max_samples:
        print(f"  サンプル数が多いため、最初の{max_samples}件のみ処理します")

    for i, result in enumerate(learning_results[:max_samples]):
        if args.verbose and (i + 1) % 100 == 0:
            print(f"  処理中: {i + 1}/{min(max_samples, len(learning_results))}")
        elif (i + 1) % 1000 == 0:
            print(f"  処理中: {i + 1}/{min(max_samples, len(learning_results))}")

        article_id = result["article_id"]
        article = fetch_article_content(args.alt_dsn, article_id)

        if article:
            title = article.get("title", "")
            content = article.get("content", "")
            combined_text = f"{title} {content}"
            tokens = tokenize_text(combined_text)

            samples.append({
                "genre": result["genre"],
                "confidence": float(result.get("confidence", 0.0)),
                "tokens": tokens,
                "title": title,
            })

    print(f"有効なサンプル数: {len(samples)}件")

    # ジャンル別のサンプル数を確認
    genre_counts = Counter(s["genre"].lower() for s in samples)
    print("\nジャンル別サンプル数:")
    for genre in GENRES:
        count = genre_counts.get(genre, 0)
        if count > 0:
            print(f"  {genre:20s}: {count:4d}件")
        elif count < args.min_samples:
            print(f"  {genre:20s}: {count:4d}件 (警告: 最小サンプル数未満)")

    # 特徴量カウントを構築
    print("\n特徴量を計算中...")
    feature_counts, genre_totals = build_feature_counts(samples)

    # 重み付けを計算
    print("TF-IDF重み付けを計算中...")
    tfidf_weights, idf_values = compute_tfidf_weights(feature_counts, genre_totals)

    print("Embedding重み付けを計算中...")
    embedding_weights = compute_embedding_weights(feature_counts)

    print("バイアスを計算中...")
    bias = compute_bias(genre_totals)

    # 結果をJSON形式で出力
    weights = {
        "_comment": "統計的に計算されたジャンル分類器の重み付け。recap_genre_learning_resultsテーブルから取得したデータに基づく。",
        "feature_dim": len(FEATURE_VOCAB),
        "embedding_dim": EMBEDDING_DIM,
        "genres": GENRES,
        "tfidf_weights": tfidf_weights,
        "embedding_weights": embedding_weights,
        "bias": bias,
        "feature_vocab": FEATURE_VOCAB,
        "feature_idf": idf_values,
        "bm25_k1": 1.6,
        "bm25_b": 0.75,
        "average_doc_len": 320.0,
    }

    args.output.write_text(
        json.dumps(weights, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )

    print(f"\n重み付けを {args.output} に保存しました")
    print(f"  特徴量次元: {len(FEATURE_VOCAB)}")
    print(f"  Embedding次元: {EMBEDDING_DIM}")
    print(f"  ジャンル数: {len(GENRES)}")


if __name__ == "__main__":
    main()

