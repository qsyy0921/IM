from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any

from nexusim_ai_eval.replay_compatibility import (
    load_replay_version_bump_rehearsal,
    rehearse_replay_version_bump,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "replay_version_bump_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_replay_version_bump_rehearsal_passes_fixture() -> None:
    result = load_replay_version_bump_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["current_reader_policy_ref"] == "replay-reader-policy:low-sensitive:v1"
    assert result["old_replay_bundle_hash"]
    assert {item["status"] for item in result["required_ref_results"]} == {"PASS"}
    assert {item["status"] for item in result["deprecated_field_results"]} == {"PASS"}


def test_replay_version_bump_blocks_missing_required_ref() -> None:
    payload = _fixture_payload()
    payload["required_refs"]["version_refs"].append("version:missing-reader:v9")

    result = rehearse_replay_version_bump(payload)

    assert result["status"] == "FAIL"
    assert (
        "missing required refs in version_refs: version:missing-reader:v9"
        in result["blocked_promotion_reasons"]
    )


def test_replay_version_bump_blocks_raw_payload_returned() -> None:
    payload = _fixture_payload()
    payload["old_replay_bundle"]["raw_payload_returned"] = True

    result = rehearse_replay_version_bump(payload)

    assert result["status"] == "FAIL"
    assert "old replay bundle returns raw payload" in result["blocked_promotion_reasons"]


def test_replay_version_bump_requires_deprecated_field_fail_closed() -> None:
    payload = deepcopy(_fixture_payload())
    payload["deprecated_field_records"][0]["reader_behavior"] = "READ_LEGACY"

    result = rehearse_replay_version_bump(payload)

    assert result["status"] == "FAIL"
    assert (
        "deprecated field not fail-closed or expired: legacy-field:provider-body-archive"
        in result["blocked_promotion_reasons"]
    )
