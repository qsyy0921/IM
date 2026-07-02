"""Fixture-only Runtime / Workflow ownership rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_runtime_workflow_ownership_rehearsal"

_EXPECTED_OWNER_BY_STATE_KIND = {
    "agent-run": "Agent Runtime",
    "agent-step": "Agent Runtime",
    "agent-checkpoint": "Agent Runtime",
    "runtime-wakeup-consumption": "Agent Runtime",
    "replay-index": "Agent Runtime",
    "budget-ledger": "Agent Runtime",
    "approval-wait": "workflow-service",
    "external-callback-wait": "workflow-service",
    "workflow-decision": "workflow-service",
    "workflow-timer": "workflow-service",
    "side-effect-execution": "action-executor",
    "audit-archive": "audit-service",
    "candidate-generation": "Python AI Worker",
}

_WAKEUP_STATUSES = {
    "CONSUMED",
    "DUPLICATE_REJECTED",
    "MISMATCH_REJECTED",
    "STALE_REJECTED",
    "CANCELLED_REJECTED",
}

_RESUME_STATUSES = {
    "RESUMED",
    "REJECTED_STALE_CHECKPOINT",
    "REJECTED_CANCELLED",
    "REJECTED_CORRELATION",
}

_CONTROL_TYPES = {"CANCEL", "RESUME", "REPLAY", "INSPECT"}
_CONTROL_STATUSES = {"ACCEPTED", "REJECTED"}


def load_runtime_workflow_ownership_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive Runtime / Workflow ownership rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load runtime workflow ownership rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("runtime workflow ownership rehearsal must be an object")
    return rehearse_runtime_workflow_ownership(payload)


def rehearse_runtime_workflow_ownership(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify Runtime / Workflow ownership and resume safety with fixture refs."""

    assert_low_sensitive_eval_payload(payload, "runtime workflow ownership rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    owner_results = _ownership_results(
        _record_list(payload.get("ownership_assertions", []), "ownership_assertions")
    )
    checkpoint_results, checkpoint_index = _checkpoint_results(
        _record_list(payload.get("checkpoint_records", []), "checkpoint_records")
    )
    wakeup_results, wakeup_index = _wakeup_results(
        _record_list(payload.get("wakeup_records", []), "wakeup_records"),
        checkpoint_index["current"],
    )
    resume_results = _resume_results(
        _record_list(payload.get("resume_records", []), "resume_records"),
        checkpoint_index,
        wakeup_index,
    )
    control_results = _operator_control_results(
        _record_list(
            payload.get("operator_control_records", []),
            "operator_control_records",
        )
    )
    budget_results = _budget_results(
        _record_list(payload.get("budget_ledger_records", []), "budget_ledger_records")
    )

    blocked_reasons = _blocked_reasons(
        owner_results
        + checkpoint_results
        + wakeup_results
        + resume_results
        + control_results
        + budget_results
    )
    if not wakeup_results:
        blocked_reasons.append("wakeup rehearsal records missing")
    if not resume_results:
        blocked_reasons.append("resume rehearsal records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "ownership_results": owner_results,
        "checkpoint_results": checkpoint_results,
        "wakeup_results": wakeup_results,
        "resume_results": resume_results,
        "operator_control_results": control_results,
        "budget_results": budget_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "runtime workflow ownership rehearsal result",
    )
    return result_payload


def _ownership_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        state_ref = _required_string(record, "state_ref")
        state_kind = _required_string(record, "state_kind")
        owner = _required_string(record, "owner")
        expected_owner = _EXPECTED_OWNER_BY_STATE_KIND.get(state_kind)
        forbidden_refs = set(_string_list(record.get("forbidden_state_refs", []), "forbidden_state_refs"))
        rejected_refs = set(
            _string_list(record.get("rejected_forbidden_state_refs", []), "rejected_forbidden_state_refs")
        )

        status = "PASS"
        reason = ""
        if expected_owner is None:
            status = "FAIL"
            reason = f"unsupported ownership state kind: {state_kind}"
        elif owner != expected_owner:
            status = "FAIL"
            reason = f"owner mismatch for {state_ref}: {owner} != {expected_owner}"
        elif not forbidden_refs.issubset(rejected_refs):
            status = "FAIL"
            reason = f"forbidden ownership refs not rejected for {state_ref}"
        elif _bool(record.get("contains_production_state"), default=False):
            status = "FAIL"
            reason = f"production state leaked into ownership rehearsal: {state_ref}"
        results.append(_result(state_ref, status, reason))
    return results


