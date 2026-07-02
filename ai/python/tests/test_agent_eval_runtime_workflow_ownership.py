from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.runtime_workflow_ownership import (
    load_runtime_workflow_ownership_rehearsal,
    rehearse_runtime_workflow_ownership,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "runtime_workflow_ownership_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_runtime_workflow_ownership_rehearsal_passes_fixture() -> None:
    result = load_runtime_workflow_ownership_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "ownership_results",
        "checkpoint_results",
        "wakeup_results",
        "resume_results",
        "operator_control_results",
        "budget_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_runtime_workflow_ownership_blocks_duplicate_wakeup_consumption() -> None:
    payload = _fixture_payload()
    payload["wakeup_records"][1]["consumption_status"] = "CONSUMED"

    result = rehearse_runtime_workflow_ownership(payload)

    assert result["status"] == "FAIL"
    assert (
        "multiple wakeups consumed for dedupe key: dedupe:workflow-decision:approval-42"
        in result["blocked_promotion_reasons"]
    )


def test_runtime_workflow_ownership_blocks_unrejected_stale_checkpoint() -> None:
    payload = _fixture_payload()
    payload["checkpoint_records"][0]["rejected_stale_checkpoint_refs"] = []

    result = rehearse_runtime_workflow_ownership(payload)

    assert result["status"] == "FAIL"
    assert (
        "stale checkpoint refs not rejected: checkpoint:approval-wait:v2"
        in result["blocked_promotion_reasons"]
    )


def test_runtime_workflow_ownership_blocks_cancelled_resume() -> None:
    payload = _fixture_payload()
    payload["resume_records"][2]["resume_status"] = "RESUMED"

    result = rehearse_runtime_workflow_ownership(payload)

    assert result["status"] == "FAIL"
    assert "resume ignored cancelled state: resume:approval-wait:cancelled" in result[
        "blocked_promotion_reasons"
    ]


def test_runtime_workflow_ownership_blocks_operator_replay_side_effect() -> None:
    payload = deepcopy(_fixture_payload())
    payload["operator_control_records"][2]["side_effect_reexecuted"] = True

    result = rehearse_runtime_workflow_ownership(payload)

    assert result["status"] == "FAIL"
    assert "operator replay re-executes side effect: agentops-control:replay:approval-wait" in result[
        "blocked_promotion_reasons"
    ]


def test_runtime_workflow_ownership_blocks_over_budget_continuation() -> None:
    payload = _fixture_payload()
    payload["budget_ledger_records"][1]["continued_after_over_budget"] = True

    result = rehearse_runtime_workflow_ownership(payload)

    assert result["status"] == "FAIL"
    assert "over-budget run continued: budget-ledger:approval-wait-over" in result[
        "blocked_promotion_reasons"
    ]
