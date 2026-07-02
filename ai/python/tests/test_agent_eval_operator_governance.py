from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.operator_governance import (
    load_operator_governance_rehearsal,
    rehearse_operator_governance,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "operator_governance_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_operator_governance_rehearsal_passes_fixture() -> None:
    result = load_operator_governance_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["operator_surface_results"]} == {"PASS"}
    assert {item["status"] for item in result["surface_coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_operator_governance_blocks_missing_surface_coverage() -> None:
    payload = _fixture_payload()
    payload["operator_surface_records"] = [
        record
        for record in payload["operator_surface_records"]
        if record["surface_kind"] != "rollback-governance"
    ]

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert "operator surface missing coverage: rollback-governance" in result[
        "blocked_promotion_reasons"
    ]


def test_operator_governance_blocks_actionless_memory_view() -> None:
    payload = _fixture_payload()
    payload["operator_surface_records"][0]["actionless_view"] = True

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "operator governance surface is inspect-only without action: "
        "operator-surface:memory-governance"
    ) in result["blocked_promotion_reasons"]


def test_operator_governance_blocks_unauthorized_approval_actor() -> None:
    payload = _fixture_payload()
    payload["operator_surface_records"][3]["unauthorized_actor_allowed"] = True

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "operator governance surface allows unauthorized actor: "
        "operator-surface:approval-governance"
    ) in result["blocked_promotion_reasons"]


def test_operator_governance_blocks_body_exposure() -> None:
    payload = _fixture_payload()
    payload["operator_surface_records"][1]["body_exposed"] = True

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert "operator governance surface exposes body payload: operator-surface:evidence-governance" in result[
        "blocked_promotion_reasons"
    ]


def test_operator_governance_blocks_python_override() -> None:
    payload = _fixture_payload()
    payload["operator_surface_records"][6]["python_override_allowed"] = True

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "python override allowed for operator governance surface: "
        "operator-surface:kill-switch-governance"
    ) in result["blocked_promotion_reasons"]


def test_operator_governance_blocks_release_for_failed_surface() -> None:
    payload = deepcopy(_fixture_payload())
    payload["operator_surface_records"][5]["missing_audit"] = True

    result = rehearse_operator_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "failed operator governance evidence not blocked: "
        "operator-governance-gate:complete-surfaces"
    ) in result["blocked_promotion_reasons"]
