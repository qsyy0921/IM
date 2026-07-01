from __future__ import annotations

import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.memory_calibration import (
    load_memory_calibration_payload,
    run_memory_admission_calibration,
)


ROOT = Path(__file__).resolve().parents[3]
PYTHON_ROOT = ROOT / "ai" / "python"
CALIBRATION_SAMPLE = (
    PYTHON_ROOT / "fixtures" / "agent_eval" / "memory_calibration_sample.json"
)
CLI = PYTHON_ROOT / "scripts" / "run_agent_memory_calibration.py"


def _sample_payload() -> dict[str, Any]:
    return json.loads(CALIBRATION_SAMPLE.read_text(encoding="utf-8"))


def test_run_memory_calibration_recommends_all_policies() -> None:
    report = run_memory_admission_calibration(_sample_payload())

    assert report["status"] == "PASS"
    assert report["recommended_confidence_threshold_ref"] == (
        "confidence-threshold:memory-admission:v1:0.60"
    )
    assert report["recommended_policy_revocation_window_ref"] == (
        "revocation-window:policy-source:research-v1:close-after-24h"
    )
    assert report["recommended_review_backoff_policy_ref"] == (
        "review-backoff:memory-admission:research-v1:initial-10m"
    )
    assert report["memory_gate_case_count"] == 6
    assert report["policy_window_case_count"] == 3
    assert report["review_backoff_case_count"] == 5
    assert report["blocked_promotion_reasons"] == []
    assert report["retention_metadata"]["raw_payload_retained"] is False
    assert report["retention_metadata"]["production_data_retained"] is False


def test_confidence_threshold_candidates_report_mismatches() -> None:
    report = run_memory_admission_calibration(_sample_payload())
    threshold_results = {
        result["threshold_ref"]: result for result in report["confidence_threshold_results"]
    }

    low_threshold = threshold_results["confidence-threshold:memory-admission:v1:0.40"]
    high_threshold = threshold_results["confidence-threshold:memory-admission:v1:0.75"]
    recommended = threshold_results["confidence-threshold:memory-admission:v1:0.60"]

    assert low_threshold["meets_acceptance"] is False
    assert high_threshold["meets_acceptance"] is False
    assert recommended["meets_acceptance"] is True
    assert low_threshold["mismatches"][0]["case_id"] == (
        "locomo-like-ambiguous-group-owner-reject"
    )
    assert high_threshold["mismatches"][0]["case_id"] == (
        "longmemeval-like-personal-repeated-preference-auto-admit"
    )
    assert recommended["exact_match_rate"] == 1.0
    assert recommended["pollution_block_rate"] == 1.0


def test_policy_window_candidates_report_mismatches() -> None:
    report = run_memory_admission_calibration(_sample_payload())
    window_results = {
        result["window_ref"]: result
        for result in report["policy_revocation_window_results"]
    }

    short_window = window_results[
        "revocation-window:policy-source:research-v1:close-after-2h"
    ]
    recommended = window_results[
        "revocation-window:policy-source:research-v1:close-after-24h"
    ]

    assert short_window["meets_acceptance"] is False
    assert short_window["mismatches"][0]["case_id"] == (
        "policy-window-tenant-retention-recent-open"
    )
    assert recommended["meets_acceptance"] is True
    assert recommended["match_rate"] == 1.0
    assert recommended["retention_policy_match_rate"] == 1.0


def test_review_backoff_candidates_report_mismatches() -> None:
    report = run_memory_admission_calibration(_sample_payload())
    backoff_results = {
        result["backoff_policy_ref"]: result for result in report["review_backoff_results"]
    }

    slow_backoff = backoff_results["review-backoff:memory-admission:research-v1:initial-30m"]
    fast_escalation = backoff_results[
        "review-backoff:memory-admission:research-v1:escalate-60m"
    ]
    recommended = backoff_results[
        "review-backoff:memory-admission:research-v1:initial-10m"
    ]

    assert slow_backoff["meets_acceptance"] is False
    assert slow_backoff["mismatches"][0]["case_id"] == "review-backoff-first-retry"
    assert fast_escalation["meets_acceptance"] is False
    assert fast_escalation["mismatches"][0]["case_id"] == "review-backoff-second-retry"
    assert recommended["meets_acceptance"] is True
    assert recommended["action_match_rate"] == 1.0


def test_memory_calibration_blocks_when_no_threshold_meets_acceptance() -> None:
    payload = _sample_payload()
    payload["confidence_threshold_candidates"] = [
        candidate
        for candidate in payload["confidence_threshold_candidates"]
        if candidate["threshold_ref"] == "confidence-threshold:memory-admission:v1:0.75"
    ]

    report = run_memory_admission_calibration(payload)

    assert report["status"] == "FAIL"
    assert report["recommended_confidence_threshold_ref"] == ""
    assert report["recommended_policy_revocation_window_ref"]
    assert report["recommended_review_backoff_policy_ref"]
    assert report["blocked_promotion_reasons"] == [
        "memory calibration blocked: no confidence threshold meets acceptance"
    ]


def test_load_memory_calibration_payload_rejects_sensitive_payload(tmp_path: Path) -> None:
    unsafe_path = tmp_path / "unsafe-calibration.json"
    payload = _sample_payload()
    payload["backend_url"] = "http://localhost:8080"
    unsafe_path.write_text(json.dumps(payload, sort_keys=True), encoding="utf-8")

    try:
        load_memory_calibration_payload(unsafe_path)
    except ValueError as exc:
        assert "forbidden eval field" in str(exc)
    else:
        raise AssertionError("expected sensitive memory calibration payload to be rejected")


def test_memory_calibration_cli_writes_report(tmp_path: Path) -> None:
    report_out = tmp_path / "memory-calibration-report.json"

    completed = subprocess.run(
        [
            sys.executable,
            str(CLI),
            str(CALIBRATION_SAMPLE),
            "--report-out",
            str(report_out),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0
    stdout_payload = json.loads(completed.stdout)
    report_payload = json.loads(report_out.read_text(encoding="utf-8"))
    assert stdout_payload["status"] == "PASS"
    assert report_payload["recommended_confidence_threshold_ref"] == (
        stdout_payload["recommended_confidence_threshold_ref"]
    )


def test_memory_calibration_cli_exits_one_for_blocked_calibration(
    tmp_path: Path,
) -> None:
    payload = deepcopy(_sample_payload())
    payload["confidence_threshold_candidates"] = [
        candidate
        for candidate in payload["confidence_threshold_candidates"]
        if candidate["threshold_ref"] == "confidence-threshold:memory-admission:v1:0.75"
    ]
    payload_path = tmp_path / "blocked-calibration.json"
    payload_path.write_text(json.dumps(payload, sort_keys=True), encoding="utf-8")

    completed = subprocess.run(
        [sys.executable, str(CLI), str(payload_path)],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 1
    stdout_payload = json.loads(completed.stdout)
    assert stdout_payload["status"] == "FAIL"
    assert "no confidence threshold meets acceptance" in (
        stdout_payload["blocked_promotion_reasons"][0]
    )
