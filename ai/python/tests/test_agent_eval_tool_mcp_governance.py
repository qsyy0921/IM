from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.tool_mcp_governance import (
    load_tool_mcp_governance_rehearsal,
    rehearse_tool_mcp_governance,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "tool_mcp_governance_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_tool_mcp_governance_rehearsal_passes_fixture() -> None:
    result = load_tool_mcp_governance_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "ownership_results",
        "capability_lease_results",
        "provider_attestation_results",
        "prepare_results",
        "provider_onboarding_results",
        "tool_output_results",
        "execution_handoff_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_tool_mcp_governance_blocks_invalid_lease_prepare() -> None:
    payload = _fixture_payload()
    payload["capability_lease_records"][1]["prepare_allowed"] = True

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "invalid lease allowed prepare: capability-lease:approval-writer:expired-v1"
        in result["blocked_promotion_reasons"]
    )


def test_tool_mcp_governance_blocks_untrusted_provider_selection() -> None:
    payload = _fixture_payload()
    payload["provider_attestation_records"][1]["trusted_provider_selected"] = True

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "untrusted provider selected as trusted: attestation:mcp-provider:trusted-approval-writer:v1"
        in result["blocked_promotion_reasons"]
    )


def test_tool_mcp_governance_blocks_execution_before_reprepare() -> None:
    payload = deepcopy(_fixture_payload())
    payload["prepare_records"][1]["executed_before_reprepare"] = True

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert "execution happened before re-prepare: prepared-tool:approval-writer:stale-v1" in result[
        "blocked_promotion_reasons"
    ]


def test_tool_mcp_governance_blocks_unknown_provider_production_prepare() -> None:
    payload = _fixture_payload()
    payload["provider_onboarding_records"][0]["production_prepare_allowed"] = True

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert "unreviewed provider allowed production prepare: mcp-provider:unknown-writer" in result[
        "blocked_promotion_reasons"
    ]


def test_tool_mcp_governance_blocks_tool_output_as_permission_authority() -> None:
    payload = _fixture_payload()
    payload["tool_output_records"][0]["used_as_permission_authority"] = True

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert "tool output used as permission authority: tool-output:approval-writer:dry-run" in result[
        "blocked_promotion_reasons"
    ]


def test_tool_mcp_governance_blocks_executor_accepting_stale_prepare() -> None:
    payload = _fixture_payload()
    payload["execution_handoff_records"][1]["execution_status"] = "ACCEPTED"

    result = rehearse_tool_mcp_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "stale prepare accepted by executor: execution:approval-writer:stale-prepare-rejected"
        in result["blocked_promotion_reasons"]
    )
