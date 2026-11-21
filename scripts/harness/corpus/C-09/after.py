"""BUG: renaming `format` to `fmt` to silence Ruff A002 breaks BaseHTTPRequestHandler override."""

from http.server import BaseHTTPRequestHandler


class _OpsHandler(BaseHTTPRequestHandler):
    def log_message(
        self,
        fmt: str,
        *args: object,
    ) -> None:
        _ = (fmt, args)
