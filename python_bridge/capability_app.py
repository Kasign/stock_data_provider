"""Internal-only Python capability API for the Go analysis core.

Run from the repository root with:
    uvicorn python_bridge.capability_app:app --host 0.0.0.0 --port 8090

Routes are intentionally thin. Capability implementations live under
`python_bridge/capabilities/`; this module owns only HTTP validation and auth.
"""
from __future__ import annotations

import hmac
import os
from datetime import date, datetime
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

from python_bridge.capabilities import llm, market_data, notification, persistence, search

app = FastAPI(title="DSA Python Capability Bridge", version="1.0.0", docs_url=None, redoc_url=None)


class StockBundleRequest(BaseModel):
    stock_code: str = Field(min_length=1, max_length=32)
    lookback_days: int = Field(default=60, ge=20, le=400)
    as_of: datetime | None = None


class RegionRequest(BaseModel):
    region: str = Field(default="cn", pattern="^(cn|hk|us|jp|kr)$")
    as_of: datetime | None = None


class StockNewsRequest(BaseModel):
    stock_code: str = Field(min_length=1, max_length=32)
    stock_name: str = Field(default="", max_length=128)
    max_results: int = Field(default=8, ge=1, le=20)


class StockAnalysisRequest(BaseModel):
    context: dict[str, Any]
    news_context: str = ""
    report_type: str = "detailed"
    query_id: str = Field(min_length=1, max_length=128)
    analysis_context_pack_summary: str = ""
    report_language: str = "zh"
    search_performed: bool = False
    news_result_count: int | None = Field(default=None, ge=0)
    news_evidence_present: bool = False


class MarketReviewGenerationRequest(BaseModel):
    overview: dict[str, Any]
    news: list[dict[str, Any]] = Field(default_factory=list)
    region: str = Field(default="cn", pattern="^(cn|hk|us|jp|kr)$")
    query_id: str = Field(min_length=1, max_length=128)
    report_language: str = "zh"


class DailyBarPayload(BaseModel):
    date: date
    open: float = Field(gt=0)
    high: float = Field(gt=0)
    low: float = Field(gt=0)
    close: float = Field(gt=0)
    volume: float = Field(ge=0)
    amount: float = Field(ge=0)
    pct_chg: float = 0.0
    missing_fields: list[str] = Field(default_factory=list)


class DailyBarsPersistenceRequest(BaseModel):
    stock_code: str = Field(min_length=1, max_length=32)
    source: str = Field(default="unknown", max_length=128)
    bars: list[DailyBarPayload] = Field(default_factory=list, max_length=400)


class StockPersistenceRequest(BaseModel):
    result: dict[str, Any]
    context: dict[str, Any]
    legacy_context: dict[str, Any] = Field(default_factory=dict)
    analysis_context_pack_summary: str = ""
    report_type: str = "detailed"
    query_id: str = Field(min_length=1, max_length=128)


class MarketPersistenceRequest(BaseModel):
    report: str = Field(min_length=1)
    region: str = Field(default="cn", pattern="^(cn|hk|us|jp|kr)$")
    query_id: str = Field(min_length=1, max_length=128)
    overview: dict[str, Any] = Field(default_factory=dict)


class StockNotificationRequest(BaseModel):
    result: dict[str, Any]
    report_type: str = "detailed"
    query_id: str = ""
    dedup_key: str = Field(default="", max_length=256)


class MarketNotificationRequest(BaseModel):
    report: str = Field(min_length=1)
    region: str = Field(default="cn", pattern="^(cn|hk|us|jp|kr)$")
    query_id: str = Field(min_length=1, max_length=128)


async def require_bridge_token(x_python_bridge_token: str | None = Header(default=None)) -> None:
    expected = os.getenv("PYTHON_BRIDGE_TOKEN", "").strip()
    if len(expected) < 32 or any(marker in expected.lower() for marker in ("replace-with", "change-me", "changeme", "your-secret", "example-secret")):
        raise HTTPException(status_code=503, detail={"code": "bridge_auth_not_configured", "message": "PYTHON_BRIDGE_TOKEN must be a non-placeholder secret with at least 32 characters"})
    if not x_python_bridge_token or not hmac.compare_digest(x_python_bridge_token, expected):
        raise HTTPException(status_code=401, detail={"code": "unauthorized", "message": "valid X-Python-Bridge-Token header required"})


