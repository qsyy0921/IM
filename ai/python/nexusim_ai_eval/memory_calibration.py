"""Offline memory admission calibration for public-dataset-style samples.

The calibration runner evaluates threshold and review-policy candidates over
local low-sensitive memory cases. It does not admit durable memory, call
providers, or connect to NexusIM backend services.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Literal, cast

from nexusim_ai_eval.contracts import (
    HARNESS_VERSION,
    SCHEMA_VERSION,
    assert_low_sensitive_eval_payload,
    sha256_json,
)
from nexusim_ai_eval.reporting import build_retention_metadata


MemoryGate = Literal["AUTO_ADMIT", "NEEDS_REVIEW", "REJECT"]
WindowState = Literal["OPEN", "CLOSED"]
ReviewAction = Literal["WAIT", "RETRY", "ESCALATE", "REDRIVE"]

_MEMORY_GATES = {"AUTO_ADMIT", "NEEDS_REVIEW", "REJECT"}
_WINDOW_STATES = {"OPEN", "CLOSED"}
_REVIEW_ACTIONS = {"WAIT", "RETRY", "ESCALATE", "REDRIVE"}

DEFAULT_MEMORY_CALIBRATION_RETENTION_POLICY_REF = (
    "retention:agent-memory-calibration-report:research-v1"
)


@dataclass(frozen=True)
class AcceptancePolicy:
    min_confidence_exact_match_rate: float
    min_auto_admit_precision: float
    min_reject_recall: float
    min_pollution_block_rate: float
    min_policy_window_match_rate: float
    min_policy_window_retention_match_rate: float
    min_review_backoff_match_rate: float
    min_review_queue_match_rate: float


@dataclass(frozen=True)
class ConfidenceThresholdCandidate:
    threshold_ref: str
    minimum_confidence: float
    auto_admit_scopes: list[str]
    review_scopes: list[str]
    reject_scopes: list[str]


@dataclass(frozen=True)
class MemoryGateCase:
    case_id: str
    dataset_name: str
    dataset_version: str
    memory_scope: str
    candidate_confidence: float
    expected_gate: MemoryGate
    source_backed: bool
    source_visible: bool
    policy_source_revoked: bool
    conflict_detected: bool
    candidate_high_risk: bool


@dataclass(frozen=True)
class PolicyRevocationWindowCandidate:
    window_ref: str
    close_after_hours: int
    retention_policy_ref: str


@dataclass(frozen=True)
class PolicyWindowCase:
    case_id: str
    dataset_name: str
    dataset_version: str
    policy_source_ref: str
    revocation_age_hours: int
    expected_window_state: WindowState
    expected_retention_policy_ref: str


@dataclass(frozen=True)
class ReviewBackoffCandidate:
    backoff_policy_ref: str
    initial_delay_minutes: int
    multiplier: float
    max_attempts: int
    escalation_after_minutes: int
    operator_queue_ref: str


@dataclass(frozen=True)
class ReviewBackoffCase:
    case_id: str
    dataset_name: str
    dataset_version: str
    review_age_minutes: int
    review_attempt_count: int
    review_redrive_required: bool
    expected_review_action: ReviewAction
    expected_operator_queue_ref: str


def load_memory_calibration_payload(path: Path) -> dict[str, Any]:
    """Load a low-sensitive memory calibration payload."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load memory calibration payload") from exc
    if not isinstance(payload, dict):
        raise ValueError("memory calibration payload must be an object")
    assert_low_sensitive_eval_payload(payload, "memory calibration payload")
    return payload


