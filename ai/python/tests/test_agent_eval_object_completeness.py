from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.object_completeness import (
    load_object_completeness_rehearsal,
    rehearse_object_completeness,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "object_completeness_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_object_completeness_rehearsal_passes_fixture() -> None:
    result = load_object_completeness_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    assert {item["status"] for item in result["object_group_results"]} == {"PASS"}
    assert {item["status"] for item in result["object_coverage_results"]} == {"PASS"}
    assert {item["status"] for item in result["promotion_gate_results"]} == {"PASS"}


def test_object_completeness_blocks_missing_object_coverage() -> None:
    payload = _fixture_payload()
    payload["object_group_records"][-1]["object_names"].remove("ReplayView")

    result = rehearse_object_completeness(payload)

    assert result["status"] == "FAIL"
    assert "production object missing coverage: ReplayView" in result["blocked_promotion_reasons"]


def test_object_completeness_requires_version_policy() -> None:
    payload = _fixture_payload()
    payload["object_group_records"][0]["version_policy_ref"] = ""

    try:
        rehearse_object_completeness(payload)
    except ValueError as exc:
        assert "version_policy_ref is required" in str(exc)
    else:
        raise AssertionError("expected missing version_policy_ref to be rejected")


def test_object_completeness_blocks_python_durable_owner() -> None:
    payload = _fixture_payload()
    payload["object_group_records"][2]["owner_ref"] = "owner:python-ai-worker"
    payload["object_group_records"][2]["required_owner_fragments"] = ["python"]

    result = rehearse_object_completeness(payload)

    assert result["status"] == "FAIL"
    assert "python owns durable production object group: object-group:runtime" in result[
        "blocked_promotion_reasons"
    ]


def test_object_completeness_blocks_active_memory_wrong_owner() -> None:
    payload = _fixture_payload()
    payload["object_group_records"][4]["owner_ref"] = "owner:agent-runtime"
    payload["object_group_records"][4]["required_owner_fragments"] = ["agent-runtime"]

    result = rehearse_object_completeness(payload)

    assert result["status"] == "FAIL"
    assert "active memory object group is not memory-service owned: object-group:memory-service" in result[
        "blocked_promotion_reasons"
    ]


def test_object_completeness_blocks_production_contract_authorization() -> None:
    payload = _fixture_payload()
    payload["object_group_records"][1]["production_contract_authorized"] = True

    result = rehearse_object_completeness(payload)

    assert result["status"] == "FAIL"
    assert "object group authorizes production contract: object-group:contract-versioning" in result[
        "blocked_promotion_reasons"
    ]


def test_object_completeness_blocks_release_for_failed_group() -> None:
    payload = deepcopy(_fixture_payload())
    payload["object_group_records"][6]["owner_ref"] = "owner:python-ai-worker"
    payload["object_group_records"][6]["required_owner_fragments"] = ["python"]

    result = rehearse_object_completeness(payload)

    assert result["status"] == "FAIL"
    assert "failed object completeness evidence not blocked: object-gate:complete-catalog" in result[
        "blocked_promotion_reasons"
    ]
