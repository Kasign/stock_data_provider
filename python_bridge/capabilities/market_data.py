"""Market-data capability adapters; these wrap existing provider APIs only."""
from __future__ import annotations

import logging
from datetime import datetime
from zoneinfo import ZoneInfo
from typing import Any

from fastapi import HTTPException

from .common import _bar_records, _jsonable, _market_overview_payload, _quote_payload

logger = logging.getLogger(__name__)


def _market_timezone(stock_code: str) -> str:
    code = str(stock_code or "").strip().upper()
    if code.startswith("HK") or code.endswith(".HK"):
        return "Asia/Hong_Kong"
    if code.endswith(".TWO") or code.endswith(".TW"):
        return "Asia/Taipei"
    if code.endswith(".KS") or code.endswith(".KQ"):
        return "Asia/Seoul"
    if code.endswith(".T"):
        return "Asia/Tokyo"
    if code.endswith((".SH", ".SZ", ".BJ", ".CN")) or code.startswith(("SH", "SZ", "BJ")) or code.isdigit():
        return "Asia/Shanghai"
    # Bare alphabetic tickers and explicitly tagged US symbols default to New York.
    if code.endswith(".US") or any(char.isalpha() for char in code):
        return "America/New_York"
    return "Asia/Shanghai"



def _annotate_legacy_bars(bars: list[dict[str, Any]], source: str) -> list[dict[str, Any]]:
    annotated: list[dict[str, Any]] = []
    for original in bars:
        bar = dict(original)
        bar.update({
            "source": source or "unknown",
            "adjustment_basis": "unknown",
            "volume_unit": "unknown",
            "amount_unit": "unknown",
            "raw_volume": bar.get("volume"),
            "raw_volume_unit": "unknown",
            "raw_amount": bar.get("amount"),
            "raw_amount_unit": "unknown",
            "quality_status": "unknown",
            "quality_reasons": ["legacy_provider_unit_contract_unverified", "adjustment_basis_unverified"],
            "normalization_version": "legacy-unverified",
        })
        missing = list(bar.get("missing_fields") or [])
        for field in ("volume", "amount"):
            if field not in missing:
                missing.append(field)
        bar["missing_fields"] = missing
        annotated.append(bar)
    return annotated

