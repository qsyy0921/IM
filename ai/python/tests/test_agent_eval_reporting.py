from __future__ import annotations

import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.reporting import (
    build_baseline_refresh_approval_manifest,
    build_baseline_refresh_review,
    build_report_matrix_payload,
    build_retention_metadata,
    ensure_not_same_path,
    generate_current_report_payload,
    load_report_matrix_plan,
    load_report_payload,
    run_report_matrix_plan,
    write_json_artifact,
)


ROOT = Path(__file__).resolve().parents[3]
PYTHON_ROOT = ROOT / "ai" / "python"
CORE_FIXTURE = PYTHON_ROOT / "fixtures" / "agent_eval" / "synthetic_core_scenarios.json"
BASELINE = (
    PYTHON_ROOT / "fixtures" / "agent_eval" / "baselines" / "synthetic_core_scenarios_baseline.json"
)
CLI = PYTHON_ROOT / "scripts" / "run_agent_eval_current_report.py"
MATRIX_CLI = PYTHON_ROOT / "scripts" / "run_agent_eval_report_matrix.py"
QASPER_ADAPTER = (
    PYTHON_ROOT
    / "fixtures"
    / "agent_eval"
    / "adapter_samples"
    / "qasper_like_rag_samples.json"
)


def _baseline_report() -> dict[str, Any]:
    return json.loads(BASELINE.read_text(encoding="utf-8"))


def test_generate_current_report_payload_runs_fixture() -> None:
    report = generate_current_report_payload(CORE_FIXTURE)

    assert report["suite_id"] == "synthetic-agent-core-scenarios-v1"
    assert report["status"] == "PASS"
    assert report["case_count"] >= 10
    assert "replay_completeness" in report["aggregate_scores"]
    assert report["retention_metadata"]["raw_payload_retained"] is False
    assert report["retention_metadata"]["provider_response_retained"] is False


def test_build_baseline_refresh_review_accepts_matching_report() -> None:
    current = generate_current_report_payload(CORE_FIXTURE)
    review = build_baseline_refresh_review(
        current_report_payload=current,
        current_report_path=Path("artifacts/current-report.json"),
        baseline_report_payload=_baseline_report(),
        baseline_report_path=BASELINE,
    )

    assert review["status"] == "PASS"
    assert review["refresh_recommendation"] == "BASELINE_REFRESH_ALLOWED"
    assert review["blocked_promotion_reasons"] == []
    assert review["comparison"]["status"] == "PASS"
    assert review["retention_metadata"]["artifact_kind"] == (
        "agent_eval_baseline_refresh_review"
    )


def test_build_baseline_refresh_review_blocks_regressed_report() -> None:
    current = generate_current_report_payload(CORE_FIXTURE)
    stricter_baseline = deepcopy(current)
    stricter_baseline["aggregate_scores"]["memory_precision"] = 1.0
    review = build_baseline_refresh_review(
        current_report_payload=current,
        current_report_path=Path("artifacts/current-report.json"),
        baseline_report_payload=stricter_baseline,
        baseline_report_path=BASELINE,
    )

    assert review["status"] == "FAIL"
    assert review["refresh_recommendation"] == "BASELINE_REFRESH_BLOCKED"
    assert "aggregate score regressed" in review["blocked_promotion_reasons"]


def test_write_json_artifact_refuses_overwrite_without_force(tmp_path: Path) -> None:
    target = tmp_path / "report.json"
    payload = {"schema_version": 1, "status": "PASS", "safe": "ok"}
    write_json_artifact(target, payload)

    try:
        write_json_artifact(target, payload)
    except ValueError as exc:
        assert "refusing to overwrite" in str(exc)
    else:
        raise AssertionError("expected overwrite to be refused")


def test_load_report_payload_rejects_sensitive_payload(tmp_path: Path) -> None:
    report_path = tmp_path / "unsafe-report.json"
    report_path.write_text(
        json.dumps({"schema_version": 1, "status": "PASS", "backend_url": "http://x"}),
        encoding="utf-8",
    )

    try:
        load_report_payload(report_path, "unsafe report")
    except ValueError as exc:
        assert "forbidden eval field" in str(exc)
    else:
        raise AssertionError("expected sensitive report to be rejected")


def test_ensure_not_same_path_rejects_baseline_overwrite(tmp_path: Path) -> None:
    baseline = tmp_path / "baseline.json"
    baseline.write_text("{}", encoding="utf-8")

    try:
        ensure_not_same_path(baseline, baseline, "report-out", "baseline")
    except ValueError as exc:
        assert "must not overwrite" in str(exc)
    else:
        raise AssertionError("expected same path to be rejected")


