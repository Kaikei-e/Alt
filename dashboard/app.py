
import streamlit as st
import threading
import os
# Put sse_server import inside a try-except block or ensure it's in path?
# It's in the same dir and docker sets workdir to /app (where app.py is).
from sse_server import run_background as start_sse_thread
from tabs import overview, classification, clustering, summarization, log_analysis, system_monitor_tab, admin_jobs
from utils import TIME_WINDOWS

# --- Configuration ---
st.set_page_config(layout="wide", page_title="Recap System Dashboard")

# --- Background Services ---
@st.cache_resource
def init_sse_server():
    """Start the SSE server in a background thread once."""
    import logging
    logger = logging.getLogger(__name__)
    try:
        logger.info("Initializing SSE server...")
        start_sse_thread()
        logger.info("SSE server initialization completed")
        return True
    except Exception as e:
        logger.error(f"Failed to initialize SSE server: {e}")
        import traceback
        logger.error(traceback.format_exc())
        return False

if init_sse_server():
    # Server started successfully
    pass

# --- Dashboard Layout ---
st.title("Recap System Evaluation Dashboard")

time_range = st.radio(
    "Time Range",
    options=["4h", "24h", "3d"],
    index=0,
    horizontal=True,
)
window_seconds = TIME_WINDOWS.get(time_range, TIME_WINDOWS["4h"])

tabs_ui = st.tabs(["Overview", "Classification", "Clustering", "Summarization", "Log Analysis", "System Monitor", "Admin Jobs"])

with tabs_ui[0]:
    overview.render_overview(window_seconds)

with tabs_ui[1]:
    classification.render_classification(window_seconds)

with tabs_ui[2]:
    clustering.render_clustering(window_seconds)

with tabs_ui[3]:
    summarization.render_summarization(window_seconds)

with tabs_ui[4]:
    log_analysis.render_log_analysis(window_seconds)

with tabs_ui[5]:
    system_monitor_tab.render_system_monitor(window_seconds)

with tabs_ui[6]:
    admin_jobs.render_admin_jobs(window_seconds)
