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
from pathlib import Path
from typing import Any
from urllib.parse import urlparse, urlunparse

from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncConnection, create_async_engine

DEFAULT_POSTGRES_PORT = 5432
SENTENCE_PREVIEW_MAX_LEN = 100

# Database connection will be constructed manually

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


def _read_recap_secret_password() -> str | None:
    """secrets/recap_db_password.txt が存在すればパスワードを読み込む"""
    script_dir = Path(__file__).parent
    repo_root = script_dir.parent
    secret_path = repo_root / "secrets" / "recap_db_password.txt"
    if not secret_path.exists():
        return None
    try:
        with secret_path.open() as f:
            password = f.read().strip()
        logger.info("Loaded password from %s", secret_path)
    except (OSError, UnicodeDecodeError) as e:
        logger.warning("Failed to read password from %s: %s", secret_path, e)
        return None
    else:
        return password


def _build_db_url_from_env() -> str:
    """個別環境変数とsecretsファイルからデータベース接続URLを構築する"""
    db_host = os.getenv("RECAP_DB_HOST", "localhost")
    db_port = os.getenv("RECAP_DB_PORT", "5435")
    db_user = os.getenv("RECAP_DB_USER", "recap_user")
    db_name = os.getenv("RECAP_DB_NAME", "recap")
    db_password = os.getenv("RECAP_DB_PASSWORD")

    if not db_password:
        db_password = _read_recap_secret_password()

    if not db_password:
        msg = (
            "No database password found. Set RECAP_DB_URL / RECAP_SUBWORKER_DB_URL, "
            "or RECAP_DB_PASSWORD, or provide secrets/recap_db_password.txt."
        )
        raise SystemExit(msg)

    return f"postgresql+asyncpg://{db_user}:{db_password}@{db_host}:{db_port}/{db_name}"


def _rewrite_local_recap_host(db_url: str) -> str:
    """コンテナ名 recap-db を localhost とローカルポートへ書き換える"""
    if "recap-db" not in db_url:
        return db_url
    u = urlparse(db_url)
    host = (u.hostname or "").replace("recap-db", "localhost")
    port = (
        u.port
        if u.port and u.port != DEFAULT_POSTGRES_PORT
        else int(os.getenv("RECAP_DB_PORT", "5435"))
    )
    userinfo = u.username or ""
    if u.password:
        userinfo = f"{userinfo}:{u.password}"
    netloc = f"{userinfo}@{host}:{port}" if userinfo else f"{host}:{port}"
    return urlunparse((u.scheme, netloc, u.path, u.params, u.query, u.fragment))


def _inject_secret_password(db_url: str) -> str:
    """URLにパスワードが無い場合のみ secrets/recap_db_password.txt から補完する"""
    if "@" in db_url and ":" in db_url.split("@", 1)[0]:
        return db_url
    password = _read_recap_secret_password()
    if not password:
        return db_url
    u = urlparse(db_url)
    if "@" not in u.netloc:
        return db_url
    user_pass, host_port = u.netloc.rsplit("@", 1)
    if ":" in user_pass:
        user, _ = user_pass.split(":", 1)
        new_netloc = f"{user}:{password}@{host_port}"
    else:
        new_netloc = f"{user_pass}:{password}@{host_port}"
    return urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))


def get_db_url() -> str:
    """データベース接続URLを取得"""
    db_url = os.getenv("RECAP_DB_URL") or os.getenv("RECAP_SUBWORKER_DB_URL")
    if not db_url:
        return _build_db_url_from_env()
    db_url = _rewrite_local_recap_host(db_url)
    return _inject_secret_password(db_url)


async def fetch_latest_job(conn: AsyncConnection) -> dict[str, Any] | None:
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
        "note": row.note,
    }


async def fetch_preprocess_metrics(conn: AsyncConnection, job_id: str) -> dict[str, Any] | None:
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
        "avg_chars_per_article": float(row.avg_chars_per_article)
        if row.avg_chars_per_article
        else None,
        "languages_detected": dict(row.languages_detected) if row.languages_detected else {},
    }


