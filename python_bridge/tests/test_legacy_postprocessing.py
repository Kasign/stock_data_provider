from __future__ import annotations

import sys
import types

from python_bridge.capabilities.llm import _apply_legacy_stock_result_postprocessing


class FakeAnalysisResult:
    def __init__(self):
        self.current_price = None
        self.change_pct = None
        self.query_id = ""
        self.search_performed = False
        self.news_result_count = None
        self.news_result_count_known = False
        self.news_evidence_present = False
        self.fundamental_context = None
        self.report_language = "zh"
        self.operation_advice = "观望"
        self.action = None


def test_legacy_postprocessing_preserves_critical_pipeline_repairs(monkeypatch):
    calls: list[tuple[str, object]] = []

    def record(name):
        def _helper(*args, **kwargs):
            calls.append((name, (args, kwargs)))
        return _helper

    fake_analyzer = types.ModuleType("src.analyzer")
    fake_analyzer.normalize_chip_structure_availability = record("chip")
    fake_analyzer.fill_price_position_if_needed = record("price_position")
    fake_analyzer.stabilize_decision_with_structure = record("decision_stability")
    fake_analyzer.populate_decision_action_fields = record("action_fields")
    monkeypatch.setitem(sys.modules, "src.analyzer", fake_analyzer)

    result = FakeAnalysisResult()
    context = {
        "code": "600396",
        "realtime": {"price": 11.2, "change_pct": 1.1},
        "chip": {"avg_cost": 10.5, "profit_ratio": 0.6},
        "trend_analysis": {"ma5": 10.8, "support_levels": [10.8]},
        "fundamental_context": {"capital_flow": {"status": "available"}},
    }
    _apply_legacy_stock_result_postprocessing(
        result,
        context,
        report_type="detailed",
        query_id="q-1",
        search_performed=True,
        news_result_count=2,
        news_evidence_present=True,
    )

    assert result.current_price == 11.2
    assert result.change_pct == 1.1
    assert result.query_id == "q-1"
    assert result.search_performed is True
    assert result.news_result_count == 2
    assert result.news_result_count_known is True
    assert result.news_evidence_present is True
    assert result.fundamental_context == context["fundamental_context"]
    assert [name for name, _ in calls] == ["chip", "price_position", "decision_stability", "action_fields"]
    assert calls[0][1][0][1] == context["chip"]
    assert calls[1][1][0][2] == context["realtime"]
    assert calls[-1][1][1]["report_type"] == "detailed"


def test_plain_mapping_backend_keeps_price_and_evidence_metadata():
    result = {"code": "600396", "success": True}
    context = {"realtime": {"price": 10.0, "change_pct": -1.2}}
    _apply_legacy_stock_result_postprocessing(
        result,
        context,
        report_type="simple",
        query_id="q-map",
        search_performed=False,
        news_result_count=None,
        news_evidence_present=False,
    )
    assert result["query_id"] == "q-map"
    assert result["current_price"] == 10.0
    assert result["change_pct"] == -1.2
    assert result["search_performed"] is False
    assert result["news_result_count"] is None
    assert result["news_result_count_known"] is False
