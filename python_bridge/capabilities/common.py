"""Shared protocol conversion helpers for Python capability adapters."""
from __future__ import annotations

import logging
import math
from dataclasses import asdict, is_dataclass
from datetime import date, datetime, timedelta, timezone
from enum import Enum
from typing import Any, Mapping

from fastapi import HTTPException

logger = logging.getLogger(__name__)

def _safe_float(value: Any, default: float | None = None) -> float | None:
    try:
        if isinstance(value, str):
            value = value.strip().replace(",", "")
        result = float(value)
        if math.isnan(result) or math.isinf(result):
            return default
        return result
    except (TypeError, ValueError, OverflowError):
        return default


def _jsonable(value: Any) -> Any:
    """Convert provider objects, pandas values and enums into bounded JSON values."""
    if value is None or isinstance(value, (str, int, bool)):
        return value
    if isinstance(value, float):
        return value if math.isfinite(value) else None
    if isinstance(value, Enum):
        return value.value
    if isinstance(value, (datetime, date)):
        return value.isoformat()
    if hasattr(value, "to_pydatetime"):
        try:
            return value.to_pydatetime().isoformat()
        except Exception:
            pass
    if is_dataclass(value):
        return _jsonable(asdict(value))
    if hasattr(value, "model_dump"):
        try:
            return _jsonable(value.model_dump())
        except Exception:
            pass
    if hasattr(value, "to_dict") and callable(value.to_dict):
        try:
            return _jsonable(value.to_dict())
        except Exception:
            pass
    if isinstance(value, Mapping):
        return {str(k): _jsonable(v) for k, v in value.items()}
    if isinstance(value, (list, tuple, set)):
        return [_jsonable(v) for v in value]
    # Numpy scalar values commonly expose item(); avoid importing numpy globally.
    item = getattr(value, "item", None)
    if callable(item):
        try:
            return _jsonable(item())
        except Exception:
            pass
    if hasattr(value, "__dict__"):
        return _jsonable(vars(value))
    return str(value)


def _bar_records(frame: Any) -> list[dict[str, Any]]:
    if frame is None or getattr(frame, "empty", True):
        return []
    try:
        rows = frame.to_dict(orient="records")
    except Exception as exc:
        raise HTTPException(status_code=502, detail={"code": "invalid_daily_bars", "message": "data provider did not return a tabular daily history"}) from exc
    bars: list[dict[str, Any]] = []
    for row in rows:
        normalized = {str(key).lower(): value for key, value in dict(row).items()}
        raw_date = normalized.get("date", normalized.get("日期", normalized.get("trade_date", "")))
        if isinstance(raw_date, datetime):
            date_text = raw_date.date().isoformat()
        elif isinstance(raw_date, date):
            date_text = raw_date.isoformat()
        elif hasattr(raw_date, "to_pydatetime"):
            converted = raw_date.to_pydatetime()
            date_text = converted.date().isoformat() if isinstance(converted, datetime) else str(converted)[:10]
        elif hasattr(raw_date, "isoformat"):
            date_text = str(raw_date.isoformat())[:10]
        else:
            date_text = str(raw_date or "")[:10]
        bar = {"date": date_text}
        missing_fields: list[str] = []
        for key, aliases in {
            "open": ("open", "开盘"), "high": ("high", "最高"), "low": ("low", "最低"),
            "close": ("close", "收盘"), "volume": ("volume", "成交量"),
            "amount": ("amount", "成交额"), "pct_chg": ("pct_chg", "涨跌幅", "change_pct"),
        }.items():
            raw = next((normalized.get(alias) for alias in aliases if normalized.get(alias) is not None), None)
            value = _safe_float(raw)
            if value is None:
                # Preserve missing numeric fields explicitly. Go uses this marker
                # to emit null to the legacy LLM context and to store SQL NULL,
                # instead of silently converting unavailable values to a real zero.
                if key in {"volume", "amount", "pct_chg"}:
                    missing_fields.append(key)
                value = 0.0
            bar[key] = value
        # A zero/missing OHLC value is not a usable daily price for an equity bar.
        # Drop the malformed bar instead of presenting a fabricated price to the LLM.
        if bar["date"] and all(bar[field] > 0 for field in ("open", "high", "low", "close")):
            if missing_fields:
                bar["missing_fields"] = missing_fields
            bars.append(bar)
    bars.sort(key=lambda item: item["date"])
    return bars


