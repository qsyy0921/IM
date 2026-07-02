from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.memory_admission_governance import (
    load_memory_admission_governance_rehearsal,
    rehearse_memory_admission_governance,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "memory_admission_governance_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_memory_admission_governance_rehearsal_passes_fixture() -> None:
    result = load_memory_admission_governance_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "ownership_results",
        "category_threshold_results",
        "revocation_results",
        "retrieval_eligibility_results",
        "admission_explanation_results",
        "operator_memory_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_memory_admission_governance_blocks_python_active_decision() -> None:
    payload = _fixture_payload()
    payload["ownership_assertions"][0]["python_made_active_decision"] = True

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "python made active memory decision: memory-candidate:worker-output:personal-style"
        in result["blocked_promotion_reasons"]
    )


def test_memory_admission_governance_blocks_missing_group_threshold_ref() -> None:
    payload = _fixture_payload()
    payload["category_threshold_records"][1]["present_refs"].remove("audience_refs")

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert (
        "missing category threshold refs for memory-candidate:group:ops-norm: audience_refs"
        in result["blocked_promotion_reasons"]
    )


def test_memory_admission_governance_blocks_revoked_retrieval() -> None:
    payload = deepcopy(_fixture_payload())
    payload["revocation_records"][0]["retrieved_after_revocation"] = True

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert "revoked memory retrieved: memory-revocation:project-decision-legacy" in result[
        "blocked_promotion_reasons"
    ]


def test_memory_admission_governance_blocks_non_active_eligible_memory() -> None:
    payload = _fixture_payload()
    payload["retrieval_eligibility_records"][2]["retrieval_eligible"] = True
    payload["retrieval_eligibility_records"][2]["expected_retrieval_eligible"] = True

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert "non-active memory is retrieval eligible: memory:procedural:scheduling-v2" in result[
        "blocked_promotion_reasons"
    ]


def test_memory_admission_governance_blocks_raw_explanation_dependency() -> None:
    payload = _fixture_payload()
    payload["admission_explanation_records"][0]["raw_text_required"] = True

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert "admission explanation requires raw text: memory-decision:personal-style:active" in result[
        "blocked_promotion_reasons"
    ]


def test_memory_admission_governance_blocks_operator_python_override() -> None:
    payload = _fixture_payload()
    payload["operator_memory_records"][1]["python_override"] = True

    result = rehearse_memory_admission_governance(payload)

    assert result["status"] == "FAIL"
    assert "python overrides memory operator action: memory-operator:correct:project-decision-7" in result[
        "blocked_promotion_reasons"
    ]
