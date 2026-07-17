import streamlit as st
import pandas as pd

from utils import _interval_params, fetch_table_or_warn


def render_admin_jobs(window_seconds: int) -> None:
    st.header("Admin Jobs (Graph / Learning)")
    df = fetch_table_or_warn(
        "admin_jobs",
        """
        SELECT job_id, kind, status, started_at, finished_at, error, result
        FROM admin_jobs
        WHERE started_at > NOW() - (:window_seconds || ' seconds')::interval
        ORDER BY started_at DESC
        LIMIT 200
        """,
        _interval_params(window_seconds),
    )
    if df is None:
        return

    if df.empty:
        st.info("No admin jobs found.")
        return

    df["job_id"] = df["job_id"].astype(str)
    if "finished_at" in df.columns:
        df["duration_seconds"] = (
            pd.to_datetime(df["finished_at"]) - pd.to_datetime(df["started_at"])
        ).dt.total_seconds()

    running_count = (df["status"] == "running").sum()
    failed_count = (df["status"] == "failed").sum()
    succeeded_count = (df["status"] == "succeeded").sum() + (df["status"] == "partial").sum()

    col1, col2, col3 = st.columns(3)
    col1.metric("Running", int(running_count))
    col2.metric("Succeeded/Partial", int(succeeded_count))
    col3.metric("Failed", int(failed_count))

    st.subheader("Latest Jobs")
    st.dataframe(df)
