"""Backend-isolated interview demo runner for the NexusIM Agent layer.

The demo consumes only synthetic refs. It does not connect to NexusIM backend
services, model providers, MCP providers or production data stores.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    ALLOWED_FAILURE_CLASSES,
    HARNESS_VERSION,
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
    stable_ref,
)
from nexusim_ai_eval.evaluator import eval_report_to_payload, run_eval_suite
from nexusim_ai_eval.reporting import write_json_artifact


DEMO_FIXTURE_KIND = "agent_interview_demo"
DEMO_BUNDLE_KIND = "agent_interview_demo_replay_bundle"
DEMO_HARNESS_VERSION = "agent-interview-demo-harness-v1"

_NO_FAILURE_VALUES = {"", "NONE", "PASS"}
_DIRECT_EXECUTION_STATES = {"active", "admitted", "executed", "side_effect_executed"}
_ALLOWED_MEMORY_STATES = {"candidate_only", "needs_review", "rejected", "none"}
_ALLOWED_TOOL_STATES = {"prepared", "blocked", "quarantined", "approval_required", "none"}
_ALLOWED_APPROVAL_STATES = {
    "not_required",
    "proposal_only",
    "required",
    "approved_fixture",
    "rejected_fixture",
    "timeout_fixture",
}
_ALLOWED_ACTION_STATES = {
    "not_executed",
    "proposal_only",
    "blocked_fixture",
    "fixture_result",
}
_FORBIDDEN_DEMO_FIELDS = {
    "provider_request_body",
    "provider_response_body",
    "raw_message_body",
    "raw_provider_body",
    "raw_provider_output",
    "raw_prompt",
    "raw_tool_output",
}
_PRODUCTION_CONNECTION_FIELDS = {
    "backend_connected",
    "production_backend_connected",
    "real_backend_connected",
    "uses_production_backend",
    "uses_real_backend",
}
_CASE_KIND_TO_FAMILY = {
    "happy_path": "STATE_DIFF",
    "denied_evidence": "CONTEXT_EVIDENCE",
    "memory_needs_review": "MEMORY_ADMISSION",
    "unsafe_tool_output": "TOOL_SECURITY",
    "approval_required": "POLICY_HITL",
}


@dataclass(frozen=True)
class InterviewDemoCase:
    case_id: str
    user_request_ref: str
    conversation_ref: str
    visible_message_refs: list[str]
    forbidden_message_refs: list[str]
    policy_scope_ref: str
    evidence_pack_ref: str
    context_package_ref: str
    expected_memory_candidate_ref: str
    expected_tool_intent_ref: str
    approval_fixture_ref: str
    action_fixture_ref: str
    expected_failure_class: str
    case_kind: str = ""
    context_source_refs: list[str] = field(default_factory=list)
    memory_candidate_state: str = "candidate_only"
    tool_intent_state: str = "prepared"
    approval_state: str = "proposal_only"
    action_state: str = "not_executed"
    blocked_reason_refs: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class InterviewDemoFixture:
    schema_version: int
    demo_id: str
    send_message_trace_ref: str
    message_committed_ref: str
    agent_request_ref: str
    cases: list[InterviewDemoCase]


@dataclass(frozen=True)
class InterviewCaseResult:
    case_id: str
    status: str
    expected_failure_class: str
    observed_failure_class: str
    blocked: bool
    evidence_pack_ref: str
    context_package_ref: str
    context_source_refs: list[str]
    forbidden_message_refs: list[str]
    memory_candidate_ref: str
    memory_candidate_state: str
    tool_intent_ref: str
    tool_intent_state: str
    approval_fixture_ref: str
    action_fixture_ref: str
    approval_state: str
    action_state: str
    blocked_reason_refs: list[str]
    result_ref: str


@dataclass(frozen=True)
class InterviewDemoResult:
    fixture: InterviewDemoFixture
    status: str
    case_results: list[InterviewCaseResult]
    eval_report: dict[str, Any]
    replay_bundle: dict[str, Any]
    eval_report_ref: str
    replay_bundle_ref: str
    blocked_cases: list[dict[str, str]]


@dataclass(frozen=True)
class OutputPaths:
    summary_out: Path | None = None
    report_out: Path | None = None
    replay_out: Path | None = None


def load_interview_demo_fixture(path: Path) -> InterviewDemoFixture:
    """Load a UTF-8 JSON interview demo fixture."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load interview demo fixture") from exc
    return validate_interview_demo_fixture(payload)


