
import dataclasses
import json
import time
import logging
import os
import traceback
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
import system_monitor

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Get port from environment variable or use default
PORT = int(os.getenv('SSE_PORT', 8000))

# CORS: restrict to the dashboard's own origin by default. Set SSE_ALLOWED_ORIGIN
# to override (e.g. a different nginx-fronted host), or "*" to explicitly allow any origin.
ALLOWED_ORIGIN = os.getenv('SSE_ALLOWED_ORIGIN', f'http://localhost:{PORT}')

class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    pass

class SSEHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        """Override to use our logger instead of stderr"""
        logger.info(f"{self.address_string()} - {format % args}")

    def do_OPTIONS(self):
        """Handle CORS preflight requests"""
        logger.info(f"Received OPTIONS request for path: {self.path}")
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', ALLOWED_ORIGIN)
        self.send_header('Access-Control-Allow-Methods', 'GET, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Cache-Control, Content-Type')
        self.send_header('Access-Control-Max-Age', '3600')
        self.end_headers()

    def do_GET(self):
        logger.info(f"Received GET request for path: {self.path} from {self.client_address}")

        if self.path == '/stream':
            try:
                logger.info(f"SSE connection attempt from {self.client_address}")
                logger.debug(f"Request headers: {dict(self.headers)}")

                self.send_response(200)
                self.send_header('Content-Type', 'text/event-stream')
                self.send_header('Cache-Control', 'no-cache')
                self.send_header('Connection', 'keep-alive')
                self.send_header('Access-Control-Allow-Origin', ALLOWED_ORIGIN)
                self.send_header('Access-Control-Allow-Methods', 'GET, OPTIONS')
                self.send_header('Access-Control-Allow-Headers', 'Cache-Control, Content-Type')
                self.send_header('X-Accel-Buffering', 'no')  # Disable buffering for nginx if used
                self.end_headers()

                logger.info(f"SSE connection established from {self.client_address}, starting data stream")

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
                            "top_processes": system_monitor.get_top_processes(10)
                        }
                        data_gather_time = time.time() - data_start

                        # Format as SSE
                        payload = f"data: {json.dumps(data)}\n\n"
                        self.wfile.write(payload.encode('utf-8'))
                        self.wfile.flush()

                        message_count += 1
                        if message_count % 10 == 0:  # Log every 10th message
                            logger.debug(f"Sent {message_count} messages to {self.client_address} (data gather: {data_gather_time:.3f}s)")

                        time.sleep(2) # Update interval
                    except BrokenPipeError:
                        logger.info(f"Client {self.client_address} disconnected (broken pipe)")
                        break
                    except ConnectionResetError:
                        logger.info(f"Client {self.client_address} disconnected (connection reset)")
                        break
                    except OSError as e:
                        logger.warning(f"OSError in SSE stream for {self.client_address}: {e}")
                        break
                    except Exception as e:
                        logger.error(f"Error in SSE stream for {self.client_address}: {e}")
                        logger.error(traceback.format_exc())
                        break
            except Exception as e:
                logger.error(f"Error setting up SSE connection for {self.client_address}: {e}")
                logger.error(traceback.format_exc())
                try:
                    self.send_response(500)
                    self.end_headers()
                except:
                    pass
        elif self.path == '/health':
            # Health check endpoint
            logger.debug(f"Health check request from {self.client_address}")
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
                        "cpu_available": "percent" in test_cpu
                    }
                }
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Access-Control-Allow-Origin', ALLOWED_ORIGIN)
                self.end_headers()
                self.wfile.write(json.dumps(health_status).encode('utf-8'))
                logger.debug(f"Health check response sent to {self.client_address}")
            except Exception as e:
                logger.error(f"Error in health check: {e}")
                health_status = {
                    "status": "error",
                    "service": "sse-server",
                    "error": str(e)
                }
                self.send_response(500)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Access-Control-Allow-Origin', ALLOWED_ORIGIN)
                self.end_headers()
                self.wfile.write(json.dumps(health_status).encode('utf-8'))
        else:
            logger.warning(f"404 for path: {self.path} from {self.client_address}")
            self.send_response(404)
            self.end_headers()

def start_server():
    try:
        logger.info(f"SSE Server initialization starting...")
        logger.info(f"SSE Server will bind to 0.0.0.0:{PORT}")
        logger.info(f"SSE Server environment: SSE_PORT={os.getenv('SSE_PORT', 'not set (using default 8000)')}")

        server = ThreadingHTTPServer(('0.0.0.0', PORT), SSEHandler)
        logger.info(f"SSE Server HTTP server created successfully")
        logger.info(f"SSE Server starting on 0.0.0.0:{PORT}")
        logger.info(f"SSE Server is ready to accept connections")
        logger.info(f"SSE Server endpoints: /stream (SSE), /health (health check)")
        server.serve_forever()
    except OSError as e:
        logger.error(f"Failed to start SSE server on port {PORT}: {e}")
        logger.error(f"OSError details: errno={e.errno if hasattr(e, 'errno') else 'N/A'}, strerror={e.strerror if hasattr(e, 'strerror') else 'N/A'}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error in SSE server: {e}")
        logger.error(f"Exception type: {type(e).__name__}")
        logger.error(traceback.format_exc())
        raise

if __name__ == "__main__":
    """Entry point for running SSE server as a standalone process."""
    logger.info("Starting SSE server as standalone process...")
    start_server()