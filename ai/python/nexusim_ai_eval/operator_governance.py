"""Fixture-only operator governance surface rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_operator_governance_rehearsal"

_EXPECTED_OWNER_BY_SURFACE = {
    "memory-governance": "memory-service+governance",
    "evidence-governance": "retrieval-gateway+governance",
    "replay-governance": "agent-runtime+ai-eval-service",
    "approval-governance": "workflow-service",
    "release-governance": "governance+admin-workflow",
    "failure-class-governance": "governance+ai-eval-service",
    "kill-switch-governance": "governance+control-plane",
    "rollback-governance": "governance+control-plane",
}

_GATE_DECISIONS = {"ALLOW", "BLOCK"}


def load_operator_governance_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive operator governance rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load operator governance rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("operator governance rehearsal must be an object")
    return rehearse_operator_governance(payload)


def rehearse_operator_governance(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify inspect-and-act operator surfaces before production promotion."""

    assert_low_sensitive_eval_payload(payload, "operator governance rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    surface_results, failed_surface_refs, surface_occurrences = _surface_results(
        _record_list(payload.get("operator_surface_records", []), "operator_surface_records")
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
        blocked_reasons.append("operator governance surface records missing")
    if not gate_results:
        blocked_reasons.append("operator governance promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "operator_surface_results": surface_results,
        "surface_coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "operator governance rehearsal result")
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
        owner_ref = _required_string(record, "owner_ref")
        _required_string(record, "operator_role_ref")
        _required_string(record, "auth_policy_ref")
        _required_string(record, "inspect_ref")
        _required_string(record, "action_ref")
        _required_string(record, "audit_ref")
        _required_string(record, "redaction_policy_ref")
        _required_string(record, "replay_reader_policy_ref")
        _required_string(record, "failure_class_ref")
        _required_string(record, "evidence_ref")
        _string_list(record.get("visible_refs", []), "visible_refs")
        _string_list(record.get("action_target_refs", []), "action_target_refs")
        _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs")

        surface_occurrences.setdefault(surface_kind, []).append(surface_ref)
        reason = _surface_blocker(record, surface_ref, surface_kind, owner_ref)
        status = "FAIL" if reason else "PASS"
        results.append(_result(surface_ref, status, reason))
        if reason:
            failed_surface_refs.add(surface_ref)
    return results, failed_surface_refs, surface_occurrences


def _surface_blocker(
    record: dict[str, Any],
    surface_ref: str,
    surface_kind: str,
    owner_ref: str,
) -> str:
    expected_owner = _EXPECTED_OWNER_BY_SURFACE.get(surface_kind)
    if expected_owner is None:
        return f"unsupported operator governance surface: {surface_kind}"
    if owner_ref != expected_owner:
        return f"operator governance owner mismatch: {surface_ref}"
    if not _string_list(record.get("visible_refs", []), "visible_refs"):
        return f"operator governance surface lacks visible refs: {surface_ref}"
    if not _string_list(record.get("action_target_refs", []), "action_target_refs"):
        return f"operator governance surface lacks action target refs: {surface_ref}"
    if not _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs"):
        return f"operator governance surface lacks rejection refs: {surface_ref}"
    if _bool(record.get("body_exposed"), default=False):
        return f"operator governance surface exposes body payload: {surface_ref}"
    if _bool(record.get("unauthorized_actor_allowed"), default=False):
        return f"operator governance surface allows unauthorized actor: {surface_ref}"
    if _bool(record.get("python_override_allowed"), default=False):
        return f"python override allowed for operator governance surface: {surface_ref}"
    if _bool(record.get("missing_audit"), default=False):
        return f"operator governance surface lacks audit evidence: {surface_ref}"
    if _bool(record.get("actionless_view"), default=False):
        return f"operator governance surface is inspect-only without action: {surface_ref}"
    if _bool(record.get("release_allowed_with_gap"), default=False):
        return f"operator governance gap allowed release: {surface_ref}"
    if _bool(record.get("production_contract_authorized"), default=False):
        return f"operator governance fixture authorizes production contract: {surface_ref}"
    return ""


def _surface_coverage_results(
    surface_occurrences: dict[str, list[str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for surface_kind in sorted(_EXPECTED_OWNER_BY_SURFACE):
        surface_refs = surface_occurrences.get(surface_kind, [])
        if not surface_refs:
            results.append(
                _result(surface_kind, "FAIL", f"operator surface missing coverage: {surface_kind}")
            )
        elif len(surface_refs) > 1:
            results.append(
                _result(
                    surface_kind,
                    "FAIL",
                    f"operator surface has duplicate coverage: {surface_kind}",
                )
            )
        else:
            results.append(_result(surface_kind, "PASS", ""))
    for surface_kind in sorted(set(surface_occurrences) - set(_EXPECTED_OWNER_BY_SURFACE)):
        results.append(
            _result(surface_kind, "FAIL", f"unexpected operator surface coverage: {surface_kind}")
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
            reason = f"unsupported operator governance gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"operator governance gate decision mismatch: {gate_ref}"
        elif (failed_surface_refs or failed_coverage) and actual_decision == "ALLOW":
            status = "FAIL"
            reason = f"failed operator governance evidence not blocked: {gate_ref}"
        elif _bool(record.get("missing_surface"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing operator surface did not block promotion: {gate_ref}"
        elif _bool(record.get("actionless_surface"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"actionless operator surface did not block promotion: {gate_ref}"
        elif _bool(record.get("unauthorized_surface"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"unauthorized operator surface did not block promotion: {gate_ref}"
        elif _bool(record.get("exposed_body"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"body exposure did not block operator promotion: {gate_ref}"
        elif _bool(record.get("python_override"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"python override did not block operator promotion: {gate_ref}"
        elif _bool(record.get("missing_audit"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing operator audit did not block promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked operator governance gate allowed release: {gate_ref}"
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