def validate_interview_demo_fixture(payload: Any) -> InterviewDemoFixture:
    """Validate and normalize a backend-isolated interview demo fixture."""

    if not isinstance(payload, dict):
        raise ValueError("interview demo fixture must be an object")
    _assert_no_forbidden_demo_fields(payload)
    assert_low_sensitive_eval_payload(payload, "interview demo fixture")

    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("fixture_kind", "")) != DEMO_FIXTURE_KIND:
        raise ValueError(f"fixture_kind must be {DEMO_FIXTURE_KIND}")

    cases_raw = payload.get("cases")
    if not isinstance(cases_raw, list) or not cases_raw:
        raise ValueError("cases must be a non-empty list")

    cases: list[InterviewDemoCase] = []
    seen_ids: set[str] = set()
    for index, case_payload in enumerate(cases_raw):
        if not isinstance(case_payload, dict):
            raise ValueError(f"cases[{index}] must be an object")
        case = _demo_case(case_payload, index)
        if case.case_id in seen_ids:
            raise ValueError(f"duplicate case_id: {case.case_id}")
        seen_ids.add(case.case_id)
        cases.append(case)

    fixture = InterviewDemoFixture(
        schema_version=SCHEMA_VERSION,
        demo_id=_required_string(payload, "demo_id"),
        send_message_trace_ref=_required_string(payload, "send_message_trace_ref"),
        message_committed_ref=_required_string(payload, "message_committed_ref"),
        agent_request_ref=_required_string(payload, "agent_request_ref"),
        cases=cases,
    )
    _assert_minimum_demo_coverage(fixture)
    return fixture


def run_interview_demo(fixture: InterviewDemoFixture) -> InterviewDemoResult:
    """Run the deterministic fixture-only demo and return low-sensitive outputs."""

    case_results = [_evaluate_demo_case(case) for case in fixture.cases]
    status = "PASS" if all(result.status == "PASS" for result in case_results) else "FAIL"
    blocked_cases = _blocked_cases(case_results)

    eval_report = _build_eval_report_payload(fixture, case_results)
    eval_report_ref = stable_ref("evalreport", eval_report)
    eval_report = {**eval_report, "eval_report_ref": eval_report_ref}

    replay_bundle = _build_replay_bundle_payload(
        fixture=fixture,
        case_results=case_results,
        eval_report_ref=eval_report_ref,
        eval_report_payload=eval_report,
    )
    replay_bundle_ref = _required_string(replay_bundle, "replay_bundle_ref")

    result = InterviewDemoResult(
        fixture=fixture,
        status=status,
        case_results=case_results,
        eval_report=eval_report,
        replay_bundle=replay_bundle,
        eval_report_ref=eval_report_ref,
        replay_bundle_ref=replay_bundle_ref,
        blocked_cases=blocked_cases,
    )
    assert_low_sensitive_eval_payload(
        interview_demo_result_to_payload(result),
        "interview demo summary",
    )
    assert_low_sensitive_eval_payload(result.eval_report, "interview demo eval report")
    assert_low_sensitive_eval_payload(result.replay_bundle, "interview demo replay bundle")
    return result


