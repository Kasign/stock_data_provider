from __future__ import annotations

import asyncio
from datetime import date, datetime, timezone

from fastapi.testclient import TestClient

from python_bridge.capability_app import app, require_bridge_token
from python_bridge.capabilities.common import _bar_records, _news_items, _quote_payload, _safe_float
from python_bridge.capabilities.market_data import _annotate_legacy_bars


class FakeFrame:
    empty = False

    def to_dict(self, *, orient: str):
        assert orient == "records"
        return [
            {"date": datetime(2026, 10, 19, 15, 0), "open": "10.1", "high": 11, "low": 10, "close": 10.5, "volume": 1000, "amount": float("nan"), "pct_chg": 1.2},
            {"date": "2026-10-20", "open": 10.5, "high": 12, "low": 10.4, "close": 11.0, "volume": "1,000", "amount": 20000, "pct_chg": 4.76},
        ]


def test_safe_float_rejects_non_finite_values():
    assert _safe_float("12.4") == 12.4
    assert _safe_float(float("nan"), 0.0) == 0.0
    assert _safe_float("not-a-number") is None


def test_bar_records_normalizes_and_sorts_rows():
    rows = _bar_records(FakeFrame())
    assert [row["date"] for row in rows] == ["2026-10-19", "2026-10-20"]
    assert rows[0]["close"] == 10.5
    assert rows[0]["amount"] == 0.0
    assert rows[0]["missing_fields"] == ["amount"]
    assert rows[1]["volume"] == 1000.0



def test_quote_payload_preserves_provider_time_and_requires_explicit_freshness():
    quote = {
        "code": "600519", "name": "贵州茅台", "source": "efinance",
        "price": 1313.31, "change_amount": -18.57, "change_pct": -1.39,
        "open_price": 1331.88, "pre_close": 1331.88, "volume": 123456,
        "amount": 1234567890, "currency": "CNY", "market": "cn",
        "provider_timestamp": "2026-10-21T11:28:02+08:00",
        "fetched_at": "2026-10-21T03:28:03Z", "is_stale": False,
        "stale_seconds": 1, "data_quality": "ok", "missing_fields": [],
        "fallback_from": "akshare_em",
    }
    payload = _quote_payload(quote, "600519", "贵州茅台")
    assert payload is not None
    assert payload["quote_time"] == "2026-10-21T11:28:02+08:00"
    assert payload["timestamp"] == quote["provider_timestamp"]
    assert payload["fetched_at"] == quote["fetched_at"]
    assert payload["quality_status"] == "valid"
    assert payload["source"] == "efinance"
    assert payload["volume_unit"] == "shares"
    assert payload["amount_unit"] == "CNY"
    assert payload["adjustment_basis"] == "none"
    assert payload["change"] == -18.57
    assert payload["open"] == 1331.88
    assert payload["fallback_from"] == "akshare_em"


def test_quote_payload_never_uses_fetch_time_as_provider_time():
    payload = _quote_payload({
        "code": "600519", "source": "legacy", "price": 1313,
        "fetched_at": "2026-10-21T03:28:03Z", "is_stale": None,
    }, "600519", "贵州茅台")
    assert payload is not None
    assert payload["fetched_at"] == "2026-10-21T03:28:03Z"
    assert payload["quote_time"] is None
    assert payload["timestamp"] == ""
    assert payload["quality_status"] == "unknown"
    assert "provider_timestamp_missing" in payload["quality_reasons"]
    assert "provider_staleness_unknown" in payload["quality_reasons"]


