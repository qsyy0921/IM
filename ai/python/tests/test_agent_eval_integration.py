from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path

from nexusim_ai_eval.fixtures import load_eval_suite


REPO_ROOT = Path(__file__).resolve().parents[3]
FIXTURE_PATH = REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_first_trio.json"
CORE_SCENARIOS_PATH = (
    REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_core_scenarios.json"
)
RUNTIME_CONTROL_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_runtime_control_scenarios.json"
)
RUNTIME_CONTROL_NEGATIVE_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_runtime_control_negative_scenarios.json"
)
RUNTIME_CONTROL_DEEPER_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_runtime_control_deeper_hardening_scenarios.json"
)
MCP_SECURITY_PATH = (
    REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_mcp_security_scenarios.json"
)
MCP_SECURITY_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_mcp_security_hardening_scenarios.json"
)
CONTEXT_EVIDENCE_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_context_evidence_scenarios.json"
)
CONTEXT_EVIDENCE_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_context_evidence_hardening_scenarios.json"
)
CONTEXT_EVIDENCE_DEEPER_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_context_evidence_deeper_hardening_scenarios.json"
)
MEMORY_ADMISSION_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_memory_admission_scenarios.json"
)
MEMORY_ADMISSION_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_memory_admission_hardening_scenarios.json"
)
MEMORY_ADMISSION_DEEPER_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_memory_admission_deeper_hardening_scenarios.json"
)
STATE_DIFF_PATH = (
    REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_state_diff_scenarios.json"
)
STATE_DIFF_HARDENING_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_state_diff_hardening_scenarios.json"
)