def interview_demo_result_to_payload(result: InterviewDemoResult) -> dict[str, Any]:
    """Convert a demo result into the public low-sensitive summary payload."""

    return {
        "schema_version": SCHEMA_VERSION,
        "demo_id": result.fixture.demo_id,
        "demo_status": result.status,
        "hot_path_impact": "none",
        "agent_trigger_mode": "async_ref_only",
        "production_backend_connected": False,
        "raw_message_body_used": False,
        "send_message_trace_ref": result.fixture.send_message_trace_ref,
        "message_committed_ref": result.fixture.message_committed_ref,
        "agent_request_ref": result.fixture.agent_request_ref,
        "case_count": len(result.case_results),
        "passed_case_count": sum(1 for case in result.case_results if case.status == "PASS"),
        "evidence_pack_refs": _unique(
            case.evidence_pack_ref for case in result.case_results
        ),
        "context_package_refs": _unique(
            case.context_package_ref for case in result.case_results
        ),
        "memory_candidate_refs": _unique(
            case.memory_candidate_ref for case in result.case_results
        ),
        "tool_intent_refs": _unique(case.tool_intent_ref for case in result.case_results),
        "approval_action_refs": _unique(
            ref
            for case in result.case_results
            for ref in (case.approval_fixture_ref, case.action_fixture_ref)
        ),
        "eval_report_ref": result.eval_report_ref,
        "replay_bundle_ref": result.replay_bundle_ref,
        "blocked_cases": result.blocked_cases,
    }


def write_interview_demo_outputs(
    result: InterviewDemoResult,
    paths: OutputPaths,
    *,
    force: bool = False,
) -> None:
    """Write requested low-sensitive demo artifacts."""

    _assert_distinct_output_paths(paths)
    if paths.summary_out is not None:
        write_json_artifact(
            paths.summary_out,
            interview_demo_result_to_payload(result),
            force=force,
        )
    if paths.report_out is not None:
        write_json_artifact(paths.report_out, result.eval_report, force=force)
    if paths.replay_out is not None:
        write_json_artifact(paths.replay_out, result.replay_bundle, force=force)


def _demo_case(payload: dict[str, Any], index: int) -> InterviewDemoCase:
    context_source_refs = _string_list(
        payload.get("context_source_refs", []),
        f"cases[{index}].context_source_refs",
    )
    visible_message_refs = _string_list(
        payload.get("visible_message_refs", []),
        f"cases[{index}].visible_message_refs",
    )
    forbidden_message_refs = _string_list(
        payload.get("forbidden_message_refs", []),
        f"cases[{index}].forbidden_message_refs",
    )
    case = InterviewDemoCase(
        case_id=_required_string(payload, "case_id"),
        user_request_ref=_required_string(payload, "user_request_ref"),
        conversation_ref=_required_string(payload, "conversation_ref"),
        visible_message_refs=visible_message_refs,
        forbidden_message_refs=forbidden_message_refs,
        policy_scope_ref=_required_string(payload, "policy_scope_ref"),
        evidence_pack_ref=_required_string(payload, "evidence_pack_ref"),
        context_package_ref=_required_string(payload, "context_package_ref"),
        context_source_refs=context_source_refs,
        expected_memory_candidate_ref=_required_string(
            payload,
            "expected_memory_candidate_ref",
        ),
        expected_tool_intent_ref=_required_string(payload, "expected_tool_intent_ref"),
        approval_fixture_ref=_required_string(payload, "approval_fixture_ref"),
        action_fixture_ref=_required_string(payload, "action_fixture_ref"),
        expected_failure_class=_expected_failure_class(
            payload.get("expected_failure_class", "none")
        ),
        case_kind=_string(payload.get("case_kind", "")),
        memory_candidate_state=_lower_string(
            payload.get("memory_candidate_state", "candidate_only")
        ),
        tool_intent_state=_lower_string(payload.get("tool_intent_state", "prepared")),
        approval_state=_lower_string(payload.get("approval_state", "proposal_only")),
        action_state=_lower_string(payload.get("action_state", "not_executed")),
        blocked_reason_refs=_string_list(
            payload.get("blocked_reason_refs", []),
            f"cases[{index}].blocked_reason_refs",
        ),
    )
    _validate_case_boundaries(case, index)
    return case