def test_quote_payload_rejects_stale_and_unknown_currency_metadata():
    stale = _quote_payload({
        "code": "600519", "source": "efinance", "price": 1313,
        "provider_timestamp": "2026-10-21T11:00:00+08:00", "is_stale": True,
        "data_quality": "ok", "currency": "CNY",
    }, "600519", "贵州茅台")
    assert stale is not None and stale["quality_status"] == "stale"
    overseas = _quote_payload({
        "code": "00700.HK", "source": "legacy", "price": 500,
        "provider_timestamp": "2026-10-21T11:00:00+08:00", "is_stale": False,
        "data_quality": "ok", "currency": "HKD",
    }, "00700.HK", "Tencent")
    assert overseas is not None and overseas["amount_unit"] == "unknown"

    # A-share inference must not label B-share symbols as CNY. If the provider
    # omits currency, the bridge cannot infer the correct currency contract.
    b_share = _quote_payload({
        "code": "900901", "source": "legacy", "price": 0.8, "amount": 12345,
        "provider_timestamp": "2026-10-21T11:00:00+08:00", "is_stale": False,
        "data_quality": "ok",
    }, "900901", "B股示例")
    assert b_share is not None and b_share["amount_unit"] == "unknown"
    # Even if a legacy provider incorrectly asserts CNY, B-shares are outside
    # this bridge's A-share monetary contract and must not be relabeled.
    b_share_with_bad_currency = _quote_payload({
        "code": "900901", "source": "legacy", "price": 0.8, "amount": 12345,
        "currency": "CNY", "provider_timestamp": "2026-10-21T11:00:00+08:00",
        "is_stale": False, "data_quality": "ok",
    }, "900901", "B股示例")
    assert b_share_with_bad_currency is not None and b_share_with_bad_currency["amount_unit"] == "unknown"

    from datetime import timedelta
    future = (datetime.now(timezone.utc) + timedelta(minutes=5)).isoformat()
    future_quote = _quote_payload({
        "code": "600519", "source": "efinance", "price": 1313,
        "provider_timestamp": future, "is_stale": False, "data_quality": "ok", "currency": "CNY",
    }, "600519", "贵州茅台")
    assert future_quote is not None and future_quote["quality_status"] == "unknown"
    assert "provider_timestamp_more_than_60_seconds_in_future" in future_quote["quality_reasons"]


def test_legacy_bars_mark_volume_amount_and_adjustment_unknown():
    rows = _annotate_legacy_bars([{
        "date": "2026-10-20", "open": 10, "high": 12, "low": 9,
        "close": 11, "volume": 100, "amount": 1100, "pct_chg": 1.0,
    }], "FixtureFetcher")
    assert len(rows) == 1
    bar = rows[0]
    assert bar["source"] == "FixtureFetcher"
    assert bar["volume_unit"] == "unknown"
    assert bar["amount_unit"] == "unknown"
    assert bar["adjustment_basis"] == "unknown"
    assert bar["raw_volume"] == 100
    assert bar["raw_amount"] == 1100
    assert set(bar["missing_fields"]) >= {"volume", "amount"}
    assert bar["quality_status"] == "unknown"

def test_news_items_handles_provider_objects():
    class Item:
        def __init__(self):
            self.title = "公告标题"
            self.summary = "公告摘要"
            self.url = "https://example.test/news"
            self.source = "Example"
            self.published_at = "2026-10-20"

    class Response:
        results = [Item()]

    # _jsonable converts arbitrary objects to their __dict__ representation.
    rows = _news_items(Response())
    assert len(rows) == 1
    assert rows[0]["title"] == "公告标题"
    assert rows[0]["url"] == "https://example.test/news"


def test_news_items_accepts_unwrapped_market_news_list():
    rows = _news_items([
        {"title": "大盘资金面变化", "summary": "市场摘要", "url": "https://example.test/market", "source": "Fixture"}
    ])
    assert len(rows) == 1
    assert rows[0]["title"] == "大盘资金面变化"
    assert rows[0]["summary"] == "市场摘要"


def test_bridge_auth_fails_closed_without_configured_secret(monkeypatch):
    monkeypatch.delenv("PYTHON_BRIDGE_TOKEN", raising=False)
    try:
        asyncio.run(require_bridge_token("any-token"))
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 503
    else:
        raise AssertionError("expected missing server secret to reject bridge calls")


def test_bridge_auth_rejects_wrong_secret(monkeypatch):
    monkeypatch.setenv("PYTHON_BRIDGE_TOKEN", "expected-random-secret-at-least-32-characters")
    try:
        asyncio.run(require_bridge_token("wrong-secret"))
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 401
    else:
        raise AssertionError("expected invalid bridge token to be rejected")