def _checkpoint_results(
    records: list[dict[str, Any]],
) -> tuple[list[dict[str, str]], dict[str, set[str]]]:
    current_checkpoint_refs: set[str] = set()
    stale_checkpoint_refs: set[str] = set()
    results: list[dict[str, str]] = []
    for record in records:
        checkpoint_ref = _required_string(record, "checkpoint_ref")
        _required_string(record, "run_ref")
        _required_string(record, "step_ref")
        version_ref = _required_string(record, "version_ref")
        current_version_ref = _required_string(record, "current_version_ref")
        _required_string(record, "replay_reader_policy_ref")
        _required_string(record, "audit_ref")
        _required_string(record, "replay_index_ref")
        stale_refs = set(_string_list(record.get("stale_checkpoint_refs", []), "stale_checkpoint_refs"))
        rejected_stale_refs = set(
            _string_list(record.get("rejected_stale_checkpoint_refs", []), "rejected_stale_checkpoint_refs")
        )
        stale_checkpoint_refs.update(stale_refs)

        status = "PASS"
        reason = ""
        if version_ref != current_version_ref:
            status = "FAIL"
            reason = f"checkpoint version is stale: {checkpoint_ref}"
        elif _bool(record.get("contains_raw_payload"), default=False):
            status = "FAIL"
            reason = f"checkpoint contains raw payload: {checkpoint_ref}"
        elif _bool(record.get("stores_business_fact"), default=False):
            status = "FAIL"
            reason = f"checkpoint stores business fact: {checkpoint_ref}"
        elif not stale_refs.issubset(rejected_stale_refs):
            status = "FAIL"
            reason = f"stale checkpoint refs not rejected: {checkpoint_ref}"
        else:
            current_checkpoint_refs.add(checkpoint_ref)
        results.append(_result(checkpoint_ref, status, reason))
    return results, {"current": current_checkpoint_refs, "stale": stale_checkpoint_refs}


def _wakeup_results(
    records: list[dict[str, Any]],
    current_checkpoint_refs: set[str],
) -> tuple[list[dict[str, str]], dict[str, dict[str, str]]]:
    results: list[dict[str, str]] = []
    consumed_by_dedupe: dict[str, list[str]] = {}
    consumed_by_ref: dict[str, dict[str, str]] = {}

    for record in records:
        wakeup_ref = _required_string(record, "wakeup_ref")
        dedupe_key_ref = _required_string(record, "dedupe_key_ref")
        status_value = _upper_required_string(record, "consumption_status")
        decision_ref = _required_string(record, "decision_ref")
        checkpoint_ref = _required_string(record, "checkpoint_ref")
        run_ref = _required_string(record, "run_ref")
        step_ref = _required_string(record, "step_ref")

        status = "PASS"
        reason = ""
        if status_value not in _WAKEUP_STATUSES:
            status = "FAIL"
            reason = f"unsupported wakeup consumption status: {status_value}"
        elif status_value == "CONSUMED":
            consumed_by_dedupe.setdefault(dedupe_key_ref, []).append(wakeup_ref)
            consumed_by_ref[wakeup_ref] = {
                "decision_ref": decision_ref,
                "checkpoint_ref": checkpoint_ref,
                "run_ref": run_ref,
                "step_ref": step_ref,
            }
            if checkpoint_ref not in current_checkpoint_refs:
                status = "FAIL"
                reason = f"wakeup consumed with stale checkpoint: {wakeup_ref}"
            elif _bool(record.get("side_effect_reexecuted"), default=False):
                status = "FAIL"
                reason = f"wakeup consumption re-executes side effect: {wakeup_ref}"
        elif not _string(record.get("rejection_reason_ref")):
            status = "FAIL"
            reason = f"rejected wakeup lacks reason ref: {wakeup_ref}"
        results.append(_result(wakeup_ref, status, reason))

    duplicate_dedupe_refs = {
        dedupe_key_ref
        for dedupe_key_ref, wakeup_refs in consumed_by_dedupe.items()
        if len(wakeup_refs) > 1
    }
    for dedupe_key_ref in sorted(duplicate_dedupe_refs):
        results.append(
            _result(
                dedupe_key_ref,
                "FAIL",
                f"multiple wakeups consumed for dedupe key: {dedupe_key_ref}",
            )
        )

    consumed_dedupe_refs = set(consumed_by_dedupe)
    for record in records:
        status_value = _upper_required_string(record, "consumption_status")
        dedupe_key_ref = _required_string(record, "dedupe_key_ref")
        if status_value == "DUPLICATE_REJECTED" and dedupe_key_ref not in consumed_dedupe_refs:
            results.append(
                _result(
                    _required_string(record, "wakeup_ref"),
                    "FAIL",
                    f"duplicate wakeup rejected without consumed predecessor: {dedupe_key_ref}",
                )
            )
    return results, consumed_by_ref


