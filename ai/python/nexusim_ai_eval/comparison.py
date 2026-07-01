"""Low-sensitive EvalReport baseline comparison helpers."""

from __future__ import annotations

from typing import Any

from nexusim_ai_eval.contracts import SCHEMA_VERSION, assert_low_sensitive_eval_payload


def compare_eval_reports(
    baseline_payload: dict[str, Any],
    current_payload: dict[str, Any],
    *,
    score_tolerance: float = 0.0,
) -> dict[str, Any]:
    """Compare two low-sensitive EvalReport-like payloads for regressions."""

    _validate_report_payload(baseline_payload, "baseline eval report")
    _validate_report_payload(current_payload, "current eval report")
    if score_tolerance < 0:
        raise ValueError("score_tolerance must be non-negative")

    baseline_results = _results_by_case(baseline_payload)
    current_results = _results_by_case(current_payload)
    score_deltas = _score_deltas(
        _scores(baseline_payload.get("aggregate_scores", {}), "baseline aggregate_scores"),
        _scores(current_payload.get("aggregate_scores", {}), "current aggregate_scores"),
        score_tolerance=score_tolerance,
    )
    case_deltas = _case_deltas(
        baseline_results,
        current_results,
        score_tolerance=score_tolerance,
    )
    blocked_reasons = _blocked_reasons(
        baseline_payload,
        current_payload,
        score_deltas,
        case_deltas,
    )
    return {
        "schema_version": SCHEMA_VERSION,
        "comparison_kind": "agent_eval_regression_delta",
        "status": "PASS" if not blocked_reasons else "FAIL",
        "baseline_suite_id": _string(baseline_payload.get("suite_id")),
        "current_suite_id": _string(current_payload.get("suite_id")),
        "baseline_status": _string(baseline_payload.get("status")),
        "current_status": _string(current_payload.get("status")),
        "case_count_delta": _int(current_payload.get("case_count"), "current case_count")
        - _int(baseline_payload.get("case_count"), "baseline case_count"),
        "passed_count_delta": _int(current_payload.get("passed_count"), "current passed_count")
        - _int(baseline_payload.get("passed_count"), "baseline passed_count"),
        "failed_count_delta": _int(current_payload.get("failed_count"), "current failed_count")
        - _int(baseline_payload.get("failed_count"), "baseline failed_count"),
        "aggregate_score_deltas": score_deltas,
        "failure_distribution_delta": _failure_distribution_delta(
            baseline_payload.get("failure_distribution", {}),
            current_payload.get("failure_distribution", {}),
        ),
        "case_deltas": case_deltas,
        "blocked_promotion_reasons": blocked_reasons,
    }


def _validate_report_payload(payload: dict[str, Any], context: str) -> None:
    assert_low_sensitive_eval_payload(payload, context)
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError(f"{context} schema_version must be 1")
    _required_string(payload, "suite_id", context)
    _required_string(payload, "status", context)
    for field_name in ("case_count", "passed_count", "failed_count"):
        _int(payload.get(field_name), f"{context} {field_name}")
    _scores(payload.get("aggregate_scores"), f"{context} aggregate_scores")
    _failure_distribution(payload.get("failure_distribution"), f"{context} failure_distribution")
    _results_by_case(payload)


def _score_deltas(
    baseline_scores: dict[str, float],
    current_scores: dict[str, float],
    *,
    score_tolerance: float,
) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for score_name in sorted(set(baseline_scores) | set(current_scores)):
        baseline = baseline_scores.get(score_name)
        current = current_scores.get(score_name)
        if baseline is None:
            status = "NEW"
            delta: float | None = None
        elif current is None:
            status = "MISSING"
            delta = None
        else:
            delta = round(current - baseline, 4)
            status = "REGRESSION" if delta < -score_tolerance else "PASS"
        result[score_name] = {
            "baseline": baseline,
            "current": current,
            "delta": delta,
            "status": status,
        }
    return result


