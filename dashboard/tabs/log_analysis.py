
import streamlit as st
import pandas as pd
import sqlite3
import os

def render_log_analysis():
    st.header("Log Analysis")

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
        st.warning(f"Log database not found at {LOG_DB_PATH}. Run 'analyze_logs.py' first.")
