from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.context_evidence_preservation import (
    load_context_evidence_preservation_rehearsal,
    rehearse_context_evidence_preservation,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "context_evidence_preservation_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_context_evidence_preservation_rehearsal_passes_fixture() -> None:
    result = load_context_evidence_preservation_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "source_visibility_results",
        "denied_lane_results",
        "boundary_preservation_results",
        "citation_verifier_results",
        "taint_preservation_results",
        "operator_inspect_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_context_evidence_preservation_blocks_denied_lane_body_exposure() -> None:
    payload = _fixture_payload()
    payload["denied_lane_records"][0]["body_exposed"] = True

    result = rehearse_context_evidence_preservation(payload)

    assert result["status"] == "FAIL"
    assert "denied lane body exposed: lane:cross-tenant-project" in result[
        "blocked_promotion_reasons"
    ]


def test_context_evidence_preservation_blocks_boundary_ref_loss() -> None:
    payload = _fixture_payload()
    payload["boundary_preservation_records"][0]["actual_preserved_refs"] = [
        "evidence-pack:project-alpha:v3"
    ]

    result = rehearse_context_evidence_preservation(payload)

    assert result["status"] == "FAIL"
    assert "boundary dropped required refs: boundary:retrieval-gateway-to-runtime" in result[
        "blocked_promotion_reasons"
    ]


def test_context_evidence_preservation_blocks_unsupported_citation_finalization() -> None:
    payload = deepcopy(_fixture_payload())
    payload["citation_verifier_records"][1]["finalization_status"] = "FINALIZED"

    result = rehearse_context_evidence_preservation(payload)

    assert result["status"] == "FAIL"
    assert "unsupported citation finalized: citation-verifier:cross-tenant:denied" in result[
        "blocked_promotion_reasons"
    ]


def test_context_evidence_preservation_blocks_tainted_instruction_reuse() -> None:
    payload = _fixture_payload()
    payload["taint_preservation_records"][0]["used_as_instruction"] = True

    result = rehearse_context_evidence_preservation(payload)

    assert result["status"] == "FAIL"
    assert "tainted content used as instruction: tool-output:mcp-reader:summary" in result[
        "blocked_promotion_reasons"
    ]


def test_context_evidence_preservation_blocks_operator_body_exposure() -> None:
    payload = _fixture_payload()
    payload["operator_inspect_records"][0]["body_exposed"] = True

    result = rehearse_context_evidence_preservation(payload)

    assert result["status"] == "FAIL"
    assert "operator inspect exposes body content: evidence-inspect:project-alpha:incident-1" in result[
        "blocked_promotion_reasons"
    ]
