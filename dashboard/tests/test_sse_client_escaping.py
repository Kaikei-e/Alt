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

    def test_inline_script_escapes_angle_brackets_and_ampersands(self) -> None:
        malicious = "</script><script>alert(1)&</script>"
        js = generate_sse_client_js(
            sse_host=malicious,
            sse_port=malicious,
            sse_protocol=malicious,
            sse_path=malicious,
        )
        assert "</script>" not in js
        assert "<script>" not in js
        assert "\\u003c/script\\u003e" in js
        assert "\\u0026" in js

    def test_inline_script_preserves_backslashes_without_regex_escape_error(self) -> None:
        payload = r"host\with\backslashes\1\g<0>"
        js = generate_sse_client_js(sse_host=payload)
        assert r"host\\with\\backslashes\\1\\g\u003c0\u003e" in js

    def test_system_monitor_tab_does_not_auto_load_server_secret_or_query_params(self) -> None:
        tab_path = Path(__file__).resolve().parents[1] / "tabs" / "system_monitor_tab.py"
        source = tab_path.read_text(encoding="utf-8")
        assert "load_auth_token" not in source
        assert "st.query_params" not in source

    def test_exception_details_are_not_rendered_to_streamlit_or_health_response(self) -> None:
        base = Path(__file__).resolve().parents[1]
        for rel in (
            "utils.py",
            "sse_server.py",
            "system_monitor.py",
            "tabs/log_analysis.py",
            "tabs/clustering.py",
            "tabs/overview.py",
            "tabs/recap_jobs.py",
            "tabs/admin_jobs.py",
        ):
            src = (base / rel).read_text(encoding="utf-8")
            assert "st.error(str(e))" not in src, f"{rel} still exposes raw str(e)"
            assert "\"error\": str(e)" not in src, f"{rel} still exposes raw str(e)"