def run_memory_admission_calibration(payload: dict[str, Any]) -> dict[str, Any]:
    """Evaluate memory admission calibration candidates and return a report."""

    assert_low_sensitive_eval_payload(payload, "memory calibration payload")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")

    calibration_id = _required_string(payload, "calibration_id", "memory calibration payload")
    acceptance_policy = _acceptance_policy(payload.get("acceptance_policy", {}))
    retention_policy_ref = (
        _string(payload.get("retention_policy_ref"))
        or DEFAULT_MEMORY_CALIBRATION_RETENTION_POLICY_REF
    )
    dataset_source_refs = _string_list(
        payload.get("dataset_source_refs", []),
        "dataset_source_refs",
    )
    gate_cases = _memory_gate_cases(payload.get("memory_gate_cases"))
    threshold_candidates = _threshold_candidates(
        payload.get("confidence_threshold_candidates")
    )
    window_cases = _policy_window_cases(payload.get("policy_revocation_window_cases"))
    window_candidates = _policy_window_candidates(
        payload.get("policy_revocation_window_candidates")
    )
    backoff_cases = _review_backoff_cases(payload.get("review_backoff_cases"))
    backoff_candidates = _review_backoff_candidates(
        payload.get("review_backoff_candidates")
    )

    threshold_results = [
        _evaluate_threshold_candidate(candidate, gate_cases, acceptance_policy)
        for candidate in threshold_candidates
    ]
    window_results = [
        _evaluate_window_candidate(candidate, window_cases, acceptance_policy)
        for candidate in window_candidates
    ]
    backoff_results = [
        _evaluate_backoff_candidate(candidate, backoff_cases, acceptance_policy)
        for candidate in backoff_candidates
    ]

    recommended_threshold = _recommend_threshold(threshold_results)
    recommended_window = _recommend_policy_window(window_results)
    recommended_backoff = _recommend_review_backoff(backoff_results)
    blocked_reasons = _blocked_reasons(
        recommended_threshold=recommended_threshold,
        recommended_window=recommended_window,
        recommended_backoff=recommended_backoff,
    )

    report = {
        "schema_version": SCHEMA_VERSION,
        "calibration_kind": "agent_memory_admission_calibration",
        "calibration_id": calibration_id,
        "harness_version": HARNESS_VERSION,
        "status": "PASS" if not blocked_reasons else "FAIL",
        "dataset_source_refs": dataset_source_refs,
        "dataset_refs": _dataset_refs(gate_cases, window_cases, backoff_cases),
        "dataset_case_counts": _dataset_case_counts(
            gate_cases,
            window_cases,
            backoff_cases,
        ),
        "expected_memory_gate_distribution": _expected_memory_gate_distribution(
            gate_cases
        ),
        "memory_gate_case_count": len(gate_cases),
        "policy_window_case_count": len(window_cases),
        "review_backoff_case_count": len(backoff_cases),
        "recommended_confidence_threshold_ref": _recommended_ref(
            recommended_threshold,
            "threshold_ref",
        ),
        "recommended_policy_revocation_window_ref": _recommended_ref(
            recommended_window,
            "window_ref",
        ),
        "recommended_review_backoff_policy_ref": _recommended_ref(
            recommended_backoff,
            "backoff_policy_ref",
        ),
        "confidence_threshold_results": threshold_results,
        "policy_revocation_window_results": window_results,
        "review_backoff_results": backoff_results,
        "blocked_promotion_reasons": blocked_reasons,
        "input_payload_hash": sha256_json(payload),
        "retention_metadata": build_retention_metadata(
            artifact_kind="agent_memory_admission_calibration_report",
            retention_policy_ref=retention_policy_ref,
        ),
    }
    assert_low_sensitive_eval_payload(report, "memory calibration report")
    return report


def _evaluate_threshold_candidate(
    candidate: ConfidenceThresholdCandidate,
    cases: list[MemoryGateCase],
    acceptance_policy: AcceptancePolicy,
) -> dict[str, Any]:
    predictions: list[dict[str, Any]] = []
    exact_matches = 0
    auto_admit_predictions = 0
    correct_auto_admits = 0
    expected_rejects = 0
    rejected_expected_rejects = 0
    blocked_expected_rejects = 0

    for case in cases:
        predicted_gate, reason_refs = _predict_memory_gate(candidate, case)
        if predicted_gate == case.expected_gate:
            exact_matches += 1
        if predicted_gate == "AUTO_ADMIT":
            auto_admit_predictions += 1
            if case.expected_gate == "AUTO_ADMIT":
                correct_auto_admits += 1
        if case.expected_gate == "REJECT":
            expected_rejects += 1
            if predicted_gate == "REJECT":
                rejected_expected_rejects += 1
            if predicted_gate != "AUTO_ADMIT":
                blocked_expected_rejects += 1
        predictions.append(
            {
                "case_id": case.case_id,
                "expected_gate": case.expected_gate,
                "predicted_gate": predicted_gate,
                "match": predicted_gate == case.expected_gate,
                "prediction_reason_refs": reason_refs,
            }
        )

    exact_match_rate = _ratio(exact_matches, len(cases))
    auto_admit_precision = _ratio(correct_auto_admits, auto_admit_predictions, default=1.0)
    reject_recall = _ratio(rejected_expected_rejects, expected_rejects, default=1.0)
    pollution_block_rate = _ratio(blocked_expected_rejects, expected_rejects, default=1.0)
    result = {
        "threshold_ref": candidate.threshold_ref,
        "minimum_confidence": candidate.minimum_confidence,
        "exact_match_rate": exact_match_rate,
        "auto_admit_precision": auto_admit_precision,
        "reject_recall": reject_recall,
        "pollution_block_rate": pollution_block_rate,
        "case_count": len(cases),
        "mismatches": [
            prediction for prediction in predictions if not bool(prediction["match"])
        ],
        "meets_acceptance": (
            exact_match_rate >= acceptance_policy.min_confidence_exact_match_rate
            and auto_admit_precision >= acceptance_policy.min_auto_admit_precision
            and reject_recall >= acceptance_policy.min_reject_recall
            and pollution_block_rate >= acceptance_policy.min_pollution_block_rate
        ),
    }
    assert_low_sensitive_eval_payload(result, "confidence threshold result")
    return result


