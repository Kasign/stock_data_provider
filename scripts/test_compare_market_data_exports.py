#!/usr/bin/env python3
"""Unit tests for scripts/compare_market_data_exports.py."""

from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("compare_market_data_exports.py")
SPEC = importlib.util.spec_from_file_location("compare_market_data_exports", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(MODULE)


def export(records, **overrides):
    value = {
        "schema_version": 1,
        "dataset": "daily_bars",
        "symbol": "600519.SH",
        "adjustment_basis": "none",
        "provider": "fixture",
        "captured_at_utc": "2026-10-30T00:00:00Z",
        "records": records,
    }
    value.update(overrides)
    return value


class CompareMarketDataExportsTests(unittest.TestCase):
    def write_json(self, directory: Path, name: str, value):
        path = directory / name
        path.write_text(json.dumps(value), encoding="utf-8")
        return path

    def run_cli(self, py, go, *extra):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            py_path = self.write_json(root, "python.json", py)
            go_path = self.write_json(root, "go.json", go)
            with patch("sys.argv", ["compare_market_data_exports.py", "--python", str(py_path),
                                    "--go", str(go_path), *extra]):
                return MODULE.main()

    def test_identical_samples_pass(self):
        rows = [{"trade_date": "2026-10-29", "close": 1500.0, "volume": 1000,
                 "amount": None, "quality_status": "degraded",
                 "quality_reasons": ["amount_missing"]}]
        self.assertEqual(self.run_cli(export(rows), export(rows)), 0)

    def test_numeric_tolerance_is_applied(self):
        py = export([{"trade_date": "2026-10-29", "close": 10.005}])
        go = export([{"trade_date": "2026-10-29", "close": 10.0}])
        self.assertEqual(self.run_cli(py, go), 0)
        self.assertEqual(self.run_cli(py, go, "--tolerance", "close=0.001"), 1)

    def test_null_and_zero_are_not_equal(self):
        py = export([{"trade_date": "2026-10-29", "amount": None}])
        go = export([{"trade_date": "2026-10-29", "amount": 0}])
        self.assertEqual(self.run_cli(py, go), 1)

    def test_quality_difference_blocks_parity(self):
        py = export([{"trade_date": "2026-10-29", "quality_status": "degraded"}])
        go = export([{"trade_date": "2026-10-29", "quality_status": "valid"}])
        self.assertEqual(self.run_cli(py, go), 1)

    def test_missing_field_blocks_parity(self):
        py = export([{"trade_date": "2026-10-29", "amount": None}])
        go = export([{"trade_date": "2026-10-29"}])
        self.assertEqual(self.run_cli(py, go), 1)

    def test_record_coverage_difference_blocks_parity(self):
        py = export([{"trade_date": "2026-10-28"}, {"trade_date": "2026-10-29"}])
        go = export([{"trade_date": "2026-10-29"}])
        self.assertEqual(self.run_cli(py, go), 1)

    def test_duplicate_record_key_is_rejected(self):
        py = export([{"trade_date": "2026-10-29"}, {"trade_date": "2026-10-29"}])
        go = export([{"trade_date": "2026-10-29"}])
        self.assertEqual(self.run_cli(py, go), 2)

    def test_sample_identity_mismatch_is_rejected(self):
        py = export([{"trade_date": "2026-10-29"}])
        go = export([{"trade_date": "2026-10-29"}], adjustment_basis="forward")
        self.assertEqual(self.run_cli(py, go), 2)

    def test_invalid_tolerance_is_rejected(self):
        rows = [{"trade_date": "2026-10-29"}]
        self.assertEqual(self.run_cli(export(rows), export(rows), "--tolerance", "close=-1"), 2)
        self.assertEqual(self.run_cli(export(rows), export(rows), "--tolerance", "close=nan"), 2)


if __name__ == "__main__":
    unittest.main(verbosity=2)
