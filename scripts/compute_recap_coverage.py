import argparse
import json
import os
import shutil
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

import numpy as np
import psycopg2
from psycopg2.extras import RealDictCursor
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity


def load_env_vars() -> dict[str, str]:
    env_file = Path(".env")
    env_vars: dict[str, str] = {}

    if env_file.exists():
        for raw in env_file.read_text().splitlines():
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            env_vars[key.strip()] = value.strip().strip('"').strip("'")

    for key in [
        "RECAP_DB_HOST",
        "RECAP_DB_PORT",
        "RECAP_DB_USER",
        "RECAP_DB_PASSWORD",
        "RECAP_DB_NAME",
    ]:
        if key in os.environ:
            env_vars[key] = os.environ[key]

    return env_vars


def connect_db() -> psycopg2.extensions.connection:
    cfg = load_env_vars()
    # デフォルトはlocalhost（Docker外で実行する場合）
    host = cfg.get("RECAP_DB_HOST", "localhost")
    port = cfg.get("RECAP_DB_PORT", "5435")  # デフォルトを5435に変更
    user = cfg.get("RECAP_DB_USER", "recap_user")
    password = cfg.get("RECAP_DB_PASSWORD", "")
    dbname = cfg.get("RECAP_DB_NAME", "recap")

    # secretsからパスワードを読み込む
    if not password:
        script_dir = Path(__file__).parent
        repo_root = script_dir.parent
        secret_path = repo_root / "secrets" / "recap_db_password.txt"
        if secret_path.exists():
            password = secret_path.read_text().strip()

    # Dockerコンテナ内で実行される場合はrecap-dbを使用
    # それ以外の場合はlocalhost:5435を使用
    if host == "recap-db":
        # Dockerコンテナ内で実行される場合
        port = cfg.get("RECAP_DB_PORT", "5432")
    elif host in {"localhost", "127.0.0.1"}:
        # ローカルで実行される場合（RECAP_DB_PORT 優先）
        port = cfg.get("RECAP_DB_PORT", "5435")
    else:
        # 環境変数で指定された場合
        port = cfg.get("RECAP_DB_PORT", port)

    return psycopg2.connect(
        host=host,
        port=int(port),
        user=user,
        password=password,
        dbname=dbname,
        connect_timeout=10,
    )


def normalize_bullets(raw: object) -> list[str]:
    if raw is None:
        return []
    if isinstance(raw, str):
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            return [raw]
        return normalize_bullets(parsed)
    if isinstance(raw, list):
        bullets: list[str] = []
        for entry in raw:
            if isinstance(entry, dict):
                text = entry.get("text") or entry.get("content")
                if isinstance(text, str):
                    bullets.append(text.strip())
                elif isinstance(entry.get("detail"), str):
                    bullets.append(entry["detail"].strip())
            elif isinstance(entry, str):
                bullets.append(entry.strip())
        return [b for b in bullets if b]
    if isinstance(raw, dict):
        return normalize_bullets(list(raw.values()))
    return []


def fetch_job_outputs(conn: psycopg2.extensions.connection, job_id: str) -> list[dict[str, Any]]:
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT genre, bullets_ja
            FROM recap_outputs
            WHERE job_id = %(job)s
            ORDER BY genre
            """,
            {"job": job_id},
        )
        return cur.fetchall()


def fetch_run_id(conn: psycopg2.extensions.connection, job_id: str, genre: str) -> int | None:
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id
            FROM recap_subworker_runs
            WHERE job_id = %(job)s AND genre = %(genre)s AND status IN ('succeeded', 'partial')
            ORDER BY id DESC
            LIMIT 1
            """,
            {"job": job_id, "genre": genre},
        )
        row = cur.fetchone()
        return row[0] if row else None


