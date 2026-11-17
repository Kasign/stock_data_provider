"""LLM adapters. Prompts, model routing, retry and response parsing stay in Python."""
from __future__ import annotations

import logging
import copy
import threading
from typing import Any

from fastapi import HTTPException

from .common import _jsonable

logger = logging.getLogger(__name__)
_analyzer_cache: dict[str, Any] = {}
_analyzer_cache_lock = threading.Lock()


def _analyzer_for_language(language: str | None):
    from src.analyzer import GeminiAnalyzer, get_analyzer
    from src.config import get_config
    from src.report_language import normalize_report_language

    base_config = get_config()
    normalized = normalize_report_language(language or getattr(base_config, "report_language", "zh"))
    default_language = normalize_report_language(getattr(base_config, "report_language", "zh"))
    if normalized == default_language:
        return get_analyzer()
    with _analyzer_cache_lock:
        if normalized not in _analyzer_cache:
            config_copy = copy.copy(base_config)
            config_copy.report_language = normalized
            _analyzer_cache[normalized] = GeminiAnalyzer(config=config_copy)
        return _analyzer_cache[normalized]


def _apply_legacy_stock_result_postprocessing(
    result: Any,
    context: dict[str, Any],
    *,
    report_type: str,
    query_id: str,
    search_performed: bool,
    news_result_count: int | None,
    news_evidence_present: bool,
) -> None:
    """Retain critical post-LLM safeguards from the old StockAnalysisPipeline.

    This is intentionally a bridge-layer compatibility shim: Go owns workflow
    ordering, while the migration still reuses existing Python result-normalizing
    and decision-stability helpers until equivalent Go golden tests replace them.
    """
    if result is None:
        return
    realtime = context.get("realtime") if isinstance(context.get("realtime"), dict) else {}
    if isinstance(result, dict):
        # Alternate/test backends may return plain mappings instead of the
        # project's AnalysisResult class. Preserve the fields we can safely
        # populate without pretending object-only guardrails ran.
        result["query_id"] = query_id
        result["current_price"] = realtime.get("price")
        result["change_pct"] = realtime.get("change_pct")
        result["search_performed"] = bool(search_performed)
        result["news_result_count"] = news_result_count if search_performed else None
        result["news_result_count_known"] = bool(search_performed)
        result["news_evidence_present"] = bool(news_evidence_present)
        return
    try:
        from src.analyzer import (
            fill_price_position_if_needed,
            normalize_chip_structure_availability,
            populate_decision_action_fields,
            stabilize_decision_with_structure,
        )

        chip = context.get("chip") if isinstance(context.get("chip"), dict) else None
        trend = context.get("trend_analysis") if isinstance(context.get("trend_analysis"), dict) else context.get("trend")
        fundamentals = context.get("fundamental_context") if isinstance(context.get("fundamental_context"), dict) else None

        # Match the old pipeline's explicit price snapshot assignment. Do not
        # synthesize a real-time price from an unrelated later provider call.
        if hasattr(result, "current_price"):
            result.current_price = realtime.get("price")
        if hasattr(result, "change_pct"):
            result.change_pct = realtime.get("change_pct")
        if hasattr(result, "query_id"):
            result.query_id = query_id
        if hasattr(result, "search_performed"):
            result.search_performed = bool(search_performed)
        if hasattr(result, "news_result_count"):
            result.news_result_count = news_result_count if search_performed else None
        if hasattr(result, "news_result_count_known"):
            result.news_result_count_known = bool(search_performed)
        if hasattr(result, "news_evidence_present"):
            result.news_evidence_present = bool(news_evidence_present)
        if fundamentals is not None and hasattr(result, "fundamental_context"):
            result.fundamental_context = fundamentals

        # Mirror the non-LLM repair/stability steps used by the legacy pipeline.
        # Each helper is defensive internally; the outer caller also logs failures
        # without discarding an otherwise usable model response.
        normalize_chip_structure_availability(result, chip)
        fill_price_position_if_needed(result, trend, realtime)
        stabilize_decision_with_structure(result, trend, fundamentals)
        populate_decision_action_fields(result, report_type=report_type)
    except Exception:
        logger.exception("legacy stock-result postprocessing failed for %s (query_id=%s)", context.get("code", "unknown"), query_id)


def analyze_stock(
    context: dict[str, Any],
    news_context: str,
    query_id: str,
    analysis_context_pack_summary: str,
    report_language: str | None = None,
    report_type: str = "detailed",
    search_performed: bool = False,
    news_result_count: int | None = None,
    news_evidence_present: bool = False,
) -> dict[str, Any]:
    try:
        result = _analyzer_for_language(report_language).analyze(
            context=context,
            news_context=news_context or None,
            analysis_context_pack_summary=analysis_context_pack_summary or None,
        )
        _apply_legacy_stock_result_postprocessing(
            result, context, report_type=report_type, query_id=query_id,
            search_performed=search_performed, news_result_count=news_result_count,
            news_evidence_present=news_evidence_present,
        )
        payload = _jsonable(result.to_dict() if hasattr(result, "to_dict") else result)
        if not isinstance(payload, dict):
            raise TypeError("analyzer result must serialize to an object")
        payload["query_id"] = query_id
        # Preserve evidence metadata even when a non-dataclass analyzer backend
        # returns a plain mapping rather than the legacy AnalysisResult object.
        payload["search_performed"] = bool(search_performed)
        payload["news_result_count"] = news_result_count if search_performed else None
        payload["news_result_count_known"] = bool(search_performed)
        payload["news_evidence_present"] = bool(news_evidence_present)
        return payload
    except Exception as exc:
        logger.exception("stock LLM analysis failed for %s", context.get("code", "unknown"))
        raise HTTPException(status_code=502, detail={"code": "stock_analysis_failed", "message": f"stock analysis failed: {type(exc).__name__}"}) from exc


def generate_market_review(overview_data: dict[str, Any], news: list[dict[str, Any]], region: str, query_id: str, report_language: str | None = None) -> dict[str, str]:
    try:
        from src.config import get_config
        from src.market_analyzer import MarketAnalyzer, MarketIndex, MarketOverview
        raw = dict(overview_data)
        indices = [MarketIndex(**{k: v for k, v in item.items() if k in MarketIndex.__dataclass_fields__}) for item in raw.get("indices", []) if isinstance(item, dict)]
        kwargs = {k: v for k, v in raw.items() if k in MarketOverview.__dataclass_fields__ and k != "indices"}
        kwargs["indices"] = indices
        overview = MarketOverview(**kwargs)
        normalized_news = [{
            "title": item.get("title", ""),
            "snippet": item.get("snippet") or item.get("summary", ""),
            "source": item.get("source", ""),
            "published_date": item.get("published_date") or item.get("published_at", ""),
            "url": item.get("url", ""),
        } for item in news]
        config = copy.copy(get_config())
        if report_language:
            from src.report_language import normalize_report_language
            config.report_language = normalize_report_language(report_language)
        analyzer = MarketAnalyzer(search_service=None, analyzer=_analyzer_for_language(report_language), region=region, config=config)
        report = analyzer.generate_market_review(overview, normalized_news)
        if not str(report or "").strip():
            raise ValueError("market review generated an empty report")
        return {"report": str(report)}
    except Exception as exc:
        logger.exception("market review generation failed for %s (query_id=%s)", region, query_id)
        raise HTTPException(status_code=502, detail={"code": "market_review_generation_failed", "message": f"market review generation failed: {type(exc).__name__}"}) from exc
