import json
import os
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import psycopg2
from psycopg2.extras import RealDictCursor
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity


def load_env_vars() -> Dict[str, str]:
    env_file = Path('.env')
    env_vars: Dict[str, str] = {}

    if env_file.exists():
        for raw in env_file.read_text().splitlines():
            line = raw.strip()
            if not line or line.startswith('#') or '=' not in line:
                continue
            key, value = line.split('=', 1)
            env_vars[key.strip()] = value.strip().strip('"').strip("'")

    for key in [
        'RECAP_DB_HOST',
        'RECAP_DB_PORT',
        'RECAP_DB_USER',
        'RECAP_DB_PASSWORD',
        'RECAP_DB_NAME',
    ]:
        if key in os.environ:
            env_vars[key] = os.environ[key]

    return env_vars


def connect_db() -> psycopg2.extensions.connection:
    cfg = load_env_vars()
    # デフォルトはlocalhost（Docker外で実行する場合）
    host = cfg.get('RECAP_DB_HOST', 'localhost')
    port = cfg.get('RECAP_DB_PORT', '5435')  # デフォルトを5435に変更
    user = cfg.get('RECAP_DB_USER', 'recap_user')
    password = cfg.get('RECAP_DB_PASSWORD', '')
    dbname = cfg.get('RECAP_DB_NAME', 'recap')

    # secretsからパスワードを読み込む
    if not password:
        script_dir = Path(__file__).parent
        repo_root = script_dir.parent
        secret_path = repo_root / 'secrets' / 'recap_db_password.txt'
        if secret_path.exists():
            password = secret_path.read_text().strip()

    # Dockerコンテナ内で実行される場合はrecap-dbを使用
    # それ以外の場合はlocalhost:5435を使用
    if host == 'recap-db':
        # Dockerコンテナ内で実行される場合
        port = cfg.get('RECAP_DB_PORT', '5432')
    elif host == 'localhost' or host == '127.0.0.1':
        # ローカルで実行される場合
        port = '5435'
    else:
        # 環境変数で指定された場合
        port = cfg.get('RECAP_DB_PORT', port)

    return psycopg2.connect(
        host=host,
        port=int(port),
        user=user,
        password=password,
        dbname=dbname,
        connect_timeout=10,
    )


def normalize_bullets(raw: Any) -> List[str]:
    if raw is None:
        return []
    if isinstance(raw, str):
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            return [raw]
        return normalize_bullets(parsed)
    if isinstance(raw, list):
        bullets: List[str] = []
        for entry in raw:
            if isinstance(entry, dict):
                text = entry.get('text') or entry.get('content')
                if isinstance(text, str):
                    bullets.append(text.strip())
                elif isinstance(entry.get('detail'), str):
                    bullets.append(entry['detail'].strip())
            elif isinstance(entry, str):
                bullets.append(entry.strip())
        return [b for b in bullets if b]
    if isinstance(raw, dict):
        return normalize_bullets(list(raw.values()))
    return []


def fetch_job_outputs(conn, job_id: str) -> List[Dict[str, Any]]:
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT genre, bullets_ja
            FROM recap_outputs
            WHERE job_id = %(job)s
            ORDER BY genre
            """,
            {'job': job_id},
        )
        return cur.fetchall()


def fetch_run_id(conn, job_id: str, genre: str) -> Optional[int]:
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id
            FROM recap_subworker_runs
            WHERE job_id = %(job)s AND genre = %(genre)s AND status IN ('succeeded', 'partial')
            ORDER BY id DESC
            LIMIT 1
            """,
            {'job': job_id, 'genre': genre},
        )
        row = cur.fetchone()
        return row[0] if row else None