@app.get("/healthz")
def health() -> dict[str, str]:
    return {"status": "ok", "service": "dsa-python-bridge", "version": "1.0.0"}


@app.post("/internal/v1/market-data/stock-bundle", dependencies=[Depends(require_bridge_token)])
def fetch_stock_bundle(request: StockBundleRequest) -> dict[str, Any]:
    return market_data.fetch_stock_bundle(request.stock_code, request.lookback_days, request.as_of)


@app.post("/internal/v1/market-data/stock-auxiliary", dependencies=[Depends(require_bridge_token)])
def fetch_stock_auxiliary(request: StockBundleRequest) -> dict[str, Any]:
    return market_data.fetch_stock_auxiliary(request.stock_code, request.as_of)


@app.post("/internal/v1/market-data/market-overview", dependencies=[Depends(require_bridge_token)])
def fetch_market_overview(request: RegionRequest) -> dict[str, Any]:
    return market_data.fetch_market_overview(request.region, request.as_of)


@app.post("/internal/v1/search/stock-news", dependencies=[Depends(require_bridge_token)])
def search_stock_news(request: StockNewsRequest) -> dict[str, Any]:
    return search.search_stock_news(request.stock_code, request.stock_name, request.max_results)


@app.post("/internal/v1/search/market-news", dependencies=[Depends(require_bridge_token)])
def search_market_news(request: RegionRequest) -> dict[str, Any]:
    return search.search_market_news(request.region)


@app.post("/internal/v1/llm/analyze-stock", dependencies=[Depends(require_bridge_token)])
def analyze_stock(request: StockAnalysisRequest) -> dict[str, Any]:
    return llm.analyze_stock(
        request.context, request.news_context, request.query_id,
        request.analysis_context_pack_summary, request.report_language,
        request.report_type, request.search_performed, request.news_result_count,
        request.news_evidence_present,
    )


@app.post("/internal/v1/llm/generate-market-review", dependencies=[Depends(require_bridge_token)])
def generate_market_review(request: MarketReviewGenerationRequest) -> dict[str, str]:
    return llm.generate_market_review(request.overview, request.news, request.region, request.query_id, request.report_language)


@app.post("/internal/v1/persistence/daily-bars", dependencies=[Depends(require_bridge_token)])
def persist_daily_bars(request: DailyBarsPersistenceRequest) -> dict[str, int]:
    bars = [item.model_dump() for item in request.bars]
    return persistence.save_daily_bars(request.stock_code.strip().upper(), bars, request.source)


@app.post("/internal/v1/persistence/stock-analysis", dependencies=[Depends(require_bridge_token)])
def persist_stock_analysis(request: StockPersistenceRequest) -> dict[str, int]:
    return persistence.save_stock_analysis(request.result, request.context, request.report_type, request.query_id,
                                           request.legacy_context, request.analysis_context_pack_summary)


@app.post("/internal/v1/persistence/market-review", dependencies=[Depends(require_bridge_token)])
def persist_market_review(request: MarketPersistenceRequest) -> dict[str, int]:
    return persistence.save_market_review(request.report, request.region, request.query_id, request.overview)


@app.post("/internal/v1/notification/stock-analysis", dependencies=[Depends(require_bridge_token)])
def notify_stock_analysis(request: StockNotificationRequest) -> dict[str, Any]:
    return notification.notify_stock_analysis(request.result, request.report_type, request.query_id, request.dedup_key)


@app.post("/internal/v1/notification/market-review", dependencies=[Depends(require_bridge_token)])
def notify_market_review(request: MarketNotificationRequest) -> dict[str, Any]:
    return notification.notify_market_review(request.report, request.region, request.query_id)
