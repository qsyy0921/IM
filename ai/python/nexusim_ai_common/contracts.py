"""Low-sensitive worker candidate contract helpers.

Python workers return candidates. Go services validate, authorize, audit and
persist final state.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from nexusim_ai_common.safety import assert_low_sensitive_value


ALLOWED_WORKER_KINDS = {
    "LLM",
    "EMBEDDING",
    "RERANK",
    "MEMORY_EXTRACTION",
    "PLANNER",
    "EVAL",
}

ALLOWED_STATUSES = {"CANDIDATE", "ABSTAIN", "FAILED"}

ALLOWED_OUTPUT_TYPES = {
    "TEXT_CANDIDATE",
    "EMBEDDING_CANDIDATE",
    "RERANK_CANDIDATE",
    "MEMORY_EVENT_CANDIDATE",
    "PLAN_CANDIDATE",
    "EVAL_RESULT",
}

FORBIDDEN_FIELDS = {
    "approval_id",
    "approved",
    "approved_at",
    "business_status",
    "db_dsn",
    "execution_id",
    "final_answer_id",
    "password",
    "proposal_status",
    "raw_output",
    "raw_prompt",
    "raw_provider_body",
    "refresh_token",
    "secret",
    "session_id",
    "sql",
    "token",
}


@dataclass(frozen=True)
class WorkerCandidate:
    schema_version: int
    task_id: str
    candidate_id: str
    worker_kind: str
    status: str
    output_type: str
    output_sha256: str
    source_refs: list[str] = field(default_factory=list)
    citations: list[str] = field(default_factory=list)
    safety_flags: list[str] = field(default_factory=list)
    confidence: float | None = None
    error_class: str = ""


def validate_worker_candidate(payload: dict[str, Any]) -> WorkerCandidate:
    """Validate the first-stage low-sensitive candidate contract."""

    assert_no_forbidden_fields(payload)
    assert_low_sensitive_value(payload, "worker candidate")

    required = [
        "schema_version",
        "task_id",
        "candidate_id",
        "worker_kind",
        "status",
        "output_type",
        "output_sha256",
    ]
    for field_name in required:
        if field_name not in payload:
            raise ValueError(f"missing required field: {field_name}")

    schema_version = payload["schema_version"]
    if schema_version != 1:
        raise ValueError("schema_version must be 1")

    worker_kind = normalized_string(payload["worker_kind"])
    if worker_kind not in ALLOWED_WORKER_KINDS:
        raise ValueError(f"unsupported worker_kind: {worker_kind}")

    status = normalized_string(payload["status"])
    if status not in ALLOWED_STATUSES:
        raise ValueError(f"unsupported status: {status}")

    output_type = normalized_string(payload["output_type"])
    if output_type not in ALLOWED_OUTPUT_TYPES:
        raise ValueError(f"unsupported output_type: {output_type}")

    output_sha256 = normalized_string(payload["output_sha256"])
    if status == "CANDIDATE" and len(output_sha256) != 64:
        raise ValueError("CANDIDATE output_sha256 must be a 64-char sha256 hex")
    if output_sha256 and not all(ch in "0123456789abcdef" for ch in output_sha256):
        raise ValueError("output_sha256 must be lowercase hex")

    confidence = payload.get("confidence")
    if confidence is not None:
        if not isinstance(confidence, int | float):
            raise ValueError("confidence must be numeric")
        if confidence < 0 or confidence > 1:
            raise ValueError("confidence must be in [0,1]")

    return WorkerCandidate(
        schema_version=1,
        task_id=required_string(payload, "task_id"),
        candidate_id=required_string(payload, "candidate_id"),
        worker_kind=worker_kind,
        status=status,
        output_type=output_type,
        output_sha256=output_sha256,
        source_refs=string_list(payload.get("source_refs", []), "source_refs"),
        citations=string_list(payload.get("citations", []), "citations"),
        safety_flags=string_list(payload.get("safety_flags", []), "safety_flags"),
        confidence=float(confidence) if confidence is not None else None,
        error_class=normalized_string(payload.get("error_class", "")),
    )


def assert_no_forbidden_fields(value: Any, path: str = "") -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            normalized = str(key).strip().lower()
            current_path = f"{path}.{normalized}" if path else normalized
            if normalized in FORBIDDEN_FIELDS:
                raise ValueError(f"forbidden worker candidate field: {current_path}")
            assert_no_forbidden_fields(nested, current_path)
    elif isinstance(value, list):
        for index, nested in enumerate(value):
            assert_no_forbidden_fields(nested, f"{path}[{index}]")


def required_string(payload: dict[str, Any], field_name: str) -> str:
    value = normalized_string(payload.get(field_name, ""))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def normalized_string(value: Any) -> str:
    return str(value).strip()


def string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = normalized_string(item)
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result
