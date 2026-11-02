#!/usr/bin/env python3
"""Compare sanitized Python and Go market-data exports using a shared JSON contract."""

from __future__ import annotations

import argparse
import json
import math
import sys
from collections import Counter
from pathlib import Path
from typing import Any

DEFAULT_TOLERANCES = {
    "open": 0.01,
    "high": 0.01,
    "low": 0.01,
    "close": 0.01,
    "current_price": 0.01,
    "change": 0.01,
    "change_percent": 0.001,
    "volume": 0.0,
    "amount": 0.01,
}
PROVENANCE_FIELDS = {
    "source", "source_version", "provider", "provider_time", "fetched_at",
    "captured_at_utc", "source_commit", "workflow_run_id",
}
QUALITY_FIELDS = {"quality_status", "quality_reasons"}


class InputError(ValueError):
    pass


def load_export(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise InputError(f"{path}: cannot read valid JSON: {exc}") from exc
    if not isinstance(value, dict):
        raise InputError(f"{path}: top-level JSON must be an object")
    for key in ("schema_version", "dataset", "symbol", "adjustment_basis", "records"):
        if key not in value:
            raise InputError(f"{path}: missing required key {key!r}")
    if value["schema_version"] != 1:
        raise InputError(f"{path}: unsupported schema_version {value['schema_version']!r}")
    if not isinstance(value["records"], list):
        raise InputError(f"{path}: records must be an array")
    return value


def record_key(record: dict[str, Any], configured_key: str) -> str:
    value = record.get(configured_key)
    if value is None:
        raise InputError(f"record is missing key field {configured_key!r}: {record!r}")
    return str(value)


def index_records(export: dict[str, Any], key_field: str, label: str):
    indexed: dict[str, dict[str, Any]] = {}
    duplicates: list[str] = []
    for row in export["records"]:
        if not isinstance(row, dict):
            raise InputError(f"{label}: every records[] item must be an object")
        key = record_key(row, key_field)
        if key in indexed:
            duplicates.append(key)
        indexed[key] = row
    if duplicates:
        raise InputError(f"{label}: duplicate {key_field} values: {sorted(set(duplicates))}")
    return indexed


def parse_tolerances(values: list[str]) -> dict[str, float]:
    result = dict(DEFAULT_TOLERANCES)
    for item in values:
        if "=" not in item:
            raise InputError(f"invalid tolerance {item!r}; expected FIELD=NUMBER")
        field, raw = item.split("=", 1)
        try:
            tolerance = float(raw)
        except ValueError as exc:
            raise InputError(f"invalid tolerance {item!r}") from exc
        if not math.isfinite(tolerance) or tolerance < 0:
            raise InputError(f"tolerance must be a finite non-negative number: {item!r}")
        result[field] = tolerance
    return result


def compare_value(field: str, left: Any, right: Any, tolerances: dict[str, float]):
    if left is None or right is None:
        if left is None and right is None:
            return None
        if (left is None and right == 0) or (right is None and left == 0):
            return ("null_zero", f"Python={left!r}, Go={right!r}")
        return ("null_mismatch", f"Python={left!r}, Go={right!r}")
    if isinstance(left, (int, float)) and not isinstance(left, bool) and isinstance(right, (int, float)) and not isinstance(right, bool):
        if not math.isfinite(float(left)) or not math.isfinite(float(right)):
            return ("invalid_number", f"Python={left!r}, Go={right!r}")
        tolerance = tolerances.get(field, 0.0)
        delta = abs(float(left) - float(right))
        if delta > tolerance:
            return ("numeric_mismatch", f"Python={left!r}, Go={right!r}, delta={delta:g}, tolerance={tolerance:g}")
        return None
    if type(left) is not type(right) or left != right:
        category = "quality_difference" if field in QUALITY_FIELDS else "field_mismatch"
        return (category, f"Python={left!r}, Go={right!r}")
    return None


def markdown_report(py, go, key_field, tolerances, differences, missing_python, missing_go):
    lines = [
        "# Python / Go 数据样本差分报告",
        "",
        f"- 数据域：{py['dataset']}",
        f"- 证券：{py['symbol']}",
        f"- 复权口径：{py['adjustment_basis']}",
        f"- 记录键：{key_field}",
        f"- Python 来源：{py.get('provider', py.get('source', '未提供'))}",
        f"- Go 来源：{go.get('provider', go.get('source', '未提供'))}",
        f"- Python 采集时间：{py.get('captured_at_utc', '未提供')}",
        f"- Go 采集时间：{go.get('captured_at_utc', '未提供')}",
        "",
        "## 元数据校验",
        "",
    ]
    for field in ("dataset", "symbol", "adjustment_basis"):
        status = "一致" if py[field] == go[field] else f"不一致（Python={py[field]!r}，Go={go[field]!r}）"
        lines.append(f"- {field}: {status}")
    lines += ["", "## 差异汇总", ""]
    counts = Counter(item[0] for item in differences)
    lines.append(f"- Python 独有记录：{len(missing_python)}")
    lines.append(f"- Go 独有记录：{len(missing_go)}")
    lines.append(f"- 字段差异：{len(differences)}")
    for category, count in sorted(counts.items()):
        lines.append(f"- {category}: {count}")
    lines += ["", "## 记录覆盖差异", ""]
    lines.append("- Python 独有键：" + (", ".join(missing_go) if missing_go else "无"))
    lines.append("- Go 独有键：" + (", ".join(missing_python) if missing_python else "无"))
    lines += ["", "## 字段差异明细", ""]
    if not differences:
        lines.append("无字段差异。")
    else:
        lines += ["| 记录键 | 字段 | 类型 | 说明 |", "|---|---|---|---|"]
        for key, field, category, detail in differences:
            safe = detail.replace("|", "\\|").replace("\n", " ")
            lines.append(f"| {key} | {field} | {category} | {safe} |")
    lines += ["", "## 数值容差", ""]
    for field, tolerance in sorted(tolerances.items()):
        lines.append(f"- {field}: {tolerance:g}")
    lines += [
        "",
        "## 判定规则",
        "",
        "- null 与数值 0 始终视为不同，不做自动填充。",
        "- 缺失字段、单位不同、复权口径不同、重复记录均不能被容差掩盖。",
        "- source/provider/采集时间属于来源元数据，不要求两侧相同；但会在报告中保留。",
        "- quality_status/quality_reasons 差异单独列出，不能直接判为等价。",
        "- 本工具只比较已导出的样本，不发起 Provider 请求，也不推断未提供的数据。",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--python", required=True, type=Path, help="Python 侧标准化 JSON 导出")
    parser.add_argument("--go", required=True, type=Path, help="Go 侧标准化 JSON 导出")
    parser.add_argument("--key", default="trade_date", help="records[] 的唯一键，默认 trade_date")
    parser.add_argument("--tolerance", action="append", default=[], metavar="FIELD=NUMBER",
                        help="覆盖数值容差，可重复指定")
    parser.add_argument("--output", type=Path, help="Markdown 报告输出路径；省略时输出到 stdout")
    args = parser.parse_args()

    try:
        py = load_export(args.python)
        go = load_export(args.go)
        tolerances = parse_tolerances(args.tolerance)
        for field in ("dataset", "symbol", "adjustment_basis"):
            if py[field] != go[field]:
                raise InputError(
                    f"sample identity mismatch for {field}: Python={py[field]!r}, Go={go[field]!r}"
                )
        py_rows = index_records(py, args.key, "Python")
        go_rows = index_records(go, args.key, "Go")
        missing_python = sorted(go_rows.keys() - py_rows.keys())
        missing_go = sorted(py_rows.keys() - go_rows.keys())
        differences = []
        for key in sorted(py_rows.keys() & go_rows.keys()):
            left, right = py_rows[key], go_rows[key]
            fields = sorted((left.keys() | right.keys()) - PROVENANCE_FIELDS)
            for field in fields:
                if field not in left or field not in right:
                    differences.append((key, field, "field_missing", f"Python_has={field in left}, Go_has={field in right}"))
                    continue
                result = compare_value(field, left[field], right[field], tolerances)
                if result:
                    differences.append((key, field, result[0], result[1]))
        report = markdown_report(py, go, args.key, tolerances, differences, missing_python, missing_go)
        if args.output:
            args.output.write_text(report, encoding="utf-8")
        else:
            print(report)
        # Quality metadata is part of the data contract; any difference blocks a clean parity result.
        return 1 if missing_python or missing_go or differences else 0
    except InputError as exc:
        print(f"输入错误：{exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
