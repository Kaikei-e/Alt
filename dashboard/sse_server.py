
import json
import time
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
import system_monitor

PORT = 8000

class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    pass

class SSEHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/stream':
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.send_header('Cache-Control', 'no-cache')
            self.send_header('Connection', 'keep-alive')
            self.send_header('Access-Control-Allow-Origin', '*')
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
                    break
                except Exception as e:
                    print(f"Error in SSE stream: {e}")
                    break
        else:
            self.send_response(404)
            self.end_headers()

def start_server():
    server = ThreadingHTTPServer(('0.0.0.0', PORT), SSEHandler)
    print(f"SSE Server streaming on port {PORT}")
    server.serve_forever()

def run_background():
    thread = threading.Thread(target=start_server, daemon=True)
    thread.start()