def fetch_centroid_sentences(conn, run_id: int, limit: int = 200) -> List[str]:
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id
            FROM recap_subworker_clusters
            WHERE run_id = %(run_id)s
            ORDER BY size DESC
            LIMIT %(limit)s
            """,
            {'run_id': run_id, 'limit': limit},
        )
        clusters = [row[0] for row in cur.fetchall()]

    sentences: List[str] = []
    with conn.cursor() as cur:
        for cluster_id in clusters:
            cur.execute(
                """
                SELECT sentence_text
                FROM recap_subworker_sentences
                WHERE cluster_row_id = %(cluster_id)s
                ORDER BY score DESC
                LIMIT 1
                """,
                {'cluster_id': cluster_id},
            )
            sent = cur.fetchone()
            if sent and sent[0]:
                sentences.append(sent[0].strip())
    return list(dict.fromkeys(sentences))


def compute_coverage(bullets: List[str], centroids: List[str]) -> float:
    if not bullets or not centroids:
        return 0.0

    texts = bullets + centroids
    vectorizer = TfidfVectorizer(analyzer='char_wb', ngram_range=(2, 4), max_features=2048)
    tfidf = vectorizer.fit_transform(texts)
    bullet_matrix = tfidf[: len(bullets)]
    centroid_matrix = tfidf[len(bullets) :]

    similarity = cosine_similarity(bullet_matrix, centroid_matrix)
    if similarity.size == 0:
        return 0.0
    max_similarity = np.max(similarity, axis=1)
    return float(np.mean(max_similarity))


def compute_job_metrics(conn, job_id: str) -> Dict[str, Any]:
    outputs = fetch_job_outputs(conn, job_id)
    per_genre: List[Dict[str, Any]] = []
    total_bullets = 0
    total_centroids = 0

    for row in outputs:
        genre = row['genre']
        bullets = normalize_bullets(row['bullets_ja'])
        total_bullets += len(bullets)
        run_id = fetch_run_id(conn, job_id, genre)
        if run_id is None:
            continue
        centroids = fetch_centroid_sentences(conn, run_id)
        total_centroids += len(centroids)
        coverage = compute_coverage(bullets, centroids)
        per_genre.append(
            {
                'genre': genre,
                'coverage': coverage,
                'bullets': len(bullets),
                'centroids': len(centroids),
            }
        )

    if not per_genre:
        avg_coverage = 0.0
        std_coverage = 0.0
        min_coverage = 0.0
        max_coverage = 0.0
    else:
        coverages = [g['coverage'] for g in per_genre]
        avg_coverage = float(np.mean(coverages))
        std_coverage = float(np.std(coverages)) if len(coverages) > 1 else 0.0
        min_coverage = float(np.min(coverages))
        max_coverage = float(np.max(coverages))

    return {
        'job_id': job_id,
        'genre_results': per_genre,
        'avg_coverage': avg_coverage,
        'std_coverage': std_coverage,
        'min_coverage': min_coverage,
        'max_coverage': max_coverage,
        'total_genres': len(per_genre),
        'total_bullets': total_bullets,
        'total_centroids': total_centroids,
    }


def get_recent_jobs(conn, limit: int = 2) -> List[Tuple[str, str]]:
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT job_id, kicked_at
            FROM recap_jobs
            ORDER BY kicked_at DESC
            LIMIT %(limit)s
            """,
            {'limit': limit},
        )
        return [(row['job_id'], row['kicked_at'].isoformat()) for row in cur.fetchall()]


