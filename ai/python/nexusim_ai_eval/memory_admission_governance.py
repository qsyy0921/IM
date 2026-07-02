"""Fixture-only memory admission governance rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_memory_admission_governance_rehearsal"

_EXPECTED_OWNER_BY_KIND = {
    "python-candidate": "Python AI Worker",
    "memory-candidate": "memory-service",
    "admission-decision": "memory-service",
    "active-memory": "memory-service",
    "review-task": "memory-service+workflow-service",
    "revocation-ledger": "memory-service+audit-service",
    "retrieval-eligibility": "memory-service",
    "admission-explanation": "memory-service+audit-service",
}

_CATEGORY_REQUIRED_REFS = {
    "PERSONAL": {
        "source_refs",
        "scope_ref",
        "subject_ref",
        "confirmation_or_repeated_evidence_ref",
    },
    "GROUP": {
        "source_refs",
        "scope_ref",
        "group_ref",
        "speaker_refs",
        "audience_refs",
        "membership_window_ref",
        "decision_or_confirmation_ref",
    },
    "PROJECT": {
        "source_refs",
        "scope_ref",
        "project_ref",
        "supersedes_or_conflict_ref",
        "review_policy_ref",
    },
    "PROCEDURAL": {
        "source_refs",
        "scope_ref",
        "skill_version_ref",
        "migration_or_invalidation_ref",
    },
    "POLICY": {
        "source_refs",
        "scope_ref",
        "governed_policy_source_ref",
        "policy_owner_ref",
    },
}

_ADMISSION_OUTCOMES = {"ACTIVE", "REJECTED", "NEEDS_REVIEW", "QUARANTINED"}
_MEMORY_STATES = {
    "CANDIDATE",
    "REJECTED",
    "NEEDS_REVIEW",
    "ACTIVE",
    "SUPERSEDED",
    "REVOKED",
    "EXPIRED",
    "QUARANTINED",
}
_OPERATOR_ACTIONS = {"REVIEW", "CORRECT", "REVOKE", "FORGET", "INSPECT"}


def load_memory_admission_governance_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive memory admission governance rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load memory admission governance rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("memory admission governance rehearsal must be an object")
    return rehearse_memory_admission_governance(payload)


def rehearse_memory_admission_governance(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify memory admission ownership, thresholds, revocation and UX refs."""

    assert_low_sensitive_eval_payload(payload, "memory admission governance rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    ownership_results = _ownership_results(
        _record_list(payload.get("ownership_assertions", []), "ownership_assertions")
    )
    category_results = _category_threshold_results(
        _record_list(payload.get("category_threshold_records", []), "category_threshold_records")
    )
    revocation_results = _revocation_results(
        _record_list(payload.get("revocation_records", []), "revocation_records")
    )
    retrieval_results = _retrieval_results(
        _record_list(payload.get("retrieval_eligibility_records", []), "retrieval_eligibility_records")
    )
    explanation_results = _admission_explanation_results(
        _record_list(
            payload.get("admission_explanation_records", []),
            "admission_explanation_records",
        )
    )
    operator_results = _operator_memory_results(
        _record_list(payload.get("operator_memory_records", []), "operator_memory_records")
    )

    all_results = (
        ownership_results
        + category_results
        + revocation_results
        + retrieval_results
        + explanation_results
        + operator_results
    )
    blocked_reasons = _blocked_reasons(all_results)
    if not category_results:
        blocked_reasons.append("category threshold rehearsal records missing")
    if not revocation_results:
        blocked_reasons.append("revocation rehearsal records missing")
    if not retrieval_results:
        blocked_reasons.append("retrieval eligibility rehearsal records missing")
    if not explanation_results:
        blocked_reasons.append("admission explanation rehearsal records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "ownership_results": ownership_results,
        "category_threshold_results": category_results,
        "revocation_results": revocation_results,
        "retrieval_eligibility_results": retrieval_results,
        "admission_explanation_results": explanation_results,
        "operator_memory_results": operator_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "memory admission governance rehearsal result",
    )
    return result_payload