def test_bridge_auth_accepts_matching_secret(monkeypatch):
    monkeypatch.setenv("PYTHON_BRIDGE_TOKEN", "expected-random-secret-at-least-32-characters")
    assert asyncio.run(require_bridge_token("expected-random-secret-at-least-32-characters")) is None


def test_health_does_not_require_bridge_token(monkeypatch):
    monkeypatch.delenv("PYTHON_BRIDGE_TOKEN", raising=False)
    response = TestClient(app).get("/healthz")
    assert response.status_code == 200
    assert response.json()["service"] == "dsa-python-bridge"


def test_internal_endpoint_fails_closed_before_importing_project_modules(monkeypatch):
    monkeypatch.delenv("PYTHON_BRIDGE_TOKEN", raising=False)
    response = TestClient(app).post("/internal/v1/market-data/market-overview", json={"region": "cn"}, headers={"X-Python-Bridge-Token": "anything"})
    assert response.status_code == 503
    assert response.json()["detail"]["code"] == "bridge_auth_not_configured"


def test_internal_endpoint_rejects_wrong_token(monkeypatch):
    monkeypatch.setenv("PYTHON_BRIDGE_TOKEN", "correct-random-token-at-least-32-characters")
    response = TestClient(app).post("/internal/v1/market-data/market-overview", json={"region": "cn"}, headers={"X-Python-Bridge-Token": "wrong-token"})
    assert response.status_code == 401
    assert response.json()["detail"]["code"] == "unauthorized"


def test_bar_records_drops_rows_with_invalid_ohlc():
    class InvalidFrame:
        empty = False

        def to_dict(self, *, orient: str):
            assert orient == "records"
            return [
                {"date": "2026-10-19", "open": 0, "high": 0, "low": 0, "close": 0, "volume": 100},
                {"date": "2026-10-20", "open": 10, "high": 12, "low": 9, "close": 11, "volume": 100},
            ]

    rows = _bar_records(InvalidFrame())
    assert len(rows) == 1
    assert rows[0]["date"] == "2026-10-20"


def test_fetch_stock_bundle_unpacks_provider_tuple_without_database_side_effect(monkeypatch):
    import sys
    import types

    from python_bridge.capabilities.market_data import fetch_stock_bundle

    class Frame:
        empty = False

        def to_dict(self, *, orient: str):
            assert orient == "records"
            return [{"date": "2026-10-19", "open": 10, "high": 12, "low": 9, "close": 11, "volume": 100, "amount": 1100, "pct_chg": 1.2}]

    calls: dict[str, object] = {}

    class FakeManager:
        def get_stock_name(self, code, allow_realtime=False):
            return "测试股票"

        def get_daily_data(self, code, start_date=None, end_date=None, days=30):
            calls["daily_request"] = (code, end_date, days)
            return Frame(), "FixtureFetcher"

        def get_realtime_quote(self, code, log_final_failure=False):
            return {"code": code, "name": "测试股票", "price": 11, "source": "fixture"}

        def get_chip_distribution(self, code):
            return {"code": code, "profit_ratio": 0.6, "avg_cost": 10, "concentration_90": 0.1}

        def get_fundamental_context(self, code):
            return {"status": "available"}

        def get_belong_boards(self, code):
            return []

        def close(self):
            calls["closed"] = True

    provider_module = types.ModuleType("data_provider")
    provider_module.DataFetcherManager = FakeManager
    src_module = types.ModuleType("src")
    src_module.__path__ = []
    monkeypatch.setitem(sys.modules, "data_provider", provider_module)
    monkeypatch.setitem(sys.modules, "src", src_module)

    payload = fetch_stock_bundle("600396", 60, datetime(2026, 10, 19, 18, 30, tzinfo=timezone.utc))

    assert payload["daily_bars"] and payload["daily_bars"][0]["close"] == 11.0
    assert payload["daily_bars"][0]["volume_unit"] == "unknown"
    assert payload["daily_bars"][0]["adjustment_basis"] == "unknown"
    assert set(payload["daily_bars"][0]["missing_fields"]) >= {"volume", "amount"}
    assert payload["sources"]["daily_bars"] == "FixtureFetcher"
    assert calls["daily_request"] == ("600396", "2026-10-20", 60)
    assert "saved" not in calls
    assert calls["closed"] is True


