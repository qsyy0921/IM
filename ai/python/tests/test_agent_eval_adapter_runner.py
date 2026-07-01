from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from nexusim_ai_eval.adapter_runner import convert_adapter_payload, run_adapter_payload


REPO_ROOT = Path(__file__).resolve().parents[3]
SAMPLES_DIR = REPO_ROOT / "ai" / "python" / "fixtures" / "agent_eval" / "adapter_samples"


class AgentEvalAdapterRunnerTests(unittest.TestCase):
    def test_converts_qasper_like_payload_to_eval_suite(self) -> None:
        payload = json.loads((SAMPLES_DIR / "qasper_like_rag_samples.json").read_text())

        suite = convert_adapter_payload(payload)

        self.assertEqual(suite["suite_id"], "adapter-qasper-like-rag-sample-v1")
        self.assertEqual(suite["fixture_kind"], "synthetic_im_like")
        self.assertEqual(suite["adapter_versions"], ["qasper-like-rag-adapter-v1"])
        self.assertEqual(len(suite["cases"]), 2)

    def test_runs_memory_sample_with_expected_negative_detection(self) -> None:
        payload = json.loads((SAMPLES_DIR / "statebench_like_memory_samples.json").read_text())

        report = run_adapter_payload(payload)

        self.assertEqual(report.status, "PASS")
        self.assertEqual(report.case_count, 2)
        self.assertEqual(report.failure_distribution, {"PASS": 2})
        self.assertEqual(report.eval_run.adapter_versions, ["statebench-like-memory-adapter-v1"])

    def test_rejects_backend_fields_before_conversion(self) -> None:
        payload = json.loads((SAMPLES_DIR / "qasper_like_rag_samples.json").read_text())
        payload["backend_url"] = "http://localhost:8080"

        with self.assertRaisesRegex(ValueError, "forbidden eval field"):
            convert_adapter_payload(payload)

    def test_cli_converts_sample_payload(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                "ai/python/scripts/run_agent_dataset_adapter.py",
                str(SAMPLES_DIR / "qasper_like_rag_samples.json"),
            ],
            check=True,
            capture_output=True,
            cwd=REPO_ROOT,
            text=True,
        )

        suite = json.loads(result.stdout)
        self.assertEqual(suite["suite_id"], "adapter-qasper-like-rag-sample-v1")
        self.assertEqual(suite["cases"][0]["capability_family"], "GROUNDED_RAG")

    def test_cli_runs_all_sample_payloads(self) -> None:
        for sample_name in [
            "qasper_like_rag_samples.json",
            "toolsandbox_like_tool_samples.json",
            "statebench_like_memory_samples.json",
        ]:
            with self.subTest(sample_name=sample_name):
                result = subprocess.run(
                    [
                        sys.executable,
                        "ai/python/scripts/run_agent_dataset_adapter.py",
                        "--run",
                        str(SAMPLES_DIR / sample_name),
                    ],
                    check=True,
                    capture_output=True,
                    cwd=REPO_ROOT,
                    text=True,
                )
                report = json.loads(result.stdout)
                self.assertEqual(report["status"], "PASS")
                self.assertEqual(report["failed_count"], 0)

    def test_cli_rejects_malformed_sample(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            sample_path = Path(temp_dir) / "bad-sample.json"
            sample_path.write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "adapter_name": "qasper_like_rag",
                        "suite_id": "bad-sample",
                        "backend_url": "http://localhost:8080",
                        "cases": [],
                    }
                ),
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    "ai/python/scripts/run_agent_dataset_adapter.py",
                    str(sample_path),
                ],
                capture_output=True,
                cwd=REPO_ROOT,
                text=True,
            )

        self.assertEqual(result.returncode, 2)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "FAILED")


if __name__ == "__main__":
    unittest.main()
