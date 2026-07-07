
import streamlit as st
import pandas as pd
from sqlalchemy import create_engine, text
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path


def _read_secret(env_name: str) -> str | None:
    """Read `{env_name}_FILE` (Docker secret path) if set, else `{env_name}`.

    Mirrors the `_FILE` suffix convention used elsewhere in the repo (e.g.
    recap-evaluator's config.py) so a password can be mounted as a Docker
    secret instead of a plaintext env var.
    """
    file_path = os.getenv(f"{env_name}_FILE")
    if file_path:
        return Path(file_path).read_text().strip()
    return os.getenv(env_name)


def _build_db_uri() -> str:
    """Build the recap-db connection URI.

    Prefers a pre-built `RECAP_DB_DSN` (local dev/tests). Otherwise builds
    it from discrete host/port/user/name plus a password read from
    `RECAP_DB_PASSWORD` or `RECAP_DB_PASSWORD_FILE` (Docker secret). Fails
    fast if neither is available rather than falling back to a guessable
    default credential.
    """
    dsn = os.getenv("RECAP_DB_DSN")
    if dsn:
        return dsn

    password = _read_secret("RECAP_DB_PASSWORD")
    if not password:
        raise RuntimeError(
            "RECAP_DB_DSN or RECAP_DB_PASSWORD/RECAP_DB_PASSWORD_FILE must be "
            "set; no default database credential is provided."
        )

    host = os.getenv("RECAP_DB_HOST", "recap-db")
    port = os.getenv("RECAP_DB_PORT", "5432")
    user = os.getenv("RECAP_DB_USER", "recap_user")
    name = os.getenv("RECAP_DB_NAME", "recap")
    return f"postgresql://{user}:{password}@{host}:{port}/{name}"


# --- Configuration ---
DB_URI = _build_db_uri()

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
        metrics_df = pd.json_normalize(df["metrics"].tolist())
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