def _ownership_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        object_ref = _required_string(record, "object_ref")
        object_kind = _required_string(record, "object_kind")
        owner = _required_string(record, "owner")
        expected_owner = _EXPECTED_OWNER_BY_KIND.get(object_kind)
        forbidden_refs = set(_string_list(record.get("forbidden_state_refs", []), "forbidden_state_refs"))
        rejected_refs = set(
            _string_list(record.get("rejected_forbidden_state_refs", []), "rejected_forbidden_state_refs")
        )

        status = "PASS"
        reason = ""
        if expected_owner is None:
            status = "FAIL"
            reason = f"unsupported memory object kind: {object_kind}"
        elif owner != expected_owner:
            status = "FAIL"
            reason = f"owner mismatch for {object_ref}: {owner} != {expected_owner}"
        elif _bool(record.get("python_made_active_decision"), default=False):
            status = "FAIL"
            reason = f"python made active memory decision: {object_ref}"
        elif not forbidden_refs.issubset(rejected_refs):
            status = "FAIL"
            reason = f"forbidden memory state refs not rejected: {object_ref}"
        results.append(_result(object_ref, status, reason))
    return results


def _category_threshold_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        candidate_ref = _required_string(record, "candidate_ref")
        category = _upper_required_string(record, "category")
        expected_outcome = _upper_required_string(record, "expected_outcome")
        actual_outcome = _upper_required_string(record, "actual_outcome")
        owner = _required_string(record, "decision_owner")
        threshold_policy_ref = _required_string(record, "threshold_policy_ref")
        _required_string(record, "audit_ref")
        present_refs = set(_string_list(record.get("present_refs", []), "present_refs"))
        required_refs = _CATEGORY_REQUIRED_REFS.get(category)

        status = "PASS"
        reason = ""
        if required_refs is None:
            status = "FAIL"
            reason = f"unsupported memory category: {category}"
        elif owner != "memory-service":
            status = "FAIL"
            reason = f"memory decision owner mismatch: {candidate_ref}"
        elif expected_outcome not in _ADMISSION_OUTCOMES or actual_outcome not in _ADMISSION_OUTCOMES:
            status = "FAIL"
            reason = f"unsupported memory admission outcome: {candidate_ref}"
        elif actual_outcome != expected_outcome:
            status = "FAIL"
            reason = f"memory admission outcome mismatch: {candidate_ref}"
        elif not required_refs.issubset(present_refs):
            missing = ",".join(sorted(required_refs - present_refs))
            status = "FAIL"
            reason = f"missing category threshold refs for {candidate_ref}: {missing}"
        elif not threshold_policy_ref.startswith("memory-threshold:"):
            status = "FAIL"
            reason = f"invalid threshold policy ref: {candidate_ref}"
        elif actual_outcome == "ACTIVE" and _bool(record.get("review_required"), default=False):
            if not _string(record.get("review_decision_ref")):
                status = "FAIL"
                reason = f"active memory lacks required review decision: {candidate_ref}"
        results.append(_result(candidate_ref, status, reason))
    return results


