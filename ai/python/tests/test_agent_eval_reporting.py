from __future__ import annotations

import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.reporting import (
    build_baseline_refresh_review,
    ensure_not_same_path,
    generate_current_report_payload,
    load_report_payload,
    write_json_artifact,
)


ROOT = Path(__file__).resolve().parents[3]
PYTHON_ROOT = ROOT / "ai" / "python"
CORE_FIXTURE = PYTHON_ROOT / "fixtures" / "agent_eval" / "synthetic_core_scenarios.json"
BASELINE = (
    PYTHON_ROOT / "fixtures" / "agent_eval" / "baselines" / "synthetic_core_scenarios_baseline.json"
)
CLI = PYTHON_ROOT / "scripts" / "run_agent_eval_current_report.py"


def _baseline_report() -> dict[str, Any]:
    return json.loads(BASELINE.read_text(encoding="utf-8"))


def test_generate_current_report_payload_runs_fixture() -> None:
    report = generate_current_report_payload(CORE_FIXTURE)

    assert report["suite_id"] == "synthetic-agent-core-scenarios-v1"
    assert report["status"] == "PASS"
    assert report["case_count"] >= 10
    assert "replay_completeness" in report["aggregate_scores"]


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
