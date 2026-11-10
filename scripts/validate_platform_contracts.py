#!/usr/bin/env python3
"""Validate Platform JSON Schema and Golden fixtures in CI."""

import copy
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_PATH = ROOT / "doc/platform/schema/PLAT-CTR-004-observation-1.0.0.schema.json"
GOLDEN_DIR = ROOT / "doc/platform/golden"


def load_json(path: Path):
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def main() -> None:
    schema = load_json(SCHEMA_PATH)
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())

    golden_files = sorted(GOLDEN_DIR.glob("PLAT-CTR-004-observation-*.golden.json"))
    if len(golden_files) < 2:
        raise SystemExit("expected at least two PLAT-CTR-004 Observation Golden fixtures")

    fixtures = []
    for path in golden_files:
        value = load_json(path)
        validator.validate(value)
        fixtures.append(value)
        print(f"PASS schema/golden: {path.relative_to(ROOT)}")

    base = copy.deepcopy(fixtures[0])
    negative_cases = []

    value = copy.deepcopy(base)
    value.pop("safe_message", None)
    negative_cases.append(("missing safe_message", value))

    value = copy.deepcopy(base)
    value["status"] = "unknown"
    negative_cases.append(("unknown status", value))

    value = copy.deepcopy(base)
    value["duration"] = -1
    negative_cases.append(("negative duration", value))

    value = copy.deepcopy(base)
    value["status"] = "failed"
    value.pop("reason_code", None)
    negative_cases.append(("non-healthy without reason_code", value))

    value = copy.deepcopy(base)
    value["checked_at"] = "2026-11-06T22:40:00+08:00"
    negative_cases.append(("non-UTC checked_at", value))

    value = copy.deepcopy(base)
    value["unexpected"] = "not allowed"
    negative_cases.append(("unknown field", value))

    value = copy.deepcopy(base)
    value["attributes"] = {f"k{i}": "v" for i in range(17)}
    negative_cases.append(("more than 16 attributes", value))

    for label, value in negative_cases:
        if validator.is_valid(value):
            raise SystemExit(f"FAIL schema negative case was accepted: {label}")
        print(f"PASS schema negative: {label}")


if __name__ == "__main__":
    main()
