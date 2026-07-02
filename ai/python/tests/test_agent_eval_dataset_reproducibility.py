from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.dataset_reproducibility import (
    load_dataset_reproducibility_rehearsal,
    rehearse_dataset_reproducibility,
)


ROOT = Path(__file__).resolve().parents[3]
REHEARSAL_FIXTURE = (
    ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "dataset_reproducibility_rehearsal.json"
)


def _fixture_payload() -> dict[str, Any]:
    return json.loads(REHEARSAL_FIXTURE.read_text(encoding="utf-8"))


def test_dataset_reproducibility_rehearsal_passes_fixture() -> None:
    result = load_dataset_reproducibility_rehearsal(REHEARSAL_FIXTURE)

    assert result["status"] == "PASS"
    assert result["blocked_promotion_reasons"] == []
    assert result["rehearsal_hash"]
    for result_key in (
        "dataset_manifest_results",
        "run_reproducibility_results",
        "calibration_export_results",
        "promotion_gate_results",
    ):
        assert {item["status"] for item in result[result_key]} == {"PASS"}


def test_dataset_reproducibility_blocks_production_data_manifest() -> None:
    payload = _fixture_payload()
    payload["dataset_manifest_records"][0]["production_data_used"] = True

    result = rehearse_dataset_reproducibility(payload)

    assert result["status"] == "FAIL"
    assert "dataset manifest uses production data: dataset-manifest:statebench-public:v1" in result[
        "blocked_promotion_reasons"
    ]


def test_dataset_reproducibility_blocks_missing_split_manifest() -> None:
    payload = _fixture_payload()
    payload["dataset_manifest_records"][1]["split_manifest_ref"] = ""

    try:
        rehearse_dataset_reproducibility(payload)
    except ValueError as exc:
        assert "split_manifest_ref is required" in str(exc)
    else:
        raise AssertionError("expected missing split_manifest_ref to be rejected")


def test_dataset_reproducibility_blocks_backend_connected_run() -> None:
    payload = _fixture_payload()
    payload["run_reproducibility_records"][0]["backend_connected"] = True

    result = rehearse_dataset_reproducibility(payload)

    assert result["status"] == "FAIL"
    assert "dataset run connected to backend: dataset-run:memory-calibration:v1" in result[
        "blocked_promotion_reasons"
    ]


def test_dataset_reproducibility_blocks_non_deterministic_report() -> None:
    payload = _fixture_payload()
    payload["run_reproducibility_records"][0]["repeated_report_hash_ref"] = "hash:report:changed"

    result = rehearse_dataset_reproducibility(payload)

    assert result["status"] == "FAIL"
    assert "dataset report is not deterministic: dataset-run:memory-calibration:v1" in result[
        "blocked_promotion_reasons"
    ]


def test_dataset_reproducibility_blocks_calibration_count_mismatch() -> None:
    payload = _fixture_payload()
    payload["calibration_export_records"][0]["dataset_case_counts"].pop()

    result = rehearse_dataset_reproducibility(payload)

    assert result["status"] == "FAIL"
    assert (
        "calibration export dataset counts do not match sources: "
        "calibration-export:memory-public:v1"
    ) in result["blocked_promotion_reasons"]


def test_dataset_reproducibility_blocks_changed_snapshot_release() -> None:
    payload = _fixture_payload()
    payload["promotion_gate_records"][1]["expected_gate_decision"] = "ALLOW"
    payload["promotion_gate_records"][1]["actual_gate_decision"] = "ALLOW"
    payload["promotion_gate_records"][1]["release_allowed"] = True

    result = rehearse_dataset_reproducibility(payload)

    assert result["status"] == "FAIL"
    assert "changed dataset snapshot did not block promotion: dataset-gate:snapshot-changed" in result[
        "blocked_promotion_reasons"
    ]
