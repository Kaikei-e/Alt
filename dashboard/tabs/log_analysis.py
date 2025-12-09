
import streamlit as st
import pandas as pd
from sqlalchemy import text, inspect

from utils import get_engine, filter_frame_by_window, _interval_params


def _fetch_log_errors(window_seconds: int, limit: int = 2000) -> pd.DataFrame:
    """
    Fetch log errors from recap-db (log_errors table).
    Expected columns: timestamp (timestamptz), error_type (text), error_message/raw_line.
    """
    engine = get_engine()
    query = text(
        """
        SELECT timestamp, error_type,
               COALESCE(error_message, raw_line) AS error_message,
               raw_line
        FROM log_errors
        WHERE timestamp > NOW() - (:window_seconds || ' seconds')::interval
        ORDER BY timestamp DESC
        LIMIT :limit
        """
    )
    with engine.connect() as conn:
        return pd.read_sql(query, conn, params={**_interval_params(window_seconds), "limit": limit})


def render_log_analysis(window_seconds: int):
    st.header("Log Analysis")

    # Ensure table exists before querying to avoid hard failures
    engine = get_engine()
    inspector = inspect(engine)
    if not inspector.has_table("log_errors"):
        st.warning("log_errors テーブルが見つかりません。")
        st.caption("recap-db に log_errors (timestamp, error_type, error_message/raw_line) を作成し、ログを書き込んでください。")
        return

    try:
        df_log = _fetch_log_errors(window_seconds)
    except Exception as e:
        st.warning("log_errors 取得中にエラーが発生しました。")
        with st.expander("詳細"):
            st.error(str(e))
        return

    # Fallback filter for unexpected timestamp formats
    df_log = filter_frame_by_window(df_log, "timestamp", window_seconds)

    if df_log.empty:
        st.info("ログデータがありません。")
        st.caption(
            "recap-db の log_errors テーブルにログを書き込むか、期間を広げて再表示してください。"
        )
        if table_missing:
            st.caption("テーブルが存在しない場合は、log_errors (timestamp, error_type, error_message/raw_line) を作成してください。")
        return

    # Basic Stats
    st.subheader("Error Overview")
    col1, col2 = st.columns(2)
    col1.metric("Total Recorded Errors", len(df_log))
    col2.metric("Unique Error Types", df_log["error_type"].nunique())

    # Error Distribution Chart
    st.bar_chart(df_log["error_type"].value_counts())

    # Specific Error Tracking / \"Watchlist\"
    st.subheader("Critical Error Watchlist")

    critical_patterns = {
        "DB Duplicate Key": "duplicate key value violates unique constraint",
        "LLM Validation (422)": "422 Unprocessable Entity",
        "GPU OOM": "CUDA out of memory",
    }

    watchlist_data = []
    col_name = "error_message" if "error_message" in df_log.columns else "raw_line"
    for label, pattern in critical_patterns.items():
        if col_name in df_log.columns:
            count = df_log[df_log[col_name].str.contains(pattern, case=False, na=False)].shape[0]
        else:
            count = 0
        watchlist_data.append({"Error Type": label, "Count": count})

    st.dataframe(pd.DataFrame(watchlist_data).set_index("Error Type"))

    # Detailed View
    st.subheader("Recent Error Logs")
    error_types = ["All"] + list(df_log["error_type"].unique())
    selected_type = st.selectbox("Filter by Error Type", error_types)

    if selected_type != "All":
        display_df = df_log[df_log["error_type"] == selected_type]
    else:
        display_df = df_log

    st.dataframe(display_df)
