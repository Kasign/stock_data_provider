"""Single Python entry point for calling the Go core during migration.

Python API/CLI/Bot/Scheduler integrations should call this module rather than
constructing URLs or HTTP headers independently. Callers should retain their
existing legacy path until GO_CORE_ENABLED is explicitly enabled and rollout
checks have passed.
"""
from __future__ import annotations

import json
import math
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


class _NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    """Never forward the internal Go token to a redirect target."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class GoCoreError(RuntimeError):
    def __init__(self, message: str, *, status_code: int | None = None, code: str = "go_core_error") -> None:
        super().__init__(message)
        self.status_code = status_code
        self.code = code


class GoCoreClient:
    def __init__(self, base_url: str | None = None, token: str | None = None, timeout: float | None = None) -> None:
        self.base_url = (base_url or os.getenv("GO_CORE_BASE_URL", "http://go-core:8088")).rstrip("/")
        parsed_url = urllib.parse.urlsplit(self.base_url)
        if parsed_url.scheme not in {"http", "https"} or not parsed_url.netloc or parsed_url.username or parsed_url.password:
            raise GoCoreError("GO_CORE_BASE_URL must be an HTTP(S) URL without embedded credentials", code="go_core_invalid_base_url")
        self.token = token if token is not None else os.getenv("GO_CORE_TOKEN", "")
        self._opener = urllib.request.build_opener(_NoRedirectHandler())
        try:
            self.timeout = float(timeout if timeout is not None else os.getenv("GO_CORE_CLIENT_TIMEOUT_SECONDS", "1200"))
        except (TypeError, ValueError) as exc:
            raise GoCoreError("GO_CORE_CLIENT_TIMEOUT_SECONDS must be a positive number", code="go_core_invalid_timeout") from exc
        if not math.isfinite(self.timeout) or self.timeout <= 0:
            raise GoCoreError("GO_CORE_CLIENT_TIMEOUT_SECONDS must be a positive finite number", code="go_core_invalid_timeout")
        if not self.token.strip():
            raise GoCoreError("GO_CORE_TOKEN is required", code="go_core_auth_not_configured")

    def _post(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        body = json.dumps(payload, ensure_ascii=False, allow_nan=False).encode("utf-8")
        request = urllib.request.Request(
            self.base_url + path,
            data=body,
            method="POST",
            headers={"Content-Type": "application/json", "Accept": "application/json", "X-Go-Core-Token": self.token},
        )
        try:
            with self._opener.open(request, timeout=self.timeout) as response:
                data = response.read(16 * 1024 * 1024 + 1)
                if len(data) > 16 * 1024 * 1024:
                    raise GoCoreError("Go core response exceeded 16 MiB", code="response_too_large")
                parsed = json.loads(data.decode("utf-8")) if data else {}
                if not isinstance(parsed, dict):
                    raise GoCoreError("Go core returned a non-object JSON response", code="invalid_response")
                return parsed
        except urllib.error.HTTPError as exc:
            try:
                body = exc.read(64 * 1024).decode("utf-8", errors="replace")
                parsed = json.loads(body) if body else {}
            except Exception:
                parsed = {}
            raise GoCoreError(str(parsed.get("message") or f"Go core HTTP {exc.code}"), status_code=exc.code, code=str(parsed.get("code") or "go_core_http_error")) from exc
        except urllib.error.URLError as exc:
            raise GoCoreError(f"cannot reach Go core: {exc.reason}", code="go_core_unavailable") from exc
        except (TimeoutError, json.JSONDecodeError, UnicodeDecodeError, OSError) as exc:
            raise GoCoreError(f"Go core request failed: {exc}", code="go_core_transport_error") from exc

    def analyze_stock(self, *, stock_code: str, query_id: str | None = None, report_type: str = "detailed", as_of: str | None = None, lookback_days: int = 60, notify: bool = False, search_news: bool = True, report_language: str = "zh") -> dict[str, Any]:
        payload: dict[str, Any] = {"stock_code": stock_code, "report_type": report_type, "lookback_days": lookback_days, "notify": notify, "search_news": search_news, "report_language": report_language}
        if query_id:
            payload["query_id"] = query_id
        if as_of:
            payload["as_of"] = as_of
        return self._post("/internal/v1/analysis/stock", payload)

    def analyze_stocks(self, *, stock_codes: list[str], query_id: str | None = None, report_type: str = "detailed", as_of: str | None = None, lookback_days: int = 60, notify: bool = False, search_news: bool = True, report_language: str = "zh") -> dict[str, Any]:
        payload: dict[str, Any] = {"stock_codes": stock_codes, "report_type": report_type, "lookback_days": lookback_days, "notify": notify, "search_news": search_news, "report_language": report_language}
        if query_id:
            payload["query_id"] = query_id
        if as_of:
            payload["as_of"] = as_of
        return self._post("/internal/v1/analysis/stocks", payload)

    def market_review(self, *, region: str = "cn", query_id: str | None = None, as_of: str | None = None, notify: bool = False, report_language: str = "zh") -> dict[str, Any]:
        payload: dict[str, Any] = {"region": region, "notify": notify, "report_language": report_language}
        if query_id:
            payload["query_id"] = query_id
        if as_of:
            payload["as_of"] = as_of
        return self._post("/internal/v1/analysis/market-review", payload)


def go_core_enabled() -> bool:
    """Explicit opt-in so adding this bridge cannot silently change production routing."""
    return os.getenv("GO_CORE_ENABLED", "false").strip().lower() in {"1", "true", "yes", "on"}


def get_go_core_client() -> GoCoreClient:
    return GoCoreClient()