def _evaluate_demo_case(case: InterviewDemoCase) -> InterviewCaseResult:
    observed_failure_class = _observed_failure_class(case)
    blocked = observed_failure_class != "none"
    status = _case_status(case, observed_failure_class)
    result_payload = {
        "case_id": case.case_id,
        "expected_failure_class": case.expected_failure_class,
        "observed_failure_class": observed_failure_class,
        "blocked_reason_refs": case.blocked_reason_refs,
        "evidence_pack_ref": case.evidence_pack_ref,
        "context_package_ref": case.context_package_ref,
        "memory_candidate_ref": case.expected_memory_candidate_ref,
        "tool_intent_ref": case.expected_tool_intent_ref,
        "approval_fixture_ref": case.approval_fixture_ref,
        "action_fixture_ref": case.action_fixture_ref,
    }
    return InterviewCaseResult(
        case_id=case.case_id,
        status=status,
        expected_failure_class=case.expected_failure_class,
        observed_failure_class=observed_failure_class,
        blocked=blocked,
        evidence_pack_ref=case.evidence_pack_ref,
        context_package_ref=case.context_package_ref,
        context_source_refs=list(case.context_source_refs),
        forbidden_message_refs=list(case.forbidden_message_refs),
        memory_candidate_ref=case.expected_memory_candidate_ref,
        memory_candidate_state=case.memory_candidate_state,
        tool_intent_ref=case.expected_tool_intent_ref,
        tool_intent_state=case.tool_intent_state,
        approval_fixture_ref=case.approval_fixture_ref,
        action_fixture_ref=case.action_fixture_ref,
        approval_state=case.approval_state,
        action_state=case.action_state,
        blocked_reason_refs=list(case.blocked_reason_refs),
        result_ref=stable_ref("interview_case_result", result_payload),
    )


def _observed_failure_class(case: InterviewDemoCase) -> str:
    expected = case.expected_failure_class
    if expected == "none":
        return "none"
    if expected == "POLICY_DENIED" and _denied_evidence_is_blocked(case):
        return expected
    if expected == "MEMORY_REVIEW_MISSING" and case.memory_candidate_state == "needs_review":
        return expected
    if expected == "UNSAFE_TOOL_OUTPUT" and case.tool_intent_state in {"blocked", "quarantined"}:
        return expected
    if expected == "APPROVAL_REQUIRED" and _approval_waits_without_execution(case):
        return expected
    if case.blocked_reason_refs:
        return expected
    return "none"


def _case_status(case: InterviewDemoCase, observed_failure_class: str) -> str:
    if _has_case_boundary_violation(case):
        return "FAIL"
    if case.expected_failure_class == "none":
        return "PASS" if observed_failure_class == "none" else "FAIL"
    return "PASS" if observed_failure_class == case.expected_failure_class else "FAIL"


def _has_case_boundary_violation(case: InterviewDemoCase) -> bool:
    forbidden = set(case.forbidden_message_refs)
    context = set(case.context_source_refs)
    if forbidden.intersection(context):
        return True
    if case.memory_candidate_state in _DIRECT_EXECUTION_STATES:
        return True
    if case.tool_intent_state in _DIRECT_EXECUTION_STATES:
        return True
    if case.action_state in _DIRECT_EXECUTION_STATES:
        return True
    return not _fixture_ref(case.approval_fixture_ref) or not _fixture_ref(
        case.action_fixture_ref
    )


def _denied_evidence_is_blocked(case: InterviewDemoCase) -> bool:
    return bool(case.blocked_reason_refs) and not set(
        case.forbidden_message_refs
    ).intersection(case.context_source_refs)


def _approval_waits_without_execution(case: InterviewDemoCase) -> bool:
    return (
        case.approval_state == "required"
        and case.action_state == "not_executed"
        and bool(case.blocked_reason_refs)
    )


