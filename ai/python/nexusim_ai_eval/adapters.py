"""Public-dataset adapter skeletons for isolated Agent eval.

These adapters intentionally accept already-local, low-sensitive dict payloads.
They do not download datasets, call model providers, or import backend services.
"""

from __future__ import annotations

from typing import Any, Protocol

from nexusim_ai_eval.contracts import SCHEMA_VERSION


class DatasetAdapter(Protocol):
    adapter_name: str
    adapter_version: str

    def to_eval_case(self, payload: dict[str, Any]) -> dict[str, Any]:
        """Convert one low-sensitive public-dataset-style payload to EvalCase JSON."""


class QasperLikeRagAdapter:
    adapter_name = "qasper_like_rag"
    adapter_version = "qasper-like-rag-adapter-v1"

    def to_eval_case(self, payload: dict[str, Any]) -> dict[str, Any]:
        case_id = _required(payload, "case_id")
        evidence_refs = _string_list(payload.get("evidence_refs", []), "evidence_refs")
        expected_citations = _string_list(
            payload.get("expected_citation_refs", evidence_refs), "expected_citation_refs"
        )
        return {
            "case_id": case_id,
            "dataset_name": str(payload.get("dataset_name", "qasper-like")),
            "dataset_version": str(payload.get("dataset_version", "local-skeleton")),
            "capability_family": "GROUNDED_RAG",
            "fixture_version": self.adapter_version,
            "input_refs": _string_list(payload.get("input_refs", [f"input:{case_id}"]), "input_refs"),
            "visible_evidence_refs": evidence_refs,
            "forbidden_evidence_refs": _string_list(
                payload.get("forbidden_evidence_refs", []), "forbidden_evidence_refs"
            ),
            "actual_used_refs": _string_list(
                payload.get("actual_used_refs", evidence_refs), "actual_used_refs"
            ),
            "expected_citation_refs": expected_citations,
            "actual_citation_refs": _string_list(
                payload.get("actual_citation_refs", expected_citations), "actual_citation_refs"
            ),
            "expected_failure_class": str(payload.get("expected_failure_class", "")),
            "actual_failure_class": str(payload.get("actual_failure_class", "")),
            "actual_abstained": bool(payload.get("actual_abstained", False)),
        }


class ToolSandboxLikeAdapter:
    adapter_name = "toolsandbox_like_tool"
    adapter_version = "toolsandbox-like-tool-adapter-v1"

    def to_eval_case(self, payload: dict[str, Any]) -> dict[str, Any]:
        case_id = _required(payload, "case_id")
        tool_ref = str(payload.get("tool_ref", f"tool:synthetic:{case_id}"))
        return {
            "case_id": case_id,
            "dataset_name": str(payload.get("dataset_name", "toolsandbox-like")),
            "dataset_version": str(payload.get("dataset_version", "local-skeleton")),
            "capability_family": str(payload.get("capability_family", "TOOL_SECURITY")),
            "fixture_version": self.adapter_version,
            "input_refs": _string_list(payload.get("input_refs", [f"input:{case_id}"]), "input_refs"),
            "visible_evidence_refs": _string_list(
                payload.get("visible_evidence_refs", [tool_ref]), "visible_evidence_refs"
            ),
            "actual_used_refs": _string_list(
                payload.get("actual_used_refs", [tool_ref]), "actual_used_refs"
            ),
            "expected_tool_prepare": str(payload.get("expected_tool_prepare", "BLOCKED")),
            "actual_tool_prepare": str(payload.get("actual_tool_prepare", "BLOCKED")),
            "malicious_tool_blocked": bool(payload.get("malicious_tool_blocked", True)),
            "unsafe_output_quarantined": bool(payload.get("unsafe_output_quarantined", True)),
            "expected_state_diff": _string_map(
                payload.get("expected_state_diff", {}), "expected_state_diff"
            ),
            "actual_state_diff": _string_map(payload.get("actual_state_diff", {}), "actual_state_diff"),
            "expected_failure_class": str(payload.get("expected_failure_class", "")),
            "actual_failure_class": str(payload.get("actual_failure_class", "")),
        }


class StateBenchLikeMemoryAdapter:
    adapter_name = "statebench_like_memory"
    adapter_version = "statebench-like-memory-adapter-v1"

    def to_eval_case(self, payload: dict[str, Any]) -> dict[str, Any]:
        case_id = _required(payload, "case_id")
        source_ref = str(payload.get("source_ref", f"memory-source:{case_id}"))
        return {
            "case_id": case_id,
            "dataset_name": str(payload.get("dataset_name", "statebench-like-memory")),
            "dataset_version": str(payload.get("dataset_version", "local-skeleton")),
            "capability_family": "MEMORY_ADMISSION",
            "fixture_version": self.adapter_version,
            "input_refs": _string_list(payload.get("input_refs", [f"input:{case_id}"]), "input_refs"),
            "visible_evidence_refs": _string_list(
                payload.get("visible_evidence_refs", [source_ref]), "visible_evidence_refs"
            ),
            "actual_used_refs": _string_list(
                payload.get("actual_used_refs", [source_ref]), "actual_used_refs"
            ),
            "expected_memory_outcome": str(payload.get("expected_memory_outcome", "ADMIT")),
            "actual_memory_outcome": str(payload.get("actual_memory_outcome", "ADMIT")),
            "expected_memory_scope": str(payload.get("expected_memory_scope", "GROUP")),
            "actual_memory_scope": str(payload.get("actual_memory_scope", "GROUP")),
            "revoked_memory_used": bool(payload.get("revoked_memory_used", False)),
            "expected_failure_class": str(payload.get("expected_failure_class", "")),
            "actual_failure_class": str(payload.get("actual_failure_class", "")),
        }


def adapter_by_name(adapter_name: str) -> DatasetAdapter:
    adapters: dict[str, DatasetAdapter] = {
        QasperLikeRagAdapter.adapter_name: QasperLikeRagAdapter(),
        ToolSandboxLikeAdapter.adapter_name: ToolSandboxLikeAdapter(),
        StateBenchLikeMemoryAdapter.adapter_name: StateBenchLikeMemoryAdapter(),
    }
    normalized = adapter_name.strip()
    if normalized not in adapters:
        raise ValueError(f"unsupported adapter_name: {normalized}")
    return adapters[normalized]


def suite_from_adapter_cases(
    *,
    suite_id: str,
    adapter: DatasetAdapter,
    cases: list[dict[str, Any]],
) -> dict[str, Any]:
    return {
        "schema_version": SCHEMA_VERSION,
        "suite_id": suite_id,
        "fixture_kind": "synthetic_im_like",
        "adapter_versions": [adapter.adapter_version],
        "cases": [adapter.to_eval_case(case) for case in cases],
    }


def _required(payload: dict[str, Any], field_name: str) -> str:
    value = str(payload.get(field_name, "")).strip()
    if not value:
        raise ValueError(f"{field_name} is required")
    return value


def _string_list(value: Any, field_name: str) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"{field_name} must be a list")
    result: list[str] = []
    for item in value:
        normalized = str(item).strip()
        if not normalized:
            raise ValueError(f"{field_name} contains empty item")
        result.append(normalized)
    return result


def _string_map(value: Any, field_name: str) -> dict[str, str]:
    if not isinstance(value, dict):
        raise ValueError(f"{field_name} must be an object")
    return {str(key).strip(): str(nested).strip() for key, nested in value.items()}
