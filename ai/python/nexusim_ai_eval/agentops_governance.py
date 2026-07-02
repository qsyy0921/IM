"""Fixture-only AgentOps governance rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agentops_governance_rehearsal"

_EXPECTED_OWNER_BY_KIND = {
    "agent-definition": "agent-service+governance",
    "skill-package": "skill-registry",
    "agent-release": "governance+admin-workflow",
    "release-channel": "governance",
    "baseline-approval": "governance+ai-eval-service",
    "failure-class-owner": "governance",
    "kill-switch": "governance+control-plane",
    "rollback-plan": "governance",
    "canary-report": "governance+observability",
}

_RELEASE_DECISIONS = {"ALLOW", "BLOCK", "CONDITIONAL"}
_KILL_SWITCH_STATES = {"ACTIVE", "INACTIVE"}
_RUNNING_RUN_STRATEGIES = {"CANCEL", "DRAIN", "WORKFLOW_WAIT", "NONE"}
_BASELINE_DECISIONS = {"APPROVED", "REJECTED", "NEEDS_REVIEW"}
_FAILURE_SEVERITIES = {"P0", "P1", "P2", "P3"}
_FAILURE_STATES = {"OPEN", "CLOSED", "RETIRED"}
_OPERATOR_ACTIONS = {
    "RELEASE_INSPECT",
    "KILL_SWITCH_ACTIVATE",
    "ROLLBACK",
    "FAILURE_ASSIGN",
    "BASELINE_REVIEW",
    "CANARY_HOLD",
}


def load_agentops_governance_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive AgentOps governance rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load agentops governance rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("agentops governance rehearsal must be an object")
    return rehearse_agentops_governance(payload)


def rehearse_agentops_governance(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify release blocking, kill switch and governance refs."""

    assert_low_sensitive_eval_payload(payload, "agentops governance rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    ownership_results = _ownership_results(
        _record_list(payload.get("ownership_assertions", []), "ownership_assertions")
    )
    release_results = _release_gate_results(
        _record_list(payload.get("release_gate_records", []), "release_gate_records")
    )
    kill_switch_results = _kill_switch_results(
        _record_list(payload.get("kill_switch_records", []), "kill_switch_records")
    )
    baseline_results = _baseline_results(
        _record_list(payload.get("baseline_approval_records", []), "baseline_approval_records")
    )
    failure_results = _failure_owner_results(
        _record_list(payload.get("failure_class_records", []), "failure_class_records")
    )
    canary_results = _canary_results(
        _record_list(payload.get("canary_shadow_records", []), "canary_shadow_records")
    )
    operator_results = _operator_results(
        _record_list(payload.get("operator_control_records", []), "operator_control_records")
    )

    all_results = (
        ownership_results
        + release_results
        + kill_switch_results
        + baseline_results
        + failure_results
        + canary_results
        + operator_results
    )
    blocked_reasons = _blocked_reasons(all_results)
    if not release_results:
        blocked_reasons.append("release gate rehearsal records missing")
    if not kill_switch_results:
        blocked_reasons.append("kill switch rehearsal records missing")
    if not baseline_results:
        blocked_reasons.append("baseline approval rehearsal records missing")
    if not failure_results:
        blocked_reasons.append("failure-class owner rehearsal records missing")
    if not canary_results:
        blocked_reasons.append("canary shadow rehearsal records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "ownership_results": ownership_results,
        "release_gate_results": release_results,
        "kill_switch_results": kill_switch_results,
        "baseline_approval_results": baseline_results,
        "failure_class_results": failure_results,
        "canary_shadow_results": canary_results,
        "operator_control_results": operator_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "agentops governance rehearsal result")
    return result_payload