def _build_eval_report_payload(
    fixture: InterviewDemoFixture,
    case_results: list[InterviewCaseResult],
) -> dict[str, Any]:
    eval_suite = _eval_suite_payload(fixture, case_results)
    report = run_eval_suite(eval_suite)
    report_payload = eval_report_to_payload(report)
    report_payload.update(
        {
            "demo_id": fixture.demo_id,
            "demo_kind": DEMO_FIXTURE_KIND,
            "demo_harness_version": DEMO_HARNESS_VERSION,
            "message_committed_ref": fixture.message_committed_ref,
            "send_message_trace_ref": fixture.send_message_trace_ref,
            "hot_path_impact": "none",
            "agent_trigger_mode": "async_ref_only",
            "blocked_cases": _blocked_cases(case_results),
        }
    )
    return report_payload


def _eval_suite_payload(
    fixture: InterviewDemoFixture,
    case_results: list[InterviewCaseResult],
) -> dict[str, Any]:
    results_by_id = {result.case_id: result for result in case_results}
    return {
        "schema_version": SCHEMA_VERSION,
        "suite_id": fixture.demo_id,
        "fixture_kind": "synthetic_im_like",
        "adapter_versions": [
            "interview-demo-fixture-v1",
            HARNESS_VERSION,
            DEMO_HARNESS_VERSION,
        ],
        "cases": [
            _eval_case_payload(fixture, case, results_by_id[case.case_id])
            for case in fixture.cases
        ],
    }


def _eval_case_payload(
    fixture: InterviewDemoFixture,
    case: InterviewDemoCase,
    result: InterviewCaseResult,
) -> dict[str, Any]:
    family = _CASE_KIND_TO_FAMILY.get(case.case_kind, "POLICY_HITL")
    expected_failure = _eval_failure_class(case.expected_failure_class)
    actual_failure = _eval_failure_class(result.observed_failure_class)
    base: dict[str, Any] = {
        "case_id": case.case_id,
        "dataset_name": "synthetic-im-like-agent-interview-demo",
        "dataset_version": "2026-07-02",
        "capability_family": family,
        "fixture_version": "interview-demo-fixture-v1",
        "input_refs": [
            fixture.message_committed_ref,
            fixture.agent_request_ref,
            case.user_request_ref,
            case.conversation_ref,
        ],
        "visible_evidence_refs": case.visible_message_refs,
        "forbidden_evidence_refs": case.forbidden_message_refs,
        "actual_used_refs": case.context_source_refs,
        "expected_failure_class": expected_failure,
        "actual_failure_class": actual_failure,
        "side_effect_reexecuted": False,
        "actual_replay_lineage_refs": [
            f"lineage:{fixture.message_committed_ref}",
            f"lineage:{case.evidence_pack_ref}",
            f"lineage:{case.context_package_ref}",
        ],
        "actual_replay_observability_refs": [
            f"observability:agent-interview-demo:{case.case_id}",
        ],
        "actual_replay_hash_refs": [f"hash:{sha256_json(case.case_id)[:16]}"],
        "actual_replay_version_refs": [
            "version:agent-eval-harness:v1",
            "version:agent-interview-demo:v1",
        ],
        "actual_failure_taxonomy_refs": ["failure-taxonomy:agent-eval-harness:v1"],
        "actual_trace_linkage_refs": [
            fixture.send_message_trace_ref,
            f"trace-link:agent-interview-demo:{case.case_id}",
        ],
    }

    if family == "CONTEXT_EVIDENCE":
        base.update(
            {
                "expected_source_coverage_refs": case.context_source_refs,
                "actual_source_coverage_refs": case.context_source_refs,
                "denied_lane_source_refs": case.forbidden_message_refs,
                "reported_denied_lane_source_refs": case.forbidden_message_refs,
                "denied_lane_reported": True,
                "permission_abstain_required": bool(expected_failure),
                "actual_abstained": bool(expected_failure),
            }
        )
    elif family == "MEMORY_ADMISSION":
        base.update(
            {
                "expected_memory_outcome": "REVIEW",
                "actual_memory_outcome": "REVIEW",
                "expected_memory_scope": "GROUP",
                "actual_memory_scope": "GROUP",
                "expected_memory_source_refs": case.context_source_refs,
                "actual_memory_source_refs": case.context_source_refs,
                "profile_aggregate_review_required": True,
                "profile_aggregate_reviewed": True,
            }
        )
    elif family == "TOOL_SECURITY":
        base.update(
            {
                "expected_tool_prepare": "BLOCKED",
                "actual_tool_prepare": "BLOCKED",
                "malicious_tool_blocked": True,
                "unsafe_output_quarantined": True,
                "expected_tool_provider_ref": "mcp-provider-fixture:safe-null",
                "actual_tool_provider_ref": "mcp-provider-fixture:safe-null",
            }
        )
    elif family == "STATE_DIFF":
        base.update(
            {
                "expected_state_approval_refs": [case.approval_fixture_ref],
                "actual_state_approval_refs": [case.approval_fixture_ref],
                "expected_state_prepare_refs": [case.expected_tool_intent_ref],
                "actual_state_prepare_refs": [case.expected_tool_intent_ref],
                "expected_state_audit_refs": [f"audit-fixture:{case.case_id}"],
                "actual_state_audit_refs": [f"audit-fixture:{case.case_id}"],
            }
        )
    return base


