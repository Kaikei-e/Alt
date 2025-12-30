"""Clustering quality evaluator."""

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

from recap_evaluator.config import alert_thresholds
from recap_evaluator.domain.models import AlertLevel, ClusterMetrics
from recap_evaluator.infra.database import db

logger = structlog.get_logger()


class ClusterEvaluator:
    """Evaluates clustering quality for recap jobs."""

    async def evaluate_job(self, job_id: UUID) -> dict[str, ClusterMetrics]:
        """Evaluate clustering quality for a single job, per genre."""
        results: dict[str, ClusterMetrics] = {}

        # Fetch subworker runs for this job
        runs = await db.fetch_subworker_runs(job_id)

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

            # Fetch clusters for this run
            clusters = await db.fetch_clusters_for_run(run_id)

            if not clusters:
                logger.warning(
                    "No clusters found for run",
                    job_id=str(job_id),
                    genre=genre,
                    run_id=str(run_id),
                )
                continue

            # Calculate cluster statistics
            cluster_sizes = [c["size"] for c in clusters]
            metrics = ClusterMetrics(
                num_clusters=len(clusters),
                avg_cluster_size=np.mean(cluster_sizes) if cluster_sizes else 0.0,
                min_cluster_size=min(cluster_sizes) if cluster_sizes else 0,
                max_cluster_size=max(cluster_sizes) if cluster_sizes else 0,
            )

            # Note: Internal metrics (silhouette, etc.) require embeddings
            # which are not stored in the database. We would need to either:
            # 1. Store embeddings in the database
            # 2. Re-compute embeddings from article text
            # For now, we report cluster statistics only

            # Set alert level based on cluster count
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
        """Evaluate clustering with embeddings (for when embeddings are available).

        Args:
            embeddings: Feature matrix of shape (n_samples, n_features)
            cluster_labels: Predicted cluster labels
            ground_truth_labels: Optional ground truth labels for external metrics
        """
        n_samples = len(cluster_labels)
        n_clusters = len(set(cluster_labels)) - (1 if -1 in cluster_labels else 0)

        if n_samples < 2 or n_clusters < 2:
            logger.warning(
                "Insufficient data for clustering evaluation",
                n_samples=n_samples,
                n_clusters=n_clusters,
            )
            return ClusterMetrics(num_clusters=n_clusters)

        # Calculate internal metrics
        try:
            silhouette = silhouette_score(embeddings, cluster_labels)
        except Exception:
            silhouette = 0.0

        try:
            davies_bouldin = davies_bouldin_score(embeddings, cluster_labels)
        except Exception:
            davies_bouldin = 0.0

        try:
            calinski_harabasz = calinski_harabasz_score(embeddings, cluster_labels)
        except Exception:
            calinski_harabasz = 0.0

        # Calculate cluster statistics
        unique_labels = np.unique(cluster_labels)
        cluster_sizes = [np.sum(cluster_labels == label) for label in unique_labels]

        metrics = ClusterMetrics(
            silhouette_score=float(silhouette),
            davies_bouldin_index=float(davies_bouldin),
            calinski_harabasz_index=float(calinski_harabasz),
            num_clusters=n_clusters,
            avg_cluster_size=float(np.mean(cluster_sizes)),
            min_cluster_size=int(np.min(cluster_sizes)),
            max_cluster_size=int(np.max(cluster_sizes)),
        )

        # Calculate external metrics if ground truth is provided
        if ground_truth_labels is not None:
            try:
                metrics.nmi = float(
                    normalized_mutual_info_score(ground_truth_labels, cluster_labels)
                )
                metrics.ari = float(adjusted_rand_score(ground_truth_labels, cluster_labels))
                h, c, v = homogeneity_completeness_v_measure(ground_truth_labels, cluster_labels)
                metrics.homogeneity = float(h)
                metrics.completeness = float(c)
                metrics.v_measure = float(v)
            except Exception as e:
                logger.warning("Failed to calculate external metrics", error=str(e))

        # Set alert level based on silhouette score
        threshold = alert_thresholds.get_threshold("clustering_silhouette")
        if threshold:
            if silhouette < threshold.critical:
                metrics.alert_level = AlertLevel.CRITICAL
            elif silhouette < threshold.warn:
                metrics.alert_level = AlertLevel.WARN
            else:
                metrics.alert_level = AlertLevel.OK

        return metrics

    async def evaluate_batch(
        self,
        job_ids: list[UUID],
    ) -> dict[str, ClusterMetrics]:
        """Evaluate clustering across multiple jobs, aggregated by genre."""
        all_results: dict[str, list[ClusterMetrics]] = {}

        for job_id in job_ids:
            job_results = await self.evaluate_job(job_id)
            for genre, metrics in job_results.items():
                if genre not in all_results:
                    all_results[genre] = []
                all_results[genre].append(metrics)

        # Aggregate results per genre
        aggregated: dict[str, ClusterMetrics] = {}
        for genre, metrics_list in all_results.items():
            if not metrics_list:
                continue

            # Average the metrics
            aggregated[genre] = ClusterMetrics(
                num_clusters=int(np.mean([m.num_clusters for m in metrics_list])),
                avg_cluster_size=float(np.mean([m.avg_cluster_size for m in metrics_list])),
                min_cluster_size=int(np.min([m.min_cluster_size for m in metrics_list])),
                max_cluster_size=int(np.max([m.max_cluster_size for m in metrics_list])),
                silhouette_score=float(np.mean([m.silhouette_score for m in metrics_list])),
            )

        logger.info(
            "Batch cluster evaluation completed",
            job_count=len(job_ids),
            genre_count=len(aggregated),
        )

        return aggregated


# Singleton instance
cluster_evaluator = ClusterEvaluator()