def _case_deltas(
    baseline_results: dict[str, dict[str, Any]],
    current_results: dict[str, dict[str, Any]],
    *,
    score_tolerance: float,
) -> list[dict[str, Any]]:
    deltas: list[dict[str, Any]] = []
    for case_id in sorted(set(baseline_results) | set(current_results)):
        baseline = baseline_results.get(case_id)
        current = current_results.get(case_id)
        if baseline is None:
            deltas.append(
                {
                    "case_id": case_id,
                    "status": "NEW",
                    "baseline_status": None,
                    "current_status": _string(current.get("status")) if current else None,
                    "baseline_failure_class": None,
                    "current_failure_class": _failure_class(current) if current else None,
                    "score_deltas": {},
                }
            )
            continue
        if current is None:
            deltas.append(
                {
                    "case_id": case_id,
                    "status": "MISSING",
                    "baseline_status": _string(baseline.get("status")),
                    "current_status": None,
                    "baseline_failure_class": _failure_class(baseline),
                    "current_failure_class": None,
                    "score_deltas": {},
                }
            )
            continue

        score_deltas = _score_deltas(
            _scores(baseline.get("scores", {}), f"baseline result {case_id} scores"),
            _scores(current.get("scores", {}), f"current result {case_id} scores"),
            score_tolerance=score_tolerance,
        )
        baseline_status = _string(baseline.get("status"))
        current_status = _string(current.get("status"))
        baseline_failure = _failure_class(baseline)
        current_failure = _failure_class(current)
        if any(item["status"] in {"MISSING", "REGRESSION"} for item in score_deltas.values()):
            status = "REGRESSION"
        elif baseline_status == "PASS" and current_status != "PASS":
            status = "REGRESSION"
        elif baseline_status != "PASS" and current_status == "PASS":
            status = "IMPROVEMENT"
        elif baseline_failure != current_failure:
            status = "CHANGED"
        else:
            status = "PASS"

        deltas.append(
            {
                "case_id": case_id,
                "status": status,
                "baseline_status": baseline_status,
                "current_status": current_status,
                "baseline_failure_class": baseline_failure,
                "current_failure_class": current_failure,
                "score_deltas": score_deltas,
            }
        )
    return deltas


def _blocked_reasons(
    baseline_payload: dict[str, Any],
    current_payload: dict[str, Any],
    score_deltas: dict[str, dict[str, Any]],
    case_deltas: list[dict[str, Any]],
) -> list[str]:
    reasons: list[str] = []
    if _string(baseline_payload.get("suite_id")) != _string(current_payload.get("suite_id")):
        reasons.append("suite_id changed")
    if _string(current_payload.get("status")) != "PASS":
        reasons.append("current report status is not PASS")
    if _int(current_payload.get("failed_count"), "current failed_count") > _int(
        baseline_payload.get("failed_count"), "baseline failed_count"
    ):
        reasons.append("failed_count increased")
    if any(item["status"] in {"MISSING", "REGRESSION"} for item in score_deltas.values()):
        reasons.append("aggregate score regressed")
    if any(item["status"] == "MISSING" for item in case_deltas):
        reasons.append("baseline case missing")
    if any(item["status"] == "REGRESSION" for item in case_deltas):
        reasons.append("case-level score or status regressed")
    return sorted(set(reasons))


def _failure_distribution_delta(
    baseline_raw: Any,
    current_raw: Any,
) -> dict[str, dict[str, int]]:
    baseline = _failure_distribution(baseline_raw, "baseline failure_distribution")
    current = _failure_distribution(current_raw, "current failure_distribution")
    result: dict[str, dict[str, int]] = {}
    for failure_class in sorted(set(baseline) | set(current)):
        baseline_count = baseline.get(failure_class, 0)
        current_count = current.get(failure_class, 0)
        result[failure_class] = {
            "baseline": baseline_count,
            "current": current_count,
            "delta": current_count - baseline_count,
        }
    return result


def _results_by_case(payload: dict[str, Any]) -> dict[str, dict[str, Any]]:
    raw_results = payload.get("results")
    if not isinstance(raw_results, list):
        raise ValueError("results must be a list")
    results: dict[str, dict[str, Any]] = {}
    for index, item in enumerate(raw_results):
        if not isinstance(item, dict):
            raise ValueError(f"results[{index}] must be an object")
        case_id = _required_string(item, "case_id", f"results[{index}]")
        _required_string(item, "status", f"results[{index}]")
        _scores(item.get("scores"), f"results[{index}].scores")
        if case_id in results:
            raise ValueError(f"duplicate result case_id: {case_id}")
        results[case_id] = item
    return results


def _scores(value: Any, context: str) -> dict[str, float]:
    if not isinstance(value, dict):
        raise ValueError(f"{context} must be an object")
    scores: dict[str, float] = {}
    for raw_key, raw_value in value.items():
        key = _string(raw_key)
        if not key:
            raise ValueError(f"{context} contains empty score name")
        if not isinstance(raw_value, int | float):
            raise ValueError(f"{context}.{key} must be numeric")
        scores[key] = float(raw_value)
    return scores


def _failure_distribution(value: Any, context: str) -> dict[str, int]:
    if not isinstance(value, dict):
        raise ValueError(f"{context} must be an object")
    distribution: dict[str, int] = {}
    for raw_key, raw_value in value.items():
        key = _string(raw_key)
        if not key:
            raise ValueError(f"{context} contains empty failure class")
        distribution[key] = _int(raw_value, f"{context}.{key}")
    return distribution


def _required_string(payload: dict[str, Any], field_name: str, context: str) -> str:
    value = _string(payload.get(field_name))
    if not value:
        raise ValueError(f"{context}.{field_name} is required")
    return value


def _failure_class(payload: dict[str, Any]) -> str:
    return _string(payload.get("failure_class")) or "PASS"


def _string(value: Any) -> str:
    return str(value or "").strip()


def _int(value: Any, context: str) -> int:
    if not isinstance(value, int):
        raise ValueError(f"{context} must be an integer")
    return value
