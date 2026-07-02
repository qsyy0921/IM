"""Fixture-only production object completeness rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_object_completeness_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}

_EXPECTED_OBJECT_NAMES = {
    "AgentCheckpoint",
    "AgentDefinition",
    "AgentRelease",
    "AgentReleaseConsole",
    "AgentRunStore",
    "ApprovalConsole",
    "ApprovalDecision",
    "ApprovalRequest",
    "ArchitectureDecision",
    "BaselineApproval",
    "BaselineReport",
    "BlockedPromotionReason",
    "BudgetLedger",
    "CanaryReport",
    "CancelToken",
    "CapabilityLease",
    "CitationMap",
    "CitationVerifierResult",
    "CompatibilityWindow",
    "CompensationPlan",
    "ConflictSet",
    "ContractVersion",
    "DatasetManifest",
    "DatasetSnapshot",
    "DatasetSplit",
    "DecisionReview",
    "DeniedLane",
    "DeprecationPolicy",
    "EvalSuiteManifest",
    "EvidenceCoverageReport",
    "EvidenceInspectView",
    "ExecutionIntent",
    "ExecutionReceipt",
    "FailureClassOwner",
    "FailureReviewQueue",
    "GroupConsensus",
    "KillSwitch",
    "KnowledgeState",
    "MemoryAdmissionDecision",
    "MemoryClaim",
    "MemoryInspectView",
    "MemoryReviewTask",
    "MemoryRevocationLedger",
    "MemoryScope",
    "MemorySupersessionChain",
    "MemoryVersion",
    "MigrationPolicy",
    "PreparedToolRef",
    "ProviderAttestation",
    "RedriveRequest",
    "RejectionCondition",
    "RelationMemory",
    "ReleaseChannel",
    "RepairRequest",
    "ReplayIndex",
    "ReplayReaderPolicy",
    "ReplayView",
    "RegressionDelta",
    "ResumeToken",
    "RollbackPlan",
    "RuntimeWakeup",
    "SkillPackage",
    "SourceVisibilityVersion",
    "StateDiffReport",
    "TaintLabel",
    "ToolExecutionPolicy",
    "ToolOutputEnvelope",
    "ToolProvider",
    "ToolRiskTier",
    "ToolSchemaHash",
}


def load_object_completeness_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive production object completeness rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load object completeness rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("object completeness rehearsal must be an object")
    return rehearse_object_completeness(payload)


def rehearse_object_completeness(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify conceptual Agent production objects have promotion dimensions."""

    assert_low_sensitive_eval_payload(payload, "object completeness rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    group_results, failed_group_refs, object_occurrences = _object_group_results(
        _record_list(payload.get("object_group_records", []), "object_group_records")
    )
    coverage_results = _object_coverage_results(object_occurrences)
    gate_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records"),
        failed_group_refs,
        coverage_results,
    )

    all_results = group_results + coverage_results + gate_results
    blocked_reasons = _blocked_reasons(all_results)
    if not group_results:
        blocked_reasons.append("object group records missing")
    if not gate_results:
        blocked_reasons.append("object completeness promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "object_group_results": group_results,
        "object_coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "object completeness rehearsal result")
    return result_payload


def _object_group_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], set[str], dict[str, list[str]]]:
    results: list[dict[str, str]] = []
    failed_group_refs: set[str] = set()
    object_occurrences: dict[str, list[str]] = {}
    for record in records:
        group_ref = _required_string(record, "object_group_ref")
        object_names = _string_list(record.get("object_names", []), "object_names")
        owner_ref = _required_string(record, "owner_ref")
        _required_string(record, "domain_ref")
        _required_string(record, "adr_candidate_ref")
        _required_string(record, "lifecycle_ref")
        _required_string(record, "version_policy_ref")
        _required_string(record, "compatibility_window_ref")
        _required_string(record, "permission_boundary_ref")
        _required_string(record, "audit_boundary_ref")
        _required_string(record, "replay_behavior_ref")
        _required_string(record, "redaction_policy_ref")
        _required_string(record, "operator_view_ref")
        _required_string(record, "evidence_ref")
        _string_list(record.get("consumer_refs", []), "consumer_refs")
        _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs")
        required_owner_fragments = _string_list(
            record.get("required_owner_fragments", []),
            "required_owner_fragments",
        )

        for object_name in object_names:
            object_occurrences.setdefault(object_name, []).append(group_ref)

        reason = _group_blocker(record, group_ref, object_names, owner_ref, required_owner_fragments)
        status = "FAIL" if reason else "PASS"
        results.append(_result(group_ref, status, reason))
        if reason:
            failed_group_refs.add(group_ref)
    return results, failed_group_refs, object_occurrences


