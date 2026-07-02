"""Fixture-only dataset reproducibility rehearsal helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)


REHEARSAL_KIND = "agent_dataset_reproducibility_rehearsal"

_GATE_DECISIONS = {"ALLOW", "BLOCK"}


def load_dataset_reproducibility_rehearsal(path: Path) -> dict[str, Any]:
    """Load and run a low-sensitive dataset reproducibility rehearsal."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load dataset reproducibility rehearsal") from exc
    if not isinstance(payload, dict):
        raise ValueError("dataset reproducibility rehearsal must be an object")
    return rehearse_dataset_reproducibility(payload)


def rehearse_dataset_reproducibility(payload: dict[str, Any]) -> dict[str, Any]:
    """Verify dataset manifests, deterministic reports and gate blocking."""

    assert_low_sensitive_eval_payload(payload, "dataset reproducibility rehearsal")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")
    if _string(payload.get("rehearsal_kind")) != REHEARSAL_KIND:
        raise ValueError(f"rehearsal_kind must be {REHEARSAL_KIND}")

    rehearsal_id = _required_string(payload, "rehearsal_id")
    manifest_results = _manifest_results(
        _record_list(payload.get("dataset_manifest_records", []), "dataset_manifest_records")
    )
    run_results = _run_reproducibility_results(
        _record_list(payload.get("run_reproducibility_records", []), "run_reproducibility_records")
    )
    calibration_results = _calibration_export_results(
        _record_list(payload.get("calibration_export_records", []), "calibration_export_records")
    )
    promotion_results = _promotion_gate_results(
        _record_list(payload.get("promotion_gate_records", []), "promotion_gate_records")
    )

    all_results = manifest_results + run_results + calibration_results + promotion_results
    blocked_reasons = _blocked_reasons(all_results)
    if not manifest_results:
        blocked_reasons.append("dataset manifest rehearsal records missing")
    if not run_results:
        blocked_reasons.append("run reproducibility rehearsal records missing")
    if not calibration_results:
        blocked_reasons.append("calibration export rehearsal records missing")
    if not promotion_results:
        blocked_reasons.append("promotion gate rehearsal records missing")

    result_payload = {
        "schema_version": SCHEMA_VERSION,
        "rehearsal_kind": REHEARSAL_KIND,
        "rehearsal_id": rehearsal_id,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "rehearsal_hash": sha256_json(payload),
        "dataset_manifest_results": manifest_results,
        "run_reproducibility_results": run_results,
        "calibration_export_results": calibration_results,
        "promotion_gate_results": promotion_results,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
    }
    assert_low_sensitive_eval_payload(
        result_payload,
        "dataset reproducibility rehearsal result",
    )
    return result_payload


def _manifest_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        manifest_ref = _required_string(record, "manifest_ref")
        _required_string(record, "dataset_ref")
        _required_string(record, "dataset_version_ref")
        _required_string(record, "license_ref")
        _required_string(record, "snapshot_hash_ref")
        _required_string(record, "split_manifest_ref")
        _required_string(record, "import_hash_ref")
        _required_string(record, "adapter_version_ref")
        case_count = _required_int(record, "case_count")

        status = "PASS"
        reason = ""
        if case_count <= 0:
            status = "FAIL"
            reason = f"dataset manifest has no cases: {manifest_ref}"
        elif _bool(record.get("production_data_used"), default=False):
            status = "FAIL"
            reason = f"dataset manifest uses production data: {manifest_ref}"
        elif not _bool(record.get("public_or_synthetic_only"), default=False):
            status = "FAIL"
            reason = f"dataset manifest is not public or synthetic only: {manifest_ref}"
        elif not _bool(record.get("ground_truth_separated"), default=False):
            status = "FAIL"
            reason = f"dataset ground truth not separated from product facts: {manifest_ref}"
        elif not _bool(record.get("low_sensitive_export"), default=False):
            status = "FAIL"
            reason = f"dataset manifest is not low-sensitive export: {manifest_ref}"
        results.append(_result(manifest_ref, status, reason))
    return results


