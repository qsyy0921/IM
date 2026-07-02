from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.agentops_governance import (
    load_agentops_governance_rehearsal,
    rehearse_agentops_governance,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "agentops_governance_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_agentops_governance_rehearsal_passes_fixture() -> None:
    result = load_agentops_governance_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "ownership_results",
        "release_gate_results",
        "kill_switch_results",
        "baseline_approval_results",
        "failure_class_results",
        "canary_shadow_results",
        "operator_control_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_agentops_governance_blocks_release_with_p0_p1_failure() -> None:
    payload = _fixture_payload()
    payload["release_gate_records"][1]["expected_gate_decision"] = "ALLOW"
    payload["release_gate_records"][1]["actual_gate_decision"] = "ALLOW"
    payload["release_gate_records"][1]["production_enabled"] = True

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "P0/P1 eval failure did not block release: agent-release:copilot:blocked-p1"
        in result["blocked_promotion_reasons"]
    )


def test_agentops_governance_blocks_active_switch_allowing_new_runs() -> None:
    payload = _fixture_payload()
    payload["kill_switch_records"][0]["new_runs_allowed"] = True

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "active kill switch allows new runs: kill-switch:agent-definition:copilot-v4"
        in result["blocked_promotion_reasons"]
    )


def test_agentops_governance_blocks_silent_baseline_refresh() -> None:
    payload = _fixture_payload()
    payload["baseline_approval_records"][0]["silent_refresh"] = True

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert "baseline silently refreshed: baseline:copilot:rag-tool-memory:v3" in result[
        "blocked_promotion_reasons"
    ]


def test_agentops_governance_blocks_unowned_p1_failure() -> None:
    payload = _fixture_payload()
    payload["failure_class_records"][0]["owner_ref"] = ""

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert "P0/P1 failure lacks owner: failure-class:permission-leakage" in result[
        "blocked_promotion_reasons"
    ]


def test_agentops_governance_blocks_incomparable_canary_metrics() -> None:
    payload = _fixture_payload()
    payload["canary_shadow_records"][0]["comparable_metric_refs"].remove(
        "metric:memory-pollution"
    )

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "canary metrics not comparable to offline baseline: canary:copilot:v4:beta"
        in result["blocked_promotion_reasons"]
    )


def test_agentops_governance_blocks_operator_python_override() -> None:
    payload = _fixture_payload()
    payload["operator_control_records"][2]["python_worker_override"] = True

    result = rehearse_agentops_governance(payload)

    assert result["status"] == "FAIL"
    assert "python worker overrides AgentOps operator action: agentops-operator:rollback:v4" in result[
        "blocked_promotion_reasons"
    ]
