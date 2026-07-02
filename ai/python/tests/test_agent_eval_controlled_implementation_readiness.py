from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.controlled_implementation_readiness import (
    load_controlled_implementation_readiness_rehearsal,
    rehearse_controlled_implementation_readiness,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "controlled_implementation_readiness_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def _result_by_gate(result: dict[str, Any], gate_ref: str) -> dict[str, str]:
    for item in result["readiness_results"]:
        if item["record_ref"] == gate_ref:
            return item
    raise AssertionError(f"missing gate result: {gate_ref}")


def test_controlled_implementation_readiness_rehearsal_passes_fixture() -> None:
    result = load_controlled_implementation_readiness_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["readiness_results"]} == {"PASS"}
    assert {item["status"] for item in result["scenario_coverage_results"]} == {"PASS"}
    assert _result_by_gate(
        result,
        "implementation-gate:fixture-hardening-allowed",
    )["safe_decision"] == "ALLOW"
    assert _result_by_gate(
        result,
        "implementation-gate:block-unaccepted-adr",
    )["safe_decision"] == "BLOCK"


def test_controlled_implementation_blocks_unaccepted_adr_allow() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][1]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][1]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:block-unaccepted-adr")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "accepted ADR missing for controlled implementation: "
        "implementation-gate:block-unaccepted-adr"
    )
    assert "implementation readiness gate contradicted safe policy: implementation-gate:block-unaccepted-adr" in result[
        "blocked_promotion_reasons"
    ]


def test_controlled_implementation_blocks_production_path_change() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][0]["production_path_changed"] = True
    payload["readiness_gate_records"][0]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][0]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:fixture-hardening-allowed")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "production path changed in isolated Agent Lab: "
        "implementation-gate:fixture-hardening-allowed"
    )


def test_controlled_implementation_blocks_missing_preservation_smoke() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][0]["missing_preservation_smoke"] = True
    payload["readiness_gate_records"][0]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][0]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:fixture-hardening-allowed")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "cross-service preservation evidence missing: "
        "implementation-gate:fixture-hardening-allowed"
    )


def test_controlled_implementation_blocks_python_final_owner() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][0]["python_final_owner"] = True
    payload["readiness_gate_records"][0]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][0]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:fixture-hardening-allowed")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "Python owns final Agent state: implementation-gate:fixture-hardening-allowed"
    )


def test_controlled_implementation_blocks_open_p1() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][0]["p1_open"] = True
    payload["readiness_gate_records"][0]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][0]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:fixture-hardening-allowed")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "open P1 finding blocks implementation readiness: "
        "implementation-gate:fixture-hardening-allowed"
    )


def test_controlled_implementation_blocks_missing_owner_review() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"][1]["accepted_adr"] = True
    payload["readiness_gate_records"][1]["main_review_accepted"] = True
    payload["readiness_gate_records"][1]["expected_gate_decision"] = "ALLOW"
    payload["readiness_gate_records"][1]["actual_gate_decision"] = "ALLOW"

    result = rehearse_controlled_implementation_readiness(payload)

    gate_result = _result_by_gate(result, "implementation-gate:block-unaccepted-adr")
    assert result["status"] == "FAIL"
    assert gate_result["safe_reason"] == (
        "owner review missing: implementation-gate:block-unaccepted-adr"
    )


def test_controlled_implementation_blocks_missing_scenario_coverage() -> None:
    payload = _fixture_payload()
    payload["readiness_gate_records"] = [
        record
        for record in payload["readiness_gate_records"]
        if record["scenario_kind"] != "unsafe-shortcut-blocked"
    ]

    result = rehearse_controlled_implementation_readiness(payload)

    assert result["status"] == "FAIL"
    assert (
        "controlled implementation readiness missing scenario: "
        "unsafe-shortcut-blocked"
    ) in result["blocked_promotion_reasons"]
