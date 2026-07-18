import logging
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pandas as pd
import streamlit as st
from sqlalchemy import create_engine, inspect, text
from sqlalchemy.engine import Engine
from sqlalchemy.exc import SQLAlchemyError

logger = logging.getLogger(__name__)


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
def get_engine() -> Engine:
    return create_engine(DB_URI)


def _interval_params(window_seconds: int) -> dict[str, int]:
    return {"window_seconds": max(window_seconds, 0)}


def safe_int(value: object, default: int = 0) -> int:
    """Safely convert a value to integer, handling NaN and None."""
    if pd.isna(value) or value is None:
        return default
    try:
        return int(float(value))  # type: ignore[arg-type]
    except (ValueError, TypeError):
        return default


def safe_float(value: object, default: float = 0.0) -> float:
    """Safely convert a value to float, handling NaN and None."""
    if pd.isna(value) or value is None:
        return default
    try:
        return float(value)  # type: ignore[arg-type]
    except (ValueError, TypeError):
        return default


def fetch_table_or_warn(
    table_name: str,
    query: str,
    params: dict,
) -> pd.DataFrame | None:
    """Fetch a SQL table with existence check and error handling.

    Returns None when the table is missing or the query fails (after showing
    a Streamlit warning). Returns an (possibly empty) DataFrame on success.
    """
    engine = get_engine()
    inspector = inspect(engine)
    if not inspector.has_table(table_name):
        st.warning(f"{table_name} テーブルが見つかりません。")
        return None

    try:
        with engine.connect() as conn:
            return pd.read_sql(text(query), conn, params=params)
    except SQLAlchemyError as e:
        logger.exception("Failed to fetch %s", table_name)
        st.warning(f"{table_name} 取得中にエラーが発生しました。")
        with st.expander("詳細"):
            st.error(str(e))
        return None


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
        if "timestamp" in df.columns:
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
    if not df.empty:
        if "job_id" in df.columns:
            df["job_id"] = df["job_id"].astype(str)
        if "timestamp" in df.columns:
            df["timestamp"] = pd.to_datetime(df["timestamp"])
    return df


def filter_frame_by_window(df: pd.DataFrame, column: str, window_seconds: int) -> pd.DataFrame:
    """Fallback filter for data sources without SQL time filtering (e.g., sqlite logs)."""
    if df.empty or column not in df.columns:
        return df
    ts = pd.to_datetime(df[column], errors="coerce", utc=True)
    cutoff = now_utc() - timedelta(seconds=window_seconds)
    return df.loc[ts > cutoff]