def _predict_memory_gate(
    candidate: ConfidenceThresholdCandidate,
    case: MemoryGateCase,
) -> tuple[MemoryGate, list[str]]:
    if not case.source_backed:
        return "REJECT", ["reason:memory-source-missing"]
    if not case.source_visible:
        return "REJECT", ["reason:memory-source-not-visible"]
    if case.policy_source_revoked:
        return "REJECT", ["reason:policy-source-revoked"]
    if case.memory_scope in candidate.reject_scopes:
        return "REJECT", [f"reason:scope-rejected:{case.memory_scope.lower()}"]
    if case.candidate_confidence < candidate.minimum_confidence:
        return "REJECT", [f"reason:confidence-below:{candidate.threshold_ref}"]
    if case.conflict_detected:
        return "NEEDS_REVIEW", ["reason:conflict-needs-review"]
    if case.candidate_high_risk:
        return "NEEDS_REVIEW", ["reason:high-risk-needs-review"]
    if case.memory_scope in candidate.review_scopes:
        return "NEEDS_REVIEW", [f"reason:scope-review:{case.memory_scope.lower()}"]
    if case.memory_scope in candidate.auto_admit_scopes:
        return "AUTO_ADMIT", [f"reason:scope-auto:{case.memory_scope.lower()}"]
    return "NEEDS_REVIEW", ["reason:scope-default-review"]


def _evaluate_window_candidate(
    candidate: PolicyRevocationWindowCandidate,
    cases: list[PolicyWindowCase],
    acceptance_policy: AcceptancePolicy,
) -> dict[str, Any]:
    matches = 0
    retention_matches = 0
    predictions: list[dict[str, Any]] = []
    for case in cases:
        predicted_state = _predict_policy_window(candidate, case)
        state_match = predicted_state == case.expected_window_state
        retention_match = candidate.retention_policy_ref == case.expected_retention_policy_ref
        if state_match:
            matches += 1
        if retention_match:
            retention_matches += 1
        predictions.append(
            {
                "case_id": case.case_id,
                "policy_source_ref": case.policy_source_ref,
                "expected_window_state": case.expected_window_state,
                "predicted_window_state": predicted_state,
                "expected_retention_policy_ref": case.expected_retention_policy_ref,
                "candidate_retention_policy_ref": candidate.retention_policy_ref,
                "match": state_match and retention_match,
            }
        )

    match_rate = _ratio(matches, len(cases))
    retention_match_rate = _ratio(retention_matches, len(cases))
    result = {
        "window_ref": candidate.window_ref,
        "close_after_hours": candidate.close_after_hours,
        "retention_policy_ref": candidate.retention_policy_ref,
        "match_rate": match_rate,
        "retention_policy_match_rate": retention_match_rate,
        "case_count": len(cases),
        "mismatches": [
            prediction for prediction in predictions if not bool(prediction["match"])
        ],
        "meets_acceptance": (
            match_rate >= acceptance_policy.min_policy_window_match_rate
            and retention_match_rate
            >= acceptance_policy.min_policy_window_retention_match_rate
        ),
    }
    assert_low_sensitive_eval_payload(result, "policy revocation window result")
    return result