def fetch_stock_bundle(stock_code: str, lookback_days: int, as_of: datetime | None = None) -> dict[str, Any]:
    try:
        from data_provider import DataFetcherManager
    except Exception as exc:
        raise HTTPException(status_code=503, detail={"code": "python_capability_unavailable", "message": f"provider import failed: {type(exc).__name__}"}) from exc

    code = stock_code.strip().upper()
    manager = DataFetcherManager()
    warnings: list[str] = []
    try:
        name = manager.get_stock_name(code, allow_realtime=False) or code
    except Exception as exc:
        logger.warning("stock name lookup failed for %s: %s", code, type(exc).__name__)
        name = code
        warnings.append("stock_name_lookup_failed")
    daily_source = "unknown"
    market_tz = ZoneInfo(_market_timezone(code))
    now_market = datetime.now(market_tz)
    date_anchor = as_of.astimezone(market_tz) if as_of and as_of.tzinfo is not None else (as_of.replace(tzinfo=market_tz) if as_of else now_market)
    realtime_allowed = abs((now_market - date_anchor).total_seconds()) <= 90
    try:
        end_date = date_anchor.date().isoformat() if as_of else None
        # DataFetcherManager.get_daily_data returns (DataFrame, source_name), unlike
        # the individual BaseFetcher implementations. Keep that distinction explicit.
        frame, daily_source = manager.get_daily_data(code, end_date=end_date, days=lookback_days)
        bars = _bar_records(frame)
        # The legacy manager's returned frame does not carry dependable per-row
        # unit/adjustment metadata. Its configured providers have different volume
        # conventions (and some source-specific exceptions), so never guess that
        # raw volume is shares or hands, or that every history is QFQ. Preserve the
        # observed source and raw values, but mark volume/amount as unusable until
        # this provider's contract is explicitly verified.
        bars = _annotate_legacy_bars(bars, str(daily_source or "unknown"))
        # This capability only acquires and normalizes data. The Go workflow
        # invokes the dedicated persistence capability for the legacy stock_daily
        # table so data-fetch and database-write responsibilities stay separate.
    except Exception as exc:
        logger.exception("daily history fetch failed for %s", code)
        bars = []
        warnings.append(f"daily_bars_fetch_failed:{type(exc).__name__}")
    # Recheck after history/fundamental fetches: the reference can become stale
    # while a slow provider call is running. Never append a much newer quote to
    # the caller's earlier analysis reference time.
    realtime_allowed = abs((datetime.now(market_tz) - date_anchor).total_seconds()) <= 90
    if realtime_allowed:
        try:
            quote_payload = _quote_payload(manager.get_realtime_quote(code, log_final_failure=False), code, name)
        except Exception as exc:
            logger.warning("realtime quote fetch failed for %s: %s", code, type(exc).__name__)
            quote_payload = None
            warnings.append(f"realtime_quote_fetch_failed:{type(exc).__name__}")
    else:
        quote_payload = None
        warnings.append("realtime_quote_skipped_as_of_not_live; latest quote must not leak into a historical analysis")
    try:
        raw_chip = _jsonable(manager.get_chip_distribution(code))
        chip = {
            "profit_ratio": _jsonable(raw_chip.get("profit_ratio")),
            "concentration_90": _jsonable(raw_chip.get("concentration_90")),
            "concentration_70": _jsonable(raw_chip.get("concentration_70")),
            "cost_average": _jsonable(raw_chip.get("avg_cost", raw_chip.get("cost_average"))),
            "raw": raw_chip,
        } if isinstance(raw_chip, dict) else None
    except Exception as exc:
        logger.warning("chip distribution fetch failed for %s: %s", code, type(exc).__name__)
        chip = None
        warnings.append(f"chip_fetch_failed:{type(exc).__name__}")
    try:
        fundamentals = _jsonable(manager.get_fundamental_context(code))
    except Exception as exc:
        logger.warning("fundamental context fetch failed for %s: %s", code, type(exc).__name__)
        fundamentals = None
        warnings.append(f"fundamentals_fetch_failed:{type(exc).__name__}")
    try:
        boards = _jsonable(manager.get_belong_boards(code)) or []
    except Exception as exc:
        logger.debug("board lookup failed for %s: %s", code, type(exc).__name__)
        boards = []
        warnings.append(f"board_lookup_failed:{type(exc).__name__}")
    if not bars:
        warnings.append("daily_bars_missing")
    if quote_payload is None:
        warnings.append("realtime_quote_missing")
    sources = {"daily_bars": str(daily_source or "unknown"), "realtime_quote": str((quote_payload or {}).get("source") or "unknown")}
    try:
        manager.close()
    except Exception as exc:
        logger.debug("provider manager close skipped: %s", type(exc).__name__)
    return {"code": code, "name": name, "quote": quote_payload, "daily_bars": bars,
            "chip": chip, "fundamentals": fundamentals, "boards": boards,
            "sources": sources, "warnings": warnings, "fetched_at": datetime.now().astimezone().isoformat()}



