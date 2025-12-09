import pandas as pd
import streamlit as st
from sqlalchemy import text

from utils import get_engine


def render_admin_jobs():
    st.header("Admin Jobs (Graph / Learning)")
    engine = get_engine()
    with engine.connect() as conn:
        df = pd.read_sql(
            text(
                """
                SELECT job_id, kind, status, started_at, finished_at, error, result
                FROM admin_jobs
                ORDER BY started_at DESC
                LIMIT 50
                """
            ),
            conn,
        )

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

