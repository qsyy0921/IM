from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contract_version_compatibility import (
    load_contract_version_compatibility_rehearsal,
    rehearse_contract_version_compatibility,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "contract_version_compatibility_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_contract_version_compatibility_rehearsal_passes_fixture() -> None:
    result = load_contract_version_compatibility_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert len(result["contract_results"]) == 10
    assert len(result["coverage_results"]) == 10
    assert {item["status"] for item in result["contract_results"]} == {"PASS"}
    assert {item["status"] for item in result["coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_contract_version_compatibility_blocks_missing_contract_target() -> None:
    payload = _fixture_payload()
    payload["contract_records"] = [
        record
        for record in payload["contract_records"]
        if record["contract_name"] != "ReplayBundle"
    ]

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert "required contract-version target missing: ReplayBundle" in result[
        "blocked_promotion_reasons"
    ]


def test_contract_version_compatibility_blocks_missing_reader_policy() -> None:
    payload = _fixture_payload()
    payload["contract_records"][0]["missing_replay_reader_policy"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "contract lacks replay reader policy: contract-version:EvidencePack:v1"
        in result["blocked_promotion_reasons"]
    )


def test_contract_version_compatibility_blocks_body_archive_reader() -> None:
    payload = _fixture_payload()
    payload["contract_records"][9]["body_archive_required"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "contract reader requires archived body: contract-version:ReplayBundle:v1"
        in result["blocked_promotion_reasons"]
    )


def test_contract_version_compatibility_blocks_removed_required_ref() -> None:
    payload = _fixture_payload()
    payload["contract_records"][7]["required_ref_removed"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "contract removes required preservation ref: contract-version:ExecutionReceipt:v1"
        in result["blocked_promotion_reasons"]
    )


def test_contract_version_compatibility_blocks_python_final_owner() -> None:
    payload = _fixture_payload()
    payload["contract_records"][2]["python_final_owner"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert "Python owns final contract state: contract-version:MemoryCandidate:v1" in result[
        "blocked_promotion_reasons"
    ]


def test_contract_version_compatibility_blocks_release_for_missing_window() -> None:
    payload = deepcopy(_fixture_payload())
    payload["promotion_gate_records"][0]["missing_compatibility_window"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "missing compatibility window did not block promotion: "
        "contract-version-gate:complete-matrix"
    ) in result["blocked_promotion_reasons"]


def test_contract_version_compatibility_blocks_production_contract_authorization() -> None:
    payload = deepcopy(_fixture_payload())
    payload["promotion_gate_records"][0]["production_contract_authorized"] = True

    result = rehearse_contract_version_compatibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "fixture production contract authorization did not block promotion: "
        "contract-version-gate:complete-matrix"
    ) in result["blocked_promotion_reasons"]
