"""Fixture-only Agent contract-version compatibility rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_contract_version_compatibility_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}

_EXPECTED_CONTRACT_NAMES = {
    "ApprovalDecision",
    "ContextPackage",
    "EvalReport",
    "EvidencePack",
    "ExecutionReceipt",
    "MemoryCandidate",
    "MemoryClaim",
    "PreparedToolRef",
    "ReplayBundle",
    "ToolIntent",
}

_REQUIRED_REF_FIELDS = (
    "contract_ref",
    "contract_name",
    "producer_owner",
    "current_version_ref",
    "previous_version_ref",
    "compatibility_window_ref",
    "replay_reader_policy_ref",
    "redaction_policy_ref",
    "deprecation_policy_ref",
    "migration_or_backfill_policy_ref",
    "preservation_matrix_ref",
    "audit_ref",
    "operator_ref",
    "eval_gate_ref",
)

_BLOCKER_FIELDS = (
    ("missing_compatibility_window", "contract lacks compatibility window"),
    ("missing_replay_reader_policy", "contract lacks replay reader policy"),
    ("missing_redaction_policy", "contract lacks redaction policy"),
    ("missing_deprecation_policy", "contract lacks deprecation policy"),
    ("missing_migration_policy", "contract lacks migration/backfill policy"),
    ("missing_preservation_matrix", "contract lacks preservation matrix"),
    ("unsupported_reader_version", "contract reader version unsupported"),
    ("body_archive_required", "contract reader requires archived body"),
    ("inline_content_retained", "contract retains inline content for normal replay"),
    ("required_ref_removed", "contract removes required preservation ref"),
    ("scope_widened", "contract version widens scope"),
    ("taint_dropped", "contract version drops taint"),
    ("audit_lineage_dropped", "contract version drops audit lineage"),
    ("python_final_owner", "Python owns final contract state"),
    ("production_contract_authorized", "fixture authorizes production contract"),
)


def load_contract_version_compatibility_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive contract-version compatibility rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load contract-version compatibility rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("contract-version compatibility rehearsal must be an object")
    return rehearse_contract_version_compatibility(payload)


def rehearse_contract_version_compatibility(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify future Agent contracts declare compatibility and replay policy refs."""

    assert_low_sensitive_eval_payload(payload, "contract-version compatibility rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    contract_results, failed_contract_refs, contract_occurrences = _contract_results(
        _record_list(payload.get("contract_records", []), "contract_records")
    )
    coverage_results = _coverage_results(contract_occurrences)
    gate_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records"),
        failed_contract_refs,
        coverage_results,
    )

    all_results = contract_results + coverage_results + gate_results
    blocked_reasons = _blocked_reasons(all_results)
    if not contract_results:
        blocked_reasons.append("contract-version compatibility records missing")
    if not gate_results:
        blocked_reasons.append("contract-version compatibility promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "contract_results": contract_results,
        "coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "contract-version compatibility rehearsal result",
    )
    return result_payload


def _contract_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], set[str], dict[str, list[str]]]:
    results: list[dict[str, str]] = []
    failed_contract_refs: set[str] = set()
    contract_occurrences: dict[str, list[str]] = {}
    for record in records:
        contract_ref = _required_string(record, "contract_ref")
        contract_name = _required_string(record, "contract_name")
        for field_name in _REQUIRED_REF_FIELDS:
            _required_string(record, field_name)
        if not _string_list(record.get("consumer_owners", []), "consumer_owners"):
            raise ValueError("consumer_owners must not be empty")
        if not _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs"):
            raise ValueError("rejection_condition_refs must not be empty")

        contract_occurrences.setdefault(contract_name, []).append(contract_ref)
        reason = _contract_blocker(record, contract_ref, contract_name)
        status = "FAIL" if reason else "PASS"
        results.append(_result(contract_ref, status, reason))
        if reason:
            failed_contract_refs.add(contract_ref)
    return results, failed_contract_refs, contract_occurrences


def _contract_blocker(record: dict[str, Any], contract_ref: str, contract_name: str) -> str:
    if contract_name not in _EXPECTED_CONTRACT_NAMES:
        return f"unsupported contract-version target: {contract_name}"
    for field_name, reason in _BLOCKER_FIELDS:
        if _bool(record.get(field_name), default=False):
            return f"{reason}: {contract_ref}"
    return ""


def _coverage_results(contract_occurrences: dict[str, list[str]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for contract_name in sorted(_EXPECTED_CONTRACT_NAMES):
        contract_refs = contract_occurrences.get(contract_name, [])
        if not contract_refs:
            results.append(
                _result(
                    contract_name,
                    "FAIL",
                    f"required contract-version target missing: {contract_name}",
                )
            )
        elif len(contract_refs) > 1:
            results.append(
                _result(
                    contract_name,
                    "FAIL",
                    f"duplicate contract-version target coverage: {contract_name}",
                )
            )
        else:
            results.append(_result(contract_name, "PASS", ""))
    for contract_name in sorted(set(contract_occurrences) - _EXPECTED_CONTRACT_NAMES):
        results.append(
            _result(
                contract_name,
                "FAIL",
                f"unexpected contract-version target coverage: {contract_name}",
            )
        )
    return results


def _promotion_gate_results(
    records: list[dict[str, Any]],
    failed_contract_refs: set[str],
    coverage_results: list[dict[str, str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    failed_coverage = [result for result in coverage_results if result["status"] != "PASS"]
    for record in records:
        gate_ref = _required_string(record, "gate_ref")
        contract_ref = _required_string(record, "contract_ref")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")
        _required_string(record, "review_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if expected_decision not in _GATE_DECISIONS or actual_decision not in _GATE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported contract-version gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"contract-version gate decision mismatch: {gate_ref}"
        elif (failed_contract_refs or failed_coverage) and actual_decision == "ALLOW":
            status = "FAIL"
            reason = f"failed contract-version compatibility evidence not blocked: {gate_ref}"
        elif contract_ref in failed_contract_refs and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"failed contract-version target not blocked: {contract_ref}"
        elif _bool(record.get("missing_contract_target"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing contract-version target did not block promotion: {gate_ref}"
        elif _bool(record.get("missing_compatibility_window"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing compatibility window did not block promotion: {gate_ref}"
        elif _bool(record.get("missing_replay_reader_policy"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing replay reader policy did not block promotion: {gate_ref}"
        elif _bool(record.get("reader_requires_body_archive"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"body-archive replay requirement did not block promotion: {gate_ref}"
        elif _bool(record.get("removed_required_ref"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"removed required ref did not block promotion: {gate_ref}"
        elif _bool(record.get("production_contract_authorized"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"fixture production contract authorization did not block promotion: {gate_ref}"
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