def fetch_stock_auxiliary(stock_code: str, as_of: datetime | None = None) -> dict[str, Any]:
    """Fetch optional chip/fundamental/board enrichments only.

    Realtime quotes and daily K-lines are owned by the independent Go market-data
    service in the migration topology. Do not fetch them a second time here.
    These auxiliary legacy APIs currently do not reliably expose their leaf
    Provider, so sources remain explicitly unknown rather than being fabricated.
    """
    code = stock_code.strip().upper()
    if as_of is not None:
        market_tz = ZoneInfo(_market_timezone(code))
        now_market = datetime.now(market_tz)
        if as_of.tzinfo is None:
            date_anchor = as_of.replace(tzinfo=market_tz)
        else:
            date_anchor = as_of.astimezone(market_tz)
        if abs((now_market - date_anchor).total_seconds()) > 90:
            # The legacy manager only returns latest chip/fundamental/board snapshots.
            # Never inject today's snapshot into a historical point-in-time analysis.
            return {
                "code": code,
                "name": code,
                "chip": None,
                "fundamentals": None,
                "boards": [],
                "sources": {"chip_distribution": "not_fetched_historical", "fundamentals": "not_fetched_historical", "boards": "not_fetched_historical"},
                "warnings": ["auxiliary_data_skipped_for_historical_as_of_point_in_time_unavailable"],
                "fetched_at": datetime.now().astimezone().isoformat(),
            }
    try:
        from data_provider import DataFetcherManager
    except Exception as exc:
        raise HTTPException(status_code=503, detail={"code": "python_capability_unavailable", "message": f"provider import failed: {type(exc).__name__}"}) from exc

    manager = DataFetcherManager()
    warnings: list[str] = []
    try:
        try:
            name = manager.get_stock_name(code, allow_realtime=False) or code
        except Exception as exc:
            name = code
            warnings.append(f"stock_name_lookup_failed:{type(exc).__name__}")
        try:
            raw_chip = _jsonable(manager.get_chip_distribution(code))
            chip = {
                "profit_ratio": _jsonable(raw_chip.get("profit_ratio")),
                "concentration_90": _jsonable(raw_chip.get("concentration_90")),
                "concentration_70": _jsonable(raw_chip.get("concentration_70")),
                "cost_average": _jsonable(raw_chip.get("avg_cost", raw_chip.get("cost_average"))),
                "raw": raw_chip,
            } if isinstance(raw_chip, dict) else None
        except Exception as exc:
            chip = None
            warnings.append(f"chip_fetch_failed:{type(exc).__name__}")
        try:
            fundamentals = _jsonable(manager.get_fundamental_context(code))
        except Exception as exc:
            fundamentals = None
            warnings.append(f"fundamentals_fetch_failed:{type(exc).__name__}")
        try:
            boards = _jsonable(manager.get_belong_boards(code)) or []
        except Exception as exc:
            boards = []
            warnings.append(f"board_lookup_failed:{type(exc).__name__}")
        return {
            "code": code,
            "name": name,
            "chip": chip,
            "fundamentals": fundamentals,
            "boards": boards,
            "sources": {"chip_distribution": "unknown", "fundamentals": "unknown", "boards": "unknown"},
            "warnings": warnings,
            "fetched_at": datetime.now().astimezone().isoformat(),
        }
    finally:
        try:
            manager.close()
        except Exception as exc:
            logger.debug("provider manager close skipped: %s", type(exc).__name__)

def fetch_market_overview(region: str, as_of: datetime | None = None) -> dict[str, Any]:
    # The current legacy MarketAnalyzer only fetches a live/latest snapshot. Do
    # not rewrite overview.date to as_of: that would mislabel current data as a
    # historical snapshot. Go rejects historical as_of market-review requests.
    analyzer = None
    try:
        from src.config import get_config
        from src.market_analyzer import MarketAnalyzer
        analyzer = MarketAnalyzer(search_service=None, analyzer=None, region=region, config=get_config())
        overview = analyzer.get_market_overview()
        return _market_overview_payload(overview, region)
    except Exception as exc:
        logger.exception("market overview fetch failed for %s (as_of=%s)", region, as_of.isoformat() if as_of else "latest")
        raise HTTPException(status_code=502, detail={"code": "market_overview_failed", "message": f"market overview fetch failed: {type(exc).__name__}"}) from exc
    finally:
        manager = getattr(analyzer, "data_manager", None) if analyzer is not None else None
        close = getattr(manager, "close", None)
        if callable(close):
            try:
                close()
            except Exception as exc:
                logger.debug("market overview provider cleanup skipped: %s", type(exc).__name__)
