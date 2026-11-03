from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from threading import Thread

import pytest

from python_bridge.go_core_client import GoCoreClient, GoCoreError, go_core_enabled


class _Server(BaseHTTPRequestHandler):
    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size) or b"{}")
        self.server.last_request = (self.path, self.headers.get("X-Go-Core-Token"), payload)
        body = json.dumps({"accepted": True, "path": self.path}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


def test_go_core_client_posts_through_single_client_with_auth():
    server = HTTPServer(("127.0.0.1", 0), _Server)
    worker = Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        client = GoCoreClient(f"http://127.0.0.1:{server.server_port}", "secret-token", 2)
        result = client.analyze_stock(stock_code="600396", query_id="test-query", search_news=False)
        assert result["accepted"] is True
        path, token, payload = server.last_request
        assert path == "/internal/v1/analysis/stock"
        assert token == "secret-token"
        assert payload["stock_code"] == "600396"
        assert payload["search_news"] is False
    finally:
        server.shutdown()
        worker.join(timeout=2)
        server.server_close()


def test_go_core_client_rejects_invalid_url_and_timeout():
    with pytest.raises(GoCoreError) as url_error:
        GoCoreClient("file:///tmp/nope", "token", 1)
    assert url_error.value.code == "go_core_invalid_base_url"

    with pytest.raises(GoCoreError) as timeout_error:
        GoCoreClient("http://127.0.0.1:8088", "token", float("inf"))
    assert timeout_error.value.code == "go_core_invalid_timeout"


def test_go_core_client_does_not_follow_redirects():
    target = HTTPServer(("127.0.0.1", 0), _Server)
    target_worker = Thread(target=target.serve_forever, daemon=True)
    target_worker.start()

    class RedirectHandler(BaseHTTPRequestHandler):
        def do_POST(self):
            self.send_response(302)
            self.send_header("Location", f"http://127.0.0.1:{target.server_port}/capture")
            self.end_headers()

        def log_message(self, *_args):
            pass

    redirector = HTTPServer(("127.0.0.1", 0), RedirectHandler)
    redirect_worker = Thread(target=redirector.serve_forever, daemon=True)
    redirect_worker.start()
    try:
        client = GoCoreClient(f"http://127.0.0.1:{redirector.server_port}", "secret-token", 2)
        with pytest.raises(GoCoreError) as error:
            client.analyze_stock(stock_code="600396")
        assert error.value.status_code == 302
        assert not hasattr(target, "last_request")
    finally:
        redirector.shutdown()
        redirect_worker.join(timeout=2)
        redirector.server_close()
        target.shutdown()
        target_worker.join(timeout=2)
        target.server_close()


def test_go_core_enabled_is_explicit_opt_in(monkeypatch):
    monkeypatch.delenv("GO_CORE_ENABLED", raising=False)
    assert go_core_enabled() is False
    monkeypatch.setenv("GO_CORE_ENABLED", "true")
    assert go_core_enabled() is True
