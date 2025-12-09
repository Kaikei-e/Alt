
import streamlit as st
import pandas as pd
from sqlalchemy import create_engine, text
import os
from datetime import datetime, timedelta, timezone

# --- Configuration ---
# Allow overriding data source via environment variable for local vs docker usage
DB_URI = os.getenv("RECAP_DB_DSN", "postgresql://recap:recap@localhost:5435/recap_db")

# --- Time window helpers ---
TIME_WINDOWS = {
    "4h": 4 * 3600,
    "24h": 24 * 3600,
    "3d": 72 * 3600,
}


def now_utc() -> datetime:
    return datetime.now(timezone.utc)


# --- Database Connection ---
@st.cache_resource
def get_engine():
    return create_engine(DB_URI)


def _interval_params(window_seconds: int) -> dict[str, int]:
    return {"window_seconds": max(window_seconds, 0)}


# --- Data Fetching ---
def fetch_metrics(metric_type: str, window_seconds: int, limit: int = 500) -> pd.DataFrame:
    """Fetch metrics filtered by a time window."""
    engine = get_engine()
    query = text(
        """
        SELECT job_id, timestamp, metrics
        FROM recap_system_metrics
        WHERE metric_type = :metric_type
          AND timestamp > NOW() - (:window_seconds || ' seconds')::interval
        ORDER BY timestamp DESC
        LIMIT :limit
        """
    )
    with engine.connect() as conn:
        df = pd.read_sql(
            query,
            conn,
            params={"metric_type": metric_type, "limit": limit, **_interval_params(window_seconds)},
        )

    if not df.empty:
        if "job_id" in df.columns:
            df["job_id"] = df["job_id"].astype(str)
        metrics_df = pd.json_normalize(df["metrics"])
        df = pd.concat([df.drop("metrics", axis=1), metrics_df], axis=1)
        df["timestamp"] = pd.to_datetime(df["timestamp"])
    return df


def fetch_recent_activity(window_seconds: int, limit: int = 200) -> pd.DataFrame:
    """Fetch recent system metrics activity for overview."""
    engine = get_engine()
    query = text(
        """
        SELECT job_id, metric_type, timestamp
        FROM recap_system_metrics
        WHERE timestamp > NOW() - (:window_seconds || ' seconds')::interval
        ORDER BY timestamp DESC
        LIMIT :limit
        """
    )
    with engine.connect() as conn:
        df = pd.read_sql(
            query,
            conn,
            params={"limit": limit, **_interval_params(window_seconds)},
        )
    if not df.empty and "job_id" in df.columns:
        df["job_id"] = df["job_id"].astype(str)
        df["timestamp"] = pd.to_datetime(df["timestamp"])
    return df


def filter_frame_by_window(df: pd.DataFrame, column: str, window_seconds: int) -> pd.DataFrame:
    """Fallback filter for data sources without SQL time filtering (e.g., sqlite logs)."""
    if df.empty or column not in df.columns:
        return df
    ts = pd.to_datetime(df[column], errors="coerce", utc=True)
    cutoff = now_utc() - timedelta(seconds=window_seconds)
    return df.loc[ts > cutoff]
