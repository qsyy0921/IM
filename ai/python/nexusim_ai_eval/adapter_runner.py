"""Batch adapter conversion for local public-dataset-style samples."""

from __future__ import annotations

from typing import Any

from nexusim_ai_eval.adapters import adapter_by_name, suite_from_adapter_cases
from nexusim_ai_eval.contracts import (
    EvalReport,
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    validate_eval_suite,
)
from nexusim_ai_eval.evaluator import run_eval_suite


def convert_adapter_payload(payload: dict[str, Any]) -> dict[str, Any]:
    """Convert one adapter sample payload into a validated EvalSuite payload."""

    assert_low_sensitive_eval_payload(payload, "agent adapter payload")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    adapter_name = _required_string(payload, "adapter_name")
    suite_id = _required_string(payload, "suite_id")
    cases = payload.get("cases")
    if not isinstance(cases, list) or not cases:
        raise ValueError("cases must be a non-empty list")
    normalized_cases: list[dict[str, Any]] = []
    for index, item in enumerate(cases):
        if not isinstance(item, dict):
            raise ValueError(f"cases[{index}] must be an object")
        normalized_cases.append(item)

    adapter = adapter_by_name(adapter_name)
    suite = suite_from_adapter_cases(
        suite_id=suite_id,
        adapter=adapter,
        cases=normalized_cases,
    )
    validate_eval_suite(suite)
    return suite


def run_adapter_payload(payload: dict[str, Any]) -> EvalReport:
    """Convert and run one adapter sample payload."""

    return run_eval_suite(convert_adapter_payload(payload))


def _required_string(payload: dict[str, Any], field_name: str) -> str:
    value = str(payload.get(field_name, "")).strip()
    if not value:
        raise ValueError(f"{field_name} is required")
    return value
