"""News-search adapters around the existing SearchService/MarketAnalyzer."""
from __future__ import annotations

import logging
from typing import Any

from fastapi import HTTPException

from .common import _news_items

logger = logging.getLogger(__name__)


def search_stock_news(stock_code: str, stock_name: str, max_results: int) -> dict[str, Any]:
    try:
        from src.search_service import get_search_service
        service = get_search_service()
        responses = service.search_comprehensive_intel(
            stock_code=stock_code.strip().upper(),
            stock_name=stock_name or stock_code,
            max_searches=5,
        )
        items: list[dict[str, Any]] = []
        for dimension, response in (responses or {}).items():
            for item in _news_items(response):
                item["dimension"] = str(dimension)
                items.append(item)
                if len(items) >= max_results:
                    break
            if len(items) >= max_results:
                break
        return {"items": items, "success": True}
    except Exception as exc:
        logger.exception("stock news search failed for %s", stock_code)
        raise HTTPException(status_code=502, detail={"code": "stock_news_search_failed", "message": f"news search failed: {type(exc).__name__}"}) from exc


def search_market_news(region: str) -> dict[str, Any]:
    try:
        from src.config import get_config
        from src.market_analyzer import MarketAnalyzer
        from src.search_service import get_search_service
        analyzer = MarketAnalyzer(search_service=get_search_service(), analyzer=None, region=region, config=get_config())
        return {"items": _news_items(analyzer.search_market_news())}
    except Exception as exc:
        logger.exception("market news search failed for %s", region)
        raise HTTPException(status_code=502, detail={"code": "market_news_search_failed", "message": f"market news search failed: {type(exc).__name__}"}) from exc