def test_current_report_cli_writes_report_and_review(tmp_path: Path) -> None:
    report_out = tmp_path / "current-report.json"
    review_out = tmp_path / "baseline-refresh-review.json"

    completed = subprocess.run(
        [
            sys.executable,
            str(CLI),
            str(CORE_FIXTURE),
            "--report-out",
            str(report_out),
            "--baseline",
            str(BASELINE),
            "--review-out",
            str(review_out),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0
    stdout_payload = json.loads(completed.stdout)
    report_payload = json.loads(report_out.read_text(encoding="utf-8"))
    review_payload = json.loads(review_out.read_text(encoding="utf-8"))
    assert stdout_payload["status"] == "PASS"
    assert report_payload["status"] == "PASS"
    assert review_payload["refresh_recommendation"] == "BASELINE_REFRESH_ALLOWED"
    assert review_payload["current_report_hash"] == stdout_payload["current_report_hash"]


def test_current_report_cli_blocks_regressed_baseline_review(tmp_path: Path) -> None:
    stricter_baseline = generate_current_report_payload(CORE_FIXTURE)
    stricter_baseline["aggregate_scores"]["memory_precision"] = 1.0
    baseline_path = tmp_path / "strict-baseline.json"
    baseline_path.write_text(json.dumps(stricter_baseline, sort_keys=True), encoding="utf-8")
    report_out = tmp_path / "current-report.json"

    completed = subprocess.run(
        [
            sys.executable,
            str(CLI),
            str(CORE_FIXTURE),
            "--report-out",
            str(report_out),
            "--baseline",
            str(baseline_path),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 1
    payload = json.loads(completed.stdout)
    assert payload["status"] == "FAIL"
    assert payload["refresh_recommendation"] == "BASELINE_REFRESH_BLOCKED"
    assert report_out.exists()


def test_current_report_cli_refuses_baseline_overwrite(tmp_path: Path) -> None:
    baseline_path = tmp_path / "baseline.json"
    baseline_path.write_text(json.dumps(_baseline_report(), sort_keys=True), encoding="utf-8")

    completed = subprocess.run(
        [
            sys.executable,
            str(CLI),
            str(CORE_FIXTURE),
            "--report-out",
            str(baseline_path),
            "--baseline",
            str(baseline_path),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 2
    payload = json.loads(completed.stdout)
    assert payload["error_class"] == "MALFORMED_INPUT"
    assert "must not overwrite" in payload["message"]


def test_build_retention_metadata_is_low_sensitive() -> None:
    metadata = build_retention_metadata(artifact_kind="agent_eval_report")

    assert metadata["retention_policy_ref"] == "retention:agent-eval-report:research-v1"
    assert metadata["low_sensitive_only"] is True
    assert metadata["production_data_retained"] is False


def test_build_baseline_refresh_approval_manifest_marks_pending_and_blocked() -> None:
    current = generate_current_report_payload(CORE_FIXTURE)
    passing_review = build_baseline_refresh_review(
        current_report_payload=current,
        current_report_path=Path("artifacts/current-report.json"),
        baseline_report_payload=_baseline_report(),
        baseline_report_path=BASELINE,
    )
    stricter_baseline = deepcopy(current)
    stricter_baseline["aggregate_scores"]["memory_precision"] = 1.0
    blocked_review = build_baseline_refresh_review(
        current_report_payload=current,
        current_report_path=Path("artifacts/current-report-strict.json"),
        baseline_report_payload=stricter_baseline,
        baseline_report_path=BASELINE,
    )

    manifest = build_baseline_refresh_approval_manifest(
        manifest_id="manifest:unit",
        review_payloads=[passing_review, blocked_review],
    )

    assert manifest["status"] == "FAIL"
    assert manifest["approval_required_count"] == 1
    assert manifest["blocked_count"] == 1
    assert {entry["decision_status"] for entry in manifest["entries"]} == {
        "BLOCKED",
        "PENDING_APPROVAL",
    }


def test_build_report_matrix_payload_summarizes_suite_entries() -> None:
    entry = {
        "suite_ref": "suite:core",
        "source_kind": "fixture",
        "source_path": "ai/python/fixtures/agent_eval/synthetic_core_scenarios.json",
        "suite_id": "synthetic-agent-core-scenarios-v1",
        "status": "PASS",
        "current_report_status": "PASS",
        "current_report_path": "artifacts/current-report.json",
        "current_report_hash": "hash:current",
        "case_count": 11,
        "failed_count": 0,
        "review_status": "PASS",
        "refresh_recommendation": "BASELINE_REFRESH_ALLOWED",
        "approval_required": True,
        "baseline_report_path": "artifacts/baseline.json",
        "baseline_report_hash": "hash:baseline",
        "blocked_promotion_reasons": [],
        "retention_policy_ref": "retention:agent-eval-report:research-v1",
    }

    matrix = build_report_matrix_payload(matrix_id="matrix:unit", entries=[entry])

    assert matrix["status"] == "PASS"
    assert matrix["suite_count"] == 1
    assert matrix["approval_required_count"] == 1
    assert matrix["retention_metadata"]["artifact_kind"] == "agent_eval_current_report_matrix"


def test_run_report_matrix_plan_writes_reports_reviews_and_manifest_inputs(
    tmp_path: Path,
) -> None:
    plan = {
        "schema_version": 1,
        "matrix_id": "agent-eval-report-matrix-unit-v1",
        "suites": [
            {
                "suite_ref": "suite:core",
                "fixture_path": str(CORE_FIXTURE),
                "report_out": str(tmp_path / "core-report.json"),
                "baseline_path": str(BASELINE),
                "review_out": str(tmp_path / "core-review.json"),
            },
            {
                "suite_ref": "suite:qasper",
                "adapter_payload_path": str(QASPER_ADAPTER),
                "report_out": str(tmp_path / "qasper-report.json"),
                "review_out": str(tmp_path / "qasper-review.json"),
            },
        ],
    }

    matrix, manifest = run_report_matrix_plan(plan, base_dir=ROOT)

    assert matrix["status"] == "PASS"
    assert matrix["suite_count"] == 2
    assert matrix["approval_required_count"] == 2
    assert manifest["status"] == "PASS"
    assert manifest["approval_required_count"] == 2
    assert (tmp_path / "core-report.json").exists()
    assert (tmp_path / "qasper-review.json").exists()


def test_load_report_matrix_plan_rejects_sensitive_plan(tmp_path: Path) -> None:
    plan_path = tmp_path / "unsafe-plan.json"
    plan_path.write_text(
        json.dumps(
            {
                "schema_version": 1,
                "matrix_id": "unsafe",
                "backend_url": "http://localhost:8080",
                "suites": [],
            }
        ),
        encoding="utf-8",
    )

    try:
        load_report_matrix_plan(plan_path)
    except ValueError as exc:
        assert "forbidden eval field" in str(exc)
    else:
        raise AssertionError("expected sensitive report matrix plan to be rejected")


def test_report_matrix_cli_writes_matrix_and_approval_manifest(tmp_path: Path) -> None:
    plan_path = tmp_path / "matrix-plan.json"
    plan_payload = {
        "schema_version": 1,
        "matrix_id": "agent-eval-report-matrix-cli-v1",
        "suites": [
            {
                "suite_ref": "suite:core",
                "fixture_path": str(CORE_FIXTURE),
                "report_out": str(tmp_path / "core-report.json"),
                "baseline_path": str(BASELINE),
                "review_out": str(tmp_path / "core-review.json"),
            },
            {
                "suite_ref": "suite:qasper",
                "adapter_payload_path": str(QASPER_ADAPTER),
                "report_out": str(tmp_path / "qasper-report.json"),
                "review_out": str(tmp_path / "qasper-review.json"),
            },
        ],
    }
    plan_path.write_text(json.dumps(plan_payload, sort_keys=True), encoding="utf-8")
    matrix_out = tmp_path / "matrix.json"
    approval_manifest_out = tmp_path / "approval-manifest.json"

    completed = subprocess.run(
        [
            sys.executable,
            str(MATRIX_CLI),
            str(plan_path),
            "--matrix-out",
            str(matrix_out),
            "--approval-manifest-out",
            str(approval_manifest_out),
            "--base-dir",
            str(ROOT),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0
    stdout_payload = json.loads(completed.stdout)
    matrix_payload = json.loads(matrix_out.read_text(encoding="utf-8"))
    approval_payload = json.loads(approval_manifest_out.read_text(encoding="utf-8"))
    assert stdout_payload["status"] == "PASS"
    assert matrix_payload["suite_count"] == 2
    assert approval_payload["approval_required_count"] == 2


def test_report_matrix_cli_blocks_regressed_suite(tmp_path: Path) -> None:
    stricter_baseline = generate_current_report_payload(CORE_FIXTURE)
    stricter_baseline["aggregate_scores"]["memory_precision"] = 1.0
    baseline_path = tmp_path / "strict-baseline.json"
    baseline_path.write_text(json.dumps(stricter_baseline, sort_keys=True), encoding="utf-8")
    plan_path = tmp_path / "matrix-plan.json"
    plan_path.write_text(
        json.dumps(
            {
                "schema_version": 1,
                "matrix_id": "agent-eval-report-matrix-regression-v1",
                "suites": [
                    {
                        "suite_ref": "suite:core",
                        "fixture_path": str(CORE_FIXTURE),
                        "report_out": str(tmp_path / "core-report.json"),
                        "baseline_path": str(baseline_path),
                        "review_out": str(tmp_path / "core-review.json"),
                    }
                ],
            },
            sort_keys=True,
        ),
        encoding="utf-8",
    )

    completed = subprocess.run(
        [
            sys.executable,
            str(MATRIX_CLI),
            str(plan_path),
            "--matrix-out",
            str(tmp_path / "matrix.json"),
            "--approval-manifest-out",
            str(tmp_path / "approval-manifest.json"),
            "--base-dir",
            str(ROOT),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 1
    payload = json.loads(completed.stdout)
    assert payload["status"] == "FAIL"
    assert payload["matrix"]["failed_suite_count"] == 1
    assert payload["approval_manifest"]["blocked_count"] == 1
