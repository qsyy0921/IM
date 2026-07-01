"""Low-sensitive Agent eval contract helpers.

The eval harness is offline and fixture-only. It validates synthetic inputs and
returns low-sensitive reports; it does not call NexusIM services or persist
production state.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any

from nexusim_ai_common.contracts import assert_no_forbidden_fields
from nexusim_ai_common.safety import assert_low_sensitive_value


SCHEMA_VERSION = 1
HARNESS_VERSION = "agent-eval-harness-v1"

ALLOWED_CAPABILITY_FAMILIES = {
    "GROUNDED_RAG",
    "CONTEXT_EVIDENCE",
    "MEMORY_ADMISSION",
    "TOOL_SECURITY",
    "STATE_DIFF",
    "POLICY_HITL",
    "MULTI_AGENT_HANDOFF",
    "RUNTIME_CONTROL",
}

ALLOWED_FAILURE_CLASSES = {
    "",
    "PASS",
    "POLICY_DENIED",
    "INSUFFICIENT_EVIDENCE",
    "CONFLICTING_EVIDENCE",
    "PERMISSION_LEAKAGE",
    "CITATION_MISSING",
    "SOURCE_COVERAGE_MISSING",
    "EVIDENCE_CONFLICT_NOT_DETECTED",
    "STALE_EVIDENCE_USED",
    "PERMISSION_ABSTAIN_MISSING",
    "TOOL_NOT_ALLOWED",
    "TOOL_ARGS_INVALID",
    "TOOL_POISONING_DETECTED",
    "UNSAFE_TOOL_OUTPUT",
    "MCP_PROVENANCE_MISMATCH",
    "PROVIDER_TIMEOUT",
    "APPROVAL_REQUIRED",
    "APPROVAL_REJECTED",
    "APPROVAL_TIMEOUT",
    "STATE_DIFF_MISMATCH",
    "MEMORY_SCOPE_VIOLATION",
    "MEMORY_CONFLICT",
    "MEMORY_POLLUTION",
    "HANDOFF_SCOPE_VIOLATION",
    "REPLAY_INCOMPLETE",
    "RUNTIME_EVENT_MISSING",
    "RESUME_CHECKPOINT_MISSING",
    "CANCEL_NOT_PROPAGATED",
}

FORBIDDEN_EVAL_FIELDS = {
    "backend_url",
    "business_payload",
    "database_url",
    "execution_payload",
    "im_message_text",
    "kafka_broker",
    "postgres_dsn",
    "production_endpoint",
    "provider_request_body",
    "redis_url",
    "service_url",
}


@dataclass(frozen=True)
class EvalCase:
    case_id: str
    dataset_name: str
    dataset_version: str
    capability_family: str
    fixture_version: str
    input_refs: list[str] = field(default_factory=list)
    visible_evidence_refs: list[str] = field(default_factory=list)
    forbidden_evidence_refs: list[str] = field(default_factory=list)
    actual_used_refs: list[str] = field(default_factory=list)
    expected_source_coverage_refs: list[str] = field(default_factory=list)
    actual_source_coverage_refs: list[str] = field(default_factory=list)
    conflicting_evidence_refs: list[str] = field(default_factory=list)
    stale_evidence_refs: list[str] = field(default_factory=list)
    expected_citation_refs: list[str] = field(default_factory=list)
    actual_citation_refs: list[str] = field(default_factory=list)
    expected_memory_outcome: str = ""
    actual_memory_outcome: str = ""
    expected_memory_scope: str = ""
    actual_memory_scope: str = ""
    expected_tool_prepare: str = ""
    actual_tool_prepare: str = ""
    expected_tool_provider_ref: str = ""
    actual_tool_provider_ref: str = ""
    expected_state_diff: dict[str, str] = field(default_factory=dict)
    actual_state_diff: dict[str, str] = field(default_factory=dict)
    expected_runtime_events: list[str] = field(default_factory=list)
    actual_runtime_events: list[str] = field(default_factory=list)
    expected_checkpoint_refs: list[str] = field(default_factory=list)
    actual_checkpoint_refs: list[str] = field(default_factory=list)
    expected_failure_class: str = ""
    actual_failure_class: str = ""
    actual_abstained: bool = False
    conflict_detected: bool = False
    stale_evidence_used: bool = False
    permission_abstain_required: bool = False
    malicious_tool_blocked: bool = False
    tool_description_poisoned: bool = False
    tool_description_blocked: bool = False
    tool_output_contains_instruction: bool = False
    unsafe_output_quarantined: bool = False
    revoked_memory_used: bool = False
    side_effect_reexecuted: bool = False


@dataclass(frozen=True)
class ReplayBundle:
    replay_bundle_ref: str
    case_id: str
    input_hashes: list[str]
    evidence_pack_refs: list[str]
    context_package_refs: list[str]
    prepared_tool_refs: list[str]
    workflow_decision_refs: list[str]
    execution_refs: list[str]
    memory_candidate_refs: list[str]
    checkpoint_refs: list[str]
    audit_refs: list[str]
    failure_class: str
    replay_complete: bool
    side_effect_reexecuted: bool
    raw_payload_returned: bool = False


@dataclass(frozen=True)
class EvalRun:
    run_id: str
    suite_id: str
    harness_version: str
    adapter_versions: list[str]
    case_ids: list[str]


@dataclass(frozen=True)
class EvalResult:
    case_id: str
    capability_family: str
    status: str
    failure_class: str
    scores: dict[str, float]
    replay_bundle: ReplayBundle


@dataclass(frozen=True)
class EvalReport:
    schema_version: int
    suite_id: str
    harness_version: str
    eval_run: EvalRun
    status: str
    case_count: int
    passed_count: int
    failed_count: int
    aggregate_scores: dict[str, float]
    failure_distribution: dict[str, int]
    results: list[EvalResult]


def validate_eval_suite(payload: Any) -> list[EvalCase]:
    """Validate and normalize an offline eval suite payload."""

    if not isinstance(payload, dict):
        raise ValueError("eval suite must be an object")
    assert_low_sensitive_eval_payload(payload, "agent eval suite")

    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("fixture_kind", "")) != "synthetic_im_like":
        raise ValueError("fixture_kind must be synthetic_im_like")
    _required_string(payload, "suite_id")

    cases_raw = payload.get("cases")
    if not isinstance(cases_raw, list) or not cases_raw:
        raise ValueError("cases must be a non-empty list")

    cases: list[EvalCase] = []
    seen_ids: set[str] = set()
    for index, case_payload in enumerate(cases_raw):
        if not isinstance(case_payload, dict):
            raise ValueError(f"cases[{index}] must be an object")
        case = _eval_case(case_payload, index)
        if case.case_id in seen_ids:
            raise ValueError(f"duplicate case_id: {case.case_id}")
        seen_ids.add(case.case_id)
        cases.append(case)
    return cases


def assert_low_sensitive_eval_payload(payload: Any, context: str) -> None:
    """Reject raw prompts, backend connectivity and sensitive eval payloads."""

    assert_no_forbidden_fields(payload)
    _assert_no_forbidden_eval_fields(payload)
    assert_low_sensitive_value(payload, context)


def suite_id(payload: dict[str, Any]) -> str:
    return _required_string(payload, "suite_id")


def stable_ref(prefix: str, payload: Any) -> str:
    return f"{prefix}_{sha256_json(payload)[:24]}"


def sha256_json(payload: Any) -> str:
    serialized = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


def _eval_case(payload: dict[str, Any], index: int) -> EvalCase:
    capability_family = _required_string(payload, "capability_family").upper()
    if capability_family not in ALLOWED_CAPABILITY_FAMILIES:
        raise ValueError(f"cases[{index}].capability_family unsupported: {capability_family}")

    expected_failure_class = _failure_class(payload.get("expected_failure_class", ""))
    actual_failure_class = _failure_class(payload.get("actual_failure_class", ""))

    return EvalCase(
        case_id=_required_string(payload, "case_id"),
        dataset_name=_required_string(payload, "dataset_name"),
        dataset_version=_required_string(payload, "dataset_version"),
        capability_family=capability_family,
        fixture_version=_required_string(payload, "fixture_version"),
        input_refs=_string_list(payload.get("input_refs", []), "input_refs"),
        visible_evidence_refs=_string_list(
            payload.get("visible_evidence_refs", []), "visible_evidence_refs"
        ),
        forbidden_evidence_refs=_string_list(
            payload.get("forbidden_evidence_refs", []), "forbidden_evidence_refs"
        ),
        actual_used_refs=_string_list(payload.get("actual_used_refs", []), "actual_used_refs"),
        expected_source_coverage_refs=_string_list(
            payload.get("expected_source_coverage_refs", []), "expected_source_coverage_refs"
        ),
        actual_source_coverage_refs=_string_list(
            payload.get("actual_source_coverage_refs", []), "actual_source_coverage_refs"
        ),
        conflicting_evidence_refs=_string_list(
            payload.get("conflicting_evidence_refs", []), "conflicting_evidence_refs"
        ),
        stale_evidence_refs=_string_list(
            payload.get("stale_evidence_refs", []), "stale_evidence_refs"
        ),
        expected_citation_refs=_string_list(
            payload.get("expected_citation_refs", []), "expected_citation_refs"
        ),
        actual_citation_refs=_string_list(
            payload.get("actual_citation_refs", []), "actual_citation_refs"
        ),
        expected_memory_outcome=_string(payload.get("expected_memory_outcome", "")).upper(),
        actual_memory_outcome=_string(payload.get("actual_memory_outcome", "")).upper(),
        expected_memory_scope=_string(payload.get("expected_memory_scope", "")).upper(),
        actual_memory_scope=_string(payload.get("actual_memory_scope", "")).upper(),
        expected_tool_prepare=_string(payload.get("expected_tool_prepare", "")).upper(),
        actual_tool_prepare=_string(payload.get("actual_tool_prepare", "")).upper(),
        expected_tool_provider_ref=_string(payload.get("expected_tool_provider_ref", "")),
        actual_tool_provider_ref=_string(payload.get("actual_tool_provider_ref", "")),
        expected_state_diff=_string_map(
            payload.get("expected_state_diff", {}), "expected_state_diff"
        ),
        actual_state_diff=_string_map(payload.get("actual_state_diff", {}), "actual_state_diff"),
        expected_runtime_events=_upper_string_list(
            payload.get("expected_runtime_events", []), "expected_runtime_events"
        ),
        actual_runtime_events=_upper_string_list(
            payload.get("actual_runtime_events", []), "actual_runtime_events"
        ),
        expected_checkpoint_refs=_string_list(
            payload.get("expected_checkpoint_refs", []), "expected_checkpoint_refs"
        ),
        actual_checkpoint_refs=_string_list(
            payload.get("actual_checkpoint_refs", []), "actual_checkpoint_refs"
        ),
        expected_failure_class=expected_failure_class,
        actual_failure_class=actual_failure_class,
        actual_abstained=_bool(payload.get("actual_abstained", False), "actual_abstained"),
        conflict_detected=_bool(payload.get("conflict_detected", False), "conflict_detected"),
        stale_evidence_used=_bool(
            payload.get("stale_evidence_used", False), "stale_evidence_used"
        ),
        permission_abstain_required=_bool(
            payload.get("permission_abstain_required", False), "permission_abstain_required"
        ),
        malicious_tool_blocked=_bool(
            payload.get("malicious_tool_blocked", False), "malicious_tool_blocked"
        ),
        tool_description_poisoned=_bool(
            payload.get("tool_description_poisoned", False), "tool_description_poisoned"
        ),
        tool_description_blocked=_bool(
            payload.get("tool_description_blocked", False), "tool_description_blocked"
        ),
        tool_output_contains_instruction=_bool(
            payload.get("tool_output_contains_instruction", False),
            "tool_output_contains_instruction",
        ),
        unsafe_output_quarantined=_bool(
            payload.get("unsafe_output_quarantined", False), "unsafe_output_quarantined"
        ),
        revoked_memory_used=_bool(payload.get("revoked_memory_used", False), "revoked_memory_used"),
        side_effect_reexecuted=_bool(
            payload.get("side_effect_reexecuted", False), "side_effect_reexecuted"
        ),
    )


def _failure_class(value: Any) -> str:
    normalized = _string(value).upper()
    if normalized not in ALLOWED_FAILURE_CLASSES:
        raise ValueError(f"unsupported failure class: {normalized}")
    return "" if normalized == "PASS" else normalized


def _assert_no_forbidden_eval_fields(value: Any, path: str = "") -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            normalized = str(key).strip().lower()
            current_path = f"{path}.{normalized}" if path else normalized
            if normalized in FORBIDDEN_EVAL_FIELDS:
                raise ValueError(f"forbidden eval field: {current_path}")
            _assert_no_forbidden_eval_fields(nested, current_path)
    elif isinstance(value, list):
        for index, nested in enumerate(value):
            _assert_no_forbidden_eval_fields(nested, f"{path}[{index}]")


def _required_string(payload: dict[str, Any], field_name: str) -> str:
    value = _string(payload.get(field_name, ""))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def _string(value: Any) -> str:
    return str(value).strip()


def _string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = _string(item)
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result


def _upper_string_list(value: Any, field_name: str) -> list[str]:
    return [item.upper() for item in _string_list(value, field_name)]


def _string_map(value: Any, field_name: str) -> dict[str, str]:
    if not isinstance(value, dict):
        raise ValueError(f"{field_name} must be an object")
    result: dict[str, str] = {}
    for key, nested in value.items():
        normalized_key = _string(key)
        normalized_value = _string(nested)
        if not normalized_key or not normalized_value:
            raise ValueError(f"{field_name} contains empty key or value")
        result[normalized_key] = normalized_value
    return result


def _bool(value: Any, field_name: str) -> bool:
    if not isinstance(value, bool):
        raise ValueError(f"{field_name} must be a boolean")
    return value
