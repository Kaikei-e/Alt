
import streamlit as st
import streamlit.components.v1 as components
from .system_monitor_sse_client import generate_sse_client_js


def render_system_monitor(window_seconds: int | None = None):
    st.header("System Monitor (Real-time)")

    # Get SSE connection parameters
    # This works around the iframe limitation in Streamlit components.html
    # Default to port 80 (Nginx) and path /sse/dashboard/stream
    try:
        # Try to get from query params or use default
        sse_host = st.query_params.get("sse_host", "localhost")
        sse_port = st.query_params.get("sse_port", "80")
        sse_protocol = st.query_params.get("sse_protocol", "http")
        sse_path = st.query_params.get("sse_path", "/sse/dashboard/stream")
    except:
        sse_host = "localhost"
        sse_port = "80"
        sse_protocol = "http"
        sse_path = "/sse/dashboard/stream"

    # Generate SSE client JavaScript code
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

    # Height argument allows scrolling if content gets long
    components.html(html_code, height=800, scrolling=True)

