from __future__ import annotations

import unittest

from nexusim_ai_eval.contracts import validate_eval_suite
from nexusim_ai_eval.trace import build_agent_run_trace


def suite_with_case(case: dict[str, object]) -> dict[str, object]:
    return {
        "schema_version": 1,
        "suite_id": "trace-suite",
        "fixture_kind": "synthetic_im_like",
        "cases": [case],
    }


class AgentEvalTraceTests(unittest.TestCase):
    def test_trace_contains_evidence_context_and_memory_candidate(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-memory-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "MEMORY_ADMISSION",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-memory-case"],
                    "visible_evidence_refs": ["message:fixture:group:1"],
                    "actual_used_refs": ["message:fixture:group:1"],
                    "expected_memory_outcome": "ADMIT",
                    "actual_memory_outcome": "ADMIT",
                    "expected_memory_scope": "GROUP",
                    "actual_memory_scope": "GROUP",
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "PASS")
        self.assertEqual(trace.capability_family, "MEMORY_ADMISSION")
        self.assertIsNotNone(trace.memory_candidate)
        self.assertIn("context_build", [step.step_type for step in trace.steps])
        self.assertIn("memory_candidate", [step.step_type for step in trace.steps])
        self.assertFalse(trace.context_package.permission_leakage_detected)

    def test_trace_contains_memory_admission_metadata(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-memory-rich-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "MEMORY_ADMISSION",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-memory-rich-case"],
                    "actual_used_refs": ["message:project:decision:2"],
                    "expected_memory_outcome": "ADMIT",
                    "actual_memory_outcome": "ADMIT",
                    "expected_memory_scope": "PROJECT",
                    "actual_memory_scope": "PROJECT",
                    "expected_memory_source_refs": ["message:project:decision:2"],
                    "actual_memory_source_refs": ["message:project:decision:2"],
                    "expected_memory_speaker_refs": ["user:pm"],
                    "actual_memory_speaker_refs": ["user:pm"],
                    "expected_memory_audience_refs": ["project:phoenix"],
                    "actual_memory_audience_refs": ["project:phoenix"],
                    "expected_memory_supersedes_refs": ["memory:project:decision:v1"],
                    "actual_memory_supersedes_refs": ["memory:project:decision:v1"],
                    "profile_aggregate_review_required": True,
                    "profile_aggregate_reviewed": True,
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "PASS")
        self.assertIsNotNone(trace.memory_candidate)
        assert trace.memory_candidate is not None
        self.assertEqual(trace.memory_candidate.source_refs, ["message:project:decision:2"])
        self.assertEqual(trace.memory_candidate.speaker_refs, ["user:pm"])
        self.assertEqual(trace.memory_candidate.audience_refs, ["project:phoenix"])
        self.assertEqual(trace.memory_candidate.supersedes_refs, ["memory:project:decision:v1"])
        self.assertTrue(trace.memory_candidate.profile_aggregate_reviewed)

    def test_trace_marks_permission_leakage(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-leakage-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "GROUNDED_RAG",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-leakage-case"],
                    "forbidden_evidence_refs": ["evidence:hidden"],
                    "actual_used_refs": ["evidence:hidden"],
                    "expected_failure_class": "PERMISSION_LEAKAGE",
                    "actual_failure_class": "PERMISSION_LEAKAGE",
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "FAIL")
        self.assertTrue(trace.context_package.permission_leakage_detected)
        self.assertEqual(trace.steps[1].failure_class, "PERMISSION_LEAKAGE")

    def test_trace_contains_context_evidence_metadata(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-context-evidence-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "CONTEXT_EVIDENCE",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-context-evidence-case"],
                    "visible_evidence_refs": ["evidence:old", "evidence:current"],
                    "actual_used_refs": ["evidence:current"],
                    "expected_source_coverage_refs": ["evidence:old", "evidence:current"],
                    "actual_source_coverage_refs": ["evidence:old", "evidence:current"],
                    "conflicting_evidence_refs": ["evidence:old", "evidence:current"],
                    "stale_evidence_refs": ["evidence:old"],
                    "conflict_detected": True,
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "PASS")
        self.assertEqual(trace.evidence_pack.source_coverage_refs, ["evidence:old", "evidence:current"])
        self.assertEqual(trace.evidence_pack.conflicting_source_refs, ["evidence:old", "evidence:current"])
        self.assertTrue(trace.context_package.conflict_detected)
        self.assertFalse(trace.context_package.stale_evidence_used)

    def test_trace_contains_tool_and_workflow_steps(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-tool-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "POLICY_HITL",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-tool-case"],
                    "expected_tool_prepare": "BLOCKED",
                    "actual_tool_prepare": "BLOCKED",
                    "malicious_tool_blocked": True,
                    "unsafe_output_quarantined": True,
                    "expected_failure_class": "APPROVAL_TIMEOUT",
                    "actual_failure_class": "APPROVAL_TIMEOUT",
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        step_types = [step.step_type for step in trace.steps]
        self.assertIn("tool_prepare", step_types)
        self.assertIn("workflow_wait", step_types)
        self.assertIsNotNone(trace.tool_intent)

    def test_trace_contains_mcp_security_metadata(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-mcp-security-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "TOOL_SECURITY",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-mcp-security-case"],
                    "expected_tool_prepare": "BLOCKED",
                    "actual_tool_prepare": "BLOCKED",
                    "expected_tool_provider_ref": "mcp-provider:trusted",
                    "actual_tool_provider_ref": "mcp-provider:trusted",
                    "malicious_tool_blocked": True,
                    "tool_description_poisoned": True,
                    "tool_description_blocked": True,
                    "tool_output_contains_instruction": True,
                    "unsafe_output_quarantined": True,
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "PASS")
        self.assertIsNotNone(trace.tool_intent)
        assert trace.tool_intent is not None
        self.assertEqual(trace.tool_intent.provider_ref, "mcp-provider:trusted")
        self.assertTrue(trace.tool_intent.tool_description_blocked)
        self.assertTrue(trace.tool_intent.tool_output_contains_instruction)

    def test_trace_contains_runtime_control_steps(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-runtime-control-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "RUNTIME_CONTROL",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-runtime-control-case"],
                    "expected_runtime_events": [
                        "CHECKPOINT_CREATED",
                        "CANCEL_REQUESTED",
                        "CANCEL_PROPAGATED",
                        "RESUME_COMPLETED",
                        "REPLAY_REQUESTED",
                    ],
                    "actual_runtime_events": [
                        "CHECKPOINT_CREATED",
                        "CANCEL_REQUESTED",
                        "CANCEL_PROPAGATED",
                        "RESUME_COMPLETED",
                        "REPLAY_REQUESTED",
                    ],
                    "expected_checkpoint_refs": ["checkpoint:trace-runtime"],
                    "actual_checkpoint_refs": ["checkpoint:trace-runtime"],
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        step_types = [step.step_type for step in trace.steps]
        self.assertEqual(trace.status, "PASS")
        self.assertIsNotNone(trace.runtime_control)
        self.assertIn("checkpoint", step_types)
        self.assertIn("cancel", step_types)
        self.assertIn("resume", step_types)
        self.assertIn("replay", step_types)

    def test_trace_contains_state_diff_report_metadata(self) -> None:
        case = validate_eval_suite(
            suite_with_case(
                {
                    "case_id": "trace-state-diff-report-case",
                    "dataset_name": "synthetic",
                    "dataset_version": "2026-07-01",
                    "capability_family": "STATE_DIFF",
                    "fixture_version": "fixture-v1",
                    "input_refs": ["input:trace-state-diff-report-case"],
                    "expected_state_diff": {"task:42.status": "approved"},
                    "actual_state_diff": {"task:42.status": "approved"},
                    "expected_state_approval_refs": ["approval:task-42:manager"],
                    "actual_state_approval_refs": ["approval:task-42:manager"],
                    "expected_state_prepare_refs": ["tool-prepare:task:update"],
                    "actual_state_prepare_refs": ["tool-prepare:task:update"],
                    "expected_execution_refs": ["execution:task-42:update"],
                    "actual_execution_refs": ["execution:task-42:update"],
                    "expected_state_change_refs": ["state-change:task-42:status"],
                    "actual_state_change_refs": ["state-change:task-42:status"],
                    "expected_state_audit_refs": ["audit:task-42:update"],
                    "actual_state_audit_refs": ["audit:task-42:update"],
                }
            )
        )[0]

        trace = build_agent_run_trace(case)

        self.assertEqual(trace.status, "PASS")
        self.assertIsNotNone(trace.state_diff_report)
        assert trace.state_diff_report is not None
        self.assertEqual(trace.state_diff_report.execution_refs, ["execution:task-42:update"])
        self.assertEqual(trace.state_diff_report.state_change_refs, ["state-change:task-42:status"])
        self.assertEqual(trace.state_diff_report.audit_refs, ["audit:task-42:update"])
        self.assertIn("state_diff", [step.step_type for step in trace.steps])


if __name__ == "__main__":
    unittest.main()