def _ownership_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        object_ref = _required_string(record, "object_ref")
        object_kind = _required_string(record, "object_kind")
        owner = _required_string(record, "owner")
        expected_owner = _EXPECTED_OWNER_BY_KIND.get(object_kind)
        forbidden_refs = set(
            _string_list(record.get("forbidden_state_refs", []), "forbidden_state_refs")
        )
        rejected_refs = set(
            _string_list(
                record.get("rejected_forbidden_state_refs", []),
                "rejected_forbidden_state_refs",
            )
        )

        status = "PASS"
        reason = ""
        if expected_owner is None:
            status = "FAIL"
            reason = f"unsupported governance object kind: {object_kind}"
        elif owner != expected_owner:
            status = "FAIL"
            reason = f"owner mismatch for {object_ref}: {owner} != {expected_owner}"
        elif not forbidden_refs.issubset(rejected_refs):
            status = "FAIL"
            reason = f"forbidden governance state refs not rejected: {object_ref}"
        elif _bool(record.get("python_worker_owns_decision"), default=False):
            status = "FAIL"
            reason = f"python worker owns governance decision: {object_ref}"
        elif _bool(record.get("model_output_owns_decision"), default=False):
            status = "FAIL"
            reason = f"model output owns governance decision: {object_ref}"
        results.append(_result(object_ref, status, reason))
    return results


