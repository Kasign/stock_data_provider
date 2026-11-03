"""Notification adapter; sends only when the Go workflow explicitly requests it."""
from __future__ import annotations

import logging
from typing import Any

from fastapi import HTTPException

from .common import _analysis_result_model, _jsonable

logger = logging.getLogger(__name__)


def notify_stock_analysis(result_data: dict[str, Any], report_type: str = "detailed", query_id: str = "", dedup_key: str = "") -> dict[str, Any]:
    from src.notification import get_notification_service
    result = _analysis_result_model(result_data)
    notifier = get_notification_service()
    try:
        renderer = getattr(notifier, "generate_single_stock_report", None)
        content = renderer(result) if callable(renderer) else (result.raw_response or result.analysis_summary or f"{result.name} ({result.code})")
        sent = notifier.send_with_results(str(content or ""), route_type="analysis",
                                          dedup_key=(dedup_key or query_id or None),
                                          structured_payload=result_data)
        return {"sent": bool(getattr(sent, "success", False)), "result": _jsonable(sent)}
    except Exception as exc:
        logger.exception("stock notification failed for %s", result.code)
        raise HTTPException(status_code=502, detail={"code": "stock_notification_failed", "message": f"stock notification failed: {type(exc).__name__}"}) from exc


def notify_market_review(report: str, region: str, query_id: str) -> dict[str, Any]:
    from src.notification import get_notification_service
    notifier = get_notification_service()
    try:
        sent = notifier.send_with_results(report, route_type="market_review", dedup_key=query_id)
        return {"sent": bool(getattr(sent, "success", False)), "result": _jsonable(sent)}
    except Exception as exc:
        logger.exception("market notification failed for %s", region)
        raise HTTPException(status_code=502, detail={"code": "market_notification_failed", "message": f"market notification failed: {type(exc).__name__}"}) from exc
