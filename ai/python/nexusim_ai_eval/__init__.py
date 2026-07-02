"""Offline Agent eval harness for synthetic IM-like fixtures."""

from nexusim_ai_eval.adapters import (
    QasperLikeRagAdapter,
    StateBenchLikeMemoryAdapter,
    ToolSandboxLikeAdapter,
    adapter_by_name,
    suite_from_adapter_cases,
)
from nexusim_ai_eval.adapter_runner import convert_adapter_payload, run_adapter_payload
from nexusim_ai_eval.comparison import compare_eval_reports
from nexusim_ai_eval.context_evidence_preservation import (
    load_context_evidence_preservation_rehearsal,
    rehearse_context_evidence_preservation,
)
from nexusim_ai_eval.contracts import (
    EvalCase,
    EvalRun,
    EvalReport,
    EvalResult,
    ReplayBundle,
    validate_eval_suite,
)
from nexusim_ai_eval.evaluator import run_eval_suite
from nexusim_ai_eval.memory_calibration import (
    load_memory_calibration_payload,
    run_memory_admission_calibration,
)
from nexusim_ai_eval.replay_compatibility import (
    load_replay_version_bump_rehearsal,
    rehearse_replay_version_bump,
)
from nexusim_ai_eval.runtime_workflow_ownership import (
    load_runtime_workflow_ownership_rehearsal,
    rehearse_runtime_workflow_ownership,
)
from nexusim_ai_eval.reporting import (
    build_baseline_refresh_approval_manifest,
    build_baseline_refresh_review,
    build_report_matrix_payload,
    build_retention_metadata,
    generate_current_report_payload,
    load_report_matrix_plan,
    run_report_matrix_plan,
)
from nexusim_ai_eval.trace import (
    AgentRunTrace,
    AgentStep,
    RuntimeControlFixture,
    build_agent_run_trace,
)

__all__ = [
    "EvalCase",
    "EvalRun",
    "EvalReport",
    "EvalResult",
    "ReplayBundle",
    "AgentRunTrace",
    "AgentStep",
    "RuntimeControlFixture",
    "QasperLikeRagAdapter",
    "StateBenchLikeMemoryAdapter",
    "ToolSandboxLikeAdapter",
    "adapter_by_name",
    "build_agent_run_trace",
    "build_baseline_refresh_approval_manifest",
    "build_baseline_refresh_review",
    "build_report_matrix_payload",
    "build_retention_metadata",
    "compare_eval_reports",
    "convert_adapter_payload",
    "generate_current_report_payload",
    "load_context_evidence_preservation_rehearsal",
    "load_report_matrix_plan",
    "load_memory_calibration_payload",
    "load_replay_version_bump_rehearsal",
    "load_runtime_workflow_ownership_rehearsal",
    "run_eval_suite",
    "rehearse_context_evidence_preservation",
    "rehearse_replay_version_bump",
    "rehearse_runtime_workflow_ownership",
    "run_memory_admission_calibration",
    "run_adapter_payload",
    "run_report_matrix_plan",
    "suite_from_adapter_cases",
    "validate_eval_suite",
]