def fetch_centroid_sentences(
    conn: psycopg2.extensions.connection, run_id: int, limit: int = 200
) -> list[str]:
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT s.sentence_text
            FROM (
                SELECT id
                FROM recap_subworker_clusters
                WHERE run_id = %(run_id)s
                ORDER BY size DESC
                LIMIT %(limit)s
            ) c
            JOIN LATERAL (
                SELECT sentence_text
                FROM recap_subworker_sentences
                WHERE cluster_row_id = c.id
                ORDER BY score DESC
                LIMIT 1
            ) s ON TRUE
            """,
            {"run_id": run_id, "limit": limit},
        )
        sentences = [row[0].strip() for row in cur.fetchall() if row and row[0]]
    return list(dict.fromkeys(sentences))


def compute_coverage(bullets: list[str], centroids: list[str]) -> float:
    if not bullets or not centroids:
        return 0.0

    texts = bullets + centroids
    vectorizer = TfidfVectorizer(analyzer="char_wb", ngram_range=(2, 4), max_features=2048)
    tfidf = vectorizer.fit_transform(texts)
    bullet_matrix = tfidf[: len(bullets)]
    centroid_matrix = tfidf[len(bullets) :]

    similarity = cosine_similarity(bullet_matrix, centroid_matrix)
    if similarity.size == 0:
        return 0.0
    max_similarity = np.max(similarity, axis=1)
    return float(np.mean(max_similarity))


def compute_job_metrics(conn: psycopg2.extensions.connection, job_id: str) -> dict[str, Any]:
    outputs = fetch_job_outputs(conn, job_id)
    per_genre: list[dict[str, Any]] = []
    total_bullets = 0
    total_centroids = 0

    for row in outputs:
        genre = row["genre"]
        bullets = normalize_bullets(row["bullets_ja"])
        total_bullets += len(bullets)
        run_id = fetch_run_id(conn, job_id, genre)
        if run_id is None:
            continue
        centroids = fetch_centroid_sentences(conn, run_id)
        total_centroids += len(centroids)
        coverage = compute_coverage(bullets, centroids)
        per_genre.append(
            {
                "genre": genre,
                "coverage": coverage,
                "bullets": len(bullets),
                "centroids": len(centroids),
            }
        )

    if not per_genre:
        avg_coverage = 0.0
        std_coverage = 0.0
        min_coverage = 0.0
        max_coverage = 0.0
    else:
        coverages = [g["coverage"] for g in per_genre]
        avg_coverage = float(np.mean(coverages))
        std_coverage = float(np.std(coverages)) if len(coverages) > 1 else 0.0
        min_coverage = float(np.min(coverages))
        max_coverage = float(np.max(coverages))

    return {
        "job_id": job_id,
        "genre_results": per_genre,
        "avg_coverage": avg_coverage,
        "std_coverage": std_coverage,
        "min_coverage": min_coverage,
        "max_coverage": max_coverage,
        "total_genres": len(per_genre),
        "total_bullets": total_bullets,
        "total_centroids": total_centroids,
    }


def get_recent_jobs(conn: psycopg2.extensions.connection, limit: int = 2) -> list[tuple[str, str]]:
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT job_id, kicked_at
            FROM recap_jobs
            ORDER BY kicked_at DESC
            LIMIT %(limit)s
            """,
            {"limit": limit},
        )
        return [(row["job_id"], row["kicked_at"].isoformat()) for row in cur.fetchall()]