def _predict_policy_window(
    candidate: PolicyRevocationWindowCandidate,
    case: PolicyWindowCase,
) -> WindowState:
    if case.revocation_age_hours >= candidate.close_after_hours:
        return "CLOSED"
    return "OPEN"


def _evaluate_backoff_candidate(
    candidate: ReviewBackoffCandidate,
    cases: list[ReviewBackoffCase],
    acceptance_policy: AcceptancePolicy,
) -> dict[str, Any]:
    action_matches = 0
    queue_matches = 0
    predictions: list[dict[str, Any]] = []
    for case in cases:
        predicted_action = _predict_review_action(candidate, case)
        action_match = predicted_action == case.expected_review_action
        queue_match = candidate.operator_queue_ref == case.expected_operator_queue_ref
        if action_match:
            action_matches += 1
        if queue_match:
            queue_matches += 1
        predictions.append(
            {
                "case_id": case.case_id,
                "expected_review_action": case.expected_review_action,
                "predicted_review_action": predicted_action,
                "expected_operator_queue_ref": case.expected_operator_queue_ref,
                "candidate_operator_queue_ref": candidate.operator_queue_ref,
                "match": action_match and queue_match,
            }
        )

    action_match_rate = _ratio(action_matches, len(cases))
    queue_match_rate = _ratio(queue_matches, len(cases))
    result = {
        "backoff_policy_ref": candidate.backoff_policy_ref,
        "initial_delay_minutes": candidate.initial_delay_minutes,
        "multiplier": candidate.multiplier,
        "max_attempts": candidate.max_attempts,
        "escalation_after_minutes": candidate.escalation_after_minutes,
        "operator_queue_ref": candidate.operator_queue_ref,
        "action_match_rate": action_match_rate,
        "operator_queue_match_rate": queue_match_rate,
        "case_count": len(cases),
        "mismatches": [
            prediction for prediction in predictions if not bool(prediction["match"])
        ],
        "meets_acceptance": (
            action_match_rate >= acceptance_policy.min_review_backoff_match_rate
            and queue_match_rate >= acceptance_policy.min_review_queue_match_rate
        ),
    }
    assert_low_sensitive_eval_payload(result, "review backoff result")
    return result


def _predict_review_action(
    candidate: ReviewBackoffCandidate,
    case: ReviewBackoffCase,
) -> ReviewAction:
    if case.review_redrive_required and case.review_attempt_count >= candidate.max_attempts:
        return "REDRIVE"
    if case.review_age_minutes >= candidate.escalation_after_minutes:
        return "ESCALATE"
    if case.review_attempt_count >= candidate.max_attempts:
        return "ESCALATE"
    next_retry_after = candidate.initial_delay_minutes * (
        candidate.multiplier ** case.review_attempt_count
    )
    if case.review_age_minutes >= next_retry_after:
        return "RETRY"
    return "WAIT"


def _recommend_threshold(results: list[dict[str, Any]]) -> dict[str, Any] | None:
    eligible = [result for result in results if bool(result["meets_acceptance"])]
    if not eligible:
        return None
    return sorted(
        eligible,
        key=lambda result: (
            float(result["exact_match_rate"]),
            float(result["pollution_block_rate"]),
            float(result["auto_admit_precision"]),
            float(result["reject_recall"]),
            float(result["minimum_confidence"]),
            str(result["threshold_ref"]),
        ),
        reverse=True,
    )[0]


def _recommend_policy_window(results: list[dict[str, Any]]) -> dict[str, Any] | None:
    eligible = [result for result in results if bool(result["meets_acceptance"])]
    if not eligible:
        return None
    return sorted(
        eligible,
        key=lambda result: (
            float(result["match_rate"]),
            float(result["retention_policy_match_rate"]),
            -int(result["close_after_hours"]),
            str(result["window_ref"]),
        ),
        reverse=True,
    )[0]


def _recommend_review_backoff(results: list[dict[str, Any]]) -> dict[str, Any] | None:
    eligible = [result for result in results if bool(result["meets_acceptance"])]
    if not eligible:
        return None
    return sorted(
        eligible,
        key=lambda result: (
            float(result["action_match_rate"]),
            float(result["operator_queue_match_rate"]),
            -int(result["escalation_after_minutes"]),
            -int(result["initial_delay_minutes"]),
            str(result["backoff_policy_ref"]),
        ),
        reverse=True,
    )[0]


