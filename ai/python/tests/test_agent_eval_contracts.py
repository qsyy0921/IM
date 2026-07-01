from __future__ import annotations

import unittest

from nexusim_ai_eval.contracts import validate_eval_suite


def valid_suite() -> dict[str, object]:
    return {
        "schema_version": 1,
        "suite_id": "unit-suite",
        "fixture_kind": "synthetic_im_like",
        "cases": [
            {
                "case_id": "rag-pass",
                "dataset_name": "synthetic-rag",
                "dataset_version": "2026-07-01",
                "capability_family": "GROUNDED_RAG",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:rag-pass"],
                "visible_evidence_refs": ["evidence:visible"],
                "actual_used_refs": ["evidence:visible"],
                "expected_citation_refs": ["evidence:visible"],
                "actual_citation_refs": ["evidence:visible"],
            }
        ],
    }


class AgentEvalContractTests(unittest.TestCase):
    def test_validates_synthetic_suite(self) -> None:
        cases = validate_eval_suite(valid_suite())

        self.assertEqual(len(cases), 1)
        self.assertEqual(cases[0].case_id, "rag-pass")
        self.assertEqual(cases[0].capability_family, "GROUNDED_RAG")

    def test_rejects_non_synthetic_fixture_kind(self) -> None:
        payload = valid_suite()
        payload["fixture_kind"] = "production_im"

        with self.assertRaisesRegex(ValueError, "fixture_kind"):
            validate_eval_suite(payload)

    def test_rejects_sensitive_prompt_like_fields(self) -> None:
        payload = valid_suite()
        payload["raw_prompt"] = "do not keep this"

        with self.assertRaisesRegex(ValueError, "forbidden"):
            validate_eval_suite(payload)

    def test_rejects_backend_connectivity_fields(self) -> None:
        payload = valid_suite()
        payload["backend_url"] = "http://localhost:8080"

        with self.assertRaisesRegex(ValueError, "forbidden eval field"):
            validate_eval_suite(payload)

    def test_rejects_duplicate_case_ids(self) -> None:
        payload = valid_suite()
        cases = payload["cases"]
        assert isinstance(cases, list)
        cases.append(dict(cases[0]))

        with self.assertRaisesRegex(ValueError, "duplicate case_id"):
            validate_eval_suite(payload)

    def test_validates_runtime_control_refs(self) -> None:
        payload = valid_suite()
        payload["cases"] = [
            {
                "case_id": "runtime-control-pass",
                "dataset_name": "synthetic-runtime",
                "dataset_version": "2026-07-01",
                "capability_family": "RUNTIME_CONTROL",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:runtime-control-pass"],
                "expected_runtime_events": ["checkpoint_created", "resume_completed"],
                "actual_runtime_events": ["checkpoint_created", "resume_completed"],
                "expected_checkpoint_refs": ["checkpoint:runtime"],
                "actual_checkpoint_refs": ["checkpoint:runtime"],
            }
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].capability_family, "RUNTIME_CONTROL")
        self.assertEqual(cases[0].expected_runtime_events, ["CHECKPOINT_CREATED", "RESUME_COMPLETED"])
        self.assertEqual(cases[0].actual_checkpoint_refs, ["checkpoint:runtime"])

    def test_validates_mcp_security_refs(self) -> None:
        payload = valid_suite()
        payload["cases"] = [
            {
                "case_id": "mcp-security-pass",
                "dataset_name": "synthetic-mcp",
                "dataset_version": "2026-07-01",
                "capability_family": "TOOL_SECURITY",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:mcp-security-pass"],
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
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].expected_tool_provider_ref, "mcp-provider:trusted")
        self.assertTrue(cases[0].tool_description_poisoned)
        self.assertTrue(cases[0].tool_output_contains_instruction)

    def test_validates_context_evidence_refs(self) -> None:
        payload = valid_suite()
        payload["cases"] = [
            {
                "case_id": "context-evidence-pass",
                "dataset_name": "synthetic-context",
                "dataset_version": "2026-07-01",
                "capability_family": "CONTEXT_EVIDENCE",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:context-evidence-pass"],
                "expected_source_coverage_refs": ["evidence:current"],
                "actual_source_coverage_refs": ["evidence:current"],
                "conflicting_evidence_refs": ["evidence:old", "evidence:current"],
                "stale_evidence_refs": ["evidence:old"],
                "memory_conflict_source_refs": ["memory:old", "evidence:current"],
                "memory_precedence_source_refs": ["evidence:current"],
                "unsafe_context_refs": ["tool-output:mcp-reader:instruction"],
                "context_blocked_refs": ["tool-output:mcp-reader:instruction"],
                "expected_budget_retained_refs": ["evidence:current"],
                "actual_budget_retained_refs": ["evidence:current"],
                "expected_retrieval_lanes": ["conversation", "memory"],
                "actual_retrieval_lanes": ["conversation"],
                "unavailable_retrieval_lanes": ["memory"],
                "conflict_detected": True,
                "stale_evidence_used": False,
                "permission_abstain_required": False,
                "memory_source_precedence_applied": True,
                "unsafe_context_quarantined": True,
                "context_budget_truncated": True,
                "retrieval_lane_gap_reported": True,
            }
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].capability_family, "CONTEXT_EVIDENCE")
        self.assertEqual(cases[0].expected_source_coverage_refs, ["evidence:current"])
        self.assertEqual(cases[0].memory_precedence_source_refs, ["evidence:current"])
        self.assertEqual(cases[0].context_blocked_refs, ["tool-output:mcp-reader:instruction"])
        self.assertEqual(cases[0].expected_budget_retained_refs, ["evidence:current"])
        self.assertEqual(cases[0].unavailable_retrieval_lanes, ["memory"])
        self.assertTrue(cases[0].conflict_detected)
        self.assertFalse(cases[0].stale_evidence_used)
        self.assertTrue(cases[0].memory_source_precedence_applied)
        self.assertTrue(cases[0].unsafe_context_quarantined)
        self.assertTrue(cases[0].context_budget_truncated)
        self.assertTrue(cases[0].retrieval_lane_gap_reported)

    def test_validates_memory_admission_metadata(self) -> None:
        payload = valid_suite()
        payload["cases"] = [
            {
                "case_id": "memory-admission-pass",
                "dataset_name": "synthetic-memory",
                "dataset_version": "2026-07-01",
                "capability_family": "MEMORY_ADMISSION",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:memory-admission-pass"],
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
                "stale_memory_refs": ["memory:project:decision:v1"],
                "duplicate_memory_refs": ["memory:project:decision:v1"],
                "actual_memory_dedupe_refs": ["memory:project:decision:v1"],
                "low_confidence_memory_refs": ["candidate:memory:uncertain"],
                "expected_memory_skill_refs": ["skill:memory:procedure:v1"],
                "actual_memory_skill_refs": ["skill:memory:procedure:v1"],
                "policy_memory_refs": ["candidate:policy-like:rule"],
                "governed_policy_source_refs": ["policy-source:retention:v1"],
                "review_timeout_refs": ["review-timeout:memory:project"],
                "memory_deduped": True,
                "low_confidence_memory_rejected": True,
                "policy_memory_rejected": False,
                "memory_review_timeout_recorded": True,
                "profile_aggregate_review_required": True,
                "profile_aggregate_reviewed": True,
            }
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].capability_family, "MEMORY_ADMISSION")
        self.assertEqual(cases[0].expected_memory_source_refs, ["message:project:decision:2"])
        self.assertEqual(cases[0].expected_memory_speaker_refs, ["user:pm"])
        self.assertEqual(cases[0].actual_memory_supersedes_refs, ["memory:project:decision:v1"])
        self.assertEqual(cases[0].duplicate_memory_refs, ["memory:project:decision:v1"])
        self.assertEqual(cases[0].actual_memory_dedupe_refs, ["memory:project:decision:v1"])
        self.assertEqual(cases[0].low_confidence_memory_refs, ["candidate:memory:uncertain"])
        self.assertEqual(cases[0].actual_memory_skill_refs, ["skill:memory:procedure:v1"])
        self.assertEqual(cases[0].governed_policy_source_refs, ["policy-source:retention:v1"])
        self.assertEqual(cases[0].review_timeout_refs, ["review-timeout:memory:project"])
        self.assertTrue(cases[0].memory_deduped)
        self.assertTrue(cases[0].low_confidence_memory_rejected)
        self.assertTrue(cases[0].memory_review_timeout_recorded)
        self.assertTrue(cases[0].profile_aggregate_review_required)

    def test_validates_state_diff_report_refs(self) -> None:
        payload = valid_suite()
        payload["cases"] = [
            {
                "case_id": "state-diff-report-pass",
                "dataset_name": "synthetic-state",
                "dataset_version": "2026-07-01",
                "capability_family": "STATE_DIFF",
                "fixture_version": "fixture-v1",
                "input_refs": ["input:state-diff-report-pass"],
                "expected_state_diff": {"task:1.status": "approved"},
                "actual_state_diff": {"task:1.status": "approved"},
                "expected_state_precondition_refs": ["precondition:task-1:pending"],
                "actual_state_precondition_refs": ["precondition:task-1:pending"],
                "expected_state_approval_refs": ["approval:task-1:manager"],
                "actual_state_approval_refs": ["approval:task-1:manager"],
                "expected_state_prepare_refs": ["tool-prepare:task:update"],
                "actual_state_prepare_refs": ["tool-prepare:task:update"],
                "expected_execution_refs": ["execution:task-1:update"],
                "actual_execution_refs": ["execution:task-1:update"],
                "expected_state_change_refs": ["state-change:task-1:status"],
                "actual_state_change_refs": ["state-change:task-1:status"],
                "expected_state_audit_refs": ["audit:task-1:update"],
                "actual_state_audit_refs": ["audit:task-1:update"],
                "expected_repair_refs": ["repair:task-1:prepare-expired"],
                "actual_repair_refs": ["repair:task-1:prepare-expired"],
                "expected_redrive_refs": ["redrive:task-1:attempt-2"],
                "actual_redrive_refs": ["redrive:task-1:attempt-2"],
                "partial_execution_refs": ["partial-execution:task-1:field-written"],
                "expected_idempotency_refs": ["idempotency:task-1:execution-key"],
                "actual_idempotency_refs": ["idempotency:task-1:execution-key"],
                "expected_compensating_action_refs": ["compensating-action:task-1:rollback"],
                "actual_compensating_action_refs": ["compensating-action:task-1:rollback"],
                "state_diff_report_complete": True,
                "repair_redrive_recorded": True,
                "partial_execution_detected": True,
                "idempotency_preserved": True,
                "compensating_action_recorded": True,
            }
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].capability_family, "STATE_DIFF")
        self.assertEqual(cases[0].actual_execution_refs, ["execution:task-1:update"])
        self.assertEqual(cases[0].expected_state_change_refs, ["state-change:task-1:status"])
        self.assertEqual(cases[0].actual_repair_refs, ["repair:task-1:prepare-expired"])
        self.assertEqual(cases[0].actual_redrive_refs, ["redrive:task-1:attempt-2"])
        self.assertEqual(cases[0].partial_execution_refs, ["partial-execution:task-1:field-written"])
        self.assertEqual(cases[0].actual_idempotency_refs, ["idempotency:task-1:execution-key"])
        self.assertEqual(
            cases[0].actual_compensating_action_refs,
            ["compensating-action:task-1:rollback"],
        )
        self.assertTrue(cases[0].state_diff_report_complete)
        self.assertTrue(cases[0].repair_redrive_recorded)
        self.assertTrue(cases[0].partial_execution_detected)
        self.assertTrue(cases[0].idempotency_preserved)
        self.assertTrue(cases[0].compensating_action_recorded)


if __name__ == "__main__":
    unittest.main()