async def fetch_subworker_runs(conn: AsyncConnection, job_id: str) -> list[dict[str, Any]]:
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
    return [
        {
            "run_id": row.id,
            "genre": row.genre,
            "status": row.status,
            "cluster_count": row.cluster_count,
            "started_at": row.started_at.isoformat() if row.started_at else None,
            "finished_at": row.finished_at.isoformat() if row.finished_at else None,
            "request_payload": dict(row.request_payload) if row.request_payload else {},
            "response_payload": dict(row.response_payload) if row.response_payload else {},
            "error_message": row.error_message,
        }
        for row in result
    ]


async def fetch_clusters(conn: AsyncConnection, run_ids: list[int]) -> list[dict[str, Any]]:
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
    return [
        {
            "cluster_row_id": row.id,
            "run_id": row.run_id,
            "cluster_id": row.cluster_id,
            "size": row.size,
            "label": row.label,
            "top_terms": list(row.top_terms) if row.top_terms else [],
            "stats": dict(row.stats) if row.stats else {},
        }
        for row in result
    ]


async def fetch_sentences(
    conn: AsyncConnection, cluster_row_ids: list[int]
) -> list[dict[str, Any]]:
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
    return [
        {
            "cluster_row_id": row.cluster_row_id,
            "source_article_id": row.source_article_id,
            "sentence_text": row.sentence_text[:SENTENCE_PREVIEW_MAX_LEN] + "..."
            if len(row.sentence_text) > SENTENCE_PREVIEW_MAX_LEN
            else row.sentence_text,
            "lang": row.lang,
            "score": float(row.score),
        }
        for row in result
    ]


async def fetch_diagnostics(conn: AsyncConnection, run_ids: list[int]) -> list[dict[str, Any]]:
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
    return [
        {
            "run_id": row.run_id,
            "metric": row.metric,
            "value": dict(row.value) if isinstance(row.value, dict) else row.value,
        }
        for row in result
    ]


async def fetch_job_articles(conn: AsyncConnection, job_id: str) -> dict[str, Any]:
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
        return {"total_articles": 0, "unique_hashes": 0, "unique_languages": 0}

    return {
        "total_articles": row.total_articles,
        "unique_hashes": row.unique_hashes,
        "unique_languages": row.unique_languages,
    }


async def fetch_genre_evaluation(conn: AsyncConnection) -> dict[str, Any] | None:
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
    genre_metrics = [
        {
            "genre": m_row.genre,
            "tp": m_row.tp,
            "fp": m_row.fp,
            "fn": m_row.fn_count,
            "precision": float(m_row.precision),
            "recall": float(m_row.recall),
            "f1_score": float(m_row.f1_score),
        }
        for m_row in metrics_result
    ]

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
        "genre_metrics": genre_metrics,
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


async def _collect_job_data(conn: AsyncConnection, latest_job: dict[str, Any]) -> dict[str, Any]:
    """最新Jobに紐づく各テーブルの調査データを収集して辞書にまとめる"""
    job_id = latest_job["job_id"]

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

    return {
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
            "avg_cluster_size": sum(c["size"] for c in clusters) / len(clusters) if clusters else 0,
        },
    }


def _print_genre_eval_summary(genre_evaluation: dict[str, Any]) -> None:
    """ジャンル評価結果のサマリを標準出力へ表示"""
    sys.stdout.write("\nジャンル評価結果:\n")
    sys.stdout.write(
        f"  Weighted F1: {genre_evaluation['weighted_f1']:.4f}\n"
        if genre_evaluation["weighted_f1"]
        else "  Weighted F1: N/A\n"
    )
    sys.stdout.write(
        f"  Micro F1: {genre_evaluation['micro_f1']:.4f}\n"
        if genre_evaluation["micro_f1"]
        else "  Micro F1: N/A\n"
    )
    sys.stdout.write(f"  Macro F1: {genre_evaluation['macro_f1']:.4f}\n")
    if genre_evaluation["genre_metrics"]:
        sys.stdout.write("\n  ジャンル別F1:\n")
        for gm in genre_evaluation["genre_metrics"]:
            sys.stdout.write(
                f"    {gm['genre']}: {gm['f1_score']:.4f} (P:{gm['precision']:.4f}, R:{gm['recall']:.4f})\n"
            )