def _group_blocker(
    record: dict[str, Any],
    group_ref: str,
    object_names: list[str],
    owner_ref: str,
    required_owner_fragments: list[str],
) -> str:
    if not object_names:
        return f"object group has no objects: {group_ref}"
    unknown_objects = sorted(set(object_names) - _EXPECTED_OBJECT_NAMES)
    if unknown_objects:
        return f"object group contains unknown objects: {group_ref}"
    owner_lower = owner_ref.lower()
    missing_owner_fragments = [
        fragment for fragment in required_owner_fragments if fragment.lower() not in owner_lower
    ]
    if missing_owner_fragments:
        return f"object group owner lacks required fragment: {group_ref}"
    if _bool(record.get("production_contract_authorized"), default=False):
        return f"object group authorizes production contract: {group_ref}"
    if _bool(record.get("durable_state"), default=False) and "python" in owner_lower:
        return f"python owns durable production object group: {group_ref}"
    if _bool(record.get("active_memory_truth"), default=False) and "memory-service" not in owner_lower:
        return f"active memory object group is not memory-service owned: {group_ref}"
    if _bool(record.get("approval_truth"), default=False) and "workflow-service" not in owner_lower:
        return f"approval object group is not workflow-service owned: {group_ref}"
    if _bool(record.get("execution_truth"), default=False) and "action-executor" not in owner_lower:
        return f"execution object group is not action-executor owned: {group_ref}"
    if _bool(record.get("audit_archive_truth"), default=False) and "audit-service" not in owner_lower:
        return f"audit archive object group is not audit-service owned: {group_ref}"
    return ""


def _object_coverage_results(object_occurrences: dict[str, list[str]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for object_name in sorted(_EXPECTED_OBJECT_NAMES):
        group_refs = object_occurrences.get(object_name, [])
        if not group_refs:
            results.append(
                _result(object_name, "FAIL", f"production object missing coverage: {object_name}")
            )
        elif len(group_refs) > 1:
            results.append(
                _result(object_name, "FAIL", f"production object has duplicate coverage: {object_name}")
            )
        else:
            results.append(_result(object_name, "PASS", ""))
    extra_objects = sorted(set(object_occurrences) - _EXPECTED_OBJECT_NAMES)
    for object_name in extra_objects:
        results.append(
            _result(object_name, "FAIL", f"unexpected production object coverage: {object_name}")
        )
    return results


def _promotion_gate_results(
    records: list[dict[str, Any]],
    failed_group_refs: set[str],
    coverage_results: list[dict[str, str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    failed_coverage = [result for result in coverage_results if result["status"] != "PASS"]
    for record in records:
        gate_ref = _required_string(record, "gate_ref")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")
        _required_string(record, "review_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if expected_decision not in _GATE_DECISIONS or actual_decision not in _GATE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported object completeness gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"object completeness gate decision mismatch: {gate_ref}"
        elif (failed_group_refs or failed_coverage) and actual_decision == "ALLOW":
            status = "FAIL"
            reason = f"failed object completeness evidence not blocked: {gate_ref}"
        elif _bool(record.get("missing_object"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing object did not block promotion: {gate_ref}"
        elif _bool(record.get("missing_dimension"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing object dimension did not block promotion: {gate_ref}"
        elif _bool(record.get("invalid_owner"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"invalid object owner did not block promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked object completeness gate allowed release: {gate_ref}"
        results.append(_result(gate_ref, status, reason))
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