def _quote_payload(quote: Any, stock_code: str, stock_name: str) -> dict[str, Any] | None:
    if quote is None:
        return None
    raw = _jsonable(quote)
    if not isinstance(raw, dict):
        return None
    # UnifiedRealtimeQuote already promises shares for volume. Amount unit is
    # currency-sensitive, so only label it CNY when the source says CNY or the
    # A-share code implies mainland China and no conflicting currency is given.
    aliases = {
        "change": ("change", "change_amount", "change_amt"),
        "change_pct": ("change_pct", "pct_chg", "change_percent"),
        "open": ("open", "open_price"),
        "prev_close": ("prev_close", "pre_close", "previous_close"),
        "turnover_rate": ("turnover_rate", "turnover"),
    }
    out: dict[str, Any] = {"code": str(raw.get("code") or stock_code), "name": str(raw.get("name") or stock_name)}
    for field in ("price", "change", "change_pct", "open", "high", "low", "prev_close", "volume", "amount", "volume_ratio", "turnover_rate"):
        candidates = aliases.get(field, (field,))
        value = next((raw.get(candidate) for candidate in candidates if raw.get(candidate) is not None), None)
        out[field] = _safe_float(value)

    source = raw.get("source")
    out["source"] = source.get("value") if isinstance(source, dict) else str(source or "")
    provider_timestamp = raw.get("provider_timestamp")
    # Legacy aliases are used only if they are themselves supplied by the
    # provider. fetched_at is deliberately never used as quote/event time.
    if provider_timestamp in (None, ""):
        provider_timestamp = raw.get("quote_time") or raw.get("timestamp") or raw.get("time")
    provider_timestamp_text = str(provider_timestamp or "").strip()
    out["provider_timestamp"] = provider_timestamp_text or None
    out["quote_time"] = provider_timestamp_text or None
    out["timestamp"] = provider_timestamp_text
    out["fetched_at"] = raw.get("fetched_at")
    out["is_stale"] = raw.get("is_stale")
    out["stale_seconds"] = _safe_float(raw.get("stale_seconds"))
    out["fallback_from"] = raw.get("fallback_from")
    out["data_quality"] = str(raw.get("data_quality") or "unknown").lower()
    missing = raw.get("missing_fields")
    out["missing_fields"] = [str(item) for item in missing if str(item).strip()] if isinstance(missing, list) else []
    out["volume_unit"] = "shares"  # UnifiedRealtimeQuote contract: volume is normalized to shares.
    currency = str(raw.get("currency") or "").strip().upper()
    code = str(raw.get("code") or stock_code).strip().upper()
    code_only = code.split(".", 1)[0]
    is_b_share = code_only.isdigit() and len(code_only) == 6 and code_only.startswith(("200", "900"))
    a_share_code = code_only.isdigit() and len(code_only) == 6 and not is_b_share
    # This bridge's supported normalized contract is CNY A-shares. Do not let
    # an inconsistent legacy provider label silently force a B-share into CNY.
    if is_b_share:
        out["amount_unit"] = "unknown"
    else:
        out["amount_unit"] = "CNY" if currency == "CNY" or (not currency and a_share_code) else "unknown"
    out["adjustment_basis"] = "none"  # Live trade/quote prices are not adjusted historical prices.

    reasons: list[str] = []
    timestamp_is_valid = False
    timestamp_is_future = False
    if provider_timestamp_text:
        try:
            parsed_timestamp = datetime.fromisoformat(provider_timestamp_text.replace("Z", "+00:00"))
            if parsed_timestamp.tzinfo is None:
                reasons.append("provider_timestamp_timezone_missing")
            else:
                timestamp_is_valid = True
                timestamp_is_future = parsed_timestamp.astimezone(timezone.utc) > datetime.now(timezone.utc).replace(microsecond=0) + timedelta(seconds=60)
                if timestamp_is_future:
                    reasons.append("provider_timestamp_more_than_60_seconds_in_future")
        except ValueError:
            reasons.append("provider_timestamp_invalid")
    else:
        reasons.append("provider_timestamp_missing")
    is_stale = raw.get("is_stale")
    data_quality = out["data_quality"]
    if is_stale is True:
        reasons.append("provider_quote_marked_stale")
    elif is_stale is not False:
        reasons.append("provider_staleness_unknown")
    if data_quality == "partial":
        reasons.append("provider_data_partial")
    elif data_quality == "unavailable":
        reasons.append("provider_data_unavailable")
    if not out["source"].strip():
        reasons.append("provider_source_missing")
    for field in out["missing_fields"]:
        reasons.append("provider_missing_" + field)
    out["quality_reasons"] = reasons

    # The historical bridge often returned only prices without source-time
    # metadata. Keep those values for diagnostics but never imply they are valid
    # real-time data. A quote is valid only when provider time and freshness are
    # both explicitly known and the source reports no missing fields.
    if is_stale is True:
        quality_status = "stale"
    elif not out["source"].strip() or not timestamp_is_valid or timestamp_is_future or is_stale is not False or data_quality == "unavailable":
        quality_status = "unknown"
    elif data_quality == "partial" or out["missing_fields"]:
        quality_status = "degraded"
    else:
        quality_status = "valid"
    out["quality_status"] = quality_status
    return out


