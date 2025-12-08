
import streamlit as st
import pandas as pd
from sqlalchemy import text
from utils import get_engine

def render_overview():
    st.header("Recent Activity")
    engine = get_engine()
    with engine.connect() as conn:
        recent_jobs = pd.read_sql(text("""
            SELECT job_id, metric_type, timestamp
            FROM recap_system_metrics
            ORDER BY timestamp DESC LIMIT 20
        """), conn)
    if not recent_jobs.empty and 'job_id' in recent_jobs.columns:
        recent_jobs['job_id'] = recent_jobs['job_id'].astype(str)

    st.dataframe(recent_jobs)
