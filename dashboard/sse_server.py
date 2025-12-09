
import json
import time
import threading
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
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Cache-Control, Content-Type')
        self.send_header('Access-Control-Max-Age', '3600')
        self.end_headers()

    def do_GET(self):
        logger.info(f"Received GET request for path: {self.path}")

        if self.path == '/stream':
            try:
                logger.info(f"SSE connection established from {self.client_address}")
                self.send_response(200)
                self.send_header('Content-Type', 'text/event-stream')
                self.send_header('Cache-Control', 'no-cache')
                self.send_header('Connection', 'keep-alive')
                self.send_header('Access-Control-Allow-Origin', '*')
                self.send_header('Access-Control-Allow-Methods', 'GET, OPTIONS')
                self.send_header('Access-Control-Allow-Headers', 'Cache-Control, Content-Type')
                self.send_header('X-Accel-Buffering', 'no')  # Disable buffering for nginx if used
                self.end_headers()

                while True:
                    try:
                        # Gather data
                        data = {
                            "memory": system_monitor.get_memory_info(),
                            "cpu": system_monitor.get_cpu_info(),
                            "gpu": system_monitor.get_gpu_info(),
                            "hanging_count": system_monitor.count_hanging_processes(),
                            "top_processes": system_monitor.get_top_processes(15)
                        }

                        # Format as SSE
                        payload = f"data: {json.dumps(data)}\n\n"
                        self.wfile.write(payload.encode('utf-8'))
                        self.wfile.flush()

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
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok", "service": "sse-server"}).encode('utf-8'))
        else:
            logger.warning(f"404 for path: {self.path} from {self.client_address}")
            self.send_response(404)
            self.end_headers()

def start_server():
    try:
        server = ThreadingHTTPServer(('0.0.0.0', PORT), SSEHandler)
        logger.info(f"SSE Server starting on 0.0.0.0:{PORT}")
        logger.info(f"SSE Server is ready to accept connections")
        server.serve_forever()
    except OSError as e:
        logger.error(f"Failed to start SSE server on port {PORT}: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error in SSE server: {e}")
        logger.error(traceback.format_exc())
        raise

def run_background():
    try:
        thread = threading.Thread(target=start_server, daemon=True, name="SSE-Server-Thread")
        thread.start()
        logger.info("SSE server thread started")
        # Give the server a moment to start
        time.sleep(0.5)
        if thread.is_alive():
            logger.info("SSE server thread is running")
        else:
            logger.error("SSE server thread failed to start")
    except Exception as e:
        logger.error(f"Failed to start SSE server thread: {e}")
        logger.error(traceback.format_exc())