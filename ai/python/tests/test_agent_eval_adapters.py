from __future__ import annotations

import unittest

from nexusim_ai_eval.adapters import (
    QasperLikeRagAdapter,
    StateBenchLikeMemoryAdapter,
    ToolSandboxLikeAdapter,
    suite_from_adapter_cases,
)
from nexusim_ai_eval.evaluator import run_eval_suite


class AgentEvalAdapterTests(unittest.TestCase):
    def test_qasper_like_adapter_builds_grounded_rag_case(self) -> None:
        adapter = QasperLikeRagAdapter()
        suite = suite_from_adapter_cases(
            suite_id="adapter-rag-suite",
            adapter=adapter,
            cases=[
                {
                    "case_id": "rag-adapter-case",
                    "evidence_refs": ["paper:local:chunk-1"],
                }
            ],
        )

        report = run_eval_suite(suite)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.eval_run.adapter_versions, [adapter.adapter_version])
        self.assertEqual(report.aggregate_scores["citation_coverage"], 1.0)

    def test_tool_adapter_builds_tool_security_case(self) -> None:
        adapter = ToolSandboxLikeAdapter()
        suite = suite_from_adapter_cases(
            suite_id="adapter-tool-suite",
            adapter=adapter,
            cases=[
                {
                    "case_id": "tool-adapter-case",
                    "tool_ref": "tool:fixture:unsafe-export",
                    "expected_tool_provider_ref": "mcp-provider:fixture",
                    "actual_tool_provider_ref": "mcp-provider:fixture",
                    "expected_tool_capability_lease_refs": ["capability-lease:fixture:export"],
                    "actual_tool_capability_lease_refs": ["capability-lease:fixture:export"],
                    "expected_tool_capability_scope_refs": ["capability-scope:fixture:thread"],
                    "actual_tool_capability_scope_refs": ["capability-scope:fixture:thread"],
                    "tool_capability_lease_validated": True,
                    "expected_tool_provider_attestation_refs": ["attestation:mcp-provider:fixture:v1"],
                    "actual_tool_provider_attestation_refs": ["attestation:mcp-provider:fixture:v1"],
                    "tool_provider_attestation_verified": True,
                }
            ],
        )

        report = run_eval_suite(suite)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.aggregate_scores["security_block_score"], 1.0)
        self.assertEqual(report.aggregate_scores["tool_capability_lease_score"], 1.0)
        self.assertEqual(report.aggregate_scores["mcp_provider_attestation_score"], 1.0)
        self.assertEqual(
            suite["cases"][0]["actual_tool_capability_lease_refs"],
            ["capability-lease:fixture:export"],
        )

    def test_memory_adapter_builds_memory_admission_case(self) -> None:
        adapter = StateBenchLikeMemoryAdapter()
        suite = suite_from_adapter_cases(
            suite_id="adapter-memory-suite",
            adapter=adapter,
            cases=[
                {
                    "case_id": "memory-adapter-case",
                    "source_ref": "message:fixture:group:1",
                }
            ],
        )

        report = run_eval_suite(suite)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.aggregate_scores["memory_scope_score"], 1.0)

    def test_memory_adapter_preserves_alignment_refs(self) -> None:
        adapter = StateBenchLikeMemoryAdapter()
        suite = suite_from_adapter_cases(
            suite_id="adapter-memory-alignment-suite",
            adapter=adapter,
            cases=[
                {
                    "case_id": "memory-alignment-case",
                    "source_ref": "message:project:decision:2",
                    "expected_memory_scope": "PROJECT",
                    "actual_memory_scope": "PROJECT",
                    "expected_memory_cluster_representative_refs": [
                        "message:project:decision:2"
                    ],
                    "actual_memory_cluster_representative_refs": [
                        "message:project:decision:2"
                    ],
                    "memory_cluster_representative_selected": True,
                    "expected_memory_confidence_threshold_refs": [
                        "confidence-threshold:project-medium-review"
                    ],
                    "actual_memory_confidence_threshold_refs": [
                        "confidence-threshold:project-medium-review"
                    ],
                    "memory_confidence_threshold_applied": True,
                    "expected_policy_revocation_window_refs": [
                        "revocation-window:project-policy:v1:closed"
                    ],
                    "actual_policy_revocation_window_refs": [
                        "revocation-window:project-policy:v1:closed"
                    ],
                    "policy_revocation_window_recorded": True,
                }
            ],
        )

        report = run_eval_suite(suite)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.aggregate_scores["memory_cluster_representative_score"], 1.0)
        self.assertEqual(report.aggregate_scores["memory_confidence_threshold_score"], 1.0)
        self.assertEqual(report.aggregate_scores["memory_policy_revocation_window_score"], 1.0)


if __name__ == "__main__":
    unittest.main()
