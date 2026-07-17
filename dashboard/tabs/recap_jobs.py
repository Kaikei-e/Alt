import streamlit as st

from utils import _interval_params, fetch_table_or_warn


def render_recap_jobs(window_seconds: int) -> None:
    st.header("Recap Jobs (7-Day Recap)")
    df = fetch_table_or_warn(
        "recap_jobs",
        """
        SELECT job_id, status, last_stage, kicked_at, updated_at
        FROM recap_jobs
        WHERE kicked_at > NOW() - (:window_seconds || ' seconds')::interval
        ORDER BY kicked_at DESC
        LIMIT 200
        """,
        _interval_params(window_seconds),
    )
    if df is None:
        return

    if df.empty:
        st.info("No recap jobs found.")
        return

    df["job_id"] = df["job_id"].astype(str)

    # Status metrics
    running_count = (df["status"] == "running").sum()
    failed_count = (df["status"] == "failed").sum()
    completed_count = (df["status"] == "completed").sum()

    col1, col2, col3 = st.columns(3)
    col1.metric("Running", int(running_count))
    col2.metric("Completed", int(completed_count))
    col3.metric("Failed", int(failed_count))

    st.subheader("Latest Recap Jobs")
    st.dataframe(df)
