import dataclasses
import hmac
import json
import logging
import os
import re
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from socketserver import ThreadingMixIn
from urllib.parse import parse_qs, urlparse

import system_monitor

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

# Get port from environment variable or use default
PORT = int(os.getenv("SSE_PORT", "8000"))

# CORS: restrict to the dashboard's own origin by default. Set SSE_ALLOWED_ORIGIN
# to override (e.g. a different nginx-fronted host), or "*" to explicitly allow any origin.
# NOTE: this is a browser-only defense (fetch/XHR honor it); curl/other direct
# HTTP clients ignore CORS entirely, so it must never be relied on as auth.
ALLOWED_ORIGIN = os.getenv("SSE_ALLOWED_ORIGIN", f"http://localhost:{PORT}")

# Module-level token holder, populated at startup by load_auth_token()
AUTH_TOKEN = os.getenv("SSE_AUTH_TOKEN", "")


def load_auth_token(*, fail_fast: bool = True) -> str | None:
    """Resolve SSE auth token from secret file, environment, or explicit disable.

    Requires SSE_AUTH_TOKEN_FILE (Docker secret path) or SSE_AUTH=disabled.
    Returns:
        - empty string "" if SSE_AUTH=disabled
        - the loaded token string if successfully resolved
        - None if unresolved and fail_fast is False
    Fails startup with non-zero exit if unresolved and fail_fast is True.
    """
    if os.getenv("SSE_AUTH") == "disabled":
        logger.warning(
            "sse_auth_disabled: SSE_AUTH=disabled was set explicitly; /stream is unauthenticated"
        )
        return ""

    token_file = os.getenv("SSE_AUTH_TOKEN_FILE")
    if token_file:
        try:
            with Path(token_file).open(encoding="utf-8") as f:
                token = f.read().strip()
        except OSError as e:
            logger.error("SSE_AUTH_TOKEN_FILE (%s) could not be read: %s", token_file, e)  # noqa: TRY400 - expected condition; traceback adds noise
            if fail_fast:
                sys.exit(1)
            return None
        if not token:
            logger.error("SSE_AUTH_TOKEN_FILE (%s) is empty", token_file)
            if fail_fast:
                sys.exit(1)
            return None
        logger.info("sse_auth_enabled: loaded token from %s", token_file)
        return token

    direct_token = os.getenv("SSE_AUTH_TOKEN", "").strip()
    if direct_token:
        logger.info("sse_auth_enabled: loaded token from SSE_AUTH_TOKEN")
        return direct_token

    logger.error(
        "SSE_AUTH_TOKEN_FILE is required; set SSE_AUTH=disabled to run SSE server without authentication"
    )
    if fail_fast:
        sys.exit(1)
    return None


def extract_token_param(query: str) -> str | None:
    """Extract the `token` query parameter from a raw URL query string."""
    params = parse_qs(query)
    values = params.get("token")
    return values[0] if values else None


def is_authorized(configured_token: str, token_param: str | None, auth_header: str | None) -> bool:
    """Check whether a request may access /stream or /health.

    When ``configured_token`` is empty, auth is disabled (loopback/dev mode)
    and every request is authorized. Otherwise the request must supply a
    matching `?token=` query parameter or `Authorization: Bearer <token>`
    header, compared in constant time.
    """
    if not configured_token:
        return True

    if token_param is not None and hmac.compare_digest(token_param, configured_token):
        return True

    if auth_header is not None:
        prefix = "Bearer "
        if auth_header.startswith(prefix):
            provided = auth_header[len(prefix) :]
            if hmac.compare_digest(provided, configured_token):
                return True

    return False


class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    """Threaded HTTP server for concurrent SSE clients."""


