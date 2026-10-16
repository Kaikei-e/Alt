import os
from urllib.parse import quote

import streamlit as st
import streamlit.components.v1 as components

from .system_monitor_sse_client import generate_sse_client_js


def _resolve_sse_token(provided_token: str | None = None) -> str | None:
    """Resolve the SSE auth token from explicit operator input or disabled mode.

    Server secret files (SSE_AUTH_TOKEN_FILE) and URL query parameters are
    intentionally never auto-embedded into rendered client HTML.
    """
    if os.getenv("SSE_AUTH", "").strip().lower() == "disabled":
        return ""
    if provided_token:
        stripped = provided_token.strip()
        if stripped:
            return stripped
    return None


def render_system_monitor(window_seconds: int | None = None) -> None:
    """Render the real-time system monitor tab.

    ``window_seconds`` is accepted for API parity with other tabs but unused:
    SSE streaming is live and not time-window filtered.
    """
    del window_seconds  # API parity only; SSE is live.
    st.header("System Monitor (Real-time)")

    provided_token = st.text_input("SSE Authentication Token", type="password")
    token = _resolve_sse_token(provided_token)
    if token is None:
        st.error(
            "SSE authentication token is required. "
            "Enter the SSE authentication token above, or set SSE_AUTH=disabled in local development."
        )
        return

    # Fixed loopback connection parameters; query-parameter overrides are disallowed.
    sse_host = "localhost"
    sse_port = "8502"
    sse_protocol = "http"
    sse_path = "/stream"

    if token:
        sse_path = f"{sse_path}?token={quote(token)}"

    sse_client_js = generate_sse_client_js(
        sse_host=sse_host,
        sse_port=sse_port,
        sse_protocol=sse_protocol,
        sse_path=sse_path,
        timeout_seconds=10,
        max_reconnect_attempts=5,
    )

    html_code = """
    <div id="monitor-container" style="font-family: sans-serif;">
        <div style="display: flex; gap: 20px; flex-wrap: wrap; margin-bottom: 20px;">
            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>CPU Usage</h3>
                <div style="font-size: 2em;" id="cpu">Loading...</div>
                <div id="cpu-bar" style="height: 10px; background: #444; margin-top: 10px; border-radius: 5px; overflow: hidden;">
                    <div id="cpu-fill" style="height: 100%; width: 0%; background: #00bcd4; transition: width 0.5s;"></div>
                </div>
            </div>

            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>Memory Usage</h3>
                <div style="font-size: 2em;" id="mem">Loading...</div>
                <div style="font-size: 0.8em; color: #aaa;" id="mem-detail"></div>
                <div id="mem-bar" style="height: 10px; background: #444; margin-top: 10px; border-radius: 5px; overflow: hidden;">
                    <div id="mem-fill" style="height: 100%; width: 0%; background: #4caf50; transition: width 0.5s;"></div>
                </div>
            </div>

            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>Hanging Processes</h3>
                <div style="font-size: 2em;" id="hang">Loading...</div>
                <div style="font-size: 0.8em; color: #aaa;">spawn_main / fork</div>
            </div>
        </div>

        <div id="gpu-container" style="display: flex; gap: 20px; flex-wrap: wrap; margin-bottom: 20px;">
            <!-- GPU cards injected here -->
        </div>

        <div style="background: #262730; padding: 15px; border-radius: 5px; color: white;">
            <h3>Top Processes</h3>
            <table style="width: 100%; border-collapse: collapse; font-size: 0.9em;">
                <thead style="background: #333;">
                    <tr>
                        <th style="padding: 8px; text-align: left;">PID</th>
                        <th style="padding: 8px; text-align: left;">Name</th>
                        <th style="padding: 8px; text-align: left;">CPU %</th>
                        <th style="padding: 8px; text-align: left;">Mem (MB)</th>
                    </tr>
                </thead>
                <tbody id="proc-body">
                </tbody>
            </table>
        </div>

        <div style="margin-top: 10px; font-size: 0.8em; color: #666; text-align: right;">
            Status: <span id="conn-status" style="color: orange;">Connecting...</span>
        </div>
    </div>

    <script>
""" + sse_client_js + """
    </script>
    """

    components.html(html_code, height=800, scrolling=True)
