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


if __name__ == "__main__":
    unittest.main()
