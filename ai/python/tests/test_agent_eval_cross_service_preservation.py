from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.cross_service_preservation import (
    load_cross_service_preservation_rehearsal,
    rehearse_cross_service_preservation,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "cross_service_preservation_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_cross_service_preservation_rehearsal_passes_fixture() -> None:
    result = load_cross_service_preservation_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["boundary_results"]} == {"PASS"}
    assert {item["status"] for item in result["coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_cross_service_preservation_blocks_missing_required_boundary() -> None:
    payload = _fixture_payload()
    payload["boundary_preservation_records"] = [
        record
        for record in payload["boundary_preservation_records"]
        if record["boundary_kind"] != "audit_to_agentops"
    ]

    result = rehearse_cross_service_preservation(payload)

    assert result["status"] == "FAIL"
    assert "required boundary preservation record missing: audit_to_agentops" in result[
        "blocked_promotion_reasons"
    ]


def test_cross_service_preservation_blocks_dropped_memory_version_ref() -> None:
    payload = _fixture_payload()
    memory_record = payload["boundary_preservation_records"][1]
    memory_record["actual_ref_roles"]["memory_version_refs"] = []

    result = rehearse_cross_service_preservation(payload)

    assert result["status"] == "FAIL"
    assert "boundary dropped required memory_version_refs: boundary:memory-service-to-runtime" in result[
        "blocked_promotion_reasons"
    ]


def test_cross_service_preservation_blocks_scope_widening() -> None:
    payload = _fixture_payload()
    payload["boundary_preservation_records"][3]["scope_widened"] = True

    result = rehearse_cross_service_preservation(payload)

    assert result["status"] == "FAIL"
    assert "boundary widened scope: boundary:workflow-service-to-runtime" in result[
        "blocked_promotion_reasons"
    ]


def test_cross_service_preservation_blocks_raw_payload_exposure() -> None:
    payload = _fixture_payload()
    payload["boundary_preservation_records"][4]["raw_payload_exposed"] = True

    result = rehearse_cross_service_preservation(payload)

    assert result["status"] == "FAIL"
    assert "boundary exposed raw payload: boundary:action-executor-to-eval-replay" in result[
        "blocked_promotion_reasons"
    ]


def test_cross_service_preservation_blocks_release_for_failed_boundary() -> None:
    payload = deepcopy(_fixture_payload())
    payload["boundary_preservation_records"][2]["actual_ref_roles"]["schema_hash_refs"] = []

    result = rehearse_cross_service_preservation(payload)

    assert result["status"] == "FAIL"
    assert "failed preservation boundary not blocked: boundary:mcp-gateway-to-runtime" in result[
        "blocked_promotion_reasons"
    ]
