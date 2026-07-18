import dataclasses
import json
import logging
import os
import time
import traceback
from http.server import BaseHTTPRequestHandler, HTTPServer
from socketserver import ThreadingMixIn

import system_monitor

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

# Get port from environment variable or use default
PORT = int(os.getenv("SSE_PORT", 8000))

# CORS: restrict to the dashboard's own origin by default. Set SSE_ALLOWED_ORIGIN
# to override (e.g. a different nginx-fronted host), or "*" to explicitly allow any origin.
ALLOWED_ORIGIN = os.getenv("SSE_ALLOWED_ORIGIN", f"http://localhost:{PORT}")


class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    """Threaded HTTP server for concurrent SSE clients."""


class SSEHandler(BaseHTTPRequestHandler):
    def log_message(self, format: str, *args: object) -> None:
        """Override to use our logger instead of stderr"""
        logger.info("%s - %s", self.address_string(), format % args)

    def do_OPTIONS(self) -> None:
        """Handle CORS preflight requests"""
        logger.info("Received OPTIONS request for path: %s", self.path)
        self.send_response(200)
        self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
        self.send_header("Access-Control-Allow-Methods", "GET, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Cache-Control, Content-Type")
        self.send_header("Access-Control-Max-Age", "3600")
        self.end_headers()

    def do_GET(self) -> None:
        logger.info(
            "Received GET request for path: %s from %s",
            self.path,
            self.client_address,
        )

        if self.path == "/stream":
            try:
                logger.info("SSE connection attempt from %s", self.client_address)
                logger.debug("Request headers: %s", dict(self.headers))

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

                message_count = 0
                while True:
                    try:
                        # Gather data
                        data_start = time.time()
                        data = {
                            "memory": dataclasses.asdict(system_monitor.get_memory_info()),
                            "cpu": system_monitor.get_cpu_info(),
                            "gpu": system_monitor.get_gpu_info(),
                            "hanging_count": system_monitor.count_hanging_processes(),
                            "top_processes": system_monitor.get_top_processes(10),
                        }
                        data_gather_time = time.time() - data_start

                        # Format as SSE
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
                    except (TypeError, ValueError, json.JSONEncodeError) as e:
                        logger.error(
                            "Data serialization error in SSE stream for %s: %s",
                            self.client_address,
                            e,
                            exc_info=True,
                        )
                        break
            except OSError as e:
                logger.error(
                    "Error setting up SSE connection for %s: %s",
                    self.client_address,
                    e,
                    exc_info=True,
                )
                try:
                    self.send_response(500)
                    self.end_headers()
                except OSError:
                    pass
        elif self.path == "/health":
            # Health check endpoint
            logger.debug("Health check request from %s", self.client_address)
            try:
                # Quick test of system monitor functions
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
            except (OSError, RuntimeError, TypeError, ValueError) as e:
                logger.error("Error in health check: %s", e, exc_info=True)
                health_status = {
                    "status": "error",
                    "service": "sse-server",
                    "error": str(e),
                }
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.send_header("Access-Control-Allow-Origin", ALLOWED_ORIGIN)
                self.end_headers()
                self.wfile.write(json.dumps(health_status).encode("utf-8"))
        else:
            logger.warning("404 for path: %s from %s", self.path, self.client_address)
            self.send_response(404)
            self.end_headers()


def start_server() -> None:
    try:
        logger.info("SSE Server initialization starting...")
        logger.info("SSE Server will bind to 0.0.0.0:%s", PORT)
        logger.info(
            "SSE Server environment: SSE_PORT=%s",
            os.getenv("SSE_PORT", "not set (using default 8000)"),
        )

        server = ThreadingHTTPServer(("0.0.0.0", PORT), SSEHandler)
        logger.info("SSE Server HTTP server created successfully")
        logger.info("SSE Server starting on 0.0.0.0:%s", PORT)
        logger.info("SSE Server is ready to accept connections")
        logger.info("SSE Server endpoints: /stream (SSE), /health (health check)")
        server.serve_forever()
    except OSError as e:
        logger.error("Failed to start SSE server on port %s: %s", PORT, e)
        logger.error(
            "OSError details: errno=%s, strerror=%s",
            getattr(e, "errno", "N/A"),
            getattr(e, "strerror", "N/A"),
        )
        raise
    except Exception as e:
        logger.error("Unexpected error in SSE server: %s", e)
        logger.error("Exception type: %s", type(e).__name__)
        logger.error(traceback.format_exc())
        raise


if __name__ == "__main__":
    """Entry point for running SSE server as a standalone process."""
    logger.info("Starting SSE server as standalone process...")
    start_server()
