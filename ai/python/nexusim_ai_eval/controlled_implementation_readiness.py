"""Fixture-only Agent controlled implementation readiness gate helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_controlled_implementation_readiness_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}
_IMPLEMENTATION_PHASES = {
    "fixture-only-hardening",
    "adr-candidate-review",
    "controlled-implementation",
    "production-contract",
}
_SCENARIO_KINDS = {
    "fixture-only-hardening-allowed",
    "controlled-implementation-blocked",
    "production-contract-blocked",
    "unsafe-shortcut-blocked",
}


def load_controlled_implementation_readiness_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive controlled implementation readiness rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load controlled implementation readiness rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("controlled implementation readiness rehearsal must be an object")
    return rehearse_controlled_implementation_readiness(payload)


def rehearse_controlled_implementation_readiness(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify that implementation promotion gates fail closed before accepted ADRs."""

    assert_low_sensitive_eval_payload(payload, "controlled implementation readiness rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    readiness_results, scenario_occurrences = _readiness_results(
        _record_list(payload.get("readiness_gate_records", []), "readiness_gate_records")
    )
    coverage_results = _scenario_coverage_results(scenario_occurrences)

    all_results = readiness_results + coverage_results
    blocked_reasons = _blocked_reasons(all_results)
    if not readiness_results:
        blocked_reasons.append("controlled implementation readiness records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "readiness_results": readiness_results,
        "scenario_coverage_results": coverage_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "controlled implementation readiness rehearsal result",
    )
    return result_payload


def _readiness_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], dict[str, list[str]]]:
    results: list[dict[str, str]] = []
    scenario_occurrences: dict[str, list[str]] = {}
    for record in records:
        gate_ref = _required_string(record, "gate_ref")
        scenario_kind = _required_string(record, "scenario_kind")
        requested_phase = _required_string(record, "requested_phase")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")

        _required_string(record, "candidate_ref")
        _required_string(record, "scope_ref")
        _required_string(record, "adr_status_ref")
        _required_string(record, "owner_review_ref")
        _required_string(record, "eval_evidence_ref")
        _required_string(record, "replay_reader_policy_ref")
        _required_string(record, "preservation_matrix_ref")
        _required_string(record, "operator_gate_ref")
        _required_string(record, "audit_ref")
        _required_string(record, "rollback_ref")
        _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs")

        scenario_occurrences.setdefault(scenario_kind, []).append(gate_ref)
        safe_decision, safe_reason = _safe_decision(record, gate_ref, scenario_kind, requested_phase)
        status = "PASS"
        reason = ""
        if expected_decision not in _GATE_DECISIONS or actual_decision not in _GATE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported implementation readiness decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"implementation readiness gate decision mismatch: {gate_ref}"
        elif actual_decision != safe_decision:
            status = "FAIL"
            reason = f"implementation readiness gate contradicted safe policy: {gate_ref}"
        elif actual_decision == "ALLOW" and safe_reason:
            status = "FAIL"
            reason = f"implementation readiness allowed blocked phase: {gate_ref}"
        results.append(
            {
                "record_ref": gate_ref,
                "status": status,
                "reason": reason,
                "safe_decision": safe_decision,
                "safe_reason": safe_reason,
            }
        )
    return results, scenario_occurrences


def _safe_decision(
    record: dict[str, Any],
    gate_ref: str,
    scenario_kind: str,
    requested_phase: str,
) -> tuple[str, str]:
    if scenario_kind not in _SCENARIO_KINDS:
        return "BLOCK", f"unsupported readiness scenario: {scenario_kind}"
    if requested_phase not in _IMPLEMENTATION_PHASES:
        return "BLOCK", f"unsupported implementation phase: {requested_phase}"
    if not _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs"):
        return "BLOCK", f"readiness gate lacks rejection refs: {gate_ref}"
    for field_name, reason in _COMMON_BLOCKERS:
        if _bool(record.get(field_name), default=False):
            return "BLOCK", f"{reason}: {gate_ref}"
    if requested_phase == "fixture-only-hardening":
        if not _bool(record.get("fixture_only_scope"), default=False):
            return "BLOCK", f"fixture-only readiness scope missing: {gate_ref}"
        return "ALLOW", ""
    for field_name, reason in _IMPLEMENTATION_BLOCKERS:
        if not _bool(record.get(field_name), default=False):
            return "BLOCK", f"{reason}: {gate_ref}"
    return "ALLOW", ""


_COMMON_BLOCKERS = (
    ("p0_open", "open P0 finding blocks implementation readiness"),
    ("p1_open", "open P1 finding blocks implementation readiness"),
    ("production_path_changed", "production path changed in isolated Agent Lab"),
    ("schema_contract_changed", "schema contract changed before ADR acceptance"),
    ("real_service_connected", "real service connection attempted before integration approval"),
    ("python_final_owner", "Python owns final Agent state"),
    ("missing_preservation_smoke", "cross-service preservation evidence missing"),
    ("missing_replay_policy", "replay reader policy missing"),
    ("missing_operator_gate", "operator governance gate missing"),
    ("missing_eval_gate", "eval gate evidence missing"),
)

_IMPLEMENTATION_BLOCKERS = (
    ("accepted_adr", "accepted ADR missing for controlled implementation"),
    ("main_review_accepted", "main integration review missing"),
    ("owner_review_complete", "owner review missing"),
)


def _scenario_coverage_results(
    scenario_occurrences: dict[str, list[str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for scenario_kind in sorted(_SCENARIO_KINDS):
        gate_refs = scenario_occurrences.get(scenario_kind, [])
        if not gate_refs:
            results.append(
                _result(
                    scenario_kind,
                    "FAIL",
                    f"controlled implementation readiness missing scenario: {scenario_kind}",
                )
            )
        elif len(gate_refs) > 1:
            results.append(
                _result(
                    scenario_kind,
                    "FAIL",
                    f"controlled implementation readiness duplicate scenario: {scenario_kind}",
                )
            )
        else:
            results.append(_result(scenario_kind, "PASS", ""))
    for scenario_kind in sorted(set(scenario_occurrences) - _SCENARIO_KINDS):
        results.append(
            _result(
                scenario_kind,
                "FAIL",
                f"unexpected controlled implementation readiness scenario: {scenario_kind}",
            )
        )
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
