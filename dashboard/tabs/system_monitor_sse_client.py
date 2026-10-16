"""
SSE Client JavaScript Code Loader for System Monitor

This module loads JavaScript code from a separate file and injects
configuration values for establishing and managing Server-Sent Events (SSE)
connections in Streamlit components.html.
"""

import json
import re
from pathlib import Path


def _json_for_inline_script(value: str) -> str:
    """Serialize a string as JSON safe for embedding inside an HTML <script> block."""
    encoded = json.dumps(value)
    return (
        encoded.replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("&", "\\u0026")
    )


def generate_sse_client_js(
    sse_host: str = "localhost",
    sse_port: str = "8502",
    sse_protocol: str = "http",
    sse_path: str = "/stream",
    timeout_seconds: int = 10,
    max_reconnect_attempts: int = 5,
) -> str:
    """
    Load JavaScript code from file and inject configuration values.
    """
    current_dir = Path(__file__).parent
    js_file_path = current_dir / "static" / "sse_client.js"

    try:
        with open(js_file_path, "r", encoding="utf-8") as f:
            js_code = f.read()
    except FileNotFoundError as e:
        raise FileNotFoundError(
            f"JavaScript file not found: {js_file_path}. "
            "Please ensure sse_client.js exists in the static directory."
        ) from e

    sse_host_js = _json_for_inline_script(sse_host)
    sse_port_js = _json_for_inline_script(sse_port)
    sse_protocol_js = _json_for_inline_script(sse_protocol)
    sse_path_js = _json_for_inline_script(sse_path)
    timeout_ms = int(timeout_seconds) * 1000
    max_reconnect = int(max_reconnect_attempts)

    js_code = re.sub(r"'\{\{SSE_HOST\}\}'", lambda _: sse_host_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PORT\}\}'", lambda _: sse_port_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PROTOCOL\}\}'", lambda _: sse_protocol_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PATH\}\}'", lambda _: sse_path_js, js_code)
    js_code = re.sub(r"\{\{\s*TIMEOUT_MS\s*\}\}", lambda _: str(timeout_ms), js_code)
    js_code = re.sub(
        r"\{\{\s*MAX_RECONNECT_ATTEMPTS\s*\}\}",
        lambda _: str(max_reconnect),
        js_code,
    )

    return js_code
