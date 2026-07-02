"""Fixture-only cross-service preservation matrix rehearsal helpers."""

from __future__ import annotations

import json
from collections.abc import Collection
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_cross_service_preservation_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}

_BOUNDARY_EXPECTATIONS = {
    "retrieval_to_runtime": {
        "from_owner": "retrieval-gateway",
        "to_owner": "Agent Runtime",
        "required_roles": {
            "evidence_pack_refs",
            "source_refs",
            "visibility_version_refs",
            "denied_lane_refs",
            "taint_label_refs",
        },
    },
    "memory_to_runtime": {
        "from_owner": "memory-service",
        "to_owner": "Agent Runtime",
        "required_roles": {
            "memory_refs",
            "memory_scope_refs",
            "memory_version_refs",
            "memory_status_refs",
            "revocation_replacement_refs",
        },
    },
    "mcp_to_runtime": {
        "from_owner": "mcp-gateway",
        "to_owner": "Agent Runtime",
        "required_roles": {
            "prepared_tool_refs",
            "provider_refs",
            "schema_hash_refs",
            "lease_refs",
            "attestation_refs",
            "taint_label_refs",
        },
    },
    "workflow_to_runtime": {
        "from_owner": "workflow-service",
        "to_owner": "Agent Runtime",
        "required_roles": {
            "approval_decision_refs",
            "timeout_refs",
            "wakeup_refs",
            "resume_cancel_correlation_refs",
        },
    },
    "executor_to_eval_replay": {
        "from_owner": "action-executor",
        "to_owner": "Eval/Replay",
        "required_roles": {
            "execution_refs",
            "idempotency_refs",
            "state_diff_refs",
            "audit_refs",
            "repair_redrive_refs",
        },
    },
    "audit_to_agentops": {
        "from_owner": "audit-service",
        "to_owner": "AgentOps",
        "required_roles": {
            "archive_refs",
            "actor_refs",
            "policy_refs",
            "retention_refs",
        },
    },
}


def load_cross_service_preservation_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive cross-service preservation rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load cross-service preservation rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("cross-service preservation rehearsal must be an object")
    return rehearse_cross_service_preservation(payload)


