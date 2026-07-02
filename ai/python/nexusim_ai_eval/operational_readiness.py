"""Fixture-only Agent operational readiness rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_operational_readiness_rehearsal"

_EXPECTED_OWNER_BY_KIND = {
    "runtime-step-budget": "agent-runtime+governance",
    "model-spend-budget": "model-gateway+governance",
    "tool-timeout-budget": "mcp-gateway+governance",
    "retrieval-latency-budget": "retrieval-gateway+governance",
    "eval-report-retention-budget": "ai-eval-service+audit-service",
    "canary-telemetry-budget": "governance+observability",
    "incident-escalation-budget": "governance+oncall",
}

_GATE_DECISIONS = {"ALLOW", "BLOCK"}


def load_operational_readiness_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive operational readiness rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load operational readiness rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("operational readiness rehearsal must be an object")
    return rehearse_operational_readiness(payload)


def rehearse_operational_readiness(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify budget, capacity, retention and escalation gates with refs."""

    assert_low_sensitive_eval_payload(payload, "operational readiness rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    budget_results, failed_budget_refs, budget_occurrences = _budget_results(
        _record_list(payload.get("budget_records", []), "budget_records")
    )
    coverage_results = _budget_coverage_results(budget_occurrences)
    gate_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records"),
        failed_budget_refs,
        coverage_results,
    )

    all_results = budget_results + coverage_results + gate_results
    blocked_reasons = _blocked_reasons(all_results)
    if not budget_results:
        blocked_reasons.append("operational readiness budget records missing")
    if not gate_results:
        blocked_reasons.append("operational readiness promotion gate records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "budget_results": budget_results,
        "budget_coverage_results": coverage_results,
        "promotion_gate_results": gate_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "operational readiness rehearsal result")
    return result_payload


def _budget_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], set[str], dict[str, list[str]]]:
    results: list[dict[str, str]] = []
    failed_budget_refs: set[str] = set()
    budget_occurrences: dict[str, list[str]] = {}
    for record in records:
        budget_ref = _required_string(record, "budget_ref")
        budget_kind = _required_string(record, "budget_kind")
        owner_ref = _required_string(record, "owner_ref")
        _required_string(record, "applies_to_ref")
        _required_string(record, "risk_tier_ref")
        _required_string(record, "limit_ref")
        _required_string(record, "measurement_ref")
        _required_string(record, "enforcement_ref")
        _required_string(record, "operator_view_ref")
        _required_string(record, "audit_ref")
        _required_string(record, "evidence_ref")
        _required_string(record, "release_gate_ref")
        _required_string(record, "failure_class_ref")
        _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs")

        budget_occurrences.setdefault(budget_kind, []).append(budget_ref)
        reason = _budget_blocker(record, budget_ref, budget_kind, owner_ref)
        status = "FAIL" if reason else "PASS"
        results.append(_result(budget_ref, status, reason))
        if reason:
            failed_budget_refs.add(budget_ref)
    return results, failed_budget_refs, budget_occurrences


def _budget_blocker(
    record: dict[str, Any],
    budget_ref: str,
    budget_kind: str,
    owner_ref: str,
) -> str:
    expected_owner = _EXPECTED_OWNER_BY_KIND.get(budget_kind)
    if expected_owner is None:
        return f"unsupported operational budget kind: {budget_kind}"
    if owner_ref != expected_owner:
        return f"operational budget owner mismatch: {budget_ref}"
    if not _string_list(record.get("rejection_condition_refs", []), "rejection_condition_refs"):
        return f"operational budget lacks rejection refs: {budget_ref}"
    if _bool(record.get("missing_measurement"), default=False):
        return f"operational budget lacks measurement evidence: {budget_ref}"
    if _bool(record.get("missing_operator_view"), default=False):
        return f"operational budget lacks operator view: {budget_ref}"
    if _bool(record.get("limit_exceeded"), default=False) and _bool(
        record.get("continued_after_limit"),
        default=False,
    ):
        return f"operational budget continued after limit: {budget_ref}"
    if _bool(record.get("release_allowed_with_gap"), default=False):
        return f"operational budget gap allowed release: {budget_ref}"
    if _bool(record.get("raw_body_retained"), default=False):
        return f"operational budget retains raw body: {budget_ref}"
    if _bool(record.get("python_override_allowed"), default=False):
        return f"python override allowed for operational budget: {budget_ref}"
    if _bool(record.get("unreviewed_capacity_allowed"), default=False):
        return f"unreviewed capacity allowed by operational budget: {budget_ref}"
    if _bool(record.get("production_slo_authorized"), default=False):
        return f"operational fixture authorizes production SLO: {budget_ref}"
    return ""


def _budget_coverage_results(
    budget_occurrences: dict[str, list[str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for budget_kind in sorted(_EXPECTED_OWNER_BY_KIND):
        budget_refs = budget_occurrences.get(budget_kind, [])
        if not budget_refs:
            results.append(
                _result(
                    budget_kind,
                    "FAIL",
                    f"operational budget missing coverage: {budget_kind}",
                )
            )
        elif len(budget_refs) > 1:
            results.append(
                _result(
                    budget_kind,
                    "FAIL",
                    f"operational budget has duplicate coverage: {budget_kind}",
                )
            )
        else:
            results.append(_result(budget_kind, "PASS", ""))
    for budget_kind in sorted(set(budget_occurrences) - set(_EXPECTED_OWNER_BY_KIND)):
        results.append(
            _result(
                budget_kind,
                "FAIL",
                f"unexpected operational budget coverage: {budget_kind}",
            )
        )
    return results


def _promotion_gate_results(
    records: list[dict[str, Any]],
    failed_budget_refs: set[str],
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
            reason = f"unsupported operational readiness gate decision: {gate_ref}"
        elif expected_decision != actual_decision:
            status = "FAIL"
            reason = f"operational readiness gate decision mismatch: {gate_ref}"
        elif (failed_budget_refs or failed_coverage) and actual_decision == "ALLOW":
            status = "FAIL"
            reason = f"failed operational readiness evidence not blocked: {gate_ref}"
        elif _bool(record.get("missing_budget"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing operational budget did not block promotion: {gate_ref}"
        elif _bool(record.get("missing_measurement"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing operational measurement did not block promotion: {gate_ref}"
        elif _bool(record.get("limit_gap"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"limit gap did not block operational promotion: {gate_ref}"
        elif _bool(record.get("retention_gap"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"retention gap did not block operational promotion: {gate_ref}"
        elif _bool(record.get("unreviewed_capacity"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"unreviewed capacity did not block promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked operational readiness gate allowed release: {gate_ref}"
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
