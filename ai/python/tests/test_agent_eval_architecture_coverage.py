from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.architecture_coverage import (
    load_architecture_coverage_rehearsal,
    rehearse_architecture_coverage,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "architecture_coverage_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_architecture_coverage_rehearsal_passes_fixture() -> None:
    result = load_architecture_coverage_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert len(result["surface_results"]) == 13
    assert len(result["surface_coverage_results"]) == 13
    assert {item["status"] for item in result["surface_results"]} == {"PASS"}
    assert {item["status"] for item in result["surface_coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_architecture_coverage_blocks_missing_surface() -> None:
    payload = _fixture_payload()
    payload["surface_records"] = [
        record
        for record in payload["surface_records"]
        if record["surface_kind"] != "multi-agent-a2a-boundary"
    ]

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert "architecture surface missing coverage: multi-agent-a2a-boundary" in result[
        "blocked_promotion_reasons"
    ]


def test_architecture_coverage_blocks_missing_version_policy() -> None:
    payload = _fixture_payload()
    payload["surface_records"][1]["missing_version"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert "architecture surface lacks version policy: architecture-surface:eval-replay-harness" in result[
        "blocked_promotion_reasons"
    ]


def test_architecture_coverage_blocks_open_p1() -> None:
    payload = _fixture_payload()
    payload["surface_records"][5]["p1_open"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert "architecture surface has open P1: architecture-surface:workflow-hitl-approval" in result[
        "blocked_promotion_reasons"
    ]


def test_architecture_coverage_blocks_python_final_owner() -> None:
    payload = _fixture_payload()
    payload["surface_records"][3]["python_final_owner"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert "Python owns final state for architecture surface: architecture-surface:memory-admission" in result[
        "blocked_promotion_reasons"
    ]


def test_architecture_coverage_blocks_production_contract_authorization() -> None:
    payload = _fixture_payload()
    payload["surface_records"][9]["production_contract_authorized"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert (
        "architecture fixture authorizes production contract: "
        "architecture-surface:contract-versioning-replay-policy"
    ) in result["blocked_promotion_reasons"]


def test_architecture_coverage_blocks_missing_operator_path() -> None:
    payload = _fixture_payload()
    payload["surface_records"][11]["missing_operator"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert (
        "architecture surface lacks operator path: "
        "architecture-surface:security-privacy-audit-operator-ux"
    ) in result["blocked_promotion_reasons"]


def test_architecture_coverage_blocks_release_for_failed_surface() -> None:
    payload = deepcopy(_fixture_payload())
    payload["surface_records"][10]["missing_preservation"] = True

    result = rehearse_architecture_coverage(payload)

    assert result["status"] == "FAIL"
    assert (
        "failed architecture coverage evidence not blocked: "
        "architecture-coverage-gate:complete-surfaces"
    ) in result["blocked_promotion_reasons"]