def _blocked_reasons(
    *,
    recommended_threshold: dict[str, Any] | None,
    recommended_window: dict[str, Any] | None,
    recommended_backoff: dict[str, Any] | None,
) -> list[str]:
    reasons: list[str] = []
    if recommended_threshold is None:
        reasons.append("memory calibration blocked: no confidence threshold meets acceptance")
    if recommended_window is None:
        reasons.append("memory calibration blocked: no policy revocation window meets acceptance")
    if recommended_backoff is None:
        reasons.append("memory calibration blocked: no review backoff policy meets acceptance")
    return reasons


def _recommended_ref(result: dict[str, Any] | None, key: str) -> str:
    if result is None:
        return ""
    return str(result[key])


def _dataset_refs(
    gate_cases: list[MemoryGateCase],
    window_cases: list[PolicyWindowCase],
    backoff_cases: list[ReviewBackoffCase],
) -> list[str]:
    refs: set[str] = set()
    for case in gate_cases + window_cases + backoff_cases:
        refs.add(f"dataset:{case.dataset_name}:{case.dataset_version}")
    return sorted(refs)


def _dataset_case_counts(
    gate_cases: list[MemoryGateCase],
    window_cases: list[PolicyWindowCase],
    backoff_cases: list[ReviewBackoffCase],
) -> list[dict[str, Any]]:
    counts: dict[str, dict[str, Any]] = {}
    for gate_case in gate_cases:
        entry = _dataset_count_entry(
            counts,
            gate_case.dataset_name,
            gate_case.dataset_version,
        )
        entry["memory_gate_case_count"] = int(entry["memory_gate_case_count"]) + 1
    for window_case in window_cases:
        entry = _dataset_count_entry(
            counts,
            window_case.dataset_name,
            window_case.dataset_version,
        )
        entry["policy_window_case_count"] = int(entry["policy_window_case_count"]) + 1
    for backoff_case in backoff_cases:
        entry = _dataset_count_entry(
            counts,
            backoff_case.dataset_name,
            backoff_case.dataset_version,
        )
        entry["review_backoff_case_count"] = int(entry["review_backoff_case_count"]) + 1
    for entry in counts.values():
        entry["case_count"] = (
            int(entry["memory_gate_case_count"])
            + int(entry["policy_window_case_count"])
            + int(entry["review_backoff_case_count"])
        )
    return sorted(
        counts.values(),
        key=lambda entry: (str(entry["dataset_name"]), str(entry["dataset_version"])),
    )


def _dataset_count_entry(
    counts: dict[str, dict[str, Any]],
    dataset_name: str,
    dataset_version: str,
) -> dict[str, Any]:
    key = f"{dataset_name}:{dataset_version}"
    if key not in counts:
        counts[key] = {
            "dataset_name": dataset_name,
            "dataset_version": dataset_version,
            "memory_gate_case_count": 0,
            "policy_window_case_count": 0,
            "review_backoff_case_count": 0,
            "case_count": 0,
        }
    return counts[key]


def _expected_memory_gate_distribution(cases: list[MemoryGateCase]) -> dict[str, int]:
    counts = {"AUTO_ADMIT": 0, "NEEDS_REVIEW": 0, "REJECT": 0}
    for case in cases:
        counts[case.expected_gate] += 1
    return counts


def _acceptance_policy(value: Any) -> AcceptancePolicy:
    if value is None:
        value = {}
    if not isinstance(value, dict):
        raise ValueError("acceptance_policy must be an object")
    return AcceptancePolicy(
        min_confidence_exact_match_rate=_float_default(
            value,
            "min_confidence_exact_match_rate",
            0.9,
        ),
        min_auto_admit_precision=_float_default(value, "min_auto_admit_precision", 1.0),
        min_reject_recall=_float_default(value, "min_reject_recall", 1.0),
        min_pollution_block_rate=_float_default(value, "min_pollution_block_rate", 1.0),
        min_policy_window_match_rate=_float_default(
            value,
            "min_policy_window_match_rate",
            1.0,
        ),
        min_policy_window_retention_match_rate=_float_default(
            value,
            "min_policy_window_retention_match_rate",
            1.0,
        ),
        min_review_backoff_match_rate=_float_default(
            value,
            "min_review_backoff_match_rate",
            1.0,
        ),
        min_review_queue_match_rate=_float_default(value, "min_review_queue_match_rate", 1.0),
    )