class SSEHandler(BaseHTTPRequestHandler):
    def log_message(self, format: str, *args: object) -> None:  # noqa: A002 - matches BaseHTTPRequestHandler.log_message override
        """Override to use our logger instead of stderr, stripping query params to prevent token leakage."""
        msg = format % args
        sanitized_msg = re.sub(r"\?[^\s\"']*", "", msg)
        logger.info("%s - %s", self.address_string(), sanitized_msg)

    def do_OPTIONS(self) -> None:
        """Handle CORS preflight requests"""
        path = urlparse(self.path).path
        logger.info("Received OPTIONS request for path: %s", path)
        self.send_response(200)
        self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
        self.send_header("Access-Control-Allow-Methods", "GET, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Cache-Control, Content-Type")
        self.send_header("Access-Control-Max-Age", "3600")
        self.end_headers()

    def _authorized(self) -> bool:
        parsed = urlparse(self.path)
        token_param = extract_token_param(parsed.query)
        auth_header = self.headers.get("Authorization")
        return is_authorized(AUTH_TOKEN, token_param, auth_header)

    def _handle_stream(self) -> None:
        try:
            logger.info("SSE connection attempt from %s", self.client_address)
            logger.debug(
                "Request headers: %s",
                {k: v for k, v in self.headers.items() if k.lower() != "authorization"},
            )

            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
            self.send_header("Access-Control-Allow-Methods", "GET, OPTIONS")
            self.send_header("Access-Control-Allow-Headers", "Cache-Control, Content-Type")
            self.send_header("X-Accel-Buffering", "no")  # Disable buffering for nginx if used
            self.end_headers()

            logger.info(
                "SSE connection established from %s, starting data stream",
                self.client_address,
            )

            self._stream_events()
        except OSError:
            logger.exception(
                "Error setting up SSE connection for %s",
                self.client_address,
            )
            try:
                self.send_response(500)
                self.end_headers()
            except OSError:
                pass

    def _stream_events(self) -> None:
        message_count = 0
        while True:
            try:
                data_start = time.time()
                data = {
                    "memory": dataclasses.asdict(system_monitor.get_memory_info()),
                    "cpu": system_monitor.get_cpu_info(),
                    "gpu": system_monitor.get_gpu_info(),
                    "hanging_count": system_monitor.count_hanging_processes(),
                    "top_processes": system_monitor.get_top_processes(10),
                }
                data_gather_time = time.time() - data_start

                payload = f"data: {json.dumps(data)}\n\n"
                self.wfile.write(payload.encode("utf-8"))
                self.wfile.flush()

                message_count += 1
                if message_count % 10 == 0:  # Log every 10th message
                    logger.debug(
                        "Sent %s messages to %s (data gather: %.3fs)",
                        message_count,
                        self.client_address,
                        data_gather_time,
                    )

                time.sleep(2)  # Update interval
            except BrokenPipeError:
                logger.info(
                    "Client %s disconnected (broken pipe)",
                    self.client_address,
                )
                break
            except ConnectionResetError:
                logger.info(
                    "Client %s disconnected (connection reset)",
                    self.client_address,
                )
                break
            except OSError as e:
                logger.warning(
                    "OSError in SSE stream for %s: %s",
                    self.client_address,
                    e,
                )
                break
            except TypeError, ValueError:
                logger.exception(
                    "Data serialization error in SSE stream for %s",
                    self.client_address,
                )
                break

    def _handle_health(self) -> None:
        logger.debug("Health check request from %s", self.client_address)
        try:
            test_memory = system_monitor.get_memory_info()
            test_cpu = system_monitor.get_cpu_info()
            health_status = {
                "status": "ok",
                "service": "sse-server",
                "port": PORT,
                "system_monitor": {
                    "memory_available": test_memory.total > 0,
                    "cpu_available": "percent" in test_cpu,
                },
            }
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
            self.end_headers()
            self.wfile.write(json.dumps(health_status).encode("utf-8"))
            logger.debug("Health check response sent to %s", self.client_address)
        except OSError, RuntimeError, TypeError, ValueError:
            logger.exception("Error in health check")
            health_status = {
                "status": "error",
                "service": "sse-server",
                "error": "internal server error",
            }
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
            self.end_headers()
            self.wfile.write(json.dumps(health_status).encode("utf-8"))

    def do_GET(self) -> None:
        path = urlparse(self.path).path
        logger.info(
            "Received GET request for path: %s from %s",
            path,
            self.client_address,
        )

        # /stream requires token authorization; /health and readiness paths are exempt
        if path == "/stream" and not self._authorized():
            logger.warning(
                "401 unauthorized request for path: %s from %s",
                path,
                self.client_address,
            )
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
            self.end_headers()
            self.wfile.write(
                json.dumps({"status": "error", "error": "unauthorized"}).encode("utf-8")
            )
            return

        if path == "/stream":
            self._handle_stream()
        elif path in ("/health", "/healthz", "/readyz"):
            self._handle_health()
        else:
            logger.warning("404 for path: %s from %s", path, self.client_address)
            self.send_response(404)
            self.end_headers()


def start_server() -> None:
    global AUTH_TOKEN  # noqa: PLW0603 - updates module-level AUTH_TOKEN at server startup
    try:
        logger.info("SSE Server initialization starting...")
        logger.info("SSE Server will bind to 0.0.0.0:%s", PORT)
        logger.info(
            "SSE Server environment: SSE_PORT=%s",
            os.getenv("SSE_PORT", "not set (using default 8000)"),
        )
        loaded = load_auth_token(fail_fast=True)
        AUTH_TOKEN = loaded if loaded is not None else ""

        server = ThreadingHTTPServer(("0.0.0.0", PORT), SSEHandler)  # noqa: S104 - container server intentionally binds all interfaces
        logger.info("SSE Server HTTP server created successfully")
        logger.info("SSE Server starting on 0.0.0.0:%s", PORT)
        logger.info("SSE Server is ready to accept connections")
        logger.info("SSE Server endpoints: /stream (SSE), /health (health check)")
        server.serve_forever()
    except SystemExit:
        raise
    except OSError as e:
        logger.error("Failed to start SSE server on port %s: %s", PORT, e)  # noqa: TRY400 - expected condition; traceback adds noise
        logger.error(  # noqa: TRY400 - expected condition; traceback adds noise
            "OSError details: errno=%s, strerror=%s",
            getattr(e, "errno", "N/A"),
            getattr(e, "strerror", "N/A"),
        )
        raise
    except Exception:
        logger.exception("Unexpected error in SSE server")
        raise


if __name__ == "__main__":
    """Entry point for running SSE server as a standalone process."""
    logger.info("Starting SSE server as standalone process...")
    start_server()
