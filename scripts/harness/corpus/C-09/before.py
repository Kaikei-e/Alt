"""Valid BaseHTTPRequestHandler.log_message override preserving parameter name `format`."""

from http.server import BaseHTTPRequestHandler


class _OpsHandler(BaseHTTPRequestHandler):
    def log_message(
        self,
        format: str,  # noqa: A002  # BaseHTTPRequestHandler.log_message override signature
        *args: object,
    ) -> None:
        _ = (format, args)