def _threshold_candidates(value: Any) -> list[ConfidenceThresholdCandidate]:
    result: list[ConfidenceThresholdCandidate] = []
    for index, item in enumerate(_object_list(value, "confidence_threshold_candidates")):
        result.append(
            ConfidenceThresholdCandidate(
                threshold_ref=_required_string(
                    item,
                    "threshold_ref",
                    f"confidence_threshold_candidates[{index}]",
                ),
                minimum_confidence=_required_float(
                    item,
                    "minimum_confidence",
                    f"confidence_threshold_candidates[{index}]",
                ),
                auto_admit_scopes=_upper_string_list(
                    item.get("auto_admit_scopes", []),
                    f"confidence_threshold_candidates[{index}].auto_admit_scopes",
                ),
                review_scopes=_upper_string_list(
                    item.get("review_scopes", []),
                    f"confidence_threshold_candidates[{index}].review_scopes",
                ),
                reject_scopes=_upper_string_list(
                    item.get("reject_scopes", []),
                    f"confidence_threshold_candidates[{index}].reject_scopes",
                ),
            )
        )
    return result


def _memory_gate_cases(value: Any) -> list[MemoryGateCase]:
    result: list[MemoryGateCase] = []
    for index, item in enumerate(_object_list(value, "memory_gate_cases")):
        context = f"memory_gate_cases[{index}]"
        result.append(
            MemoryGateCase(
                case_id=_required_string(item, "case_id", context),
                dataset_name=_required_string(item, "dataset_name", context),
                dataset_version=_required_string(item, "dataset_version", context),
                memory_scope=_required_string(item, "memory_scope", context).upper(),
                candidate_confidence=_required_float(item, "candidate_confidence", context),
                expected_gate=_memory_gate(item.get("expected_gate"), f"{context}.expected_gate"),
                source_backed=_bool_default(item, "source_backed", True),
                source_visible=_bool_default(item, "source_visible", True),
                policy_source_revoked=_bool_default(item, "policy_source_revoked", False),
                conflict_detected=_bool_default(item, "conflict_detected", False),
                candidate_high_risk=_bool_default(item, "candidate_high_risk", False),
            )
        )
    return result


def _policy_window_candidates(value: Any) -> list[PolicyRevocationWindowCandidate]:
    result: list[PolicyRevocationWindowCandidate] = []
    for index, item in enumerate(_object_list(value, "policy_revocation_window_candidates")):
        context = f"policy_revocation_window_candidates[{index}]"
        result.append(
            PolicyRevocationWindowCandidate(
                window_ref=_required_string(item, "window_ref", context),
                close_after_hours=_required_int(item, "close_after_hours", context),
                retention_policy_ref=_required_string(item, "retention_policy_ref", context),
            )
        )
    return result


def _policy_window_cases(value: Any) -> list[PolicyWindowCase]:
    result: list[PolicyWindowCase] = []
    for index, item in enumerate(_object_list(value, "policy_revocation_window_cases")):
        context = f"policy_revocation_window_cases[{index}]"
        result.append(
            PolicyWindowCase(
                case_id=_required_string(item, "case_id", context),
                dataset_name=_required_string(item, "dataset_name", context),
                dataset_version=_required_string(item, "dataset_version", context),
                policy_source_ref=_required_string(item, "policy_source_ref", context),
                revocation_age_hours=_required_int(item, "revocation_age_hours", context),
                expected_window_state=_window_state(
                    item.get("expected_window_state"),
                    f"{context}.expected_window_state",
                ),
                expected_retention_policy_ref=_required_string(
                    item,
                    "expected_retention_policy_ref",
                    context,
                ),
            )
        )
    return result


def _review_backoff_candidates(value: Any) -> list[ReviewBackoffCandidate]:
    result: list[ReviewBackoffCandidate] = []
    for index, item in enumerate(_object_list(value, "review_backoff_candidates")):
        context = f"review_backoff_candidates[{index}]"
        result.append(
            ReviewBackoffCandidate(
                backoff_policy_ref=_required_string(item, "backoff_policy_ref", context),
                initial_delay_minutes=_required_int(
                    item,
                    "initial_delay_minutes",
                    context,
                ),
                multiplier=_required_float(item, "multiplier", context),
                max_attempts=_required_int(item, "max_attempts", context),
                escalation_after_minutes=_required_int(
                    item,
                    "escalation_after_minutes",
                    context,
                ),
                operator_queue_ref=_required_string(item, "operator_queue_ref", context),
            )
        )
    return result