def check_job_exists(conn: psycopg2.extensions.connection, job_id: str) -> dict[str, Any] | None:
    """指定されたJob IDが存在するか確認"""
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT job_id, kicked_at, note
            FROM recap_jobs
            WHERE job_id = %(job_id)s
            """,
            {"job_id": job_id},
        )
        row = cur.fetchone()
        return dict(row) if row else None


def get_job_status_summary(conn: psycopg2.extensions.connection, job_id: str) -> dict[str, Any]:
    """Jobのステータスサマリを取得"""
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT
                status,
                COUNT(*) as count,
                COUNT(DISTINCT genre) as genre_count
            FROM recap_subworker_runs
            WHERE job_id = %(job_id)s
            GROUP BY status
            """,
            {"job_id": job_id},
        )
        status_counts = {
            row["status"]: {"count": row["count"], "genre_count": row["genre_count"]}
            for row in cur.fetchall()
        }

        cur.execute(
            """
            SELECT COUNT(DISTINCT genre) as total_genres
            FROM recap_subworker_runs
            WHERE job_id = %(job_id)s
            """,
            {"job_id": job_id},
        )
        total_genres = cur.fetchone()["total_genres"] if cur.rowcount > 0 else 0

        cur.execute(
            """
            SELECT COUNT(DISTINCT genre) as output_count
            FROM recap_outputs
            WHERE job_id = %(job_id)s
            """,
            {"job_id": job_id},
        )
        output_count = cur.fetchone()["output_count"] if cur.rowcount > 0 else 0

        return {
            "status_counts": status_counts,
            "total_genres": total_genres,
            "output_count": output_count,
        }


