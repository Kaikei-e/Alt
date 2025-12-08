
import streamlit as st
import pandas as pd
from sqlalchemy import create_engine, text
import os

# --- Configuration ---
# Allow overriding data source via environment variable for local vs docker usage
DB_URI = os.getenv("RECAP_DB_DSN", "postgresql://recap:recap@localhost:5435/recap_db")

# --- Database Connection ---
@st.cache_resource
def get_engine():
    return create_engine(DB_URI)

# --- Data Fetching ---
def fetch_metrics(metric_type, limit=100) -> pd.DataFrame:
    engine = get_engine()
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
        if 'job_id' in df.columns:
            df['job_id'] = df['job_id'].astype(str)
        # Convert JSON metrics to columns
        # Handle potential connection errors or malformed json inside catch logic if needed,
        # but kept simple as per original
        metrics_df = pd.json_normalize(df['metrics'])
        df = pd.concat([df.drop('metrics', axis=1), metrics_df], axis=1)
        # Convert timestamp to datetime
        df['timestamp'] = pd.to_datetime(df['timestamp'])
    return df
