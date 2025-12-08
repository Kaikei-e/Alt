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

tabs = st.tabs(["Overview", "Classification", "Clustering", "Summarization", "Log Analysis"])

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
        # Ensure expected columns exist
        for col in ['accuracy', 'macro_f1', 'hamming_loss']:
            if col not in df_cls.columns:
                df_cls[col] = 0.0

        st.subheader("Accuracy Over Time")
        st.line_chart(df_cls, x="timestamp", y="accuracy")

        st.subheader("Latest Metrics")
        latest = df_cls.iloc[0]
        col1, col2, col3 = st.columns(3)
        col1.metric("Accuracy", f"{latest['accuracy']:.2%}")
        col2.metric("Macro F1", f"{latest['macro_f1']:.2f}")
        col3.metric("Hamming Loss", f"{latest['hamming_loss']:.4f}")

        if 'per_genre' in df_cls.columns:
            st.subheader("Per-Genre F1 Scores (Latest)")
            f1_cols = [c for c in df_cls.columns if "per_genre" in c and ".f1-score" in c]
            if f1_cols:
                latest_f1 = df_cls.iloc[0][f1_cols]
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
        for col in ['silhouette_score', 'dbcv_score']:
             if col not in df_clu.columns:
                 df_clu[col] = 0.0

        st.subheader("Clustering Quality")
        chart_data = df_clu[['timestamp', 'silhouette_score', 'dbcv_score']].copy()
        chart_data = chart_data.set_index('timestamp')
        st.line_chart(chart_data)

        col1, col2 = st.columns(2)
        latest = df_clu.iloc[0]
        col1.metric("Silhouette Score", f"{latest['silhouette_score']:.3f}")
        col2.metric("DBCV Score", f"{latest['dbcv_score']:.3f}")

    else:
        st.info("No clustering metrics found.")

with tabs[3]:
    st.header("Summarization Metrics")
    df_sum = fetch_metrics("summarization")
    if not df_sum.empty:
        st.subheader("Performance Metrics")
        expected_cols = ['json_validation_errors', 'summary_length_bullets', 'processing_time_ms', 'relevance', 'faithfulness']
        for col in expected_cols:
            if col not in df_sum.columns:
                df_sum[col] = 0

        st.line_chart(df_sum, x="timestamp", y="processing_time_ms")

        col1, col2, col3, col4 = st.columns(4)
        latest = df_sum.iloc[0]
        col1.metric("JSON Errors", int(latest['json_validation_errors']))
        col2.metric("Length", int(latest['summary_length_bullets']))
        col3.metric("Time (ms)", int(latest['processing_time_ms']))
        col4.metric("Faithfulness", f"{float(latest['faithfulness']):.2f}")

        st.subheader("Error Rate")
        st.bar_chart(df_sum, x="timestamp", y="json_validation_errors")
    else:
        st.info("No summarization metrics found.")

with tabs[4]:
    st.header("Log Analysis")
    import sqlite3

    LOG_DB_PATH = os.getenv("RECAP_LOG_DB", "recap_logs.db")
    if os.path.exists(LOG_DB_PATH):
        try:
            conn_log = sqlite3.connect(LOG_DB_PATH)
            df_log = pd.read_sql("SELECT * FROM log_errors ORDER BY timestamp DESC LIMIT 500", conn_log)
            conn_log.close()

            if not df_log.empty:
                st.subheader("Error Distribution")
                st.bar_chart(df_log['error_type'].value_counts())

                st.subheader("Recent Errors")
                st.dataframe(df_log)
            else:
                st.info("No errors recorded in log database.")
        except Exception as e:
            st.error(f"Error reading log database: {e}")
    else:
        st.warning(f"Log database not found at {LOG_DB_PATH}. Run 'analyze_logs.py' first.")
