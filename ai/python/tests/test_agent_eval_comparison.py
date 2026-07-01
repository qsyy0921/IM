from __future__ import annotations

import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.comparison import compare_eval_reports
from nexusim_ai_eval.evaluator import eval_report_to_payload, run_eval_suite
from nexusim_ai_eval.fixtures import load_eval_suite


ROOT = Path(__file__).resolve().parents[3]
PYTHON_ROOT = ROOT / "ai" / "python"
CORE_FIXTURE = PYTHON_ROOT / "fixtures" / "agent_eval" / "synthetic_core_scenarios.json"
BASELINE = (
    PYTHON_ROOT / "fixtures" / "agent_eval" / "baselines" / "synthetic_core_scenarios_baseline.json"
)
CLI = PYTHON_ROOT / "scripts" / "run_agent_eval_regression.py"


def _core_report() -> dict[str, Any]:
    return eval_report_to_payload(run_eval_suite(load_eval_suite(CORE_FIXTURE)))


def _baseline_report() -> dict[str, Any]:
    return json.loads(BASELINE.read_text(encoding="utf-8"))


def test_compare_eval_reports_accepts_matching_current_report() -> None:
    comparison = compare_eval_reports(_baseline_report(), _core_report())

    assert comparison["status"] == "PASS"
    assert comparison["blocked_promotion_reasons"] == []
    assert comparison["case_count_delta"] == 0


def test_compare_eval_reports_blocks_score_regression() -> None:
    current = _core_report()
    current["aggregate_scores"]["citation_coverage"] = 0.0
    current["results"][0]["scores"]["citation_coverage"] = 0.0

    comparison = compare_eval_reports(_baseline_report(), current)

    assert comparison["status"] == "FAIL"
    assert "aggregate score regressed" in comparison["blocked_promotion_reasons"]
    assert "case-level score or status regressed" in comparison["blocked_promotion_reasons"]


def test_compare_eval_reports_blocks_missing_baseline_case() -> None:
    current = _core_report()
    current["results"] = current["results"][1:]
    current["case_count"] -= 1
    current["passed_count"] -= 1

    comparison = compare_eval_reports(_baseline_report(), current)

    assert comparison["status"] == "FAIL"
    assert "baseline case missing" in comparison["blocked_promotion_reasons"]


def test_compare_eval_reports_rejects_sensitive_payload() -> None:
    current = deepcopy(_core_report())
    current["backend_url"] = "http://localhost:8080"

    try:
        compare_eval_reports(_baseline_report(), current)
    except ValueError as exc:
        assert "forbidden eval field" in str(exc)
    else:
        raise AssertionError("expected sensitive payload to be rejected")


def test_regression_cli_compares_baseline_to_current_report_file(tmp_path: Path) -> None:
    current_path = tmp_path / "current-report.json"
    current_path.write_text(json.dumps(_core_report(), sort_keys=True), encoding="utf-8")

    completed = subprocess.run(
        [sys.executable, str(CLI), str(BASELINE), str(current_path)],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0
    payload = json.loads(completed.stdout)
    assert payload["status"] == "PASS"


def test_regression_cli_fails_closed_on_malformed_report(tmp_path: Path) -> None:
    malformed = tmp_path / "malformed.json"
    malformed.write_text(json.dumps({"schema_version": 1}), encoding="utf-8")

    completed = subprocess.run(
        [sys.executable, str(CLI), str(BASELINE), str(malformed)],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 2
    payload = json.loads(completed.stdout)
    assert payload["error_class"] == "MALFORMED_INPUT"
