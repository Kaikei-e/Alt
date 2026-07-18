import logging
import os

import requests
import streamlit as st
from tabs import (
    admin_jobs,
    classification,
    clustering,
    log_analysis,
    overview,
    recap_jobs,
    summarization,
    system_monitor_tab,
)
from utils import TIME_WINDOWS

# --- Configuration ---
st.set_page_config(layout="wide", page_title="Recap System Dashboard")

# --- Background Services ---
# Configure logging to ensure logs are visible
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    force=True,  # Force reconfiguration even if logging was already configured
)


@st.cache_resource
def check_sse_server_health() -> bool:
    """Check if the SSE server (running as separate process) is healthy."""
    logger = logging.getLogger(__name__)
    sse_port = int(os.getenv("SSE_PORT", 8000))
    health_url = f"http://localhost:{sse_port}/health"

    try:
        logger.info("Checking SSE server health at %s (SSE_PORT=%s)", health_url, sse_port)
        response = requests.get(health_url, timeout=5)
        if response.status_code == 200:
            health_data = response.json()
            logger.info("SSE server health check passed: %s", health_data)
            return True
        else:
            logger.warning(
                "SSE server health check returned status %s (expected 200)",
                response.status_code,
            )
            return False
    except requests.exceptions.Timeout:
        logger.warning("SSE server health check timed out. Server may still be starting.")
        return False
    except requests.exceptions.ConnectionError:
        logger.warning(
            "SSE server health check connection error. Server may still be starting."
        )
        return False
    except requests.exceptions.RequestException as e:
        logger.warning("SSE server health check failed: %s", e)
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
tabs_ui = st.tabs(
    [
        "Overview",  # Overview
        "Classification",  # Pipeline
        "Clustering",  # Pipeline
        "Summarization",  # Pipeline
        "System Monitor",  # Monitoring
        "Log Analysis",  # Monitoring
        "Admin Jobs",  # Jobs
        "Recap Jobs",  # Jobs
    ]
)

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