def _run_reproducibility_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        run_ref = _required_string(record, "run_ref")
        _required_string(record, "manifest_ref")
        _required_string(record, "adapter_version_ref")
        _required_string(record, "harness_version_ref")
        _required_string(record, "determinism_ref")
        report_hash_ref = _required_string(record, "report_hash_ref")
        repeated_report_hash_ref = _required_string(record, "repeated_report_hash_ref")
        _required_string(record, "retention_policy_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if report_hash_ref != repeated_report_hash_ref:
            status = "FAIL"
            reason = f"dataset report is not deterministic: {run_ref}"
        elif _bool(record.get("backend_connected"), default=False):
            status = "FAIL"
            reason = f"dataset run connected to backend: {run_ref}"
        elif _bool(record.get("model_provider_called"), default=False):
            status = "FAIL"
            reason = f"dataset run called model provider: {run_ref}"
        elif _bool(record.get("raw_payload_retained"), default=False):
            status = "FAIL"
            reason = f"dataset run retained raw payload: {run_ref}"
        results.append(_result(run_ref, status, reason))
    return results


def _calibration_export_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        export_ref = _required_string(record, "export_ref")
        dataset_source_refs = _string_list(
            record.get("dataset_source_refs", []), "dataset_source_refs"
        )
        dataset_case_counts = _case_count_records(
            record.get("dataset_case_counts", []), "dataset_case_counts"
        )
        _required_string(record, "input_payload_hash_ref")
        report_hash_ref = _required_string(record, "report_hash_ref")
        repeated_report_hash_ref = _required_string(record, "repeated_report_hash_ref")
        _required_string(record, "recommended_confidence_threshold_ref")
        _required_string(record, "recommended_policy_window_ref")
        _required_string(record, "recommended_review_backoff_ref")

        status = "PASS"
        reason = ""
        dataset_refs_with_counts = {item["dataset_source_ref"] for item in dataset_case_counts}
        if not dataset_source_refs:
            status = "FAIL"
            reason = f"calibration export lacks dataset source refs: {export_ref}"
        elif set(dataset_source_refs) != dataset_refs_with_counts:
            status = "FAIL"
            reason = f"calibration export dataset counts do not match sources: {export_ref}"
        elif sum(item["case_count"] for item in dataset_case_counts) <= 0:
            status = "FAIL"
            reason = f"calibration export has no cases: {export_ref}"
        elif report_hash_ref != repeated_report_hash_ref:
            status = "FAIL"
            reason = f"calibration export report is not deterministic: {export_ref}"
        elif _bool(record.get("production_data_used"), default=False):
            status = "FAIL"
            reason = f"calibration export uses production data: {export_ref}"
        results.append(_result(export_ref, status, reason))
    return results


def _promotion_gate_results(records: list[dict[str, Any]]) -> list[dict[str, str]]:
    results: list[dict[str, str]] = []
    for record in records:
        gate_ref = _required_string(record, "gate_ref")
        expected_decision = _upper_required_string(record, "expected_gate_decision")
        actual_decision = _upper_required_string(record, "actual_gate_decision")
        _required_string(record, "manifest_ref")
        _required_string(record, "eval_report_ref")
        _required_string(record, "baseline_review_ref")
        _required_string(record, "audit_ref")

        status = "PASS"
        reason = ""
        if expected_decision not in _GATE_DECISIONS or actual_decision not in _GATE_DECISIONS:
            status = "FAIL"
            reason = f"unsupported dataset promotion gate decision: {gate_ref}"
        elif actual_decision != expected_decision:
            status = "FAIL"
            reason = f"dataset promotion gate decision mismatch: {gate_ref}"
        elif _bool(record.get("manifest_missing"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"missing dataset manifest did not block promotion: {gate_ref}"
        elif _bool(record.get("snapshot_changed"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"changed dataset snapshot did not block promotion: {gate_ref}"
        elif _bool(record.get("import_hash_mismatch"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"dataset import hash mismatch did not block promotion: {gate_ref}"
        elif _bool(record.get("non_deterministic_report"), default=False) and actual_decision != "BLOCK":
            status = "FAIL"
            reason = f"non-deterministic dataset report did not block promotion: {gate_ref}"
        elif _bool(record.get("release_allowed"), default=False) and actual_decision != "ALLOW":
            status = "FAIL"
            reason = f"blocked dataset gate allowed release: {gate_ref}"
        results.append(_result(gate_ref, status, reason))
    return results


def _case_count_records(value: Any, field_name: str) -> list[dict[str, Any]]:
    records = _record_list(value, field_name)
    result: list[dict[str, Any]] = []
    for index, record in enumerate(records):
        result.append(
            {
                "dataset_source_ref": _required_string(record, "dataset_source_ref"),
                "case_count": _required_int(record, "case_count"),
            }
        )
        if result[index]["case_count"] <= 0:
            raise ValueError(f"{field_name}[{index}].case_count must be positive")
    return result


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


def _required_int(payload: dict[str, Any], field_name: str) -> int:
    if field_name not in payload:
        raise ValueError(f"{field_name} is required")
    value = payload[field_name]
    if not isinstance(value, int) or isinstance(value, bool):
        raise ValueError(f"{field_name} must be an integer")
    return value


def _bool(value: Any, *, default: bool) -> bool:
    if value is None:
        return default
    if not isinstance(value, bool):
        raise ValueError("expected bool")
    return value