def _build_replay_bundle_payload(
    *,
    fixture: InterviewDemoFixture,
    case_results: list[InterviewCaseResult],
    eval_report_ref: str,
    eval_report_payload: dict[str, Any],
) -> dict[str, Any]:
    per_case_bundles = [
        result_payload["replay_bundle"]
        for result_payload in eval_report_payload.get("results", [])
        if isinstance(result_payload, dict)
    ]
    base: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "bundle_kind": DEMO_BUNDLE_KIND,
        "demo_id": fixture.demo_id,
        "demo_harness_version": DEMO_HARNESS_VERSION,
        "send_message_trace_ref": fixture.send_message_trace_ref,
        "message_committed_ref": fixture.message_committed_ref,
        "agent_request_ref": fixture.agent_request_ref,
        "eval_report_ref": eval_report_ref,
        "case_result_refs": [case.result_ref for case in case_results],
        "case_replay_bundle_refs": [
            _string(bundle.get("replay_bundle_ref", ""))
            for bundle in per_case_bundles
            if _string(bundle.get("replay_bundle_ref", ""))
        ],
        "input_hashes": [
            sha256_json(ref)
            for ref in [
                fixture.send_message_trace_ref,
                fixture.message_committed_ref,
                fixture.agent_request_ref,
            ]
        ],
        "evidence_pack_refs": _unique(case.evidence_pack_ref for case in case_results),
        "context_package_refs": _unique(case.context_package_ref for case in case_results),
        "memory_candidate_refs": _unique(case.memory_candidate_ref for case in case_results),
        "tool_intent_refs": _unique(case.tool_intent_ref for case in case_results),
        "approval_action_refs": _unique(
            ref
            for case in case_results
            for ref in (case.approval_fixture_ref, case.action_fixture_ref)
        ),
        "blocked_cases": _blocked_cases(case_results),
        "replay_complete": all(case.status == "PASS" for case in case_results),
        "side_effect_reexecuted": False,
        "raw_payload_returned": False,
    }
    replay_bundle_ref = stable_ref("replay", base)
    return {**base, "replay_bundle_ref": replay_bundle_ref}


def _blocked_cases(case_results: list[InterviewCaseResult]) -> list[dict[str, str]]:
    return [
        {
            "case_id": case.case_id,
            "expected_failure_class": case.expected_failure_class,
            "observed_failure_class": case.observed_failure_class,
            "blocked_reason_ref": case.blocked_reason_refs[0],
        }
        for case in case_results
        if case.blocked and case.blocked_reason_refs
    ]


def _assert_minimum_demo_coverage(fixture: InterviewDemoFixture) -> None:
    present = {case.case_kind for case in fixture.cases}
    required = set(_CASE_KIND_TO_FAMILY)
    missing = sorted(required - present)
    if missing:
        raise ValueError(f"interview demo cases missing required kinds: {missing}")


