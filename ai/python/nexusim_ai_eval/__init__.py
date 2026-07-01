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
from nexusim_ai_eval.contracts import (
    EvalCase,
    EvalRun,
    EvalReport,
    EvalResult,
    ReplayBundle,
    validate_eval_suite,
)
from nexusim_ai_eval.evaluator import run_eval_suite
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
    "load_report_matrix_plan",
    "run_eval_suite",
    "run_adapter_payload",
    "run_report_matrix_plan",
    "suite_from_adapter_cases",
    "validate_eval_suite",
]