def _revocation_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        revocation_ref = _required_string(record, "revocation_ref")
        affected_refs = set(_string_list(record.get("affected_memory_refs", []), "affected_memory_refs"))
        invalidated_refs = set(
            _string_list(record.get("actual_dependency_invalidation_refs", []), "actual_dependency_invalidation_refs")
        )
        _required_string(record, "authority_decision_ref")
        _required_string(record, "retrieval_change_ref")
        _required_string(record, "retention_policy_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if not affected_refs:
            status = "FAIL"
            reason = f"revocation lacks affected memory refs: {revocation_ref}"
        elif _bool(record.get("retrieved_after_revocation"), default=False):
            status = "FAIL"
            reason = f"revoked memory retrieved: {revocation_ref}"
        elif not affected_refs.issubset(invalidated_refs):
            status = "FAIL"
            reason = f"revocation did not invalidate dependent memories: {revocation_ref}"
        elif _bool(record.get("body_retained"), default=False):
            status = "FAIL"
            reason = f"revocation retained body payload: {revocation_ref}"
        results.append(_result(revocation_ref, status, reason))
    return results


def _retrieval_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        memory_ref = _required_string(record, "memory_ref")
        state = _upper_required_string(record, "lifecycle_state")
        retrieval_eligible = _bool(record.get("retrieval_eligible"), default=False)
        expected_retrieval_eligible = _bool(
            record.get("expected_retrieval_eligible"),
            default=False,
        )
        _required_string(record, "scope_ref")
        _required_string(record, "version_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if state not in _MEMORY_STATES:
            status = "FAIL"
            reason = f"unsupported memory lifecycle state: {state}"
        elif retrieval_eligible != expected_retrieval_eligible:
            status = "FAIL"
            reason = f"memory retrieval eligibility mismatch: {memory_ref}"
        elif state != "ACTIVE" and retrieval_eligible:
            status = "FAIL"
            reason = f"non-active memory is retrieval eligible: {memory_ref}"
        elif _bool(record.get("source_revoked"), default=False) and retrieval_eligible:
            status = "FAIL"
            reason = f"revoked source memory is retrieval eligible: {memory_ref}"
        elif _bool(record.get("source_unavailable_fallback"), default=False):
            status = "FAIL"
            reason = f"memory used as source-service fallback: {memory_ref}"
        results.append(_result(memory_ref, status, reason))
    return results


def _admission_explanation_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        decision_ref = _required_string(record, "decision_ref")
        source_refs = _string_list(record.get("source_refs", []), "source_refs")
        _required_string(record, "candidate_hash_ref")
        _required_string(record, "scope_ref")
        _required_string(record, "confidence_ref")
        _required_string(record, "calibration_ref")
        _required_string(record, "reason_ref")
        _required_string(record, "audit_ref")
        decision_outcome = _upper_required_string(record, "decision_outcome")

        status = "PASS"
        reason = ""
        if decision_outcome not in _ADMISSION_OUTCOMES:
            status = "FAIL"
            reason = f"unsupported explanation outcome: {decision_ref}"
        elif not source_refs:
            status = "FAIL"
            reason = f"admission explanation lacks source refs: {decision_ref}"
        elif _bool(record.get("raw_text_required"), default=False):
            status = "FAIL"
            reason = f"admission explanation requires raw text: {decision_ref}"
        elif decision_outcome == "ACTIVE" and _bool(
            record.get("review_required"),
            default=False,
        ):
            if not _string(record.get("review_decision_ref")):
                status = "FAIL"
                reason = f"active explanation lacks review decision: {decision_ref}"
        results.append(_result(decision_ref, status, reason))
    return results


def _operator_memory_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        control_ref = _required_string(record, "control_ref")
        action = _upper_required_string(record, "action")
        visible_refs = _string_list(record.get("visible_refs", []), "visible_refs")
        _required_string(record, "redaction_policy_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if action not in _OPERATOR_ACTIONS:
            status = "FAIL"
            reason = f"unsupported memory operator action: {action}"
        elif not visible_refs:
            status = "FAIL"
            reason = f"memory operator action lacks visible refs: {control_ref}"
        elif _bool(record.get("body_exposed"), default=False):
            status = "FAIL"
            reason = f"memory operator action exposes body payload: {control_ref}"
        elif _bool(record.get("python_override"), default=False):
            status = "FAIL"
            reason = f"python overrides memory operator action: {control_ref}"
        elif action in {"CORRECT", "REVOKE", "FORGET"} and not _string(
            record.get("authority_decision_ref")
        ):
            status = "FAIL"
            reason = f"memory operator action lacks authority decision: {control_ref}"
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