def _news_items(response: Any) -> list[dict[str, Any]]:
    """Normalize either a SearchResponse-like object or an already-unwrapped list.

    The legacy market-news method returns a list of result items, while stock
    search returns one SearchResponse per dimension. Supporting both shapes is
    required so market news is not silently dropped at the bridge boundary.
    """
    raw_results = getattr(response, "results", None)
    if raw_results is None and isinstance(response, Mapping):
        raw_results = response.get("results", response.get("items", []))
    if raw_results is None and isinstance(response, (list, tuple)):
        raw_results = response
    items: list[dict[str, Any]] = []
    for raw in list(raw_results or []):
        item = _jsonable(raw)
        if not isinstance(item, dict):
            continue
        title = str(item.get("title") or item.get("name") or "").strip()
        summary = str(item.get("summary") or item.get("content") or item.get("snippet") or "").strip()
        if not title and not summary:
            continue
        items.append({
            "title": title, "summary": summary,
            "url": str(item.get("url") or item.get("link") or ""),
            "source": str(item.get("source") or item.get("provider") or ""),
            "published_at": str(item.get("published_at") or item.get("published_date") or item.get("publish_date") or item.get("publish_time") or item.get("date") or ""),
            "relevance": _safe_float(item.get("relevance") or item.get("score"), 0.0) or 0.0,
        })
    return items


def _market_overview_payload(overview: Any, region: str) -> dict[str, Any]:
    data = _jsonable(overview)
    if not isinstance(data, dict):
        raise HTTPException(status_code=502, detail={"code": "invalid_market_overview", "message": "MarketAnalyzer returned an unsupported overview object"})
    data["region"] = region
    data.setdefault("indices", [])
    return data


def _analysis_result_model(result_data: dict[str, Any]):
    from src.analyzer import AnalysisResult
    import dataclasses
    if dataclasses.is_dataclass(AnalysisResult):
        allowed = {field.name for field in dataclasses.fields(AnalysisResult)}
    else:
        allowed = set(getattr(AnalysisResult, "__annotations__", {}))
    payload = {key: value for key, value in result_data.items() if key in allowed}
    if "code" not in payload or not payload["code"]:
        raise HTTPException(status_code=422, detail={"code": "invalid_analysis_result", "message": "analysis result must include code"})
    try:
        return AnalysisResult(**payload)
    except TypeError as exc:
        raise HTTPException(status_code=422, detail={"code": "invalid_analysis_result", "message": "analysis result fields are incompatible with the installed project version"}) from exc


