"""SQLAlchemy table definitions for recap-subworker.

Re-exports table definitions from db.dao for backward compatibility.
The canonical table definitions remain in db/dao.py to avoid breaking
existing migrations and references.
"""

from ...db.dao import (
    admin_jobs_table,
    cluster_evidence_table,
    clusters_table,
    diagnostics_table,
    genre_evaluation_metrics_table,
    genre_evaluation_runs_table,
    metadata,
    run_diagnostics_table,
    runs_table,
    sentences_table,
    system_metrics_table,
)

__all__ = [
    "metadata",
    "runs_table",
    "clusters_table",
    "sentences_table",
    "diagnostics_table",
    "run_diagnostics_table",
    "cluster_evidence_table",
    "admin_jobs_table",
    "genre_evaluation_runs_table",
    "genre_evaluation_metrics_table",
    "system_metrics_table",
]
