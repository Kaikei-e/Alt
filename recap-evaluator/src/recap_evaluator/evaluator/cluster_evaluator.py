"""Clustering quality evaluator with DI."""

from uuid import UUID

import numpy as np
import structlog
from sklearn.metrics import (
    adjusted_rand_score,
    calinski_harabasz_score,
    davies_bouldin_score,
    homogeneity_completeness_v_measure,
    normalized_mutual_info_score,
    silhouette_score,
)

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import AlertLevel, ClusterMetrics
from recap_evaluator.port.database_port import DatabasePort

logger = structlog.get_logger()


class ClusterEvaluator:
    """Evaluates clustering quality for recap jobs."""

    def __init__(self, db: DatabasePort, thresholds: AlertThresholds) -> None:
        self._db = db
        self._thresholds = thresholds

    async def evaluate_job(self, job_id: UUID) -> dict[str, ClusterMetrics]:
        results: dict[str, ClusterMetrics] = {}
        runs = await self._db.fetch_subworker_runs(job_id)

        for run in runs:
            genre = run["genre"]
            run_id = run["run_id"]

            if run["status"] != "succeeded":
                logger.warning(
                    "Skipping failed subworker run",
                    job_id=str(job_id),
                    genre=genre,
                    status=run["status"],
                )
                continue

            clusters = await self._db.fetch_clusters_for_run(run_id)
            if not clusters:
                continue

            cluster_sizes = [c["size"] for c in clusters]
            metrics = ClusterMetrics(
                num_clusters=len(clusters),
                avg_cluster_size=float(np.mean(cluster_sizes)) if cluster_sizes else 0.0,
                min_cluster_size=min(cluster_sizes) if cluster_sizes else 0,
                max_cluster_size=max(cluster_sizes) if cluster_sizes else 0,
            )

            if len(clusters) < 3:
                metrics.alert_level = AlertLevel.WARN
            else:
                metrics.alert_level = AlertLevel.OK

            results[genre] = metrics

        return results

    def evaluate_with_embeddings(
        self,
        embeddings: np.ndarray,
        cluster_labels: np.ndarray,
        ground_truth_labels: np.ndarray | None = None,
    ) -> ClusterMetrics:
        n_samples = len(cluster_labels)
        n_clusters = len(set(cluster_labels)) - (1 if -1 in cluster_labels else 0)

        if n_samples < 2 or n_clusters < 2:
            return ClusterMetrics(num_clusters=n_clusters)

        try:
            sil = silhouette_score(embeddings, cluster_labels)
        except Exception:
            sil = 0.0

        try:
            db_score = davies_bouldin_score(embeddings, cluster_labels)
        except Exception:
            db_score = 0.0

        try:
            ch_score = calinski_harabasz_score(embeddings, cluster_labels)
        except Exception:
            ch_score = 0.0

        unique_labels = np.unique(cluster_labels)
        cluster_sizes = [int(np.sum(cluster_labels == label)) for label in unique_labels]

        metrics = ClusterMetrics(
            silhouette_score=float(sil),
            davies_bouldin_index=float(db_score),
            calinski_harabasz_index=float(ch_score),
            num_clusters=n_clusters,
            avg_cluster_size=float(np.mean(cluster_sizes)),
            min_cluster_size=int(np.min(cluster_sizes)),
            max_cluster_size=int(np.max(cluster_sizes)),
        )

        if ground_truth_labels is not None:
            try:
                metrics.nmi = float(
                    normalized_mutual_info_score(ground_truth_labels, cluster_labels)
                )
                metrics.ari = float(
                    adjusted_rand_score(ground_truth_labels, cluster_labels)
                )
                h, c, v = homogeneity_completeness_v_measure(
                    ground_truth_labels, cluster_labels
                )
                metrics.homogeneity = float(h)
                metrics.completeness = float(c)
                metrics.v_measure = float(v)
            except Exception as e:
                logger.warning("Failed to calculate external metrics", error=str(e))

        warn = self._thresholds.get_warn("clustering_silhouette")
        critical = self._thresholds.get_critical("clustering_silhouette")
        if critical is not None and sil < critical:
            metrics.alert_level = AlertLevel.CRITICAL
        elif warn is not None and sil < warn:
            metrics.alert_level = AlertLevel.WARN
        else:
            metrics.alert_level = AlertLevel.OK

        return metrics

    async def evaluate_batch(
        self, job_ids: list[UUID]
    ) -> dict[str, ClusterMetrics]:
        all_results: dict[str, list[ClusterMetrics]] = {}

        for job_id in job_ids:
            job_results = await self.evaluate_job(job_id)
            for genre, metrics in job_results.items():
                if genre not in all_results:
                    all_results[genre] = []
                all_results[genre].append(metrics)

        aggregated: dict[str, ClusterMetrics] = {}
        for genre, metrics_list in all_results.items():
            if not metrics_list:
                continue

            aggregated[genre] = ClusterMetrics(
                num_clusters=int(np.mean([m.num_clusters for m in metrics_list])),
                avg_cluster_size=float(
                    np.mean([m.avg_cluster_size for m in metrics_list])
                ),
                min_cluster_size=int(np.min([m.min_cluster_size for m in metrics_list])),
                max_cluster_size=int(np.max([m.max_cluster_size for m in metrics_list])),
                silhouette_score=float(
                    np.mean([m.silhouette_score for m in metrics_list])
                ),
            )

        logger.info(
            "Batch cluster evaluation completed",
            job_count=len(job_ids),
            genre_count=len(aggregated),
        )

        return aggregated
