"""Fixture-only replay compatibility rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_replay_version_bump_rehearsal"

_ALLOWED_DEPRECATED_FIELD_BEHAVIORS = {
    "EXPIRED",
    "FAIL_CLOSED",
    "IGNORED_WITH_DEPRECATION",
}

_REPLAY_BUNDLE_LIST_FIELDS = {
    "input_hashes",
    "evidence_pack_refs",
    "context_package_refs",
    "prepared_tool_refs",
    "workflow_decision_refs",
    "execution_refs",
    "memory_candidate_refs",
    "checkpoint_refs",
    "audit_refs",
    "lineage_refs",
    "observability_refs",
    "hash_refs",
    "version_refs",
    "failure_taxonomy_refs",
    "trace_linkage_refs",
}


def load_replay_version_bump_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive ReplayBundle version-bump rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load replay version-bump rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("replay version-bump rehearsal must be an object")
    return rehearse_replay_version_bump(payload)


def rehearse_replay_version_bump(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify an old ReplayBundle remains explainable by low-sensitive refs."""

    assert_low_sensitive_eval_payload(payload, "replay version-bump rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    current_reader_policy_ref = _required_string(payload, "current_reader_policy_ref")
    old_contract_version_ref = _required_string(payload, "old_contract_version_ref")
    current_contract_version_ref = _required_string(payload, "current_contract_version_ref")
    old_bundle = _object(payload.get("old_replay_bundle"), "old_replay_bundle")
    required_refs = _object(payload.get("required_refs", {}), "required_refs")
    deprecated_records = _record_list(
        payload.get("deprecated_field_records", []),
        "deprecated_field_records",
    )

    blocked_reasons: list[str] = []
    required_ref_results = _required_ref_results(old_bundle, required_refs)
    for result in required_ref_results:
        if result["status"] != "PASS":
            blocked_reasons.append(result["reason"])

    for reason in _bundle_shape_blockers(old_bundle):
        blocked_reasons.append(reason)

    deprecated_field_results = _deprecated_field_results(deprecated_records)
    if not deprecated_field_results:
        blocked_reasons.append("deprecated field rehearsal missing")
    for result in deprecated_field_results:
        if result["status"] != "PASS":
            blocked_reasons.append(result["reason"])

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "old_contract_version_ref": old_contract_version_ref,
        "current_contract_version_ref": current_contract_version_ref,
        "current_reader_policy_ref": current_reader_policy_ref,
        "old_replay_bundle_ref": _required_string(old_bundle, "replay_bundle_ref"),
        "old_replay_bundle_hash": sha256_json(old_bundle),
        "required_ref_results": required_ref_results,
        "deprecated_field_results": deprecated_field_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(result_payload, "replay version-bump rehearsal result")
    return result_payload


def _required_ref_results(
    old_bundle: dict[str, Any],
    required_refs: dict[str, Any],
) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for field_name in sorted(required_refs):
        if field_name not in _REPLAY_BUNDLE_LIST_FIELDS:
            results.append(
                {
                    "field": field_name,
                    "status": "FAIL",
                    "reason": f"unsupported required replay field: {field_name}",
                }
            )
            continue
        expected_refs = _string_list(required_refs.get(field_name, []), field_name)
        actual_refs = _string_list(old_bundle.get(field_name, []), field_name)
        missing_refs = sorted(set(expected_refs) - set(actual_refs))
        if missing_refs:
            results.append(
                {
                    "field": field_name,
                    "status": "FAIL",
                    "reason": f"missing required refs in {field_name}: {','.join(missing_refs)}",
                }
            )
        else:
            results.append(
                {
                    "field": field_name,
                    "status": "PASS",
                    "reason": "",
                }
            )
    return results


def _bundle_shape_blockers(old_bundle: dict[str, Any]) -> list[str]:
    blockers: list[str] = []
    for field_name in sorted(_REPLAY_BUNDLE_LIST_FIELDS):
        if field_name in old_bundle:
            _string_list(old_bundle.get(field_name), field_name)
    if not _bool(old_bundle.get("replay_complete"), default=False):
        blockers.append("old replay bundle is not complete")
    if _bool(old_bundle.get("side_effect_reexecuted"), default=False):
        blockers.append("old replay bundle re-executes side effects")
    if _bool(old_bundle.get("raw_payload_returned"), default=False):
        blockers.append("old replay bundle returns raw payload")
    return blockers


def _deprecated_field_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        field_ref = _required_string(record, "field_ref")
        behavior = _required_string(record, "reader_behavior").upper()
        reason_ref = _required_string(record, "reason_ref")
        if behavior not in _ALLOWED_DEPRECATED_FIELD_BEHAVIORS:
            results.append(
                {
                    "field_ref": field_ref,
                    "status": "FAIL",
                    "reason": f"deprecated field not fail-closed or expired: {field_ref}",
                    "reason_ref": reason_ref,
                }
            )
        else:
            results.append(
                {
                    "field_ref": field_ref,
                    "status": "PASS",
                    "reason": "",
                    "reason_ref": reason_ref,
                }
            )
    return results


def _object(value: Any, context: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ValueError(f"{context} must be an object")
    return value


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