def _release_gate_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        release_ref = _required_string(record, "release_ref")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")
        _required_string(record, "agent_definition_ref")
        skill_refs = _string_list(record.get("skill_package_refs", []), "skill_package_refs")
        _required_string(record, "owner_ref")
        _required_string(record, "eval_report_ref")
        _required_string(record, "replay_bundle_ref")
        _required_string(record, "baseline_approval_ref")
        _required_string(record, "rollback_ref")
        _required_string(record, "disable_switch_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if expected_decision not in _RELEASE_DECISIONS or actual_decision not in _RELEASE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported release gate decision: {release_ref}"
        elif actual_decision != expected_decision:
            status = "FAIL"
            reason = f"release gate decision mismatch: {release_ref}"
        elif not skill_refs:
            status = "FAIL"
            reason = f"release lacks skill package refs: {release_ref}"
        elif _bool(record.get("has_p0_p1_eval_failure"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"P0/P1 eval failure did not block release: {release_ref}"
        elif _bool(record.get("replay_gap"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"replay gap did not block release: {release_ref}"
        elif _bool(record.get("audit_gap"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"audit gap did not block release: {release_ref}"
        elif _bool(record.get("baseline_missing"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing baseline approval did not block release: {release_ref}"
        elif _bool(record.get("release_pinned"), default=False) is False:
            status = "FAIL"
            reason = f"release is not pinned: {release_ref}"
        elif _bool(record.get("production_enabled"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"non-allow release enabled production: {release_ref}"
        results.append(_result(release_ref, status, reason))
    return results


def _kill_switch_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        kill_switch_ref = _required_string(record, "kill_switch_ref")
        state = _upper_required_string(record, "switch_state")
        _required_string(record, "owner_ref")
        _required_string(record, "scope_kind")
        _required_string(record, "scope_ref")
        _required_string(record, "activation_reason_ref")
        _required_string(record, "audit_ref")
        _required_string(record, "rollback_ref")
        propagation_targets = _string_list(
            record.get("propagation_target_refs", []), "propagation_target_refs"
        )
        propagation_acks = set(
            _string_list(record.get("propagation_ack_refs", []), "propagation_ack_refs")
        )
        running_strategy = _upper_required_string(record, "running_run_strategy")

        status = "PASS"
        reason = ""
        if state not in _KILL_SWITCH_STATES:
            status = "FAIL"
            reason = f"unsupported kill switch state: {kill_switch_ref}"
        elif running_strategy not in _RUNNING_RUN_STRATEGIES:
            status = "FAIL"
            reason = f"unsupported running run strategy: {kill_switch_ref}"
        elif state == "ACTIVE":
            if _bool(record.get("new_runs_allowed"), default=False):
                status = "FAIL"
                reason = f"active kill switch allows new runs: {kill_switch_ref}"
            elif not propagation_targets:
                status = "FAIL"
                reason = f"active kill switch lacks propagation targets: {kill_switch_ref}"
            elif not set(propagation_targets).issubset(propagation_acks):
                status = "FAIL"
                reason = f"kill switch propagation missing ack: {kill_switch_ref}"
            elif running_strategy == "NONE":
                status = "FAIL"
                reason = f"active kill switch lacks running-run behavior: {kill_switch_ref}"
        if status == "PASS" and _bool(record.get("python_worker_owns_decision"), default=False):
            status = "FAIL"
            reason = f"python worker owns kill switch decision: {kill_switch_ref}"
        results.append(_result(kill_switch_ref, status, reason))
    return results


def _baseline_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        baseline_ref = _required_string(record, "baseline_ref")
        decision = _upper_required_string(record, "baseline_decision")
        _required_string(record, "approval_record_ref")
        _required_string(record, "approver_ref")
        _required_string(record, "eval_report_ref")
        _required_string(record, "dataset_manifest_ref")
        _required_string(record, "replay_reader_policy_ref")
        _required_string(record, "audit_ref")

        changed = any(
            _bool(record.get(field_name), default=False)
            for field_name in (
                "failure_classes_changed",
                "dataset_changed",
                "risk_tier_changed",
                "required_suites_changed",
            )
        )

        status = "PASS"
        reason = ""
        if decision not in _BASELINE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported baseline decision: {baseline_ref}"
        elif _bool(record.get("silent_refresh"), default=False):
            status = "FAIL"
            reason = f"baseline silently refreshed: {baseline_ref}"
        elif changed and not _bool(record.get("explicit_approval"), default=False):
            status = "FAIL"
            reason = f"changed baseline lacks explicit approval: {baseline_ref}"
        elif decision == "APPROVED" and _bool(record.get("has_p0_p1_regression"), default=False):
            status = "FAIL"
            reason = f"baseline approved with P0/P1 regression: {baseline_ref}"
        results.append(_result(baseline_ref, status, reason))
    return results


def _failure_owner_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        failure_class_ref = _required_string(record, "failure_class_ref")
        severity = _upper_required_string(record, "severity")
        state = _upper_required_string(record, "owner_workflow_state")
        owner_ref = _string(record.get("owner_ref"))
        _required_string(record, "first_seen_report_ref")
        _required_string(record, "replay_bundle_ref")
        _required_string(record, "audit_ref")
        fixture_ref = _string(record.get("regression_fixture_ref"))
        no_fixture_reason_ref = _string(record.get("no_fixture_reason_ref"))

        status = "PASS"
        reason = ""
        if severity not in _FAILURE_SEVERITIES:
            status = "FAIL"
            reason = f"unsupported failure severity: {failure_class_ref}"
        elif state not in _FAILURE_STATES:
            status = "FAIL"
            reason = f"unsupported failure owner state: {failure_class_ref}"
        elif severity in {"P0", "P1"} and not owner_ref:
            status = "FAIL"
            reason = f"P0/P1 failure lacks owner: {failure_class_ref}"
        elif severity in {"P0", "P1"} and not (fixture_ref or no_fixture_reason_ref):
            status = "FAIL"
            reason = f"P0/P1 failure lacks regression disposition: {failure_class_ref}"
        elif severity in {"P0", "P1"} and not _bool(
            record.get("release_blocking"),
            default=False,
        ):
            status = "FAIL"
            reason = f"P0/P1 failure is not release blocking: {failure_class_ref}"
        elif _bool(record.get("baseline_refresh_allowed"), default=False) and state == "OPEN":
            status = "FAIL"
            reason = f"open failure allows baseline refresh: {failure_class_ref}"
        results.append(_result(failure_class_ref, status, reason))
    return results


def _canary_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        canary_report_ref = _required_string(record, "canary_report_ref")
        _required_string(record, "release_ref")
        _required_string(record, "offline_baseline_ref")
        _required_string(record, "shadow_report_ref")
        _required_string(record, "audit_ref")
        metric_refs = set(_string_list(record.get("metric_refs", []), "metric_refs"))
        comparable_refs = set(
            _string_list(record.get("comparable_metric_refs", []), "comparable_metric_refs")
        )

        status = "PASS"
        reason = ""
        if not metric_refs:
            status = "FAIL"
            reason = f"canary lacks metric refs: {canary_report_ref}"
        elif not metric_refs.issubset(comparable_refs):
            status = "FAIL"
            reason = f"canary metrics not comparable to offline baseline: {canary_report_ref}"
        elif _bool(record.get("p0_p1_regression"), default=False):
            if not _string(record.get("rollback_or_hold_ref")):
                status = "FAIL"
                reason = f"canary regression lacks rollback or hold ref: {canary_report_ref}"
            elif _bool(record.get("promoted_to_production"), default=False):
                status = "FAIL"
                reason = f"canary P0/P1 regression promoted release: {canary_report_ref}"
        elif not _bool(record.get("shadow_only"), default=False) and not _string(
            record.get("canary_decision_ref")
        ):
            status = "FAIL"
            reason = f"canary decision missing: {canary_report_ref}"
        results.append(_result(canary_report_ref, status, reason))
    return results


def _operator_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        control_ref = _required_string(record, "control_ref")
        action = _upper_required_string(record, "action")
        visible_refs = _string_list(record.get("visible_refs", []), "visible_refs")
        _required_string(record, "operator_authority_ref")
        _required_string(record, "redaction_policy_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if action not in _OPERATOR_ACTIONS:
            status = "FAIL"
            reason = f"unsupported AgentOps operator action: {action}"
        elif not visible_refs:
            status = "FAIL"
            reason = f"AgentOps operator action lacks visible refs: {control_ref}"
        elif _bool(record.get("body_exposed"), default=False):
            status = "FAIL"
            reason = f"AgentOps operator action exposes body payload: {control_ref}"
        elif _bool(record.get("unauthorized_actor"), default=False):
            status = "FAIL"
            reason = f"unauthorized AgentOps operator action: {control_ref}"
        elif _bool(record.get("python_worker_override"), default=False):
            status = "FAIL"
            reason = f"python worker overrides AgentOps operator action: {control_ref}"
        results.append(_result(control_ref, status, reason))
    return results


def _blocked_reasons(results: list[dict[str, str]]) -> list[str]:
    return [result["reason"] for result in results if result["status"] != "PASS"]


def _result(record_ref: str, status: str, reason: str) -> dict[str, str]:
    return {"record_ref": record_ref, "status": status, "reason": reason}


def _record_list(value: Any, context: str) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        raise ValueError(f"{context} must be a list")
    records: list[dict[str, Any]] = []
    for index, item in enumerate(value):
        if not isinstance(item, dict):
            raise ValueError(f"{context}[{index}] must be an object")
        records.append(item)
    return records


def _required_string(payload: dict[str, Any], field_name: str) -> str:
    value = _string(payload.get(field_name))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def _upper_required_string(payload: dict[str, Any], field_name: str) -> str:
    return _required_string(payload, field_name).upper()


def _string(value: Any) -> str:
    if value is None:
        return ""
    if not isinstance(value, str):
        raise ValueError("expected string")
    return value.strip()


def _string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    refs: list[str] = []
    for index, item in enumerate(value):
        if not isinstance(item, str) or not item.strip():
            raise ValueError(f"{field_name}[{index}] must be a non-empty string")
        refs.append(item.strip())
    return refs


def _bool(value: Any, *, default: bool) -> bool:
    if value is None:
        return default
    if not isinstance(value, bool):
        raise ValueError("expected bool")
    return value
