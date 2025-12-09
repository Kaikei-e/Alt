
import streamlit as st
import pandas as pd
import sqlite3
import os
from utils import filter_frame_by_window


def render_log_analysis(window_seconds: int):
    st.header("Log Analysis")

    # Use environment variable for DB path, default to relative
    LOG_DB_PATH = os.getenv("RECAP_LOG_DB", "recap_logs.db")

    if not os.path.exists(LOG_DB_PATH):
        # Auto-initialize empty DB to prevent error
        try:
            conn = sqlite3.connect(LOG_DB_PATH)
            # Create table with expected schema
            cursor = conn.cursor()
            cursor.execute("""
                CREATE TABLE IF NOT EXISTS log_errors (
                    timestamp TEXT,
                    error_type TEXT,
                    raw_line TEXT
                )
            """)
            conn.commit()
            conn.close()
            st.info(f"Initialized new log database at {LOG_DB_PATH}. Waiting for logs...")
        except Exception as e:
            st.error(f"Failed to initialize log database: {e}")
            return

    try:
        conn_log = sqlite3.connect(LOG_DB_PATH)
        # Fetch more logs to analyze trends
        df_log = pd.read_sql("SELECT * FROM log_errors ORDER BY timestamp DESC LIMIT 2000", conn_log)
        conn_log.close()

        df_log = filter_frame_by_window(df_log, "timestamp", window_seconds)

        if df_log.empty:
            st.success("No errors recorded in log database!")
            return

        # Basic Stats
        st.subheader("Error Overview")
        col1, col2 = st.columns(2)
        col1.metric("Total Recorded Errors", len(df_log))
        col2.metric("Unique Error Types", df_log['error_type'].nunique())

        # Error Distribution Chart
        st.bar_chart(df_log['error_type'].value_counts())

        # Specific Error Tracking / "Watchlist"
        st.subheader("Critical Error Watchlist")

        # Define known critical errors to watch for
        critical_patterns = {
            "DB Duplicate Key": "duplicate key value violates unique constraint",
            "LLM Validation (422)": "422 Unprocessable Entity",
            "GPU OOM": "CUDA out of memory"
        }

        # Check presence
        watchlist_data = []
        for label, pattern in critical_patterns.items():
            # Check if pattern is in error_message (assuming column exists) or error_type
            # The schema usually has 'error_message' and 'error_type'
            if 'error_message' in df_log.columns:
                count = df_log[df_log['error_message'].str.contains(pattern, case=False, na=False)].shape[0]
            else:
                count = 0
            watchlist_data.append({"Error Type": label, "Count": count})

        st.dataframe(pd.DataFrame(watchlist_data).set_index("Error Type"))

        # Detailed View
        st.subheader("Recent Error Logs")
        # Filter filters
        error_types = ["All"] + list(df_log['error_type'].unique())
        selected_type = st.selectbox("Filter by Error Type", error_types)

        if selected_type != "All":
            display_df = df_log[df_log['error_type'] == selected_type]
        else:
            display_df = df_log

        st.dataframe(display_df)

    except Exception as e:
        st.error(f"Error reading log database: {e}")
