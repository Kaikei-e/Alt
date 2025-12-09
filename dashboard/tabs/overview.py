
import streamlit as st
from utils import fetch_recent_activity


def render_overview(window_seconds: int):
    st.header("Recent Activity")
    recent_jobs = fetch_recent_activity(window_seconds)
    st.dataframe(recent_jobs)