def test_stock_persistence_writes_legacy_context_snapshot(monkeypatch):
    import dataclasses
    import sys
    import types

    from python_bridge.capabilities.persistence import save_stock_analysis

    @dataclasses.dataclass
    class FakeAnalysisResult:
        code: str
        query_id: str = ""
        analysis_summary: str = ""
        news_summary: str = ""

    saved: dict[str, object] = {}

    class FakeAnalysisRepository:
        def save(self, **kwargs):
            saved.update(kwargs)
            return 42

    src_module = types.ModuleType("src")
    src_module.__path__ = []
    analyzer_module = types.ModuleType("src.analyzer")
    analyzer_module.AnalysisResult = FakeAnalysisResult
    repositories_module = types.ModuleType("src.repositories")
    repositories_module.__path__ = []
    repo_module = types.ModuleType("src.repositories.analysis_repo")
    repo_module.AnalysisRepository = FakeAnalysisRepository
    monkeypatch.setitem(sys.modules, "src", src_module)
    monkeypatch.setitem(sys.modules, "src.analyzer", analyzer_module)
    monkeypatch.setitem(sys.modules, "src.repositories", repositories_module)
    monkeypatch.setitem(sys.modules, "src.repositories.analysis_repo", repo_module)

    result = save_stock_analysis(
        {"code": "600396", "analysis_summary": "fixture"},
        {"code": "600396", "data_quality": {"score": 90}},
        "detailed",
        "query-1",
        {"today": {"close": 11}},
        "quality summary",
    )

    assert result["saved_history_id"] == 42
    snapshot = saved["context_snapshot"]
    assert snapshot["context_version"] == "go-core-v1"
    assert snapshot["enhanced_context"]["today"]["close"] == 11
    assert snapshot["analysis_context_pack_summary"] == "quality summary"
    assert snapshot["go_analysis_context"]["data_quality"]["score"] == 90



def test_daily_bar_persistence_writes_only_through_persistence_adapter(monkeypatch):
    import sys
    import types

    from python_bridge.capabilities.persistence import save_daily_bars

    writes = {}

    class FakeDB:
        def save_daily_data(self, frame, code, source):
            writes["rows"] = frame.to_dict(orient="records")
            writes["code"] = code
            writes["source"] = source
            return 1

    class FakeDatabaseManager:
        @staticmethod
        def get_instance():
            return FakeDB()

    src_module = types.ModuleType("src")
    src_module.__path__ = []
    storage_module = types.ModuleType("src.storage")
    storage_module.DatabaseManager = FakeDatabaseManager
    monkeypatch.setitem(sys.modules, "src", src_module)
    monkeypatch.setitem(sys.modules, "src.storage", storage_module)

    result = save_daily_bars("600396", [{"date": "2026-10-20", "open": 10.0, "high": 12.0, "low": 9.0, "close": 11.0, "volume": 100, "amount": 0.0, "pct_chg": 0.0, "missing_fields": ["amount", "pct_chg"]}], "FixtureFetcher")
    assert result == {"saved_count": 1}
    assert writes["code"] == "600396"
    assert writes["source"] == "FixtureFetcher"
    assert writes["rows"][0]["date"] == "2026-10-20"
    assert writes["rows"][0]["amount"] is None
    assert writes["rows"][0]["pct_chg"] is None


