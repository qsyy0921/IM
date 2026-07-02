"""Fixture-only Agent architecture surface coverage rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_architecture_coverage_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}

_EXPECTED_SURFACE_KINDS = {
    "agent-runtime-harness",
    "eval-replay-harness",
    "context-evidencepack-rag",
    "memory-admission",
    "tool-mcp-boundary",
    "workflow-hitl-approval",
    "action-executor-handoff",
    "multi-agent-a2a-boundary",
    "agentops-governance",
    "contract-versioning-replay-policy",
    "cross-service-ref-preservation",
    "security-privacy-audit-operator-ux",
    "open-dataset-synthetic-eval-path",
}

_REQUIRED_REF_FIELDS = (
    "surface_ref",
    "surface_kind",
    "owner_ref",
    "sdd_ref",
    "research_ref",
    "adr_candidate_ref",
    "fixture_evidence_ref",
    "lifecycle_ref",
    "version_policy_ref",
    "replay_policy_ref",
    "preservation_ref",
    "audit_ref",
    "operator_ref",
    "eval_gate_ref",
)

_BLOCKER_FIELDS = (
    ("p0_open", "architecture surface has open P0"),
    ("p1_open", "architecture surface has open P1"),
    ("unresolved_blocker", "architecture surface has unresolved blocker"),
    ("missing_owner", "architecture surface lacks owner"),
    ("missing_version", "architecture surface lacks version policy"),
    ("missing_replay", "architecture surface lacks replay policy"),
    ("missing_preservation", "architecture surface lacks preservation refs"),
    ("missing_operator", "architecture surface lacks operator path"),
    ("missing_eval", "architecture surface lacks eval evidence"),
    ("python_final_owner", "Python owns final state for architecture surface"),
    ("real_service_required", "architecture surface requires real service in isolated phase"),
    ("production_contract_authorized", "architecture fixture authorizes production contract"),
)


def load_architecture_coverage_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive architecture coverage rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load architecture coverage rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("architecture coverage rehearsal must be an object")
    return rehearse_architecture_coverage(payload)


def rehearse_architecture_coverage(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify all required Agent architecture surfaces have fail-closed evidence."""

    assert_low_sensitive_eval_payload(payload, "architecture coverage rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    surface_results, failed_surface_refs, surface_occurrences = _surface_results(
        _record_list(payload.get("surface_records", []), "surface_records")
    )
    coverage_results = _surface_coverage_results(surface_occurrences)
    gate_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records"),
        failed_surface_refs,
        coverage_results,
    )

    all_results = surface_results + coverage_results + gate_results
    blocked_reasons = _blocked_reasons(all_results)
    if not surface_results:
        blocked_reasons.append("architecture surface records missing")
    if not gate_results:
        blocked_reasons.append("architecture coverage promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "surface_results": surface_results,
        "surface_coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "architecture coverage rehearsal result")
    return result_payload


def _surface_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], set[str], dict[str, list[str]]]:
    results: list[dict[str, str]] = []
    failed_surface_refs: set[str] = set()
    surface_occurrences: dict[str, list[str]] = {}
    for record in records:
        surface_ref = _required_string(record, "surface_ref")
        surface_kind = _required_string(record, "surface_kind")
        for field_name in _REQUIRED_REF_FIELDS:
            _required_string(record, field_name)
        _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs")

        surface_occurrences.setdefault(surface_kind, []).append(surface_ref)
        reason = _surface_blocker(record, surface_ref, surface_kind)
        status = "FAIL" if reason else "PASS"
        results.append(_result(surface_ref, status, reason))
        if reason:
            failed_surface_refs.add(surface_ref)
    return results, failed_surface_refs, surface_occurrences


def _surface_blocker(record: dict[str, Any], surface_ref: str, surface_kind: str) -> str:
    if surface_kind not in _EXPECTED_SURFACE_KINDS:
        return f"unsupported architecture surface kind: {surface_kind}"
    if not _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs"):
        return f"architecture surface lacks rejection refs: {surface_ref}"
    for field_name, reason in _BLOCKER_FIELDS:
        if _bool(record.get(field_name), default=False):
            return f"{reason}: {surface_ref}"
    return ""


def _surface_coverage_results(surface_occurrences: dict[str, list[str]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for surface_kind in sorted(_EXPECTED_SURFACE_KINDS):
        surface_refs = surface_occurrences.get(surface_kind, [])
        if not surface_refs:
            results.append(
                _result(
                    surface_kind,
                    "FAIL",
                    f"architecture surface missing coverage: {surface_kind}",
                )
            )
        elif len(surface_refs) > 1:
            results.append(
                _result(
                    surface_kind,
                    "FAIL",
                    f"architecture surface has duplicate coverage: {surface_kind}",
                )
            )
        else:
            results.append(_result(surface_kind, "PASS", ""))
    for surface_kind in sorted(set(surface_occurrences) - _EXPECTED_SURFACE_KINDS):
        results.append(
            _result(
                surface_kind,
                "FAIL",
                f"unexpected architecture surface coverage: {surface_kind}",
            )
        )
    return results


def _promotion_gate_results(
    records: list[dict[str, Any]],
    failed_surface_refs: set[str],
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
            reason = f"unsupported architecture coverage gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"architecture coverage gate decision mismatch: {gate_ref}"
        elif (failed_surface_refs or failed_coverage) and actual_decision == "ALLOW":
            status = "FAIL"
            reason = f"failed architecture coverage evidence not blocked: {gate_ref}"
        elif _bool(record.get("missing_surface"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing architecture surface did not block promotion: {gate_ref}"
        elif _bool(record.get("missing_dimension"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing architecture surface dimension did not block promotion: {gate_ref}"
        elif _bool(record.get("open_p1"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"open P1 did not block architecture promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked architecture coverage gate allowed release: {gate_ref}"
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