def _resume_results(
    records: list[dict[str, Any]],
    checkpoint_index: dict[str, set[str]],
    wakeup_index: dict[str, dict[str, str]],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        resume_ref = _required_string(record, "resume_ref")
        status_value = _upper_required_string(record, "resume_status")
        checkpoint_ref = _required_string(record, "checkpoint_ref")
        wakeup_ref = _required_string(record, "wakeup_ref")
        decision_ref = _required_string(record, "decision_ref")
        cancel_state = _upper_required_string(record, "cancel_state")

        status = "PASS"
        reason = ""
        if status_value not in _RESUME_STATUSES:
            status = "FAIL"
            reason = f"unsupported resume status: {status_value}"
        elif status_value == "RESUMED":
            wakeup_record = wakeup_index.get(wakeup_ref)
            if checkpoint_ref not in checkpoint_index["current"]:
                status = "FAIL"
                reason = f"resume used stale checkpoint: {resume_ref}"
            elif wakeup_record is None:
                status = "FAIL"
                reason = f"resume used unconsumed wakeup: {resume_ref}"
            elif wakeup_record["decision_ref"] != decision_ref:
                status = "FAIL"
                reason = f"resume decision correlation mismatch: {resume_ref}"
            elif wakeup_record["checkpoint_ref"] != checkpoint_ref:
                status = "FAIL"
                reason = f"resume checkpoint correlation mismatch: {resume_ref}"
            elif cancel_state == "CANCELLED":
                status = "FAIL"
                reason = f"resume ignored cancelled state: {resume_ref}"
            elif _bool(record.get("side_effect_reexecuted"), default=False):
                status = "FAIL"
                reason = f"resume re-executes side effect: {resume_ref}"
        elif status_value == "REJECTED_STALE_CHECKPOINT":
            if checkpoint_ref in checkpoint_index["current"]:
                status = "FAIL"
                reason = f"current checkpoint rejected as stale: {resume_ref}"
            elif checkpoint_ref not in checkpoint_index["stale"]:
                status = "FAIL"
                reason = f"stale checkpoint rejection lacks stale ref: {resume_ref}"
            elif not _string(record.get("rejection_reason_ref")):
                status = "FAIL"
                reason = f"stale checkpoint rejection lacks reason ref: {resume_ref}"
        elif status_value == "REJECTED_CANCELLED":
            if cancel_state != "CANCELLED":
                status = "FAIL"
                reason = f"cancel rejection lacks cancelled state: {resume_ref}"
            elif not _string(record.get("rejection_reason_ref")):
                status = "FAIL"
                reason = f"cancel rejection lacks reason ref: {resume_ref}"
        elif not _string(record.get("rejection_reason_ref")):
            status = "FAIL"
            reason = f"correlation rejection lacks reason ref: {resume_ref}"
        results.append(_result(resume_ref, status, reason))
    return results


def _operator_control_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        control_ref = _required_string(record, "control_ref")
        control_type = _upper_required_string(record, "control_type")
        status_value = _upper_required_string(record, "control_status")
        visible_refs = _string_list(record.get("visible_refs", []), "visible_refs")
        _required_string(record, "redaction_policy_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if control_type not in _CONTROL_TYPES:
            status = "FAIL"
            reason = f"unsupported operator control type: {control_type}"
        elif status_value not in _CONTROL_STATUSES:
            status = "FAIL"
            reason = f"unsupported operator control status: {status_value}"
        elif not visible_refs:
            status = "FAIL"
            reason = f"operator control lacks inspectable refs: {control_ref}"
        elif _bool(record.get("exposes_sensitive_payload"), default=False):
            status = "FAIL"
            reason = f"operator control exposes sensitive payload: {control_ref}"
        elif control_type == "REPLAY" and _bool(
            record.get("side_effect_reexecuted"),
            default=False,
        ):
            status = "FAIL"
            reason = f"operator replay re-executes side effect: {control_ref}"
        elif status_value == "REJECTED" and not _string(record.get("rejection_reason_ref")):
            status = "FAIL"
            reason = f"rejected operator control lacks reason ref: {control_ref}"
        results.append(_result(control_ref, status, reason))
    return results


def _budget_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        budget_ledger_ref = _required_string(record, "budget_ledger_ref")
        owner = _required_string(record, "owner")
        _required_string(record, "run_ref")
        limit_refs = _string_list(record.get("limit_refs", []), "limit_refs")
        usage_refs = _string_list(record.get("usage_refs", []), "usage_refs")
        over_budget = _bool(record.get("over_budget"), default=False)
        continued_after_over_budget = _bool(
            record.get("continued_after_over_budget"),
            default=False,
        )

        status = "PASS"
        reason = ""
        if owner != "Agent Runtime":
            status = "FAIL"
            reason = f"budget ledger owner mismatch: {budget_ledger_ref}"
        elif not limit_refs or not usage_refs:
            status = "FAIL"
            reason = f"budget ledger lacks limit or usage refs: {budget_ledger_ref}"
        elif over_budget and continued_after_over_budget:
            status = "FAIL"
            reason = f"over-budget run continued: {budget_ledger_ref}"
        elif over_budget and not (
            _string(record.get("review_ref")) or _string(record.get("rejection_reason_ref"))
        ):
            status = "FAIL"
            reason = f"over-budget run lacks review or rejection ref: {budget_ledger_ref}"
        results.append(_result(budget_ledger_ref, status, reason))
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
