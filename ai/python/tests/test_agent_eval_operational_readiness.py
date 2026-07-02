from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.operational_readiness import (
    load_operational_readiness_rehearsal,
    rehearse_operational_readiness,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "operational_readiness_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_operational_readiness_rehearsal_passes_fixture() -> None:
    result = load_operational_readiness_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["budget_results"]} == {"PASS"}
    assert {item["status"] for item in result["budget_coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_operational_readiness_blocks_missing_budget_coverage() -> None:
    payload = _fixture_payload()
    payload["budget_records"] = [
        record
        for record in payload["budget_records"]
        if record["budget_kind"] != "canary-telemetry-budget"
    ]

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert "operational budget missing coverage: canary-telemetry-budget" in result[
        "blocked_promotion_reasons"
    ]


def test_operational_readiness_blocks_owner_mismatch() -> None:
    payload = _fixture_payload()
    payload["budget_records"][0]["owner_ref"] = "python-worker"

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert "operational budget owner mismatch: budget:runtime-step:standard-agent" in result[
        "blocked_promotion_reasons"
    ]


def test_operational_readiness_blocks_missing_measurement() -> None:
    payload = _fixture_payload()
    payload["budget_records"][1]["missing_measurement"] = True

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert "operational budget lacks measurement evidence: budget:model-spend:agent-run" in result[
        "blocked_promotion_reasons"
    ]


def test_operational_readiness_blocks_over_limit_continuation() -> None:
    payload = _fixture_payload()
    payload["budget_records"][2]["limit_exceeded"] = True
    payload["budget_records"][2]["continued_after_limit"] = True

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert "operational budget continued after limit: budget:tool-timeout:mcp-prepare" in result[
        "blocked_promotion_reasons"
    ]


def test_operational_readiness_blocks_raw_body_retention() -> None:
    payload = _fixture_payload()
    payload["budget_records"][4]["raw_body_retained"] = True

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert (
        "operational budget retains raw body: "
        "budget:eval-report-retention:low-sensitive"
    ) in result["blocked_promotion_reasons"]


def test_operational_readiness_blocks_production_slo_authorization() -> None:
    payload = _fixture_payload()
    payload["budget_records"][6]["production_slo_authorized"] = True

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert (
        "operational fixture authorizes production SLO: "
        "budget:incident-escalation:p0p1"
    ) in result["blocked_promotion_reasons"]


def test_operational_readiness_blocks_release_for_failed_evidence() -> None:
    payload = deepcopy(_fixture_payload())
    payload["budget_records"][5]["unreviewed_capacity_allowed"] = True

    result = rehearse_operational_readiness(payload)

    assert result["status"] == "FAIL"
    assert "failed operational readiness evidence not blocked: operational-gate:complete-budgets" in result[
        "blocked_promotion_reasons"
    ]
