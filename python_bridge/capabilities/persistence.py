"""Persistence adapters; only this module writes existing Python ORM records."""
from __future__ import annotations

import logging
from typing import Any

from fastapi import HTTPException

from .common import _analysis_result_model

logger = logging.getLogger(__name__)


def save_daily_bars(stock_code: str, bars: list[dict[str, Any]], source: str = "unknown") -> dict[str, int]:
    """Persist standardized bars to the legacy stock_daily store.

    This adapter is the only migration-bridge capability that writes stock_daily.
    The provider adapter returns normalized JSON only and never writes the database.
    """
    if not bars:
        return {"saved_count": 0}
    try:
        import pandas as pd
        from src.storage import DatabaseManager

        frame_rows = []
        for row in bars:
            missing = {str(field).strip().lower() for field in row.get("missing_fields", [])}
            frame_rows.append({
                "date": row.get("date"),
                "open": row.get("open"),
                "high": row.get("high"),
                "low": row.get("low"),
                "close": row.get("close"),
                "volume": None if "volume" in missing else row.get("volume", 0.0),
                "amount": None if "amount" in missing else row.get("amount", 0.0),
                "pct_chg": None if "pct_chg" in missing else row.get("pct_chg", 0.0),
            })
        frame = pd.DataFrame(frame_rows)
        saved_count = DatabaseManager.get_instance().save_daily_data(frame, stock_code, source or "unknown")
    except Exception as exc:
        logger.exception("persist daily bars failed for %s", stock_code)
        raise HTTPException(status_code=500, detail={"code": "daily_bars_persistence_failed", "message": f"daily history persistence failed: {type(exc).__name__}"}) from exc
    if not isinstance(saved_count, int) or isinstance(saved_count, bool) or saved_count < 0:
        raise HTTPException(status_code=500, detail={"code": "daily_bars_persistence_failed", "message": "database adapter returned an invalid saved row count"})
    return {"saved_count": saved_count}


def save_stock_analysis(
    result_data: dict[str, Any],
    context: dict[str, Any],
    report_type: str,
    query_id: str,
    legacy_context: dict[str, Any] | None = None,
    analysis_context_pack_summary: str = "",
) -> dict[str, int]:
    from src.repositories.analysis_repo import AnalysisRepository
    result = _analysis_result_model(result_data)
    result.query_id = query_id
    try:
        # Keep old snapshot keys so existing Python history/report consumers can
        # continue rendering records written by Go during the migration window.
        snapshot = {
            "context_version": "go-core-v1",
            "go_analysis_context": context,
            "enhanced_context": legacy_context or {},
            "analysis_context_pack_summary": analysis_context_pack_summary,
            "news_content": str(result_data.get("news_summary") or ""),
            "realtime_quote_raw": context.get("quote"),
            "chip_distribution_raw": context.get("chip"),
        }
        saved_id = AnalysisRepository().save(result=result, query_id=query_id, report_type=report_type,
                                             news_content=str(result_data.get("news_summary") or ""), context_snapshot=snapshot)
    except Exception as exc:
        logger.exception("persist stock analysis failed for %s", result_data.get("code", "unknown"))
        raise HTTPException(status_code=500, detail={"code": "stock_persistence_failed", "message": f"persist stock analysis failed: {type(exc).__name__}"}) from exc
    if not isinstance(saved_id, int) or isinstance(saved_id, bool) or saved_id <= 0:
        raise HTTPException(status_code=500, detail={"code": "stock_persistence_failed", "message": "repository did not confirm a saved history record"})
    return {"saved_history_id": saved_id}


def save_market_review(report: str, region: str, query_id: str, overview: dict[str, Any] | None = None) -> dict[str, int]:
    from src.config import get_config
    from src.core.market_review import _persist_market_review_history
    try:
        payload = {
            "version": 1,
            "kind": "market_review",
            "region": region,
            "title": "",
            "sections": [{"key": "full_review", "title": "Review", "markdown": report}],
            "markdown_report": report,
            "market_overview": overview or {},
        }
        saved_id = _persist_market_review_history(review_report=report, markdown_report=report,
                                                  region=region, config=get_config(), query_id=query_id,
                                                  market_review_payload=payload)
    except Exception as exc:
        logger.exception("persist market review failed for %s", region)
        raise HTTPException(status_code=500, detail={"code": "market_persistence_failed", "message": f"persist market review failed: {type(exc).__name__}"}) from exc
    if not isinstance(saved_id, int) or saved_id <= 0:
        raise HTTPException(status_code=500, detail={"code": "market_persistence_failed", "message": "repository did not confirm a saved history record"})
    return {"saved_history_id": saved_id}
