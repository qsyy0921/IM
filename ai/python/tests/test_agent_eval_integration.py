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
MCP_SECURITY_PATH = (
    REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_mcp_security_scenarios.json"
)
CONTEXT_EVIDENCE_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_context_evidence_scenarios.json"
)
MEMORY_ADMISSION_PATH = (
    REPO_ROOT
    / "ai"
    / "python"
    / "fixtures"
    / "agent_eval"
    / "synthetic_memory_admission_scenarios.json"
)
STATE_DIFF_PATH = (
    REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "synthetic_state_diff_scenarios.json"
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


if __name__ == "__main__":
    unittest.main()
