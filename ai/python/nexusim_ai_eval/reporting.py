"""Current EvalReport generation and baseline refresh review helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.adapter_runner import run_adapter_payload
from nexusim_ai_eval.comparison import compare_eval_reports
from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)
from nexusim_ai_eval.evaluator import eval_report_to_payload, run_eval_suite
from nexusim_ai_eval.fixtures import load_eval_suite


DEFAULT_RETENTION_POLICY_REF = "retention:agent-eval-report:research-v1"
DEFAULT_APPROVAL_POLICY_REF = "approval-policy:agent-eval-baseline-refresh:manual-v1"


def generate_current_report_payload(
    fixture_path: Path,
    *,
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Run one synthetic fixture suite and return a low-sensitive EvalReport payload."""

    suite_payload = load_eval_suite(fixture_path)
    report_payload = eval_report_to_payload(run_eval_suite(suite_payload))
    report_payload["retention_metadata"] = build_retention_metadata(
        artifact_kind="agent_eval_report",
        retention_policy_ref=retention_policy_ref,
    )
    assert_low_sensitive_eval_payload(report_payload, "current EvalReport")
    return report_payload


def generate_adapter_report_payload(
    adapter_payload_path: Path,
    *,
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Run one local public-dataset-style adapter payload as an EvalReport."""

    try:
        adapter_payload = json.loads(adapter_payload_path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load adapter payload") from exc
    if not isinstance(adapter_payload, dict):
        raise ValueError("adapter payload must be an object")
    report_payload = eval_report_to_payload(run_adapter_payload(adapter_payload))
    report_payload["retention_metadata"] = build_retention_metadata(
        artifact_kind="agent_eval_report",
        retention_policy_ref=retention_policy_ref,
    )
    assert_low_sensitive_eval_payload(report_payload, "adapter EvalReport")
    return report_payload


def build_baseline_refresh_review(
    *,
    current_report_payload: dict[str, Any],
    current_report_path: Path,
    baseline_report_payload: dict[str, Any] | None = None,
    baseline_report_path: Path | None = None,
    score_tolerance: float = 0.0,
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Build a low-sensitive review payload for manual baseline refresh decisions."""

    assert_low_sensitive_eval_payload(current_report_payload, "current EvalReport")
    comparison: dict[str, Any] | None = None
    blocked_reasons: list[str] = []
    baseline_hash: str | None = None
    if baseline_report_payload is not None:
        comparison = compare_eval_reports(
            baseline_report_payload,
            current_report_payload,
            score_tolerance=score_tolerance,
        )
        blocked_reasons = list(comparison["blocked_promotion_reasons"])
        baseline_hash = sha256_json(baseline_report_payload)

    current_status = _string(current_report_payload.get("status"))
    status = "PASS"
    if current_status != "PASS":
        status = "FAIL"
        blocked_reasons.append("current report status is not PASS")
    if comparison is not None and comparison["status"] != "PASS":
        status = "FAIL"

    review_payload = {
        "schema_version": SCHEMA_VERSION,
        "review_kind": "agent_eval_baseline_refresh_review",
        "status": status,
        "refresh_recommendation": _refresh_recommendation(
            has_baseline=baseline_report_payload is not None,
            status=status,
        ),
        "current_suite_id": _string(current_report_payload.get("suite_id")),
        "current_report_status": current_status,
        "current_case_count": _int(current_report_payload.get("case_count"), "current case_count"),
        "current_report_path": str(current_report_path),
        "current_report_hash": sha256_json(current_report_payload),
        "baseline_report_path": str(baseline_report_path) if baseline_report_path else "",
        "baseline_report_hash": baseline_hash or "",
        "score_tolerance": score_tolerance,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
        "retention_metadata": build_retention_metadata(
            artifact_kind="agent_eval_baseline_refresh_review",
            retention_policy_ref=retention_policy_ref,
        ),
        "comparison": comparison,
    }
    assert_low_sensitive_eval_payload(review_payload, "baseline refresh review")
    return review_payload


def load_report_matrix_plan(path: Path) -> dict[str, Any]:
    """Load a low-sensitive report-matrix plan."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load report matrix plan") from exc
    if not isinstance(payload, dict):
        raise ValueError("report matrix plan must be an object")
    assert_low_sensitive_eval_payload(payload, "report matrix plan")
    return payload


def run_report_matrix_plan(
    plan_payload: dict[str, Any],
    *,
    base_dir: Path,
    score_tolerance: float = 0.0,
    force: bool = False,
) -> tuple[dict[str, Any], dict[str, Any]]:
    """Run a fixture-only multi-suite current report matrix plan."""

    assert_low_sensitive_eval_payload(plan_payload, "report matrix plan")
    if score_tolerance < 0:
        raise ValueError("score_tolerance must be non-negative")

    retention_policy_ref = (
        _string(plan_payload.get("retention_policy_ref")) or DEFAULT_RETENTION_POLICY_REF
    )
    approval_policy_ref = (
        _string(plan_payload.get("approval_policy_ref")) or DEFAULT_APPROVAL_POLICY_REF
    )
    matrix_id = _required_string(plan_payload, "matrix_id", "report matrix plan")
    suite_specs = _suite_specs(plan_payload)
    resolved_specs = [
        _resolve_suite_spec(spec, base_dir=base_dir, index=index)
        for index, spec in enumerate(suite_specs)
    ]
    _validate_report_matrix_paths(resolved_specs)

    matrix_entries: list[dict[str, Any]] = []
    review_payloads: list[dict[str, Any]] = []
    for spec in resolved_specs:
        current_report = _generate_report_for_spec(
            spec,
            retention_policy_ref=retention_policy_ref,
        )
        baseline_report = (
            load_report_payload(spec["baseline_path"], "baseline report")
            if spec.get("baseline_path")
            else None
        )
        review = build_baseline_refresh_review(
            current_report_payload=current_report,
            current_report_path=spec["report_out"],
            baseline_report_payload=baseline_report,
            baseline_report_path=spec.get("baseline_path"),
            score_tolerance=score_tolerance,
            retention_policy_ref=retention_policy_ref,
        )
        write_json_artifact(spec["report_out"], current_report, force=force)
        if spec.get("review_out"):
            write_json_artifact(spec["review_out"], review, force=force)
        matrix_entries.append(
            _report_matrix_entry(
                spec=spec,
                current_report=current_report,
                review=review,
                retention_policy_ref=retention_policy_ref,
            )
        )
        review_payloads.append(review)

    matrix_payload = build_report_matrix_payload(
        matrix_id=matrix_id,
        entries=matrix_entries,
        retention_policy_ref=retention_policy_ref,
    )
    approval_manifest = build_baseline_refresh_approval_manifest(
        manifest_id=f"{matrix_id}:baseline-refresh-approval",
        review_payloads=review_payloads,
        approval_policy_ref=approval_policy_ref,
        retention_policy_ref=retention_policy_ref,
    )
    return matrix_payload, approval_manifest


def build_report_matrix_payload(
    *,
    matrix_id: str,
    entries: list[dict[str, Any]],
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Build a low-sensitive multi-suite current report matrix."""

    status = "PASS" if all(entry["status"] == "PASS" for entry in entries) else "FAIL"
    matrix_payload = {
        "schema_version": SCHEMA_VERSION,
        "matrix_kind": "agent_eval_current_report_matrix",
        "matrix_id": _string(matrix_id),
        "status": status,
        "suite_count": len(entries),
        "passed_suite_count": sum(1 for entry in entries if entry["status"] == "PASS"),
        "failed_suite_count": sum(1 for entry in entries if entry["status"] != "PASS"),
        "approval_required_count": sum(
            1 for entry in entries if entry["approval_required"]
        ),
        "blocked_suite_count": sum(1 for entry in entries if entry["review_status"] != "PASS"),
        "entries": sorted(entries, key=lambda entry: entry["suite_ref"]),
        "retention_metadata": build_retention_metadata(
            artifact_kind="agent_eval_current_report_matrix",
            retention_policy_ref=retention_policy_ref,
        ),
    }
    assert_low_sensitive_eval_payload(matrix_payload, "current report matrix")
    return matrix_payload


def build_baseline_refresh_approval_manifest(
    *,
    manifest_id: str,
    review_payloads: list[dict[str, Any]],
    approval_policy_ref: str = DEFAULT_APPROVAL_POLICY_REF,
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Build a low-sensitive manual approval manifest for baseline refreshes."""

    entries = [
        _approval_manifest_entry(
            review_payload=review,
            approval_policy_ref=approval_policy_ref,
        )
        for review in review_payloads
    ]
    blocked_count = sum(1 for entry in entries if entry["decision_status"] == "BLOCKED")
    manifest = {
        "schema_version": SCHEMA_VERSION,
        "manifest_kind": "agent_eval_baseline_refresh_approval_manifest",
        "manifest_id": _string(manifest_id),
        "status": "PASS" if blocked_count == 0 else "FAIL",
        "approval_policy_ref": _string(approval_policy_ref),
        "approval_required_count": sum(
            1 for entry in entries if entry["approval_required"]
        ),
        "blocked_count": blocked_count,
        "entries": sorted(entries, key=lambda entry: entry["suite_id"]),
        "retention_metadata": build_retention_metadata(
            artifact_kind="agent_eval_baseline_refresh_approval_manifest",
            retention_policy_ref=retention_policy_ref,
        ),
    }
    assert_low_sensitive_eval_payload(manifest, "baseline refresh approval manifest")
    return manifest


def build_retention_metadata(
    *,
    artifact_kind: str,
    retention_policy_ref: str = DEFAULT_RETENTION_POLICY_REF,
) -> dict[str, Any]:
    """Return shared low-sensitive retention metadata for eval artifacts."""

    metadata = {
        "schema_version": SCHEMA_VERSION,
        "metadata_kind": "agent_eval_retention_metadata",
        "artifact_kind": _string(artifact_kind),
        "retention_policy_ref": _string(retention_policy_ref),
        "retention_class": "AGENT_EVAL_RESEARCH_LOW_SENSITIVE",
        "low_sensitive_only": True,
        "raw_payload_retained": False,
        "provider_response_retained": False,
        "production_data_retained": False,
    }
    assert_low_sensitive_eval_payload(metadata, "retention metadata")
    return metadata


def load_report_payload(path: Path, context: str) -> dict[str, Any]:
    """Load a low-sensitive EvalReport-like JSON object."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"failed to load {context}") from exc
    if not isinstance(payload, dict):
        raise ValueError(f"{context} must be an object")
    assert_low_sensitive_eval_payload(payload, context)
    return payload


def write_json_artifact(path: Path, payload: dict[str, Any], *, force: bool = False) -> None:
    """Write a low-sensitive JSON artifact, refusing accidental overwrite by default."""

    assert_low_sensitive_eval_payload(payload, f"artifact {path.name}")
    if path.exists() and not force:
        raise ValueError(f"refusing to overwrite existing artifact: {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(payload, ensure_ascii=True, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def ensure_not_same_path(left: Path, right: Path, left_label: str, right_label: str) -> None:
    """Reject artifact targets that would overwrite protected inputs."""

    if left.resolve() == right.resolve():
        raise ValueError(f"{left_label} must not overwrite {right_label}")


def _refresh_recommendation(*, has_baseline: bool, status: str) -> str:
    if status != "PASS":
        return "BASELINE_REFRESH_BLOCKED"
    if has_baseline:
        return "BASELINE_REFRESH_ALLOWED"
    return "CREATE_BASELINE_REVIEW_REQUIRED"


def _generate_report_for_spec(
    spec: dict[str, Any],
    *,
    retention_policy_ref: str,
) -> dict[str, Any]:
    if spec["source_kind"] == "fixture":
        return generate_current_report_payload(
            spec["fixture_path"],
            retention_policy_ref=retention_policy_ref,
        )
    return generate_adapter_report_payload(
        spec["adapter_payload_path"],
        retention_policy_ref=retention_policy_ref,
    )


def _report_matrix_entry(
    *,
    spec: dict[str, Any],
    current_report: dict[str, Any],
    review: dict[str, Any],
    retention_policy_ref: str,
) -> dict[str, Any]:
    status = (
        "PASS"
        if _string(current_report.get("status")) == "PASS" and _string(review.get("status")) == "PASS"
        else "FAIL"
    )
    entry = {
        "suite_ref": spec["suite_ref"],
        "source_kind": spec["source_kind"],
        "source_path": str(spec["source_path"]),
        "suite_id": _string(current_report.get("suite_id")),
        "status": status,
        "current_report_status": _string(current_report.get("status")),
        "current_report_path": str(spec["report_out"]),
        "current_report_hash": sha256_json(current_report),
        "case_count": _int(current_report.get("case_count"), "current case_count"),
        "failed_count": _int(current_report.get("failed_count"), "current failed_count"),
        "review_status": _string(review.get("status")),
        "refresh_recommendation": _string(review.get("refresh_recommendation")),
        "approval_required": _approval_required(review),
        "baseline_report_path": _string(review.get("baseline_report_path")),
        "baseline_report_hash": _string(review.get("baseline_report_hash")),
        "blocked_promotion_reasons": list(review.get("blocked_promotion_reasons", [])),
        "retention_policy_ref": retention_policy_ref,
    }
    assert_low_sensitive_eval_payload(entry, "report matrix entry")
    return entry


def _approval_manifest_entry(
    *,
    review_payload: dict[str, Any],
    approval_policy_ref: str,
) -> dict[str, Any]:
    assert_low_sensitive_eval_payload(review_payload, "baseline refresh review")
    approval_required = _approval_required(review_payload)
    decision_status = "PENDING_APPROVAL" if approval_required else "BLOCKED"
    entry_payload = {
        "suite_id": _string(review_payload.get("current_suite_id")),
        "current_report_hash": _string(review_payload.get("current_report_hash")),
        "baseline_report_hash": _string(review_payload.get("baseline_report_hash")),
        "approval_policy_ref": _string(approval_policy_ref),
    }
    entry = {
        "approval_item_ref": f"approvalitem_{sha256_json(entry_payload)[:24]}",
        "suite_id": entry_payload["suite_id"],
        "current_report_path": _string(review_payload.get("current_report_path")),
        "current_report_hash": entry_payload["current_report_hash"],
        "baseline_report_path": _string(review_payload.get("baseline_report_path")),
        "baseline_report_hash": entry_payload["baseline_report_hash"],
        "refresh_recommendation": _string(review_payload.get("refresh_recommendation")),
        "decision_status": decision_status,
        "approval_required": approval_required,
        "blocked_promotion_reasons": list(review_payload.get("blocked_promotion_reasons", [])),
    }
    assert_low_sensitive_eval_payload(entry, "approval manifest entry")
    return entry


def _approval_required(review_payload: dict[str, Any]) -> bool:
    return (
        _string(review_payload.get("status")) == "PASS"
        and _string(review_payload.get("refresh_recommendation"))
        in {"BASELINE_REFRESH_ALLOWED", "CREATE_BASELINE_REVIEW_REQUIRED"}
    )


def _suite_specs(plan_payload: dict[str, Any]) -> list[dict[str, Any]]:
    suites = plan_payload.get("suites")
    if not isinstance(suites, list) or not suites:
        raise ValueError("report matrix plan.suites must be a non-empty list")
    result: list[dict[str, Any]] = []
    for index, item in enumerate(suites):
        if not isinstance(item, dict):
            raise ValueError(f"report matrix plan.suites[{index}] must be an object")
        result.append(item)
    return result


def _resolve_suite_spec(
    spec: dict[str, Any],
    *,
    base_dir: Path,
    index: int,
) -> dict[str, Any]:
    suite_ref = _required_string(spec, "suite_ref", f"suites[{index}]")
    fixture_value = _string(spec.get("fixture_path"))
    adapter_value = _string(spec.get("adapter_payload_path"))
    if bool(fixture_value) == bool(adapter_value):
        raise ValueError(
            f"suites[{index}] must set exactly one of fixture_path or adapter_payload_path"
        )
    report_out = _resolve_path(
        _required_string(spec, "report_out", f"suites[{index}]"),
        base_dir=base_dir,
    )
    review_out = _optional_path(spec.get("review_out"), base_dir=base_dir)
    baseline_path = _optional_path(spec.get("baseline_path"), base_dir=base_dir)
    if fixture_value:
        fixture_path = _resolve_path(fixture_value, base_dir=base_dir)
        return {
            "suite_ref": suite_ref,
            "source_kind": "fixture",
            "source_path": fixture_path,
            "fixture_path": fixture_path,
            "report_out": report_out,
            "review_out": review_out,
            "baseline_path": baseline_path,
        }
    adapter_payload_path = _resolve_path(adapter_value, base_dir=base_dir)
    return {
        "suite_ref": suite_ref,
        "source_kind": "adapter_payload",
        "source_path": adapter_payload_path,
        "adapter_payload_path": adapter_payload_path,
        "report_out": report_out,
        "review_out": review_out,
        "baseline_path": baseline_path,
    }


def _validate_report_matrix_paths(resolved_specs: list[dict[str, Any]]) -> None:
    output_paths: set[Path] = set()
    suite_refs: set[str] = set()
    for spec in resolved_specs:
        suite_ref = spec["suite_ref"]
        if suite_ref in suite_refs:
            raise ValueError(f"duplicate suite_ref: {suite_ref}")
        suite_refs.add(suite_ref)

        report_out = spec["report_out"]
        _remember_output_path(output_paths, report_out)
        ensure_not_same_path(report_out, spec["source_path"], "report_out", "source_path")
        if spec.get("baseline_path"):
            ensure_not_same_path(report_out, spec["baseline_path"], "report_out", "baseline")
        if spec.get("review_out"):
            review_out = spec["review_out"]
            _remember_output_path(output_paths, review_out)
            ensure_not_same_path(report_out, review_out, "report_out", "review_out")
            ensure_not_same_path(review_out, spec["source_path"], "review_out", "source_path")
            if spec.get("baseline_path"):
                ensure_not_same_path(review_out, spec["baseline_path"], "review_out", "baseline")


def _remember_output_path(output_paths: set[Path], path: Path) -> None:
    resolved = path.resolve()
    if resolved in output_paths:
        raise ValueError(f"duplicate output artifact path: {path}")
    output_paths.add(resolved)


def _resolve_path(value: str, *, base_dir: Path) -> Path:
    path = Path(value)
    return path if path.is_absolute() else base_dir / path


def _optional_path(value: Any, *, base_dir: Path) -> Path | None:
    normalized = _string(value)
    if not normalized:
        return None
    return _resolve_path(normalized, base_dir=base_dir)


def _required_string(payload: dict[str, Any], field_name: str, context: str) -> str:
    value = _string(payload.get(field_name))
    if not value:
        raise ValueError(f"{context}.{field_name} is required")
    return value


def _string(value: Any) -> str:
    return str(value or "").strip()


def _int(value: Any, context: str) -> int:
    if not isinstance(value, int):
        raise ValueError(f"{context} must be an integer")
    return value
