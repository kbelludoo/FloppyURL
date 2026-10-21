#!/usr/bin/env python3
"""Minimal local-only mock for testing both NVIDIA chat clients without credentials."""
from http.server import BaseHTTPRequestHandler, HTTPServer
import json

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/v1/chat/completions" or self.headers.get("Authorization") != "Bearer local-mock-key":
            self.send_error(403)
            return
        length = int(self.headers.get("Content-Length", "0"))
        try:
            data = json.loads(self.rfile.read(length))
            assert data["messages"][1]["content"] == "Diga apenas: teste de integracao"
            assert isinstance(data["model"], str)
        except (AssertionError, KeyError, ValueError, TypeError):
            self.send_error(400)
            return
        payload = json.dumps({"choices":[{"message":{"role":"assistant","content":"ok-local"},"finish_reason":"stop"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        pass

if __name__ == "__main__":
    HTTPServer(("127.0.0.1", 18765), Handler).serve_forever()
