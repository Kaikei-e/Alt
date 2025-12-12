
import streamlit as st
import os
import sys
import time
import requests
from tabs import overview, classification, clustering, summarization, log_analysis, system_monitor_tab, admin_jobs, recap_jobs
from utils import TIME_WINDOWS

# --- Configuration ---
st.set_page_config(layout="wide", page_title="Recap System Dashboard")

# --- Background Services ---
# Configure logging to ensure logs are visible
import logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    force=True  # Force reconfiguration even if logging was already configured
)

@st.cache_resource
def check_sse_server_health():
    """Check if the SSE server (running as separate process) is healthy."""
    logger = logging.getLogger(__name__)
    sse_port = int(os.getenv('SSE_PORT', 8000))
    health_url = f"http://localhost:{sse_port}/health"

    try:
        logger.info(f"Checking SSE server health at {health_url} (SSE_PORT={sse_port})")
        response = requests.get(health_url, timeout=5)
        if response.status_code == 200:
            health_data = response.json()
            logger.info(f"SSE server health check passed: {health_data}")
            return True
        else:
            logger.warning(f"SSE server health check returned status {response.status_code} (expected 200)")
            return False
    except requests.exceptions.Timeout:
        logger.warning(f"SSE server health check timed out. Server may still be starting.")
        return False
    except requests.exceptions.ConnectionError:
        logger.warning(f"SSE server health check connection error. Server may still be starting.")
        return False
    except requests.exceptions.RequestException as e:
        logger.warning(f"SSE server health check failed: {e}")
        return False

# Check SSE server health at startup (non-blocking, just for logging)
logger = logging.getLogger(__name__)
sse_server_healthy = check_sse_server_health()
if not sse_server_healthy:
    logger.warning("⚠️ SSE server health check failed at startup. It may still be initializing.")

# --- Dashboard Layout ---
st.title("Recap System Evaluation Dashboard")

time_range = st.radio(
    "Time Range",
    options=["4h", "24h", "3d"],
    index=0,
    horizontal=True,
)
window_seconds = TIME_WINDOWS.get(time_range, TIME_WINDOWS["4h"])

# Tab organization:
# 1. Overview - Overall summary
# 2-4. Pipeline - Processing stages (Classification, Clustering, Summarization)
# 5-6. Monitoring - System monitoring and analysis (System Monitor, Log Analysis)
# 7-8. Jobs - Job management (Admin Jobs, Recap Jobs)
tabs_ui = st.tabs([
    "Overview",  # Overview
    "Classification",  # Pipeline
    "Clustering",  # Pipeline
    "Summarization",  # Pipeline
    "System Monitor",  # Monitoring
    "Log Analysis",  # Monitoring
    "Admin Jobs",  # Jobs
    "Recap Jobs",  # Jobs
])

with tabs_ui[0]:
    overview.render_overview(window_seconds)

with tabs_ui[1]:
    classification.render_classification(window_seconds)

with tabs_ui[2]:
    clustering.render_clustering(window_seconds)

with tabs_ui[3]:
    summarization.render_summarization(window_seconds)

with tabs_ui[4]:
    system_monitor_tab.render_system_monitor(window_seconds)

with tabs_ui[5]:
    log_analysis.render_log_analysis(window_seconds)

with tabs_ui[6]:
    admin_jobs.render_admin_jobs(window_seconds)

with tabs_ui[7]:
    recap_jobs.render_recap_jobs(window_seconds)
