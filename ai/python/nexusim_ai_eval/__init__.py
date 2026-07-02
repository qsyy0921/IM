"""Offline Agent eval harness for synthetic IM-like fixtures."""

from nexusim_ai_eval.adapters import (
    QasperLikeRagAdapter,
    StateBenchLikeMemoryAdapter,
    ToolSandboxLikeAdapter,
    adapter_by_name,
    suite_from_adapter_cases,
)
from nexusim_ai_eval.adapter_runner import convert_adapter_payload, run_adapter_payload
from nexusim_ai_eval.agentops_governance import (
    load_agentops_governance_rehearsal,
    rehearse_agentops_governance,
)
from nexusim_ai_eval.comparison import compare_eval_reports
from nexusim_ai_eval.context_evidence_preservation import (
    load_context_evidence_preservation_rehearsal,
    rehearse_context_evidence_preservation,
)
from nexusim_ai_eval.cross_service_preservation import (
    load_cross_service_preservation_rehearsal,
    rehearse_cross_service_preservation,
)
from nexusim_ai_eval.dataset_reproducibility import (
    load_dataset_reproducibility_rehearsal,
    rehearse_dataset_reproducibility,
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
from nexusim_ai_eval.memory_admission_governance import (
    load_memory_admission_governance_rehearsal,
    rehearse_memory_admission_governance,
)
from nexusim_ai_eval.multi_agent_handoff import (
    load_multi_agent_handoff_rehearsal,
    rehearse_multi_agent_handoff,
)
from nexusim_ai_eval.object_completeness import (
    load_object_completeness_rehearsal,
    rehearse_object_completeness,
)
from nexusim_ai_eval.operator_governance import (
    load_operator_governance_rehearsal,
    rehearse_operator_governance,
)
from nexusim_ai_eval.replay_compatibility import (
    load_replay_version_bump_rehearsal,
    rehearse_replay_version_bump,
)
from nexusim_ai_eval.runtime_workflow_ownership import (
    load_runtime_workflow_ownership_rehearsal,
    rehearse_runtime_workflow_ownership,
)
from nexusim_ai_eval.tool_mcp_governance import (
    load_tool_mcp_governance_rehearsal,
    rehearse_tool_mcp_governance,
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
    "load_agentops_governance_rehearsal",
    "load_context_evidence_preservation_rehearsal",
    "load_cross_service_preservation_rehearsal",
    "load_dataset_reproducibility_rehearsal",
    "load_report_matrix_plan",
    "load_memory_calibration_payload",
    "load_memory_admission_governance_rehearsal",
    "load_multi_agent_handoff_rehearsal",
    "load_object_completeness_rehearsal",
    "load_operator_governance_rehearsal",
    "load_replay_version_bump_rehearsal",
    "load_runtime_workflow_ownership_rehearsal",
    "load_tool_mcp_governance_rehearsal",
    "run_eval_suite",
    "rehearse_agentops_governance",
    "rehearse_context_evidence_preservation",
    "rehearse_cross_service_preservation",
    "rehearse_dataset_reproducibility",
    "rehearse_memory_admission_governance",
    "rehearse_multi_agent_handoff",
    "rehearse_object_completeness",
    "rehearse_operator_governance",
    "rehearse_replay_version_bump",
    "rehearse_runtime_workflow_ownership",
    "rehearse_tool_mcp_governance",
    "run_memory_admission_calibration",
    "run_adapter_payload",
    "run_report_matrix_plan",
    "suite_from_adapter_cases",
    "validate_eval_suite",
]