def get_preprocess_metrics(
    conn: psycopg2.extensions.connection, job_id: str
) -> dict[str, Any] | None:
    """前処理メトリクスを取得"""
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT *
            FROM recap_preprocess_metrics
            WHERE job_id = %(job_id)s
            """,
            {"job_id": job_id},
        )
        row = cur.fetchone()
        return dict(row) if row else None


def _evaluate_verify_jobs(
    conn: psycopg2.extensions.connection, jobs: list[tuple[str, Any]]
) -> dict[str, Any]:
    """検証モードで各Jobのステータス・前処理・カバレッジメトリクスを評価する"""
    sys.stdout.write("\n[2] 各Jobの詳細評価\n")
    sys.stdout.write("=" * 80 + "\n")
    results: dict[str, Any] = {}
    jobs_to_evaluate = [
        ("latest" if i == 0 else f"previous-{i}", job_id)
        for i, (job_id, _kicked_at) in enumerate(jobs)
    ]

    for label, job_id in jobs_to_evaluate:
        sys.stdout.write(f"\n--- {label.upper()}: {job_id[:8]}... ---\n")
        status_summary = get_job_status_summary(conn, job_id)
        sys.stdout.write(f"ステータス分布: {status_summary['status_counts']}\n")
        sys.stdout.write(f"総ジャンル数: {status_summary['total_genres']}\n")
        sys.stdout.write(f"出力ジャンル数: {status_summary['output_count']}\n")

        preprocess = get_preprocess_metrics(conn, job_id)
        if preprocess:
            sys.stdout.write(
                f"前処理: 取得={preprocess['total_articles_fetched']}, "
                f"処理={preprocess['articles_processed']}\n"
            )

        metrics = compute_job_metrics(conn, job_id)
        job_kicked_at = next((ka for jid, ka in jobs if jid == job_id), "N/A")
        metrics["kicked_at"] = job_kicked_at
        results[job_id] = {
            "label": label,
            "status_summary": status_summary,
            "preprocess": preprocess,
            "metrics": metrics,
        }

        sys.stdout.write("\nカバレッジメトリクス:\n")
        sys.stdout.write(f"  平均: {metrics['avg_coverage']:.4f}\n")
        sys.stdout.write(f"  標準偏差: {metrics['std_coverage']:.4f}\n")
        sys.stdout.write(f"  最小値: {metrics['min_coverage']:.4f}\n")
        sys.stdout.write(f"  最大値: {metrics['max_coverage']:.4f}\n")
        sys.stdout.write(f"  ジャンル数: {metrics['total_genres']}\n")

        sys.stdout.write("\nジャンル別カバレッジ:\n")
        sorted_genres = sorted(metrics["genre_results"], key=lambda x: x["coverage"], reverse=True)
        for g in sorted_genres:
            sys.stdout.write(
                f"  {g['genre']}: {g['coverage']:.4f} "
                f"(bullets={g['bullets']}, centroids={g['centroids']})\n"
            )

    return results


def _save_verification_results(results: dict[str, Any]) -> None:
    """検証結果をタイムスタンプ付きJSONと最新スナップショットへ保存する"""
    output_dir = Path(__file__).parent / "reports"
    output_dir.mkdir(exist_ok=True)
    timestamp = datetime.now().astimezone().strftime("%Y%m%d_%H%M%S")
    output_file = output_dir / f"recap_verification_results_{timestamp}.json"
    with output_file.open("w", encoding="utf-8") as f:
        json.dump(results, f, indent=2, ensure_ascii=False, default=str)
    sys.stdout.write(f"\n結果を保存しました: {output_file}\n")

    latest_link = output_dir / "recap_verification_results.json"
    if latest_link.exists() or latest_link.is_symlink():
        latest_link.unlink()
    shutil.copy(output_file, latest_link)
    sys.stdout.write(f"最新結果を更新: {latest_link}\n")


def _run_verify_mode(conn: psycopg2.extensions.connection, num_jobs: int) -> None:
    """検証モード: 最新N件のJobを評価してJSON保存する"""
    sys.stdout.write("=" * 80 + "\n")
    sys.stdout.write("Recap評価レポート - 最新Jobの評価\n")
    sys.stdout.write(f"実行日時: {datetime.now().astimezone().strftime('%Y-%m-%d %H:%M:%S')}\n")
    sys.stdout.write("=" * 80 + "\n")

    sys.stdout.write(f"\n[1] 最新{num_jobs}件のJobを取得\n")
    sys.stdout.write("-" * 80 + "\n")
    jobs = get_recent_jobs(conn, limit=num_jobs)
    sys.stdout.write(f"取得Job数: {len(jobs)}\n")
    for job_id, kicked_at in jobs:
        sys.stdout.write(f"  Job {job_id[:8]}... kicked_at={kicked_at}\n")

    results = _evaluate_verify_jobs(conn, jobs)
    _save_verification_results(results)


def _run_summary_mode(conn: psycopg2.extensions.connection) -> None:
    """通常モード: 最新のJobのカバレッジを表示する"""
    jobs = get_recent_jobs(conn)
    results = []
    for job_id, kicked_at in jobs:
        metrics = compute_job_metrics(conn, job_id)
        metrics["kicked_at"] = kicked_at
        results.append(metrics)

    for res in results:
        sys.stdout.write(f"Job {res['job_id']} kicked_at {res['kicked_at']}\n")
        sys.stdout.write(f"  Genres covered: {res['total_genres']}\n")
        sys.stdout.write(f"  Average coverage (TF-IDF): {res['avg_coverage']:.4f}\n")
        sys.stdout.write(f"  Std deviation: {res['std_coverage']:.4f}\n")
        sys.stdout.write(f"  Min: {res['min_coverage']:.4f}, Max: {res['max_coverage']:.4f}\n")
        sys.stdout.write(
            f"  Bullets: {res['total_bullets']}, Centroids: {res['total_centroids']}\n"
        )
        for genre_info in sorted(res["genre_results"], key=lambda g: g["coverage"], reverse=True):
            coverage = genre_info["coverage"]
            sys.stdout.write(
                f"    {genre_info['genre']} -> coverage={coverage:.4f} bullets={genre_info['bullets']} centroids={genre_info['centroids']}\n"
            )
        sys.stdout.write("\n")


def main() -> None:
    parser = argparse.ArgumentParser(description="Compute recap coverage metrics")
    parser.add_argument("--verify", "-v", action="store_true", help="Verification mode")
    parser.add_argument("--jobs", type=int, default=5, help="Number of recent jobs to evaluate")
    args = parser.parse_args()
    verify_mode = args.verify
    num_jobs = args.jobs

    conn = connect_db()
    try:
        if verify_mode:
            _run_verify_mode(conn, num_jobs)
        else:
            _run_summary_mode(conn)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
