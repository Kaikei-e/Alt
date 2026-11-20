"""
SSE Client JavaScript Code Loader for System Monitor

This module loads JavaScript code from a separate file and injects
configuration values for establishing and managing Server-Sent Events (SSE)
connections in Streamlit components.html.
"""

import json
import re
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True, slots=True)
class SseClientConfig:
    """Configuration values injected into the SSE client JavaScript."""

    sse_host: str = "localhost"
    sse_port: str = "8502"
    sse_protocol: str = "http"
    sse_path: str = "/stream"
    timeout_seconds: int = 10
    max_reconnect_attempts: int = 5


def _json_for_inline_script(value: str) -> str:
    """Serialize a string as JSON safe for embedding inside an HTML <script> block."""
    encoded = json.dumps(value)
    return encoded.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def generate_sse_client_js(config: SseClientConfig | None = None) -> str:
    """
    Load JavaScript code from file and inject configuration values.
    """
    cfg = config or SseClientConfig()
    current_dir = Path(__file__).parent
    js_file_path = current_dir / "static" / "sse_client.js"

    try:
        js_code = js_file_path.read_text(encoding="utf-8")
    except FileNotFoundError as e:
        msg = (
            f"JavaScript file not found: {js_file_path}. "
            "Please ensure sse_client.js exists in the static directory."
        )
        raise FileNotFoundError(msg) from e

    sse_host_js = _json_for_inline_script(cfg.sse_host)
    sse_port_js = _json_for_inline_script(cfg.sse_port)
    sse_protocol_js = _json_for_inline_script(cfg.sse_protocol)
    sse_path_js = _json_for_inline_script(cfg.sse_path)
    timeout_ms = int(cfg.timeout_seconds) * 1000
    max_reconnect = int(cfg.max_reconnect_attempts)

    js_code = re.sub(r"'\{\{SSE_HOST\}\}'", lambda _: sse_host_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PORT\}\}'", lambda _: sse_port_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PROTOCOL\}\}'", lambda _: sse_protocol_js, js_code)
    js_code = re.sub(r"'\{\{SSE_PATH\}\}'", lambda _: sse_path_js, js_code)
    js_code = re.sub(r"\{\{\s*TIMEOUT_MS\s*\}\}", lambda _: str(timeout_ms), js_code)
    return re.sub(
        r"\{\{\s*MAX_RECONNECT_ATTEMPTS\s*\}\}",
        lambda _: str(max_reconnect),
        js_code,
    )
