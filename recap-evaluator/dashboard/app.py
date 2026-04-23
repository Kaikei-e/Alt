"""Recap quality dashboard (manual launch only).

Launch: ``cd recap-evaluator && uv run streamlit run dashboard/app.py``

Reads directly from recap-db via RECAP_DB_DSN env var. Plots the 5 Morning-Letter
quality axes alongside genre_count and fallback_count so we can compare
before/after of prompt / packing / classifier changes.
"""

from __future__ import annotations

import os

import pandas as pd
import psycopg
import streamlit as st

DEFAULT_DSN = "postgresql://recap_user:recap_password@localhost:5435/recap"
DSN = os.environ.get("RECAP_DB_DSN", DEFAULT_DSN)


def fetch_df(sql: str, params: tuple = ()) -> pd.DataFrame:
    with psycopg.connect(DSN) as conn, conn.cursor() as cur:
        cur.execute(sql, params)
        cols = [d.name for d in cur.description] if cur.description else []
        rows = cur.fetchall()
    return pd.DataFrame(rows, columns=cols)


def latest_3days_jobs(limit: int = 14) -> pd.DataFrame:
    return fetch_df(
        """
        SELECT job_id, window_days, status, last_stage, kicked_at, updated_at
        FROM recap_jobs
        WHERE window_days = 3
        ORDER BY kicked_at DESC
        LIMIT %s
        """,
        (limit,),
    )


def genre_counts(job_ids: list[str]) -> pd.DataFrame:
    if not job_ids:
        return pd.DataFrame(columns=["job_id", "genre_count"])
    return fetch_df(
        """
        SELECT job_id, COUNT(*) AS genre_count
        FROM recap_outputs
        WHERE job_id = ANY(%s)
        GROUP BY job_id
        """,
        (job_ids,),
    )


def failed_task_counts(job_ids: list[str]) -> pd.DataFrame:
    if not job_ids:
        return pd.DataFrame(columns=["job_id", "stage", "failed_count"])
    return fetch_df(
        """
        SELECT job_id, stage, COUNT(*) AS failed_count
        FROM recap_failed_tasks
        WHERE job_id = ANY(%s)
        GROUP BY job_id, stage
        ORDER BY job_id
        """,
        (job_ids,),
    )


def recent_summary_evaluations(limit: int = 14) -> pd.DataFrame:
    return fetch_df(
        """
        SELECT evaluation_id, created_at, evaluation_type, metrics
        FROM recap_evaluation_runs
        WHERE evaluation_type IN ('summary', 'full')
        ORDER BY created_at DESC
        LIMIT %s
        """,
        (limit,),
    )


def main() -> None:
    st.set_page_config(page_title="Recap Quality Dashboard", layout="wide")
    st.title("Recap Quality Dashboard — 3days focus")

    limit = st.sidebar.number_input("Recent jobs to display", min_value=1, max_value=60, value=14)

    st.header(f"Latest {limit} 3days jobs")
    jobs = latest_3days_jobs(limit=int(limit))
    if jobs.empty:
        st.warning("No 3days jobs found")
        return
    st.dataframe(jobs, width="stretch")

    job_ids = [str(j) for j in jobs["job_id"].tolist()]

    col_left, col_right = st.columns(2)

    with col_left:
        st.subheader("Genre coverage")
        g = genre_counts(job_ids)
        merged = jobs.merge(g, on="job_id", how="left").fillna({"genre_count": 0})
        st.bar_chart(merged.set_index("kicked_at")["genre_count"])

    with col_right:
        st.subheader("Failed task counts (by stage)")
        f = failed_task_counts(job_ids)
        if f.empty:
            st.info("No failed tasks recorded")
        else:
            pivot = f.pivot(index="job_id", columns="stage", values="failed_count").fillna(0)
            st.bar_chart(pivot)

    st.header("Summary evaluation history")
    evals = recent_summary_evaluations(limit=int(limit))
    if evals.empty:
        st.info("No summary evaluations yet — run `POST /api/v1/evaluations/summary` first.")
    else:
        extracted_rows = []
        for _, row in evals.iterrows():
            metrics = row["metrics"] or {}
            extracted_rows.append({
                "created_at": row["created_at"],
                "fallback_rate": metrics.get("fallback_rate"),
                "json_repair_rate": metrics.get("json_repair_rate"),
                "redundancy_score": metrics.get("redundancy_score"),
                "readability_score": metrics.get("readability_score"),
                "source_grounding_score": metrics.get("source_grounding_score"),
                "overall_quality_score": metrics.get("overall_quality_score"),
            })
        df = pd.DataFrame(extracted_rows).set_index("created_at").sort_index()
        st.line_chart(df)


if __name__ == "__main__":
    main()
