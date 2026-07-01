"""Current EvalReport generation and baseline refresh review helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.comparison import compare_eval_reports
from nexusim_ai_eval.contracts import (
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)
from nexusim_ai_eval.evaluator import eval_report_to_payload, run_eval_suite
from nexusim_ai_eval.fixtures import load_eval_suite


def generate_current_report_payload(fixture_path: Path) -> dict[str, Any]:
    """Run one synthetic fixture suite and return a low-sensitive EvalReport payload."""

    suite_payload = load_eval_suite(fixture_path)
    return eval_report_to_payload(run_eval_suite(suite_payload))


def build_baseline_refresh_review(
    *,
    current_report_payload: dict[str, Any],
    current_report_path: Path,
    baseline_report_payload: dict[str, Any] | None = None,
    baseline_report_path: Path | None = None,
    score_tolerance: float = 0.0,
) -> dict[str, Any]:
    """Build a low-sensitive review payload for manual baseline refresh decisions."""

    assert_low_sensitive_eval_payload(current_report_payload, "current EvalReport")
    comparison: dict[str, Any] | None = None
    blocked_reasons: list[str] = []
    baseline_hash: str | None = None
    if baseline_report_payload is not None:
        comparison = compare_eval_reports(
            baseline_report_payload,
            current_report_payload,
            score_tolerance=score_tolerance,
        )
        blocked_reasons = list(comparison["blocked_promotion_reasons"])
        baseline_hash = sha256_json(baseline_report_payload)

    current_status = _string(current_report_payload.get("status"))
    status = "PASS"
    if current_status != "PASS":
        status = "FAIL"
        blocked_reasons.append("current report status is not PASS")
    if comparison is not None and comparison["status"] != "PASS":
        status = "FAIL"

    review_payload = {
        "schema_version": SCHEMA_VERSION,
        "review_kind": "agent_eval_baseline_refresh_review",
        "status": status,
        "refresh_recommendation": _refresh_recommendation(
            has_baseline=baseline_report_payload is not None,
            status=status,
        ),
        "current_suite_id": _string(current_report_payload.get("suite_id")),
        "current_report_status": current_status,
        "current_case_count": _int(current_report_payload.get("case_count"), "current case_count"),
        "current_report_path": str(current_report_path),
        "current_report_hash": sha256_json(current_report_payload),
        "baseline_report_path": str(baseline_report_path) if baseline_report_path else "",
        "baseline_report_hash": baseline_hash or "",
        "score_tolerance": score_tolerance,
        "blocked_promotion_reasons": sorted(set(blocked_reasons)),
        "comparison": comparison,
    }
    assert_low_sensitive_eval_payload(review_payload, "baseline refresh review")
    return review_payload


def load_report_payload(path: Path, context: str) -> dict[str, Any]:
    """Load a low-sensitive EvalReport-like JSON object."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"failed to load {context}") from exc
    if not isinstance(payload, dict):
        raise ValueError(f"{context} must be an object")
    assert_low_sensitive_eval_payload(payload, context)
    return payload


def write_json_artifact(path: Path, payload: dict[str, Any], *, force: bool = False) -> None:
    """Write a low-sensitive JSON artifact, refusing accidental overwrite by default."""

    assert_low_sensitive_eval_payload(payload, f"artifact {path.name}")
    if path.exists() and not force:
        raise ValueError(f"refusing to overwrite existing artifact: {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(payload, ensure_ascii=True, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def ensure_not_same_path(left: Path, right: Path, left_label: str, right_label: str) -> None:
    """Reject artifact targets that would overwrite protected inputs."""

    if left.resolve() == right.resolve():
        raise ValueError(f"{left_label} must not overwrite {right_label}")


def _refresh_recommendation(*, has_baseline: bool, status: str) -> str:
    if status != "PASS":
        return "BASELINE_REFRESH_BLOCKED"
    if has_baseline:
        return "BASELINE_REFRESH_ALLOWED"
    return "CREATE_BASELINE_REVIEW_REQUIRED"


def _string(value: Any) -> str:
    return str(value or "").strip()


def _int(value: Any, context: str) -> int:
    if not isinstance(value, int):
        raise ValueError(f"{context} must be an integer")
    return value