def _review_backoff_cases(value: Any) -> list[ReviewBackoffCase]:
    result: list[ReviewBackoffCase] = []
    for index, item in enumerate(_object_list(value, "review_backoff_cases")):
        context = f"review_backoff_cases[{index}]"
        result.append(
            ReviewBackoffCase(
                case_id=_required_string(item, "case_id", context),
                dataset_name=_required_string(item, "dataset_name", context),
                dataset_version=_required_string(item, "dataset_version", context),
                review_age_minutes=_required_int(item, "review_age_minutes", context),
                review_attempt_count=_required_int(item, "review_attempt_count", context),
                review_redrive_required=_bool_default(
                    item,
                    "review_redrive_required",
                    False,
                ),
                expected_review_action=_review_action(
                    item.get("expected_review_action"),
                    f"{context}.expected_review_action",
                ),
                expected_operator_queue_ref=_required_string(
                    item,
                    "expected_operator_queue_ref",
                    context,
                ),
            )
        )
    return result


def _object_list(value: Any, field_name: str) -> list[dict[str, Any]]:
    if not isinstance(value, list) or not value:
        raise ValueError(f"{field_name} must be a non-empty list")
    result: list[dict[str, Any]] = []
    for index, item in enumerate(value):
        if not isinstance(item, dict):
            raise ValueError(f"{field_name}[{index}] must be an object")
        result.append(item)
    return result


def _memory_gate(value: Any, context: str) -> MemoryGate:
    normalized = _string(value).upper()
    if normalized not in _MEMORY_GATES:
        raise ValueError(f"{context} must be one of {sorted(_MEMORY_GATES)}")
    return cast(MemoryGate, normalized)


def _window_state(value: Any, context: str) -> WindowState:
    normalized = _string(value).upper()
    if normalized not in _WINDOW_STATES:
        raise ValueError(f"{context} must be one of {sorted(_WINDOW_STATES)}")
    return cast(WindowState, normalized)


def _review_action(value: Any, context: str) -> ReviewAction:
    normalized = _string(value).upper()
    if normalized not in _REVIEW_ACTIONS:
        raise ValueError(f"{context} must be one of {sorted(_REVIEW_ACTIONS)}")
    return cast(ReviewAction, normalized)


def _required_string(payload: dict[str, Any], field_name: str, context: str) -> str:
    value = _string(payload.get(field_name))
    if not value:
        raise ValueError(f"{context}.{field_name} is required")
    return value


def _string(value: Any) -> str:
    return str(value or "").strip()


def _string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = _string(item)
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result


def _upper_string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = _string(item).upper()
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result


def _required_float(payload: dict[str, Any], field_name: str, context: str) -> float:
    if field_name not in payload:
        raise ValueError(f"{context}.{field_name} is required")
    value = payload[field_name]
    if not isinstance(value, int | float) or isinstance(value, bool):
        raise ValueError(f"{context}.{field_name} must be numeric")
    result = float(value)
    if result < 0:
        raise ValueError(f"{context}.{field_name} must be non-negative")
    return result


def _float_default(payload: dict[str, Any], field_name: str, default: float) -> float:
    if field_name not in payload:
        return default
    return _required_float(payload, field_name, "acceptance_policy")


def _required_int(payload: dict[str, Any], field_name: str, context: str) -> int:
    if field_name not in payload:
        raise ValueError(f"{context}.{field_name} is required")
    value = payload[field_name]
    if not isinstance(value, int) or isinstance(value, bool):
        raise ValueError(f"{context}.{field_name} must be an integer")
    if value < 0:
        raise ValueError(f"{context}.{field_name} must be non-negative")
    return value


def _bool_default(payload: dict[str, Any], field_name: str, default: bool) -> bool:
    if field_name not in payload:
        return default
    value = payload[field_name]
    if not isinstance(value, bool):
        raise ValueError(f"{field_name} must be a boolean")
    return value


def _ratio(numerator: int, denominator: int, *, default: float = 0.0) -> float:
    if denominator == 0:
        return default
    return round(numerator / denominator, 4)