def rehearse_cross_service_preservation(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify required refs survive cross-service Agent boundary handoffs."""

    assert_low_sensitive_eval_payload(payload, "cross-service preservation rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    boundary_results, failed_boundary_refs, seen_boundary_kinds = _boundary_results(
        _record_list(payload.get("boundary_preservation_records", []), "boundary_preservation_records")
    )
    coverage_results = _coverage_results(seen_boundary_kinds)
    gate_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records"),
        failed_boundary_refs,
    )

    all_results = boundary_results + coverage_results + gate_results
    blocked_reasons = _blocked_reasons(all_results)
    if not boundary_results:
        blocked_reasons.append("cross-service boundary preservation records missing")
    if not gate_results:
        blocked_reasons.append("cross-service preservation promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "boundary_results": boundary_results,
        "coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "cross-service preservation rehearsal result",
    )
    return result_payload


def _boundary_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], set[str], set[str]]:
    results: list[dict[str, str]] = []
    failed_boundary_refs: set[str] = set()
    seen_boundary_kinds: set[str] = set()
    for record in records:
        boundary_ref = _required_string(record, "boundary_ref")
        boundary_kind = _required_string(record, "boundary_kind")
        from_owner = _required_string(record, "from_owner")
        to_owner = _required_string(record, "to_owner")
        expectation = _BOUNDARY_EXPECTATIONS.get(boundary_kind)
        expected_ref_roles = _role_map(record.get("expected_ref_roles", {}), "expected_ref_roles")
        actual_ref_roles = _role_map(record.get("actual_ref_roles", {}), "actual_ref_roles")

        status = "PASS"
        reason = ""
        if expectation is None:
            status = "FAIL"
            reason = f"unsupported preservation boundary kind: {boundary_kind}"
        else:
            seen_boundary_kinds.add(boundary_kind)
            if from_owner != expectation["from_owner"] or to_owner != expectation["to_owner"]:
                status = "FAIL"
                reason = f"preservation boundary owner mismatch: {boundary_ref}"
            else:
                reason = _ref_role_blocker(
                    boundary_ref,
                    expectation["required_roles"],
                    expected_ref_roles,
                    actual_ref_roles,
                )
                if reason:
                    status = "FAIL"

        if status == "PASS":
            reason = _metadata_blocker(record, boundary_ref)
            if reason:
                status = "FAIL"

        results.append(_result(boundary_ref, status, reason))
        if status != "PASS":
            failed_boundary_refs.add(boundary_ref)
    return results, failed_boundary_refs, seen_boundary_kinds


def _ref_role_blocker(
    boundary_ref: str,
    required_roles: Collection[str],
    expected_ref_roles: dict[str, list[str]],
    actual_ref_roles: dict[str, list[str]],
) -> str:
    for role_name in sorted(required_roles):
        expected_refs = expected_ref_roles.get(role_name, [])
        actual_refs = actual_ref_roles.get(role_name, [])
        if not expected_refs:
            return f"boundary lacks required {role_name}: {boundary_ref}"
        missing_refs = sorted(set(expected_refs) - set(actual_refs))
        if missing_refs:
            return f"boundary dropped required {role_name}: {boundary_ref}"
    return ""


def _metadata_blocker(record: dict[str, Any], boundary_ref: str) -> str:
    _required_string(record, "compatibility_window_ref")
    _required_string(record, "replay_reader_policy_ref")
    _required_string(record, "redaction_policy_ref")
    _required_string(record, "taint_policy_ref")
    scope_blocker = _subset_blocker(record, "scope_refs", boundary_ref)
    if scope_blocker:
        return scope_blocker
    version_blocker = _subset_blocker(record, "version_refs", boundary_ref)
    if version_blocker:
        return version_blocker
    taint_blocker = _subset_blocker(record, "taint_label_refs", boundary_ref)
    if taint_blocker:
        return taint_blocker
    audit_blocker = _subset_blocker(record, "audit_lineage_refs", boundary_ref)
    if audit_blocker:
        return audit_blocker
    if _bool(record.get("scope_widened"), default=False):
        return f"boundary widened scope: {boundary_ref}"
    if _bool(record.get("version_downgraded"), default=False):
        return f"boundary downgraded version: {boundary_ref}"
    if _bool(record.get("taint_policy_changed"), default=False):
        return f"boundary changed taint policy: {boundary_ref}"
    if _bool(record.get("audit_lineage_missing"), default=False):
        return f"boundary lost audit lineage: {boundary_ref}"
    if _bool(record.get("raw_payload_exposed"), default=False):
        return f"boundary exposed raw payload: {boundary_ref}"
    if _bool(record.get("production_data_used"), default=False):
        return f"boundary used production data: {boundary_ref}"
    if _bool(record.get("side_effect_reexecuted"), default=False):
        return f"boundary re-executed side effect: {boundary_ref}"
    return ""


def _subset_blocker(record: dict[str, Any], suffix: str, boundary_ref: str) -> str:
    expected_refs = set(
        _string_list(record.get(f"expected_{suffix}", []), f"expected_{suffix}")
    )
    actual_refs = set(_string_list(record.get(f"actual_{suffix}", []), f"actual_{suffix}"))
    if not expected_refs:
        return f"boundary lacks expected {suffix}: {boundary_ref}"
    if not expected_refs.issubset(actual_refs):
        return f"boundary dropped {suffix}: {boundary_ref}"
    return ""


def _coverage_results(seen_boundary_kinds: set[str]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for boundary_kind in sorted(_BOUNDARY_EXPECTATIONS):
        if boundary_kind in seen_boundary_kinds:
            results.append(_result(boundary_kind, "PASS", ""))
        else:
            results.append(
                _result(
                    boundary_kind,
                    "FAIL",
                    f"required boundary preservation record missing: {boundary_kind}",
                )
            )
    return results


def _promotion_gate_results(
    records: list[dict[str, Any]],
    failed_boundary_refs: set[str],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        gate_ref = _required_string(record, "gate_ref")
        boundary_ref = _required_string(record, "boundary_ref")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")
        _required_string(record, "audit_ref")
        _required_string(record, "review_ref")

        status = "PASS"
        reason = ""
        if expected_decision not in _GATE_DECISIONS or actual_decision not in _GATE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported preservation gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"preservation gate decision mismatch: {gate_ref}"
        elif failed_boundary_refs and actual_decision == "ALLOW":
            failed_ref = sorted(failed_boundary_refs)[0]
            status = "FAIL"
            reason = f"failed preservation boundary not blocked: {failed_ref}"
        elif boundary_ref in failed_boundary_refs and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"failed preservation boundary not blocked: {boundary_ref}"
        elif _bool(record.get("missing_required_boundary"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing preservation boundary did not block promotion: {gate_ref}"
        elif _bool(record.get("dropped_required_ref"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"dropped preservation ref did not block promotion: {gate_ref}"
        elif _bool(record.get("scope_widened"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"scope widening did not block promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked preservation gate allowed release: {gate_ref}"
        results.append(_result(gate_ref, status, reason))
    return results


def _blocked_reasons(results: list[dict[str, str]]) -> list[str]:
    return [result["reason"] for result in results if result["status"] != "PASS"]


def _result(record_ref: str, status: str, reason: str) -> dict[str, str]:
    return {"record_ref": record_ref, "status": status, "reason": reason}


def _role_map(value: Any, field_name: str) -> dict[str, list[str]]:
    if not isinstance(value, dict):
        raise ValueError(f"{field_name} must be an object")
    result: dict[str, list[str]] = {}
    for role_name, refs in value.items():
        normalized_role = _string(role_name)
        if not normalized_role:
            raise ValueError(f"{field_name} contains empty role")
        result[normalized_role] = _string_list(refs, f"{field_name}.{normalized_role}")
    return result


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
