from __future__ import annotations

import json
import subprocess
import sys
from copy import deepcopy
from pathlib import Path
from typing import Any

import pytest

from nexusim_ai_eval.interview_demo import (
    OutputPaths,
    interview_demo_result_to_payload,
    load_interview_demo_fixture,
    run_interview_demo,
    validate_interview_demo_fixture,
    write_interview_demo_outputs,
)


ROOT = Path(__file__).resolve().parents[3]
FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "interview_send_message_agent_demo.json"
)
CLI = ROOT / "ai" / "python" / "scripts" / "run_agent_interview_demo.py"


def _fixture_payload() -> dict[str, Any]:
    return json.loads(FIXTURE.read_text(encoding="utf-8"))


def _demo_result() -> tuple[dict[str, Any], Any]:
    fixture = load_interview_demo_fixture(FIXTURE)
    result = run_interview_demo(fixture)
    return interview_demo_result_to_payload(result), result


def test_interview_demo_fixture_loads() -> None:
    fixture = load_interview_demo_fixture(FIXTURE)

    assert fixture.demo_id == "interview-send-message-agent-demo-v1"
    assert fixture.message_committed_ref == "message-committed:demo-001"
    assert len(fixture.cases) == 5


def test_interview_demo_rejects_production_backend_connection_fields() -> None:
    payload = _fixture_payload()
    payload["production_backend_connected"] = True

    with pytest.raises(ValueError, match="production backend connection"):
        validate_interview_demo_fixture(payload)


@pytest.mark.parametrize(
    "field_name",
    ["raw_message_body", "raw_prompt", "provider_request_body", "secret"],
)
def test_interview_demo_rejects_raw_provider_and_secret_fields(field_name: str) -> None:
    payload = _fixture_payload()
    payload["cases"][0][field_name] = "redacted-fixture-value"

    with pytest.raises(ValueError):
        validate_interview_demo_fixture(payload)


def test_interview_demo_reports_no_agent_hot_path_impact() -> None:
    summary, _ = _demo_result()

    assert summary["demo_status"] == "PASS"
    assert summary["hot_path_impact"] == "none"
    assert summary["agent_trigger_mode"] == "async_ref_only"
    assert summary["production_backend_connected"] is False
    assert summary["raw_message_body_used"] is False


def test_interview_demo_denied_evidence_refs_do_not_enter_context() -> None:
    _, result = _demo_result()
    denied = next(
        case for case in result.case_results if case.case_id == "denied-evidence-block"
    )

    assert set(denied.forbidden_message_refs).isdisjoint(denied.context_source_refs)
    assert denied.observed_failure_class == "POLICY_DENIED"
    assert denied.status == "PASS"


def test_interview_demo_memory_candidate_never_becomes_active() -> None:
    summary, result = _demo_result()
    memory_case = next(case for case in result.case_results if case.case_id == "memory-needs-review")

    assert memory_case.memory_candidate_state == "needs_review"
    assert all(case.memory_candidate_state != "active" for case in result.case_results)
    assert "memory-candidate:demo:tentative-release-decision" in summary[
        "memory_candidate_refs"
    ]


def test_interview_demo_tool_intent_and_action_do_not_execute_side_effects() -> None:
    summary, result = _demo_result()
    tool_case = next(
        case for case in result.case_results if case.case_id == "unsafe-tool-output-blocked"
    )
    approval_case = next(
        case
        for case in result.case_results
        if case.case_id == "approval-required-not-executed"
    )

    assert tool_case.tool_intent_state == "blocked"
    assert approval_case.action_state == "not_executed"
    assert all(case.action_state != "executed" for case in result.case_results)
    assert result.replay_bundle["side_effect_reexecuted"] is False
    assert all("fixture" in ref for ref in summary["approval_action_refs"])


def test_interview_demo_emits_eval_report_and_replay_bundle_refs() -> None:
    summary, result = _demo_result()

    assert summary["eval_report_ref"].startswith("evalreport_")
    assert summary["replay_bundle_ref"].startswith("replay_")
    assert result.eval_report["eval_report_ref"] == summary["eval_report_ref"]
    assert result.replay_bundle["replay_bundle_ref"] == summary["replay_bundle_ref"]
    assert result.eval_report["status"] == "PASS"
    assert result.replay_bundle["replay_complete"] is True


def test_interview_demo_blocked_cases_include_expected_failure_classes() -> None:
    summary, _ = _demo_result()
    classes = {case["expected_failure_class"] for case in summary["blocked_cases"]}

    assert {
        "POLICY_DENIED",
        "MEMORY_REVIEW_MISSING",
        "UNSAFE_TOOL_OUTPUT",
        "APPROVAL_REQUIRED",
    }.issubset(classes)


def test_interview_demo_refuses_output_overwrite_without_force(tmp_path: Path) -> None:
    _, result = _demo_result()
    summary_out = tmp_path / "summary.json"
    paths = OutputPaths(summary_out=summary_out)

    write_interview_demo_outputs(result, paths)
    with pytest.raises(ValueError, match="refusing to overwrite"):
        write_interview_demo_outputs(result, paths)


def test_interview_demo_cli_writes_summary_eval_report_and_replay(tmp_path: Path) -> None:
    summary_out = tmp_path / "summary.json"
    report_out = tmp_path / "eval-report.json"
    replay_out = tmp_path / "replay-bundle.json"

    completed = subprocess.run(
        [
            sys.executable,
            str(CLI),
            str(FIXTURE),
            "--summary-out",
            str(summary_out),
            "--report-out",
            str(report_out),
            "--replay-out",
            str(replay_out),
        ],
        check=False,
        cwd=ROOT,
        capture_output=True,
        text=True,
    )

    assert completed.returncode == 0
    stdout_summary = json.loads(completed.stdout)
    summary = json.loads(summary_out.read_text(encoding="utf-8"))
    report = json.loads(report_out.read_text(encoding="utf-8"))
    replay = json.loads(replay_out.read_text(encoding="utf-8"))
    assert stdout_summary == summary
    assert summary["demo_status"] == "PASS"
    assert report["eval_report_ref"] == summary["eval_report_ref"]
    assert replay["replay_bundle_ref"] == summary["replay_bundle_ref"]


def test_interview_demo_blocks_forbidden_ref_in_context() -> None:
    payload = deepcopy(_fixture_payload())
    payload["cases"][1]["context_source_refs"].append("message-ref:manager-dm:budget-note")

    with pytest.raises(ValueError, match="context_source_refs includes forbidden refs"):
        validate_interview_demo_fixture(payload)
