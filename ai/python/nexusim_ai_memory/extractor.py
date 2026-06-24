"""Hash-only collaborative memory extraction candidate builder.

The extractor consumes explicit low-sensitive message payloads and emits
candidate metadata only. It does not persist facts, approve memory, call IM
services, or return raw message text.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from typing import Any

from nexusim_ai_common.contracts import validate_worker_candidate
from nexusim_ai_common.safety import assert_low_sensitive_value


SCHEMA_VERSION = 1
EXTRACTOR_VERSION = "memory-extraction-candidate-v1"
MAX_MESSAGE_TEXT_CHARS = 4000

_CUE_PATTERN = re.compile(
    r"^\s*(decision|task|status|blocker|file|profile_signal)\s*:\s*(.+?)\s*$",
    re.IGNORECASE,
)

_EVENT_TYPE_BY_CUE = {
    "decision": "DECISION",
    "task": "TASK",
    "status": "STATUS",
    "blocker": "BLOCKER",
    "file": "FILE",
    "profile_signal": "PROFILE_SIGNAL",
}


@dataclass(frozen=True)
class SourceMessage:
    message_id: str
    conversation_seq: int
    speaker_id: str
    text: str


@dataclass(frozen=True)
class ExtractedCue:
    cue: str
    fact_text: str
    line_index: int


def run_memory_extraction(payload: Any) -> tuple[dict[str, Any], int]:
    """Return result payload and process-style exit code."""

    if not isinstance(payload, dict):
        return _failed_result({}, "MALFORMED_INPUT"), 2

    try:
        return build_memory_extraction_result(payload), 0
    except ValueError as exc:
        message = str(exc).lower()
        if "sensitive" in message or "forbidden" in message:
            return _failed_result(payload, "UNSAFE_INPUT"), 2
        return _failed_result(payload, "MALFORMED_INPUT"), 2


def build_memory_extraction_result(payload: dict[str, Any]) -> dict[str, Any]:
    """Build low-sensitive memory candidates from an explicit message batch."""

    assert_low_sensitive_value(payload, "memory extraction request")
    if payload.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("schema_version must be 1")

    task_id = _required_string(payload, "task_id")
    tenant_id = _required_string(payload, "tenant_id")
    conversation_id = _required_string(payload, "conversation_id")
    messages = _messages(payload.get("messages", []))

    candidates: list[dict[str, Any]] = []
    ordinary_message_count = 0
    event_type_counts: dict[str, int] = {}
    for message in messages:
        extracted = _extract_cues(message.text)
        if not extracted:
            ordinary_message_count += 1
            continue
        for cue in extracted:
            candidate = _build_candidate(
                task_id=task_id,
                tenant_id=tenant_id,
                conversation_id=conversation_id,
                message=message,
                cue=cue,
            )
            candidates.append(candidate)
            event_type = str(candidate["memory_event_type"])
            event_type_counts[event_type] = event_type_counts.get(event_type, 0) + 1

    return {
        "schema_version": SCHEMA_VERSION,
        "task_id": task_id,
        "extractor_version": EXTRACTOR_VERSION,
        "status": "COMPLETED",
        "tenant_id_hash": _sha256(tenant_id),
        "conversation_id_hash": _sha256(conversation_id),
        "message_count": len(messages),
        "candidate_count": len(candidates),
        "ordinary_message_count": ordinary_message_count,
        "candidates": candidates,
        "report": {
            "schema_version": SCHEMA_VERSION,
            "scope": "low-sensitive memory extraction candidate report",
            "message_count": len(messages),
            "candidate_count": len(candidates),
            "ordinary_message_count": ordinary_message_count,
            "event_type_counts": dict(sorted(event_type_counts.items())),
            "candidate_hashes": [str(candidate["output_sha256"]) for candidate in candidates],
            "raw_text_returned": False,
            "final_memory_persisted": False,
            "requires_go_validation": True,
            "requires_review_for_profile_signal": True,
        },
    }


def _build_candidate(
    *,
    task_id: str,
    tenant_id: str,
    conversation_id: str,
    message: SourceMessage,
    cue: ExtractedCue,
) -> dict[str, Any]:
    cue_key = cue.cue.lower()
    event_type = _EVENT_TYPE_BY_CUE[cue_key]
    source_ref = f"message:{tenant_id}:{conversation_id}:{message.conversation_seq}"
    review_state = "NEEDS_REVIEW" if event_type == "PROFILE_SIGNAL" else "CANDIDATE_ONLY"
    safety_flags = ["LOW_SENSITIVE", "GO_VALIDATION_REQUIRED", review_state]
    if event_type == "PROFILE_SIGNAL":
        safety_flags.append("GROUP_SCOPE_PROFILE_SIGNAL")

    canonical = {
        "schema_version": SCHEMA_VERSION,
        "extractor_version": EXTRACTOR_VERSION,
        "tenant_id": tenant_id,
        "conversation_id": conversation_id,
        "message_id": message.message_id,
        "conversation_seq": message.conversation_seq,
        "speaker_id": message.speaker_id,
        "memory_event_type": event_type,
        "review_state": review_state,
        "fact_text": _normalize_fact(cue.fact_text),
        "source_refs": [source_ref],
        "line_index": cue.line_index,
    }
    output_sha256 = _sha256_json(canonical)
    candidate = {
        "schema_version": SCHEMA_VERSION,
        "task_id": task_id,
        "candidate_id": f"memcand_{output_sha256[:24]}",
        "worker_kind": "MEMORY_EXTRACTION",
        "status": "CANDIDATE",
        "output_type": "MEMORY_EVENT_CANDIDATE",
        "output_sha256": output_sha256,
        "source_refs": [source_ref],
        "citations": [source_ref],
        "safety_flags": safety_flags,
        "confidence": _confidence_for_event_type(event_type),
        "memory_event_type": event_type,
        "review_state": review_state,
        "fact_sha256": _sha256(_normalize_fact(cue.fact_text)),
        "speaker_id_hash": _sha256(message.speaker_id),
        "message_id_hash": _sha256(message.message_id),
        "conversation_seq": message.conversation_seq,
        "source_ref_count": 1,
    }
    validate_worker_candidate(candidate)
    return candidate


def _extract_cues(text: str) -> list[ExtractedCue]:
    extracted: list[ExtractedCue] = []
    for index, raw_line in enumerate(text.splitlines()):
        match = _CUE_PATTERN.match(raw_line)
        if match is None:
            continue
        cue = match.group(1).lower()
        fact_text = _normalize_fact(match.group(2))
        if not fact_text:
            continue
        extracted.append(ExtractedCue(cue=cue, fact_text=fact_text, line_index=index))
    return extracted


def _messages(value: Any) -> list[SourceMessage]:
    if not isinstance(value, list):
        raise ValueError("messages must be a list")
    result: list[SourceMessage] = []
    for index, item in enumerate(value):
        if not isinstance(item, dict):
            raise ValueError(f"messages[{index}] must be an object")
        text = _required_string(item, "text")
        if len(text) > MAX_MESSAGE_TEXT_CHARS:
            raise ValueError(f"messages[{index}].text exceeds max length")
        conversation_seq = item.get("conversation_seq")
        if not isinstance(conversation_seq, int) or isinstance(conversation_seq, bool):
            raise ValueError(f"messages[{index}].conversation_seq must be an integer")
        if conversation_seq <= 0:
            raise ValueError(f"messages[{index}].conversation_seq must be positive")
        result.append(
            SourceMessage(
                message_id=_required_string(item, "message_id"),
                conversation_seq=conversation_seq,
                speaker_id=_required_string(item, "speaker_id"),
                text=text,
            )
        )
    return result


def _failed_result(payload: dict[str, Any], error_class: str) -> dict[str, Any]:
    task_id = _failed_task_id(payload)
    return {
        "schema_version": SCHEMA_VERSION,
        "task_id": task_id,
        "extractor_version": EXTRACTOR_VERSION,
        "status": "FAILED",
        "error_class": error_class,
        "candidate_count": 0,
        "candidates": [],
        "report": {
            "schema_version": SCHEMA_VERSION,
            "scope": "low-sensitive memory extraction candidate report",
            "candidate_count": 0,
            "raw_text_returned": False,
            "final_memory_persisted": False,
            "requires_go_validation": True,
        },
    }


def _failed_task_id(payload: dict[str, Any]) -> str:
    task_id = _safe_string(payload.get("task_id", "unknown_task")) if payload else "unknown_task"
    if not task_id:
        return "unknown_task"
    try:
        assert_low_sensitive_value(task_id, "memory extraction failed task_id")
    except ValueError:
        return "unknown_task"
    return task_id


def _required_string(payload: dict[str, Any], field_name: str) -> str:
    value = _safe_string(payload.get(field_name, ""))
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def _safe_string(value: Any) -> str:
    return str(value).strip()


def _normalize_fact(value: str) -> str:
    return " ".join(value.strip().split())


def _confidence_for_event_type(event_type: str) -> float:
    if event_type == "PROFILE_SIGNAL":
        return 0.55
    if event_type in {"DECISION", "TASK"}:
        return 0.78
    return 0.7


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _sha256_json(value: dict[str, Any]) -> str:
    serialized = json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    return _sha256(serialized)