def test_fetch_stock_bundle_skips_current_quote_for_historical_as_of(monkeypatch):
    import sys
    import types

    from python_bridge.capabilities.market_data import fetch_stock_bundle

    class Frame:
        empty = False

        def to_dict(self, *, orient: str):
            assert orient == "records"
            return [{"date": "2020-10-19", "open": 10, "high": 12, "low": 9, "close": 11, "volume": 100}]

    calls = {"realtime": 0, "chip": 0, "fundamentals": 0, "boards": 0, "closed": False}

    class FakeManager:
        def get_stock_name(self, code, allow_realtime=False):
            return "fixture"

        def get_daily_data(self, code, start_date=None, end_date=None, days=30):
            assert end_date == "2020-10-20"
            return Frame(), "FixtureFetcher"

        def get_realtime_quote(self, code, log_final_failure=False):
            calls["realtime"] += 1
            return {"code": code, "price": 999}

        def get_chip_distribution(self, code):
            calls["chip"] += 1
            return {"avg_cost": 10}

        def get_fundamental_context(self, code):
            calls["fundamentals"] += 1
            return {"status": "latest"}

        def get_belong_boards(self, code):
            calls["boards"] += 1
            return [{"code": "BK0001"}]

        def close(self):
            calls["closed"] = True

    provider_module = types.ModuleType("data_provider")
    provider_module.DataFetcherManager = FakeManager
    src_module = types.ModuleType("src")
    src_module.__path__ = []
    monkeypatch.setitem(sys.modules, "data_provider", provider_module)
    monkeypatch.setitem(sys.modules, "src", src_module)

    payload = fetch_stock_bundle("600396", 60, datetime(2020, 10, 20, 10, 0, tzinfo=timezone.utc))
    assert payload["quote"] is None
    assert any("realtime_quote_skipped_as_of_not_live" in warning for warning in payload["warnings"])
    assert calls["realtime"] == 0
    assert payload["chip"] is None
    assert payload["fundamentals"] is None
    assert payload["boards"] == []
    assert calls["chip"] == 0
    assert calls["fundamentals"] == 0
    assert calls["boards"] == 0
    assert any("auxiliary_data_skipped_for_historical_as_of_point_in_time_unavailable" in warning for warning in payload["warnings"])
    assert calls["closed"] is True


def test_market_persistence_keeps_market_overview_in_history_payload(monkeypatch):
    import sys
    import types

    from python_bridge.capabilities.persistence import save_market_review

    captured = {}
    config_module = types.ModuleType("src.config")
    config_module.get_config = lambda: object()
    core_module = types.ModuleType("src.core")
    core_module.__path__ = []
    market_review_module = types.ModuleType("src.core.market_review")

    def persist(**kwargs):
        captured.update(kwargs)
        return 73

    market_review_module._persist_market_review_history = persist
    src_module = types.ModuleType("src")
    src_module.__path__ = []
    monkeypatch.setitem(sys.modules, "src", src_module)
    monkeypatch.setitem(sys.modules, "src.config", config_module)
    monkeypatch.setitem(sys.modules, "src.core", core_module)
    monkeypatch.setitem(sys.modules, "src.core.market_review", market_review_module)

    overview = {"indices": [{"code": "000001", "current": 3200}], "up_count": 1234}
    result = save_market_review("report body", "cn", "market-query", overview)
    assert result == {"saved_history_id": 73}
    payload = captured["market_review_payload"]
    assert payload["markdown_report"] == "report body"
    assert payload["market_overview"] == overview


def test_stock_auxiliary_skips_latest_only_data_for_historical_as_of(monkeypatch):
    from datetime import timedelta

    from python_bridge.capabilities.market_data import fetch_stock_auxiliary

    def unexpected_provider_import():
        raise AssertionError("historical requests must not initialize current-only data providers")

    import sys
    import types

    provider_module = types.ModuleType("data_provider")
    provider_module.DataFetcherManager = unexpected_provider_import
    monkeypatch.setitem(sys.modules, "data_provider", provider_module)
    as_of = datetime.now(timezone.utc) - timedelta(days=2)
    payload = fetch_stock_auxiliary("600519.SH", as_of)
    assert payload["chip"] is None
    assert payload["fundamentals"] is None
    assert payload["boards"] == []
    assert "auxiliary_data_skipped_for_historical_as_of_point_in_time_unavailable" in payload["warnings"]
    assert payload["sources"]["fundamentals"] == "not_fetched_historical"


def test_bridge_auth_rejects_template_or_short_configured_secret(monkeypatch):
    monkeypatch.setenv("PYTHON_BRIDGE_TOKEN", "short-secret")
    try:
        asyncio.run(require_bridge_token("short-secret"))
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 503
    else:
        raise AssertionError("weak configured token must fail closed")
