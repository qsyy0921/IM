from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.multi_agent_handoff import (
    load_multi_agent_handoff_rehearsal,
    rehearse_multi_agent_handoff,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "multi_agent_handoff_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_multi_agent_handoff_rehearsal_passes_fixture() -> None:
    result = load_multi_agent_handoff_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["handoff_results"]} == {"PASS"}
    assert {item["status"] for item in result["scenario_coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_multi_agent_handoff_blocks_missing_peer_scenario() -> None:
    payload = _fixture_payload()
    payload["handoff_records"] = [
        record
        for record in payload["handoff_records"]
        if record["scenario_kind"] != "future-peer-agent-candidate"
    ]

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert "multi-agent scenario missing coverage: future-peer-agent-candidate" in result[
        "blocked_promotion_reasons"
    ]


def test_multi_agent_handoff_blocks_scope_widening() -> None:
    payload = _fixture_payload()
    payload["handoff_records"][0]["scope_widened"] = True

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert "multi-agent handoff widened scope: handoff:internal-specialist:project-summary" in result[
        "blocked_promotion_reasons"
    ]


def test_multi_agent_handoff_blocks_missing_visible_evidence() -> None:
    payload = _fixture_payload()
    payload["handoff_records"][0]["visible_evidence_refs"] = []

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert (
        "multi-agent handoff lacks visible evidence refs: "
        "handoff:internal-specialist:project-summary"
    ) in result["blocked_promotion_reasons"]


def test_multi_agent_handoff_blocks_missing_primary_responsibility() -> None:
    payload = _fixture_payload()
    payload["handoff_records"][0]["primary_final_responsibility"] = False

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert "primary agent lacks final responsibility: handoff:internal-specialist:project-summary" in result[
        "blocked_promotion_reasons"
    ]


def test_multi_agent_handoff_blocks_unverified_integration() -> None:
    payload = _fixture_payload()
    payload["handoff_records"][2]["verifier_passed"] = False

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert "unverified specialist output integrated: handoff:multi-specialist:incident-review" in result[
        "blocked_promotion_reasons"
    ]


def test_multi_agent_handoff_blocks_direct_memory_admission() -> None:
    payload = _fixture_payload()
    payload["handoff_records"][1]["direct_memory_admission_allowed"] = True

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert (
        "multi-agent handoff allowed direct memory admission: "
        "handoff:future-peer-agent:policy-analysis"
    ) in result["blocked_promotion_reasons"]


def test_multi_agent_handoff_blocks_release_for_failed_handoff() -> None:
    payload = deepcopy(_fixture_payload())
    payload["handoff_records"][2]["direct_tool_execution_allowed"] = True

    result = rehearse_multi_agent_handoff(payload)

    assert result["status"] == "FAIL"
    assert "failed multi-agent handoff evidence not blocked: multi-agent-gate:bounded-delegation" in result[
        "blocked_promotion_reasons"
    ]
