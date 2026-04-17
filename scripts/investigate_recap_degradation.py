#!/usr/bin/env python3
"""
Recap精度低下の実データ調査スクリプト

最新1件のJobを詳細に調査し、精度低下の原因を特定する。
"""

import asyncio
import json
import logging
import os
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urlparse, urlunparse

import pandas as pd
from sqlalchemy import text, inspect
from sqlalchemy.ext.asyncio import create_async_engine

# Database connection will be constructed manually

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def get_db_url() -> str:
    """データベース接続URLを取得"""
    # Try environment variables first
    db_url = os.getenv("RECAP_DB_URL") or os.getenv("RECAP_SUBWORKER_DB_URL")

    if not db_url:
        # Construct from individual env vars
        db_host = os.getenv("RECAP_DB_HOST", "localhost")
        db_port = os.getenv("RECAP_DB_PORT", "5435")
        db_user = os.getenv("RECAP_DB_USER", "recap_user")
        db_name = os.getenv("RECAP_DB_NAME", "recap")
        db_password = os.getenv("RECAP_DB_PASSWORD", "recap_db_pass_DO_NOT_USE_THIS")

        # Try to load password from secrets
        script_dir = Path(__file__).parent
        repo_root = script_dir.parent
        secret_path = repo_root / "secrets" / "recap_db_password.txt"

        if secret_path.exists():
            try:
                with open(secret_path, "r") as f:
                    db_password = f.read().strip()
                logger.info(f"Loaded password from {secret_path}")
            except Exception as e:
                logger.warning(f"Failed to read password from {secret_path}: {e}")

        db_url = f"postgresql+asyncpg://{db_user}:{db_password}@{db_host}:{db_port}/{db_name}"
    else:
        # Local execution: replace container name with localhost and adjust port
        if "recap-db" in db_url:
            db_url = db_url.replace("recap-db", "localhost").replace("5432", "5435")

        # Try to load password from secrets if not in URL
        if "@" not in db_url or ":" not in db_url.split("@")[0]:
            script_dir = Path(__file__).parent
            repo_root = script_dir.parent
            secret_path = repo_root / "secrets" / "recap_db_password.txt"

            if secret_path.exists():
                try:
                    with open(secret_path, "r") as f:
                        password = f.read().strip()

                    # Replace password in URL
                    u = urlparse(db_url)
                    if '@' in u.netloc:
                        user_pass, host_port = u.netloc.rsplit('@', 1)
                        if ':' in user_pass:
                            user, _ = user_pass.split(':', 1)
                            new_netloc = f"{user}:{password}@{host_port}"
                        else:
                            new_netloc = f"{user_pass}:{password}@{host_port}"
                        db_url = urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))

                    logger.info(f"Loaded password from {secret_path}")
                except Exception as e:
                    logger.warning(f"Failed to read password from {secret_path}: {e}")

    return db_url


async def fetch_latest_job(conn) -> Optional[Dict[str, Any]]:
    """最新1件のJobを取得"""
    query = text("""
        SELECT
            job_id,
            kicked_at,
            note
        FROM recap_jobs
        ORDER BY kicked_at DESC
        LIMIT 1
    """)

    result = await conn.execute(query)
    row = result.first()

    if not row:
        return None

    return {
        "job_id": str(row.job_id),
        "kicked_at": row.kicked_at.isoformat() if row.kicked_at else None,
        "note": row.note
    }


async def fetch_preprocess_metrics(conn, job_id: str) -> Optional[Dict[str, Any]]:
    """前処理メトリクスを取得"""
    query = text("""
        SELECT
            total_articles_fetched,
            articles_processed,
            articles_dropped_empty,
            articles_html_cleaned,
            total_characters,
            avg_chars_per_article,
            languages_detected
        FROM recap_preprocess_metrics
        WHERE job_id = :job_id
    """)

    result = await conn.execute(query, {"job_id": job_id})
    row = result.first()

    if not row:
        return None

    return {
        "total_articles_fetched": row.total_articles_fetched,
        "articles_processed": row.articles_processed,
        "articles_dropped_empty": row.articles_dropped_empty,
        "articles_html_cleaned": row.articles_html_cleaned,
        "total_characters": row.total_characters,
        "avg_chars_per_article": float(row.avg_chars_per_article) if row.avg_chars_per_article else None,
        "languages_detected": dict(row.languages_detected) if row.languages_detected else {}
    }