def _print_investigation_summary(
    job_id: str, latest_job: dict[str, Any], result: dict[str, Any]
) -> None:
    """調査結果サマリを標準出力へ表示"""
    preprocess_metrics = result["preprocess_metrics"]
    job_articles = result["job_articles"]
    genre_evaluation = result["genre_evaluation"]

    sys.stdout.write("\n" + "=" * 80 + "\n")
    sys.stdout.write("調査結果サマリ\n")
    sys.stdout.write("=" * 80 + "\n")
    sys.stdout.write(f"Job ID: {job_id}\n")
    sys.stdout.write(f"実行日時: {latest_job['kicked_at']}\n")

    if preprocess_metrics:
        sys.stdout.write("\n前処理メトリクス:\n")
        sys.stdout.write(f"  取得記事数: {preprocess_metrics['total_articles_fetched']}\n")
        sys.stdout.write(f"  処理済み記事数: {preprocess_metrics['articles_processed']}\n")
        sys.stdout.write(f"  空記事除外数: {preprocess_metrics['articles_dropped_empty']}\n")
        sys.stdout.write(f"  HTMLクリーニング済み: {preprocess_metrics['articles_html_cleaned']}\n")
        if preprocess_metrics["total_articles_fetched"] > 0:
            process_ratio = (
                preprocess_metrics["articles_processed"]
                / preprocess_metrics["total_articles_fetched"]
            )
            sys.stdout.write(f"  処理率: {process_ratio:.2%}\n")

    sys.stdout.write("\n記事情報:\n")
    sys.stdout.write(f"  総記事数: {job_articles['total_articles']}\n")
    sys.stdout.write(f"  ユニークハッシュ数: {job_articles['unique_hashes']}\n")
    sys.stdout.write(f"  言語数: {job_articles['unique_languages']}\n")

    sys.stdout.write("\nサブワーカー実行結果:\n")
    sys.stdout.write(f"  総実行数: {result['summary']['total_runs']}\n")
    sys.stdout.write(f"  成功: {result['summary']['succeeded_runs']}\n")
    sys.stdout.write(f"  失敗: {result['summary']['failed_runs']}\n")
    sys.stdout.write(f"  部分成功: {result['summary']['partial_runs']}\n")
    sys.stdout.write(f"  総クラスタ数: {result['summary']['total_clusters']}\n")
    sys.stdout.write(f"  総文数: {result['summary']['total_sentences']}\n")
    sys.stdout.write(f"  平均クラスタサイズ: {result['summary']['avg_cluster_size']:.1f}\n")

    if genre_evaluation:
        _print_genre_eval_summary(genre_evaluation)

    sys.stdout.write("=" * 80 + "\n")


async def main() -> None:
    """メイン処理"""
    db_url = get_db_url()
    logger.info("Connecting to DB: %s", _safe_dsn_for_log(db_url))

    engine = create_async_engine(db_url, echo=False)

    try:
        async with engine.connect() as conn:
            tables = await conn.run_sync(lambda sync_conn: inspect(sync_conn).get_table_names())
            logger.info("Available tables: %s", sorted(tables))

            logger.info("Fetching latest job...")
            latest_job = await fetch_latest_job(conn)

            if not latest_job:
                logger.error("No jobs found in database")
                return

            job_id = latest_job["job_id"]
            logger.info("Latest job ID: %s", job_id)
            logger.info("Kicked at: %s", latest_job["kicked_at"])

            result = await _collect_job_data(conn, latest_job)

            output_dir = Path(__file__).parent / "reports"
            output_dir.mkdir(exist_ok=True)
            output_file = output_dir / f"recap-investigation-{job_id[:8]}.json"

            with output_file.open("w", encoding="utf-8") as f:
                json.dump(result, f, indent=2, ensure_ascii=False, default=str)

            logger.info("Results saved to %s", output_file)
            _print_investigation_summary(job_id, latest_job, result)

    finally:
        await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
