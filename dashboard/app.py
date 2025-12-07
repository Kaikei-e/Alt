import streamlit as st
import pandas as pd
from sqlalchemy import create_engine, text
import json
import os

# --- Configuration ---
# Allow overriding data source via environment variable for local vs docker usage
DB_URI = os.getenv("RECAP_DB_DSN", "postgresql://recap:recap@localhost:5435/recap_db")

st.set_page_config(layout="wide", page_title="Recap System Dashboard")

# --- Database Connection ---
@st.cache_resource
def get_engine():
    return create_engine(DB_URI)

engine = get_engine()

# --- Data Fetching ---
def fetch_metrics(metric_type, limit=100) -> pd.DataFrame:
    query = text("""
        SELECT job_id, timestamp, metrics
        FROM recap_system_metrics
        WHERE metric_type = :metric_type
        ORDER BY timestamp DESC
        LIMIT :limit
    """)
    with engine.connect() as conn:
        df = pd.read_sql(query, conn, params={"metric_type": metric_type, "limit": limit})

    if not df.empty:
        # Convert JSON metrics to columns
        metrics_df = pd.json_normalize(df['metrics'])
        df = pd.concat([df.drop('metrics', axis=1), metrics_df], axis=1)
        # Convert timestamp to datetime
        df['timestamp'] = pd.to_datetime(df['timestamp'])
    return df

# --- Dashboard Layout ---
st.title("Recap System Evaluation Dashboard")

tabs = st.tabs(["Overview", "Classification", "Clustering", "Summarization"])

with tabs[0]:
    st.header("Recent Activity")
    with engine.connect() as conn:
        recent_jobs = pd.read_sql(text("""
            SELECT job_id, metric_type, timestamp
            FROM recap_system_metrics
            ORDER BY timestamp DESC LIMIT 20
        """), conn)
    st.dataframe(recent_jobs)

with tabs[1]:
    st.header("Classification Metrics")
    df_cls = fetch_metrics("classification")
    if not df_cls.empty:
        st.subheader("Accuracy Over Time")
        st.line_chart(df_cls, x="timestamp", y="accuracy")

        st.subheader("Latest Metrics")
        latest = df_cls.iloc[0]
        col1, col2, col3 = st.columns(3)
        col1.metric("Accuracy", f"{latest.get('accuracy', 0):.2%}")
        col2.metric("Macro F1", f"{latest.get('macro_f1', 0):.2%}")
        col3.metric("Micro F1", f"{latest.get('micro_f1', 0):.2%}")

        if 'per_genre' in df_cls.columns:
            st.subheader("Per-Genre F1 Scores (Latest)")
            # Need to parse nested per_genre if it wasn't flattened by json_normalize properly
            # json_normalize flattens one level. if per_genre is a dict, it becomes per_genre.genre_name.f1...
            # But the structure is likely: per_genre = {"genreA": {"f1": ...}, ...}
            # So columns would be per_genre.genreA.f1, per_genre.genreA.precision, etc.

            # Extract F1 columns
            f1_cols = [c for c in df_cls.columns if "per_genre" in c and ".f1-score" in c]
            if f1_cols:
                latest_f1 = df_cls.iloc[0][f1_cols]
                # Clean column names for display
                latest_f1.index = [c.split('.')[1] for c in latest_f1.index]
                st.bar_chart(latest_f1)
            else:
                 st.info("No detailed per-genre metrics available.")
    else:
        st.info("No classification metrics found.")

with tabs[2]:
    st.header("Clustering Metrics")
    df_clu = fetch_metrics("clustering")
    if not df_clu.empty:
        # Clustering metrics captured: silhouette_score, dbcv_score, etc. inside diagnostics
        # NOTE: logic in subworker/services/run_manager.py puts them in the root of diagnostics JSON?
        # Let's assume metrics are at the top level of the JSON stored.

        st.subheader("Clustering Quality")
        chart_data = df_clu[['timestamp', 'silhouette_score', 'dbcv_score']].copy()
        chart_data = chart_data.set_index('timestamp')
        st.line_chart(chart_data)

        col1, col2 = st.columns(2)
        latest = df_clu.iloc[0]
        col1.metric("Silhouette Score", f"{latest.get('silhouette_score', 0):.3f}")
        col2.metric("DBCV Score", f"{latest.get('dbcv_score', 0):.3f}")

    else:
        st.info("No clustering metrics found.")

with tabs[3]:
    st.header("Summarization Metrics")
    df_sum = fetch_metrics("summarization")
    if not df_sum.empty:
        st.subheader("Performance Metrics")
        # Columns: json_validation_errors, summary_length_bullets, processing_time_ms

        # Line chart for processing time
        st.line_chart(df_sum, x="timestamp", y="processing_time_ms")

        col1, col2, col3 = st.columns(3)
        latest = df_sum.iloc[0]
        col1.metric("JSON Errors (Latest)", int(latest.get('json_validation_errors', 0)))
        col2.metric("Summary Length (Bullets)", int(latest.get('summary_length_bullets', 0)))
        col3.metric("Processing Time (ms)", int(latest.get('processing_time_ms', 0)))

        st.subheader("Error Rate Over Time")
        st.bar_chart(df_sum, x="timestamp", y="json_validation_errors")
    else:
        st.info("No summarization metrics found.")