async def fetch_subworker_runs(conn, job_id: str) -> List[Dict[str, Any]]:
    """サブワーカーの実行結果を取得"""
    query = text("""
        SELECT
            id,
            genre,
            status,
            cluster_count,
            started_at,
            finished_at,
            request_payload,
            response_payload,
            error_message
        FROM recap_subworker_runs
        WHERE job_id = :job_id
        ORDER BY genre, started_at
    """)

    result = await conn.execute(query, {"job_id": job_id})
    runs = []

    for row in result:
        runs.append({
            "run_id": row.id,
            "genre": row.genre,
            "status": row.status,
            "cluster_count": row.cluster_count,
            "started_at": row.started_at.isoformat() if row.started_at else None,
            "finished_at": row.finished_at.isoformat() if row.finished_at else None,
            "request_payload": dict(row.request_payload) if row.request_payload else {},
            "response_payload": dict(row.response_payload) if row.response_payload else {},
            "error_message": row.error_message
        })

    return runs


async def fetch_clusters(conn, run_ids: List[int]) -> List[Dict[str, Any]]:
    """クラスタ情報を取得"""
    if not run_ids:
        return []

    query = text("""
        SELECT
            id,
            run_id,
            cluster_id,
            size,
            label,
            top_terms,
            stats
        FROM recap_subworker_clusters
        WHERE run_id = ANY(:run_ids)
        ORDER BY run_id, cluster_id
    """)

    result = await conn.execute(query, {"run_ids": run_ids})
    clusters = []

    for row in result:
        clusters.append({
            "cluster_row_id": row.id,
            "run_id": row.run_id,
            "cluster_id": row.cluster_id,
            "size": row.size,
            "label": row.label,
            "top_terms": list(row.top_terms) if row.top_terms else [],
            "stats": dict(row.stats) if row.stats else {}
        })

    return clusters


async def fetch_sentences(conn, cluster_row_ids: List[int]) -> List[Dict[str, Any]]:
    """文情報を取得"""
    if not cluster_row_ids:
        return []

    query = text("""
        SELECT
            cluster_row_id,
            source_article_id,
            sentence_text,
            lang,
            score
        FROM recap_subworker_sentences
        WHERE cluster_row_id = ANY(:cluster_row_ids)
        ORDER BY cluster_row_id, score DESC
    """)

    result = await conn.execute(query, {"cluster_row_ids": cluster_row_ids})
    sentences = []

    for row in result:
        sentences.append({
            "cluster_row_id": row.cluster_row_id,
            "source_article_id": row.source_article_id,
            "sentence_text": row.sentence_text[:100] + "..." if len(row.sentence_text) > 100 else row.sentence_text,
            "lang": row.lang,
            "score": float(row.score)
        })

    return sentences


async def fetch_diagnostics(conn, run_ids: List[int]) -> List[Dict[str, Any]]:
    """診断メトリクスを取得"""
    if not run_ids:
        return []

    query = text("""
        SELECT
            run_id,
            metric,
            value
        FROM recap_subworker_diagnostics
        WHERE run_id = ANY(:run_ids)
        ORDER BY run_id, metric
    """)

    result = await conn.execute(query, {"run_ids": run_ids})
    diagnostics = []

    for row in result:
        diagnostics.append({
            "run_id": row.run_id,
            "metric": row.metric,
            "value": dict(row.value) if isinstance(row.value, dict) else row.value
        })

    return diagnostics


async def fetch_job_articles(conn, job_id: str) -> Dict[str, Any]:
    """記事レベルの詳細情報を取得"""
    query = text("""
        SELECT
            COUNT(*) as total_articles,
            COUNT(DISTINCT normalized_hash) as unique_hashes,
            COUNT(DISTINCT lang_hint) as unique_languages
        FROM recap_job_articles
        WHERE job_id = :job_id
    """)

    result = await conn.execute(query, {"job_id": job_id})
    row = result.first()

    if not row:
        return {
            "total_articles": 0,
            "unique_hashes": 0,
            "unique_languages": 0
        }

    return {
        "total_articles": row.total_articles,
        "unique_hashes": row.unique_hashes,
        "unique_languages": row.unique_languages
    }