def _validate_case_boundaries(case: InterviewDemoCase, index: int) -> None:
    if not case.visible_message_refs:
        raise ValueError(f"cases[{index}].visible_message_refs must not be empty")
    if not case.context_source_refs:
        raise ValueError(f"cases[{index}].context_source_refs must not be empty")
    if set(case.forbidden_message_refs).intersection(case.context_source_refs):
        raise ValueError(f"cases[{index}].context_source_refs includes forbidden refs")
    if case.memory_candidate_state not in _ALLOWED_MEMORY_STATES:
        raise ValueError(f"cases[{index}].memory_candidate_state unsupported")
    if case.tool_intent_state not in _ALLOWED_TOOL_STATES:
        raise ValueError(f"cases[{index}].tool_intent_state unsupported")
    if case.approval_state not in _ALLOWED_APPROVAL_STATES:
        raise ValueError(f"cases[{index}].approval_state unsupported")
    if case.action_state not in _ALLOWED_ACTION_STATES:
        raise ValueError(f"cases[{index}].action_state unsupported")
    if case.memory_candidate_state in _DIRECT_EXECUTION_STATES:
        raise ValueError(f"cases[{index}].memory candidate must not be ACTIVE")
    if case.tool_intent_state in _DIRECT_EXECUTION_STATES:
        raise ValueError(f"cases[{index}].ToolIntent must not execute side effects")
    if case.action_state in _DIRECT_EXECUTION_STATES:
        raise ValueError(f"cases[{index}].action fixture must not execute side effects")
    if not _fixture_ref(case.approval_fixture_ref):
        raise ValueError(f"cases[{index}].approval_fixture_ref must be fixture-only")
    if not _fixture_ref(case.action_fixture_ref):
        raise ValueError(f"cases[{index}].action_fixture_ref must be fixture-only")
    if case.expected_failure_class != "none" and not case.blocked_reason_refs:
        raise ValueError(f"cases[{index}].blocked_reason_refs required for blocked case")
    if case.case_kind and case.case_kind not in _CASE_KIND_TO_FAMILY:
        raise ValueError(f"cases[{index}].case_kind unsupported")


def _assert_no_forbidden_demo_fields(value: Any, path: str = "") -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            normalized = str(key).strip().lower()
            current_path = f"{path}.{normalized}" if path else normalized
            if normalized in _FORBIDDEN_DEMO_FIELDS:
                raise ValueError(f"forbidden interview demo field: {current_path}")
            if normalized in _PRODUCTION_CONNECTION_FIELDS and nested is True:
                raise ValueError(f"production backend connection is forbidden: {current_path}")
            _assert_no_forbidden_demo_fields(nested, current_path)
    elif isinstance(value, list):
        for index, nested in enumerate(value):
            _assert_no_forbidden_demo_fields(nested, f"{path}[{index}]")


def _assert_distinct_output_paths(paths: OutputPaths) -> None:
    output_paths = [
        ("summary-out", paths.summary_out),
        ("report-out", paths.report_out),
        ("replay-out", paths.replay_out),
    ]
    seen: dict[Path, str] = {}
    for label, path in output_paths:
        if path is None:
            continue
        resolved = path.resolve()
        if resolved in seen:
            raise ValueError(f"{label} must not overwrite {seen[resolved]}")
        seen[resolved] = label


def _expected_failure_class(value: Any) -> str:
    normalized = _string(value).upper()
    if normalized in _NO_FAILURE_VALUES:
        return "none"
    if normalized not in ALLOWED_FAILURE_CLASSES:
        raise ValueError(f"unsupported expected_failure_class: {normalized}")
    return normalized


def _eval_failure_class(value: str) -> str:
    return "" if value == "none" else value


def _required_string(payload: dict[str, Any], field_name: str) -> str:
    value = _string(payload.get(field_name, ""))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def _string(value: Any) -> str:
    return str(value).strip()


def _lower_string(value: Any) -> str:
    return _string(value).lower()


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


def _fixture_ref(value: str) -> bool:
    return "fixture" in value.lower()


def _unique(values: Any) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = _string(value)
        if normalized and normalized not in seen:
            seen.add(normalized)
            result.append(normalized)
    return result
