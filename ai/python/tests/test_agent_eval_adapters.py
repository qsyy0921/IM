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
                }
            ],
        )

        report = run_eval_suite(suite)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.aggregate_scores["security_block_score"], 1.0)

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


if __name__ == "__main__":
    unittest.main()