async def fetch_genre_evaluation(conn) -> Optional[Dict[str, Any]]:
    """最新のジャンル評価結果を取得"""
    query = text("""
        SELECT
            run_id,
            dataset_path,
            total_items,
            macro_precision,
            macro_recall,
            macro_f1,
            micro_precision,
            micro_recall,
            micro_f1,
            weighted_f1,
            macro_f1_valid,
            valid_genre_count,
            undefined_genre_count,
            created_at
        FROM recap_genre_evaluation_runs
        ORDER BY created_at DESC
        LIMIT 1
    """)

    result = await conn.execute(query)
    row = result.first()

    if not row:
        return None

    # ジャンル別メトリクスも取得
    metrics_query = text("""
        SELECT
            genre,
            tp,
            fp,
            fn_count,
            precision,
            recall,
            f1_score
        FROM recap_genre_evaluation_metrics
        WHERE run_id = :run_id
        ORDER BY genre
    """)

    metrics_result = await conn.execute(metrics_query, {"run_id": row.run_id})
    genre_metrics = []

    for m_row in metrics_result:
        genre_metrics.append({
            "genre": m_row.genre,
            "tp": m_row.tp,
            "fp": m_row.fp,
            "fn": m_row.fn_count,
            "precision": float(m_row.precision),
            "recall": float(m_row.recall),
            "f1_score": float(m_row.f1_score)
        })

    return {
        "run_id": str(row.run_id),
        "dataset_path": row.dataset_path,
        "total_items": row.total_items,
        "macro_precision": float(row.macro_precision),
        "macro_recall": float(row.macro_recall),
        "macro_f1": float(row.macro_f1),
        "micro_precision": float(row.micro_precision) if row.micro_precision else None,
        "micro_recall": float(row.micro_recall) if row.micro_recall else None,
        "micro_f1": float(row.micro_f1) if row.micro_f1 else None,
        "weighted_f1": float(row.weighted_f1) if row.weighted_f1 else None,
        "macro_f1_valid": float(row.macro_f1_valid) if row.macro_f1_valid else None,
        "valid_genre_count": row.valid_genre_count,
        "undefined_genre_count": row.undefined_genre_count,
        "created_at": row.created_at.isoformat() if row.created_at else None,
        "genre_metrics": genre_metrics
    }


def _safe_dsn_for_log(dsn: str) -> str:
    """Return a copy of *dsn* with any user/password stripped.

    ``dsn.split('@')[-1]`` leaks credentials when the password itself
    contains ``@`` or when the DSN has no userinfo at all. Parsing with
    urllib keeps the host/port/db visible without ever touching the
    secret.
    """
    parsed = urlparse(dsn)
    redacted = parsed._replace(netloc=parsed.hostname or "")
    if parsed.port is not None:
        redacted = redacted._replace(netloc=f"{parsed.hostname}:{parsed.port}")
    return urlunparse(redacted)


