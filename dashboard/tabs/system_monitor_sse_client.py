"""
SSE Client JavaScript Code Loader for System Monitor

This module loads JavaScript code from a separate file and injects
configuration values for establishing and managing Server-Sent Events (SSE)
connections in Streamlit components.html.
"""

import json
import os
import re
from pathlib import Path
from typing import Optional


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

    Args:
        sse_host: SSE server hostname
        sse_port: SSE server port
        sse_protocol: Protocol (http or https)
        sse_path: SSE endpoint path
        timeout_seconds: Connection timeout in seconds
        max_reconnect_attempts: Maximum number of reconnection attempts

    Returns:
        JavaScript code as a string with configuration values injected
    """
    # Get the directory where this Python file is located
    current_dir = Path(__file__).parent
    js_file_path = current_dir / "static" / "sse_client.js"

    # Read the JavaScript file
    try:
        with open(js_file_path, "r", encoding="utf-8") as f:
            js_code = f.read()
    except FileNotFoundError:
        raise FileNotFoundError(
            f"JavaScript file not found: {js_file_path}. "
            "Please ensure sse_client.js exists in the static directory."
        )

    # Escape values for JavaScript string literals
    sse_host_js = json.dumps(sse_host)
    sse_port_js = json.dumps(sse_port)
    sse_protocol_js = json.dumps(sse_protocol)
    sse_path_js = json.dumps(sse_path)
    timeout_ms = timeout_seconds * 1000

    # Replace placeholder values with actual configuration
    # Use regex to handle placeholders with or without spaces
    js_code = re.sub(r"'\{\{SSE_HOST\}\}'", sse_host_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PORT\}\}'", sse_port_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PROTOCOL\}\}'", sse_protocol_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PATH\}\}'", sse_path_js, js_code)
    js_code = re.sub(r'\{\{\s*TIMEOUT_MS\s*\}\}', str(timeout_ms), js_code)
    js_code = re.sub(r'\{\{\s*MAX_RECONNECT_ATTEMPTS\s*\}\}', str(max_reconnect_attempts), js_code)

    return js_code

