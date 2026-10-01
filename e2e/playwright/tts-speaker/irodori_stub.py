#!/usr/bin/env python3
"""Minimal deterministic fake Irodori-TTS upstream server for Playwright contract tests."""

import io
import json
import sys
import wave
from http.server import BaseHTTPRequestHandler, HTTPServer


def generate_pcm_wav(sample_rate: int = 48000, duration_seconds: float = 0.1) -> bytes:
    """Generate a valid mono 16-bit PCM WAV in memory."""
    buf = io.BytesIO()
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        num_frames = int(sample_rate * duration_seconds)
        # 16-bit silence
        wf.writeframes(b"\x00\x00" * num_frames)
    return buf.getvalue()


class IrodoriStubHandler(BaseHTTPRequestHandler):
    """Answers POST /v1/audio/speech with fixed 16-bit PCM WAV and GET /health with ok."""

    def do_GET(self) -> None:
        if self.path == "/health":
            body = json.dumps({"status": "ok", "runtime": {"loaded": True}}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_response(404)
        self.end_headers()

    def do_POST(self) -> None:
        if self.path == "/v1/audio/speech":
            content_length = int(self.headers.get("Content-Length", 0))
            _ = self.rfile.read(content_length)

            wav_data = generate_pcm_wav(sample_rate=48000, duration_seconds=0.1)
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.send_header("Content-Length", str(len(wav_data)))
            self.end_headers()
            self.wfile.write(wav_data)
            return

        self.send_response(404)
        self.end_headers()

    def log_message(self, format: str, *args: object) -> None:
        # Keep test runs clean
        pass


def run(port: int = 8088) -> None:
    server = HTTPServer(("0.0.0.0", port), IrodoriStubHandler)
    server.serve_forever()


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8088
    run(port)
