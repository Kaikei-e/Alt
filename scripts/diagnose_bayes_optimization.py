#!/usr/bin/env python3
"""診断スクリプト: Bayes最適化が動かない原因を特定する

このスクリプトは以下を確認します:
1. tag_label_graphの状態
2. 実際の記事のタグとtag_label_graphのタグの一致率
3. graph_boostが0になる理由
"""

import asyncio
import json
import os
import sys
from collections import Counter
from typing import Any

import structlog
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine

logger = structlog.get_logger(__name__)


async def diagnose_bayes_optimization(dsn: str, window_label: str = "7d") -> None:
    """Bayes最適化が動かない原因を診断する"""
    engine = create_async_engine(dsn)

    async with engine.begin() as conn:
        session = AsyncSession(conn)

        try:
            # 1. tag_label_graphの状態を確認
            logger.info("=== Step 1: tag_label_graphの状態を確認 ===")
            query = text("""
                SELECT
                    COUNT(*) as total_edges,
                    COUNT(DISTINCT genre) as unique_genres,
                    COUNT(DISTINCT tag) as unique_tags,
                    AVG(weight) as avg_weight,
                    MIN(weight) as min_weight,
                    MAX(weight) as max_weight
                FROM tag_label_graph
                WHERE window_label = :window_label
            """)
            result = await session.execute(query, {"window_label": window_label})
            row = result.first()
            if row:
                logger.info(
                    "tag_label_graph statistics",
                    total_edges=row.total_edges,
                    unique_genres=row.unique_genres,
                    unique_tags=row.unique_tags,
                    avg_weight=round(float(row.avg_weight or 0.0), 6),
                    min_weight=round(float(row.min_weight or 0.0), 6),
                    max_weight=round(float(row.max_weight or 0.0), 6),
                )

            # 2. 実際の記事のタグを取得
            logger.info("=== Step 2: 実際の記事のタグを取得 ===")
            query = text("""
                SELECT
                    rglr.coarse_candidates,
                    rglr.tag_profile,
                    rglr.refine_decision
                FROM recap_genre_learning_results rglr
                INNER JOIN recap_job_articles rja
                    ON rglr.job_id = rja.job_id
                    AND rglr.article_id = rja.article_id
                WHERE rja.published_at > NOW() - INTERVAL '7 days'
                  AND rglr.tag_profile IS NOT NULL
                  AND rglr.coarse_candidates IS NOT NULL
                LIMIT 100
            """)
            result = await session.execute(query)
            rows = [dict(row._mapping) for row in result.all()]
            logger.info(f"取得した記事数: {len(rows)}")

            # 3. タグの一致率を計算
            logger.info("=== Step 3: タグの一致率を計算 ===")

            # tag_label_graphのタグセットを取得
            query = text("""
                SELECT DISTINCT tag
                FROM tag_label_graph
                WHERE window_label = :window_label
            """)
            result = await session.execute(query, {"window_label": window_label})
            graph_tags = {row.tag.strip().lower() for row in result.all()}
            logger.info(f"tag_label_graphのユニークタグ数: {len(graph_tags)}")

            # 実際の記事のタグを収集
            article_tags: list[str] = []
            article_genres: list[str] = []
            matched_tags = 0
            total_tags = 0

            for row in rows:
                tag_profile = row.get("tag_profile") or {}
                if isinstance(tag_profile, str):
                    try:
                        tag_profile = json.loads(tag_profile)
                    except json.JSONDecodeError:
                        continue

                top_tags = tag_profile.get("top_tags") or []
                if not isinstance(top_tags, list):
                    continue

                for tag in top_tags:
                    if not isinstance(tag, dict):
                        continue
                    label = (tag.get("label") or "").strip().lower()
                    if label:
                        article_tags.append(label)
                        total_tags += 1
                        if label in graph_tags:
                            matched_tags += 1

                # ジャンルを取得
                refine_decision = row.get("refine_decision") or {}
                if isinstance(refine_decision, str):
                    try:
                        refine_decision = json.loads(refine_decision)
                    except json.JSONDecodeError:
                        continue
                genre = (refine_decision.get("final_genre") or "").strip().lower()
                if genre:
                    article_genres.append(genre)

            # 4. ジャンル-タグのペアの一致率を計算
            logger.info("=== Step 4: ジャンル-タグのペアの一致率を計算 ===")

            # tag_label_graphのジャンル-タグペアを取得
            query = text("""
                SELECT genre, tag
                FROM tag_label_graph
                WHERE window_label = :window_label
            """)
            result = await session.execute(query, {"window_label": window_label})
            graph_pairs = {(row.genre.strip().lower(), row.tag.strip().lower()) for row in result.all()}
            logger.info(f"tag_label_graphのジャンル-タグペア数: {len(graph_pairs)}")

            # 実際の記事のジャンル-タグペアを収集
            article_pairs: set[tuple[str, str]] = set()
            matched_pairs = 0
            total_pairs = 0

            for row in rows:
                tag_profile = row.get("tag_profile") or {}
                if isinstance(tag_profile, str):
                    try:
                        tag_profile = json.loads(tag_profile)
                    except json.JSONDecodeError:
                        continue

                refine_decision = row.get("refine_decision") or {}
                if isinstance(refine_decision, str):
                    try:
                        refine_decision = json.loads(refine_decision)
                    except json.JSONDecodeError:
                        continue

                genre = (refine_decision.get("final_genre") or "").strip().lower()
                if not genre:
                    continue

                top_tags = tag_profile.get("top_tags") or []
                if not isinstance(top_tags, list):
                    continue

                for tag in top_tags:
                    if not isinstance(tag, dict):
                        continue
                    label = (tag.get("label") or "").strip().lower()
                    if label:
                        pair = (genre, label)
                        article_pairs.add(pair)
                        total_pairs += 1
                        if pair in graph_pairs:
                            matched_pairs += 1

            # 5. 結果を出力
            logger.info("=== Step 5: 診断結果 ===")
            tag_match_rate = (matched_tags / total_tags * 100) if total_tags > 0 else 0.0
            pair_match_rate = (matched_pairs / total_pairs * 100) if total_pairs > 0 else 0.0

            logger.info(
                "タグの一致率",
                total_tags=total_tags,
                matched_tags=matched_tags,
                match_rate_pct=round(tag_match_rate, 2),
            )

            logger.info(
                "ジャンル-タグペアの一致率",
                total_pairs=total_pairs,
                matched_pairs=matched_pairs,
                match_rate_pct=round(pair_match_rate, 2),
            )

            # 6. 一致しないタグのサンプルを表示
            article_tag_set = set(article_tags)
            unmatched_tags = article_tag_set - graph_tags
            if unmatched_tags:
                logger.warning(
                    "一致しないタグのサンプル（上位10個）",
                    unmatched_count=len(unmatched_tags),
                    sample_tags=list(unmatched_tags)[:10],
                )

            # 7. 一致しないジャンル-タグペアのサンプルを表示
            unmatched_pairs = article_pairs - graph_pairs
            if unmatched_pairs:
                logger.warning(
                    "一致しないジャンル-タグペアのサンプル（上位10個）",
                    unmatched_count=len(unmatched_pairs),
                    sample_pairs=list(unmatched_pairs)[:10],
                )

            # 8. 最も頻繁に出現するタグを表示
            tag_counter = Counter(article_tags)
            logger.info(
                "最も頻繁に出現するタグ（上位10個）",
                top_tags=[(tag, count) for tag, count in tag_counter.most_common(10)],
            )

            # 9. 結論
            logger.info("=== 診断結論 ===")
            if tag_match_rate < 50:
                logger.error(
                    "タグの一致率が低すぎます",
                    match_rate_pct=round(tag_match_rate, 2),
                    recommendation="tag_label_graphの構築条件（min_confidence等）を見直してください",
                )
            elif pair_match_rate < 50:
                logger.error(
                    "ジャンル-タグペアの一致率が低すぎます",
                    match_rate_pct=round(pair_match_rate, 2),
                    recommendation="tag_label_graphの構築条件またはジャンル分類の精度を見直してください",
                )
            else:
                logger.info(
                    "一致率は良好です",
                    tag_match_rate_pct=round(tag_match_rate, 2),
                    pair_match_rate_pct=round(pair_match_rate, 2),
                    recommendation="他の原因（時間的な問題、データの不整合等）を調査してください",
                )

        finally:
            await session.close()

    await engine.dispose()


def main():
    """メイン関数"""
    dsn = os.getenv("RECAP_DB_DSN")
    if not dsn:
        print("ERROR: RECAP_DB_DSN環境変数が設定されていません", file=sys.stderr)
        sys.exit(1)

    # async DSNに変換
    if dsn.startswith("postgresql://"):
        dsn = dsn.replace("postgresql://", "postgresql+asyncpg://", 1)
    elif dsn.startswith("postgresql+psycopg2://"):
        dsn = dsn.replace("postgresql+psycopg2://", "postgresql+asyncpg://", 1)

    window_label = sys.argv[1] if len(sys.argv) > 1 else "7d"

    asyncio.run(diagnose_bayes_optimization(dsn, window_label))


if __name__ == "__main__":
    main()