class AgentEvalIntegrationTests(unittest.TestCase):
    def test_fixture_loads_and_is_low_sensitive(self) -> None:
        payload = load_eval_suite(FIXTURE_PATH)

        self.assertEqual(payload["suite_id"], "synthetic-agent-first-trio-v1")
        serialized = json.dumps(payload, ensure_ascii=False)
        self.assertNotIn("raw_prompt", serialized)
        self.assertNotIn("backend_url", serialized)

    def test_cli_outputs_pass_report_for_first_trio(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(FIXTURE_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 3)
        self.assertEqual(report["failed_count"], 0)
        self.assertEqual(report["failure_distribution"], {"PASS": 3})
        for case_result in report["results"]:
            self.assertFalse(case_result["replay_bundle"]["raw_payload_returned"])

    def test_cli_outputs_pass_report_for_core_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(CORE_SCENARIOS_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertGreaterEqual(report["case_count"], 10)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("eval_run", report)
        self.assertIn("manual-core-scenario-fixture-v1", report["eval_run"]["adapter_versions"])

    def test_cli_outputs_pass_report_for_runtime_control_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(RUNTIME_CONTROL_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 3)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("runtime_event_score", report["aggregate_scores"])
        self.assertIn("checkpoint_refs", report["results"][0]["replay_bundle"])
        self.assertTrue(report["results"][1]["replay_bundle"]["workflow_decision_refs"])

    def test_cli_outputs_pass_report_for_runtime_control_negative_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(RUNTIME_CONTROL_NEGATIVE_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 3)
        self.assertEqual(report["failed_count"], 0)
        self.assertEqual(report["failure_distribution"], {"PASS": 3})
        self.assertIn("expected_failure_match", report["aggregate_scores"])
        self.assertIn("checkpoint_score", report["aggregate_scores"])
        self.assertIn("cancel_score", report["aggregate_scores"])
        self.assertIn("runtime_event_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_runtime_control_deeper_hardening_scenarios(
        self,
    ) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(RUNTIME_CONTROL_DEEPER_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 3)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("checkpoint_version_score", report["aggregate_scores"])
        self.assertIn("workflow_wakeup_score", report["aggregate_scores"])
        self.assertIn("replay_lineage_score", report["aggregate_scores"])
        self.assertEqual(
            report["results"][2]["replay_bundle"]["lineage_refs"],
            [
                "lineage:context-package:runtime-lineage",
                "lineage:model-candidate:runtime-lineage",
                "lineage:checkpoint:replay-lineage",
                "lineage:workflow-decision:runtime-lineage",
                "lineage:audit:runtime-lineage",
            ],
        )

    def test_cli_outputs_pass_report_for_mcp_security_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(MCP_SECURITY_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 4)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("mcp_provenance_score", report["aggregate_scores"])
        self.assertIn("tool_description_poisoning_score", report["aggregate_scores"])
        self.assertIn("tool_output_instruction_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_mcp_security_hardening_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(MCP_SECURITY_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 4)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("tool_argument_schema_score", report["aggregate_scores"])
        self.assertIn("tool_prepare_expiry_score", report["aggregate_scores"])
        self.assertIn("tool_selection_attack_score", report["aggregate_scores"])
        self.assertIn("mcp_provider_selection_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_context_evidence_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(CONTEXT_EVIDENCE_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 4)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("source_coverage_score", report["aggregate_scores"])
        self.assertIn("conflict_detection_score", report["aggregate_scores"])
        self.assertIn("temporal_version_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_context_evidence_hardening_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(CONTEXT_EVIDENCE_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 4)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("memory_source_precedence_score", report["aggregate_scores"])
        self.assertIn("unsafe_context_quarantine_score", report["aggregate_scores"])
        self.assertIn("context_budget_truncation_score", report["aggregate_scores"])
        self.assertIn("retrieval_lane_gap_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_context_evidence_deeper_hardening_scenarios(
        self,
    ) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(CONTEXT_EVIDENCE_DEEPER_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 5)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("source_ranking_score", report["aggregate_scores"])
        self.assertIn("retrieval_lane_redrive_score", report["aggregate_scores"])
        self.assertIn("snippet_citation_repair_score", report["aggregate_scores"])
        self.assertIn("denied_retrieval_lane_score", report["aggregate_scores"])
        self.assertIn("context_taint_propagation_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_memory_admission_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(MEMORY_ADMISSION_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 6)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("memory_source_score", report["aggregate_scores"])
        self.assertIn("memory_speaker_score", report["aggregate_scores"])
        self.assertIn("memory_audience_score", report["aggregate_scores"])
        self.assertIn("memory_supersedes_score", report["aggregate_scores"])
        self.assertIn("memory_overgeneralization_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_memory_admission_hardening_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(MEMORY_ADMISSION_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 5)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("memory_dedupe_score", report["aggregate_scores"])
        self.assertIn("memory_low_confidence_score", report["aggregate_scores"])
        self.assertIn("memory_skill_bound_score", report["aggregate_scores"])
        self.assertIn("memory_policy_source_score", report["aggregate_scores"])
        self.assertIn("memory_review_timeout_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_memory_admission_deeper_hardening_scenarios(
        self,
    ) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(MEMORY_ADMISSION_DEEPER_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 5)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("memory_duplicate_cluster_score", report["aggregate_scores"])
        self.assertIn("memory_confidence_calibration_score", report["aggregate_scores"])
        self.assertIn("memory_procedural_migration_score", report["aggregate_scores"])
        self.assertIn("memory_policy_source_governance_score", report["aggregate_scores"])
        self.assertIn("memory_review_redrive_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_state_diff_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(STATE_DIFF_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 5)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("state_diff_score", report["aggregate_scores"])
        self.assertIn("state_report_completeness_score", report["aggregate_scores"])
        self.assertIn("state_execution_ref_score", report["aggregate_scores"])
        self.assertIn("state_unauthorized_mutation_score", report["aggregate_scores"])

    def test_cli_outputs_pass_report_for_state_diff_hardening_scenarios(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_eval_fixture.py",
                str(STATE_DIFF_HARDENING_PATH),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "PASS")
        self.assertEqual(report["case_count"], 4)
        self.assertEqual(report["failed_count"], 0)
        self.assertIn("state_repair_ref_score", report["aggregate_scores"])
        self.assertIn("state_redrive_ref_score", report["aggregate_scores"])
        self.assertIn("state_partial_execution_score", report["aggregate_scores"])
        self.assertIn("state_idempotency_score", report["aggregate_scores"])
        self.assertIn("state_compensating_action_score", report["aggregate_scores"])


if __name__ == "__main__":
    unittest.main()
