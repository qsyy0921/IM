"""Minimal candidate-only worker runtime helpers.

This module intentionally does not call databases, policy services or external
providers. It converts a low-sensitive worker request into a candidate contract
payload that Go services can validate, authorize, audit and persist.
"""

from __future__ import annotations

import hashlib
from typing import Any

from nexusim_ai_common.contracts import validate_worker_candidate
from nexusim_ai_common.safety import assert_low_sensitive_value


DEFAULT_FAILED_TASK_ID = "unknown_task"
DEFAULT_FAILED_CANDIDATE_ID = "failed_candidate"


def build_worker_candidate(payload: dict[str, Any]) -> dict[str, Any]:
    """Build and validate a low-sensitive candidate from a worker request."""

    assert_low_sensitive_value(payload, "worker request")

    text = required_string(payload, "candidate_text")
    candidate = {
        "schema_version": 1,
        "task_id": required_string(payload, "task_id"),
        "candidate_id": required_string(payload, "candidate_id"),
        "worker_kind": required_string(payload, "worker_kind"),
        "status": "CANDIDATE",
        "output_type": required_string(payload, "output_type"),
        "output_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
        "source_refs": string_list(payload.get("source_refs", []), "source_refs"),
        "citations": string_list(payload.get("citations", []), "citations"),
        "safety_flags": ["LOW_SENSITIVE"],
    }
    if "confidence" in payload:
        candidate["confidence"] = payload["confidence"]

    validate_worker_candidate(candidate)
    return candidate


def build_failed_candidate(payload: Any, error_class: str) -> dict[str, Any]:
    """Return a validated low-sensitive failure candidate."""

    source = payload if isinstance(payload, dict) else {}
    task_id = safe_string(source.get("task_id", DEFAULT_FAILED_TASK_ID))
    candidate_id = safe_string(source.get("candidate_id", DEFAULT_FAILED_CANDIDATE_ID))
    if not task_id:
        task_id = DEFAULT_FAILED_TASK_ID
    if not candidate_id:
        candidate_id = DEFAULT_FAILED_CANDIDATE_ID

    candidate = {
        "schema_version": 1,
        "task_id": task_id,
        "candidate_id": candidate_id,
        "worker_kind": "EVAL",
        "status": "FAILED",
        "output_type": "EVAL_RESULT",
        "output_sha256": "",
        "source_refs": [],
        "citations": [],
        "safety_flags": [error_class],
        "error_class": error_class,
    }
    validate_worker_candidate(candidate)
    return candidate


def run_worker_request(payload: Any) -> tuple[dict[str, Any], int]:
    """Run one request and return candidate payload plus process-style exit code."""

    if not isinstance(payload, dict):
        return build_failed_candidate(payload, "MALFORMED_INPUT"), 2

    try:
        return build_worker_candidate(payload), 0
    except ValueError as exc:
        message = str(exc).lower()
        if "sensitive" in message or "forbidden" in message:
            return build_failed_candidate(payload, "UNSAFE_INPUT"), 2
        return build_failed_candidate(payload, "MALFORMED_INPUT"), 2


def required_string(payload: dict[str, Any], field_name: str) -> str:
    value = safe_string(payload.get(field_name, ""))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def safe_string(value: Any) -> str:
    return str(value).strip()


def string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = safe_string(item)
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result