async def main():
    """メイン処理"""
    db_url = get_db_url()
    logger.info("Connecting to DB: %s", _safe_dsn_for_log(db_url))

    engine = create_async_engine(db_url, echo=False)

    try:
        async with engine.connect() as conn:
            # テーブル確認
            tables = await conn.run_sync(lambda sync_conn: inspect(sync_conn).get_table_names())
            logger.info(f"Available tables: {sorted(tables)}")

            # 最新Jobを取得
            logger.info("Fetching latest job...")
            latest_job = await fetch_latest_job(conn)

            if not latest_job:
                logger.error("No jobs found in database")
                return

            job_id = latest_job["job_id"]
            logger.info(f"Latest job ID: {job_id}")
            logger.info(f"Kicked at: {latest_job['kicked_at']}")

            # 各データを取得
            logger.info("Fetching preprocess metrics...")
            preprocess_metrics = await fetch_preprocess_metrics(conn, job_id)

            logger.info("Fetching job articles...")
            job_articles = await fetch_job_articles(conn, job_id)

            logger.info("Fetching subworker runs...")
            subworker_runs = await fetch_subworker_runs(conn, job_id)

            run_ids = [r["run_id"] for r in subworker_runs]

            logger.info("Fetching clusters...")
            clusters = await fetch_clusters(conn, run_ids)

            cluster_row_ids = [c["cluster_row_id"] for c in clusters]

            logger.info("Fetching sentences...")
            sentences = await fetch_sentences(conn, cluster_row_ids)

            logger.info("Fetching diagnostics...")
            diagnostics = await fetch_diagnostics(conn, run_ids)

            logger.info("Fetching genre evaluation...")
            genre_evaluation = await fetch_genre_evaluation(conn)

            # 結果をまとめる
            result = {
                "job": latest_job,
                "preprocess_metrics": preprocess_metrics,
                "job_articles": job_articles,
                "subworker_runs": subworker_runs,
                "clusters": clusters,
                "sentences_count": len(sentences),
                "diagnostics": diagnostics,
                "genre_evaluation": genre_evaluation,
                "summary": {
                    "total_runs": len(subworker_runs),
                    "succeeded_runs": len([r for r in subworker_runs if r["status"] == "succeeded"]),
                    "failed_runs": len([r for r in subworker_runs if r["status"] == "failed"]),
                    "partial_runs": len([r for r in subworker_runs if r["status"] == "partial"]),
                    "total_clusters": len(clusters),
                    "total_sentences": len(sentences),
                    "avg_cluster_size": sum(c["size"] for c in clusters) / len(clusters) if clusters else 0
                }
            }

            # JSON出力
            output_dir = Path(__file__).parent / "reports"
            output_dir.mkdir(exist_ok=True)
            output_file = output_dir / f"recap-investigation-{job_id[:8]}.json"

            with open(output_file, "w", encoding="utf-8") as f:
                json.dump(result, f, indent=2, ensure_ascii=False, default=str)

            logger.info(f"Results saved to {output_file}")

            # サマリを表示
            print("\n" + "="*80)
            print("調査結果サマリ")
            print("="*80)
            print(f"Job ID: {job_id}")
            print(f"実行日時: {latest_job['kicked_at']}")

            if preprocess_metrics:
                print(f"\n前処理メトリクス:")
                print(f"  取得記事数: {preprocess_metrics['total_articles_fetched']}")
                print(f"  処理済み記事数: {preprocess_metrics['articles_processed']}")
                print(f"  空記事除外数: {preprocess_metrics['articles_dropped_empty']}")
                print(f"  HTMLクリーニング済み: {preprocess_metrics['articles_html_cleaned']}")
                if preprocess_metrics['total_articles_fetched'] > 0:
                    process_ratio = preprocess_metrics['articles_processed'] / preprocess_metrics['total_articles_fetched']
                    print(f"  処理率: {process_ratio:.2%}")

            print(f"\n記事情報:")
            print(f"  総記事数: {job_articles['total_articles']}")
            print(f"  ユニークハッシュ数: {job_articles['unique_hashes']}")
            print(f"  言語数: {job_articles['unique_languages']}")

            print(f"\nサブワーカー実行結果:")
            print(f"  総実行数: {result['summary']['total_runs']}")
            print(f"  成功: {result['summary']['succeeded_runs']}")
            print(f"  失敗: {result['summary']['failed_runs']}")
            print(f"  部分成功: {result['summary']['partial_runs']}")
            print(f"  総クラスタ数: {result['summary']['total_clusters']}")
            print(f"  総文数: {result['summary']['total_sentences']}")
            print(f"  平均クラスタサイズ: {result['summary']['avg_cluster_size']:.1f}")

            if genre_evaluation:
                print(f"\nジャンル評価結果:")
                print(f"  Weighted F1: {genre_evaluation['weighted_f1']:.4f}" if genre_evaluation['weighted_f1'] else "  Weighted F1: N/A")
                print(f"  Micro F1: {genre_evaluation['micro_f1']:.4f}" if genre_evaluation['micro_f1'] else "  Micro F1: N/A")
                print(f"  Macro F1: {genre_evaluation['macro_f1']:.4f}")
                if genre_evaluation['genre_metrics']:
                    print(f"\n  ジャンル別F1:")
                    for gm in genre_evaluation['genre_metrics']:
                        print(f"    {gm['genre']}: {gm['f1_score']:.4f} (P:{gm['precision']:.4f}, R:{gm['recall']:.4f})")

            print("="*80)

    finally:
        await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())

