"""Source-level guards for the ops dashboard SSE client's HTML escaping (CWE-79).

The XSS fix lives in the client-side escapeHtml() inside sse_client.js; these
tests pin that every innerHTML interpolation keeps going through it (or
textContent), so a refactor cannot quietly reopen the sink.
"""

from pathlib import Path

from tabs.system_monitor_sse_client import generate_sse_client_js

SSE_CLIENT_JS = Path(__file__).resolve().parents[1] / "tabs" / "static" / "sse_client.js"


class TestSseClientDoesNotInterpolateUntrustedHtml:
    def test_defines_escape_html_helper(self) -> None:
        js = generate_sse_client_js()
        assert "function escapeHtml" in js

    def test_process_name_and_pid_are_not_raw_template_interpolations(self) -> None:
        text = SSE_CLIENT_JS.read_text(encoding="utf-8")
        assert "${p.pid}" not in text
        assert "${truncatedName}" not in text
        assert "${p.cpu_percent}" not in text

    def test_gpu_error_message_is_not_concatenated_into_innerhtml(self) -> None:
        text = SSE_CLIENT_JS.read_text(encoding="utf-8")
        assert "innerHTML = '<div style=\"color: #aaa; padding: 10px;\">' + message" not in text

    def test_gpu_innerhtml_fields_use_escape_html(self) -> None:
        text = SSE_CLIENT_JS.read_text(encoding="utf-8")
        assert "${escapeHtml(gpu.name)}" in text
        assert "${escapeHtml(gpu.utilization)}" in text
        assert "${escapeHtml(gpu.memory_percent)}" in text
        assert "${gpu.name}" not in text
