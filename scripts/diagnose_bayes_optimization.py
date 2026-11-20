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
from dataclasses import dataclass
from typing import Any

import structlog
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine

logger = structlog.get_logger(__name__)
MIN_MATCH_RATE_PCT = 50


@dataclass(frozen=True)
class _MatchSummary:
    """タグ・ジャンルペア一致率の診断集計"""

    article_tags: list[str]
    matched_tags: int
    total_tags: int
    graph_tags: set[str]
    article_pairs: set[tuple[str, str]]
    matched_pairs: int
    total_pairs: int
    graph_pairs: set[tuple[str, str]]


def _parse_json_dict(value: object) -> dict[str, Any] | None:
    """JSON文字列または辞書を辞書へ正規化する（不正JSON時はNone）"""
    obj = value or {}
    if isinstance(obj, str):
        try:
            obj = json.loads(obj)
        except json.JSONDecodeError:
            return None
    return obj if isinstance(obj, dict) else {}


async def _log_graph_statistics(session: AsyncSession, window_label: str) -> None:
    """Step 1: tag_label_graphの状態をログ出力する"""
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


async def _fetch_recent_learning_rows(session: AsyncSession) -> list[dict[str, Any]]:
    """Step 2: 実際の記事のタグ・ジャンルレコードを取得する"""
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
    logger.info("取得した記事数: %d", len(rows))
    return rows


def _collect_tag_matches(
    rows: list[dict[str, Any]], graph_tags: set[str]
) -> tuple[list[str], int, int]:
    """記事のタグを収集し、graph_tagsとの一致数を返す"""
    article_tags: list[str] = []
    matched_tags = 0
    total_tags = 0

    for row in rows:
        tag_profile = _parse_json_dict(row.get("tag_profile"))
        if tag_profile is None:
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

    return article_tags, matched_tags, total_tags


def _collect_pair_matches(
    rows: list[dict[str, Any]], graph_pairs: set[tuple[str, str]]
) -> tuple[set[tuple[str, str]], int, int]:
    """記事のジャンル-タグペアを収集し、graph_pairsとの一致数を返す"""
    article_pairs: set[tuple[str, str]] = set()
    matched_pairs = 0
    total_pairs = 0

    for row in rows:
        tag_profile = _parse_json_dict(row.get("tag_profile"))
        refine_decision = _parse_json_dict(row.get("refine_decision"))
        if tag_profile is None or refine_decision is None:
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

    return article_pairs, matched_pairs, total_pairs


def _log_diagnosis_results(summary: _MatchSummary) -> None:
    """Steps 5-9: 一致率・未一致サンプル・頻出タグ・結論をログ出力する"""
    logger.info("=== Step 5: 診断結果 ===")
    tag_match_rate = (
        (summary.matched_tags / summary.total_tags * 100) if summary.total_tags > 0 else 0.0
    )
    pair_match_rate = (
        (summary.matched_pairs / summary.total_pairs * 100) if summary.total_pairs > 0 else 0.0
    )

    logger.info(
        "タグの一致率",
        total_tags=summary.total_tags,
        matched_tags=summary.matched_tags,
        match_rate_pct=round(tag_match_rate, 2),
    )

    logger.info(
        "ジャンル-タグペアの一致率",
        total_pairs=summary.total_pairs,
        matched_pairs=summary.matched_pairs,
        match_rate_pct=round(pair_match_rate, 2),
    )

    unmatched_tags = set(summary.article_tags) - summary.graph_tags
    if unmatched_tags:
        logger.warning(
            "一致しないタグのサンプル（上位10個）",
            unmatched_count=len(unmatched_tags),
            sample_tags=list(unmatched_tags)[:10],
        )

    unmatched_pairs = summary.article_pairs - summary.graph_pairs
    if unmatched_pairs:
        logger.warning(
            "一致しないジャンル-タグペアのサンプル（上位10個）",
            unmatched_count=len(unmatched_pairs),
            sample_pairs=list(unmatched_pairs)[:10],
        )

    tag_counter = Counter(summary.article_tags)
    logger.info(
        "最も頻繁に出現するタグ（上位10個）",
        top_tags=list(tag_counter.most_common(10)),
    )

    logger.info("=== 診断結論 ===")
    if tag_match_rate < MIN_MATCH_RATE_PCT:
        logger.error(
            "タグの一致率が低すぎます",
            match_rate_pct=round(tag_match_rate, 2),
            recommendation="tag_label_graphの構築条件（min_confidence等）を見直してください",
        )
    elif pair_match_rate < MIN_MATCH_RATE_PCT:
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


async def diagnose_bayes_optimization(dsn: str, window_label: str = "7d") -> None:
    """Bayes最適化が動かない原因を診断する"""
    engine = create_async_engine(dsn)

    async with engine.begin() as conn:
        session = AsyncSession(conn)

        try:
            await _log_graph_statistics(session, window_label)
            rows = await _fetch_recent_learning_rows(session)

            logger.info("=== Step 3: タグの一致率を計算 ===")
            tag_query = text("""
                SELECT DISTINCT tag
                FROM tag_label_graph
                WHERE window_label = :window_label
            """)
            tag_result = await session.execute(tag_query, {"window_label": window_label})
            graph_tags = {row.tag.strip().lower() for row in tag_result.all()}
            logger.info("tag_label_graphのユニークタグ数: %d", len(graph_tags))

            article_tags, matched_tags, total_tags = _collect_tag_matches(rows, graph_tags)

            logger.info("=== Step 4: ジャンル-タグのペアの一致率を計算 ===")
            pair_query = text("""
                SELECT genre, tag
                FROM tag_label_graph
                WHERE window_label = :window_label
            """)
            pair_result = await session.execute(pair_query, {"window_label": window_label})
            graph_pairs = {
                (row.genre.strip().lower(), row.tag.strip().lower()) for row in pair_result.all()
            }
            logger.info("tag_label_graphのジャンル-タグペア数: %d", len(graph_pairs))

            article_pairs, matched_pairs, total_pairs = _collect_pair_matches(rows, graph_pairs)

            _log_diagnosis_results(
                _MatchSummary(
                    article_tags=article_tags,
                    matched_tags=matched_tags,
                    total_tags=total_tags,
                    graph_tags=graph_tags,
                    article_pairs=article_pairs,
                    matched_pairs=matched_pairs,
                    total_pairs=total_pairs,
                    graph_pairs=graph_pairs,
                )
            )

        finally:
            await session.close()

    await engine.dispose()


def main() -> None:
    """メイン関数"""
    dsn = os.getenv("RECAP_DB_DSN")
    if not dsn:
        sys.stderr.write("ERROR: RECAP_DB_DSN環境変数が設定されていません\n")
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