def check_job_exists(conn, job_id: str) -> Optional[Dict[str, Any]]:
    """指定されたJob IDが存在するか確認"""
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT job_id, kicked_at, note
            FROM recap_jobs
            WHERE job_id = %(job_id)s
            """,
            {'job_id': job_id},
        )
        row = cur.fetchone()
        return dict(row) if row else None


def get_job_status_summary(conn, job_id: str) -> Dict[str, Any]:
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
            {'job_id': job_id},
        )
        status_counts = {row['status']: {'count': row['count'], 'genre_count': row['genre_count']}
                        for row in cur.fetchall()}

        cur.execute(
            """
            SELECT COUNT(DISTINCT genre) as total_genres
            FROM recap_subworker_runs
            WHERE job_id = %(job_id)s
            """,
            {'job_id': job_id},
        )
        total_genres = cur.fetchone()['total_genres'] if cur.rowcount > 0 else 0

        cur.execute(
            """
            SELECT COUNT(DISTINCT genre) as output_count
            FROM recap_outputs
            WHERE job_id = %(job_id)s
            """,
            {'job_id': job_id},
        )
        output_count = cur.fetchone()['output_count'] if cur.rowcount > 0 else 0

        return {
            'status_counts': status_counts,
            'total_genres': total_genres,
            'output_count': output_count,
        }


def get_preprocess_metrics(conn, job_id: str) -> Optional[Dict[str, Any]]:
    """前処理メトリクスを取得"""
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(
            """
            SELECT *
            FROM recap_preprocess_metrics
            WHERE job_id = %(job_id)s
            """,
            {'job_id': job_id},
        )
        row = cur.fetchone()
        return dict(row) if row else None


def main() -> None:
    # 検証モードかどうかを確認
    verify_mode = '--verify' in sys.argv or '-v' in sys.argv
    # 評価するJob数（デフォルト: 最新5件）
    num_jobs = 5
    for arg in sys.argv:
        if arg.startswith('--jobs='):
            try:
                num_jobs = int(arg.split('=')[1])
            except ValueError:
                pass

    conn = connect_db()
    try:
        if verify_mode:
            # 検証モード: 最新のJobを評価
            print("=" * 80)
            print("Recap評価レポート - 最新Jobの評価")
            print(f"実行日時: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
            print("=" * 80)

            # 最新の完了済みJobを取得
            print(f"\n[1] 最新{num_jobs}件のJobを取得")
            print("-" * 80)
            jobs = get_recent_jobs(conn, limit=num_jobs)
            print(f"取得Job数: {len(jobs)}")
            for job_id, kicked_at in jobs:
                print(f"  Job {job_id[:8]}... kicked_at={kicked_at}")

            # 各Jobの詳細評価
            print("\n[2] 各Jobの詳細評価")
            print("=" * 80)
            results = {}
            jobs_to_evaluate = []

            # 最新Jobを追加（最新はlatest、それ以外はprevious-N）
            for i, (job_id, kicked_at) in enumerate(jobs):
                label = 'latest' if i == 0 else f'previous-{i}'
                jobs_to_evaluate.append((label, job_id))

            for label, job_id in jobs_to_evaluate:
                print(f"\n--- {label.upper()}: {job_id[:8]}... ---")
                status_summary = get_job_status_summary(conn, job_id)
                print(f"ステータス分布: {status_summary['status_counts']}")
                print(f"総ジャンル数: {status_summary['total_genres']}")
                print(f"出力ジャンル数: {status_summary['output_count']}")

                preprocess = get_preprocess_metrics(conn, job_id)
                if preprocess:
                    print(f"前処理: 取得={preprocess['total_articles_fetched']}, "
                          f"処理={preprocess['articles_processed']}")

                metrics = compute_job_metrics(conn, job_id)
                # jobs_to_evaluateのlabelと一緒にkicked_atも取得できるよう修正
                job_kicked_at = next((ka for jid, ka in jobs if jid == job_id), 'N/A')
                metrics['kicked_at'] = job_kicked_at
                results[job_id] = {
                    'label': label,
                    'status_summary': status_summary,
                    'preprocess': preprocess,
                    'metrics': metrics
                }

                print(f"\nカバレッジメトリクス:")
                print(f"  平均: {metrics['avg_coverage']:.4f}")
                print(f"  標準偏差: {metrics['std_coverage']:.4f}")
                print(f"  最小値: {metrics['min_coverage']:.4f}")
                print(f"  最大値: {metrics['max_coverage']:.4f}")
                print(f"  ジャンル数: {metrics['total_genres']}")

                print(f"\nジャンル別カバレッジ:")
                sorted_genres = sorted(
                    metrics['genre_results'],
                    key=lambda x: x['coverage'],
                    reverse=True
                )
                for g in sorted_genres:
                    print(f"  {g['genre']}: {g['coverage']:.4f} "
                          f"(bullets={g['bullets']}, centroids={g['centroids']})")

            # 結果をJSONで保存（タイムスタンプ付き）
            output_dir = Path(__file__).parent / 'reports'
            output_dir.mkdir(exist_ok=True)
            timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
            output_file = output_dir / f'recap_verification_results_{timestamp}.json'
            with open(output_file, 'w', encoding='utf-8') as f:
                json.dump(results, f, indent=2, ensure_ascii=False, default=str)
            print(f"\n結果を保存しました: {output_file}")

            # 最新結果へのシンボリックリンクも更新
            latest_link = output_dir / 'recap_verification_results.json'
            if latest_link.exists() or latest_link.is_symlink():
                latest_link.unlink()
            # シンボリックリンクではなく実ファイルとしてコピー（互換性のため）
            import shutil
            shutil.copy(output_file, latest_link)
            print(f"最新結果を更新: {latest_link}")

        else:
            # 通常モード: 最新のJobを表示
            jobs = get_recent_jobs(conn)
            results = []
            for job_id, kicked_at in jobs:
                metrics = compute_job_metrics(conn, job_id)
                metrics['kicked_at'] = kicked_at
                results.append(metrics)

            for res in results:
                print(f"Job {res['job_id']} kicked_at {res['kicked_at']}")
                print(f"  Genres covered: {res['total_genres']}")
                print(f"  Average coverage (TF-IDF): {res['avg_coverage']:.4f}")
                print(f"  Std deviation: {res['std_coverage']:.4f}")
                print(f"  Min: {res['min_coverage']:.4f}, Max: {res['max_coverage']:.4f}")
                print(f"  Bullets: {res['total_bullets']}, Centroids: {res['total_centroids']}")
                for genre_info in sorted(res['genre_results'], key=lambda g: g['coverage'], reverse=True):
                    coverage = genre_info['coverage']
                    print(
                        f"    {genre_info['genre']} -> coverage={coverage:.4f} bullets={genre_info['bullets']} centroids={genre_info['centroids']}"
                    )
                print()
    finally:
        conn.close()


if __name__ == '__main__':
    main()
