from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from nexusim_ai_common.worker import run_worker_request


def valid_request() -> dict[str, object]:
    return {
        "task_id": "task_01",
        "candidate_id": "cand_01",
        "worker_kind": "MEMORY_EXTRACTION",
        "output_type": "MEMORY_EVENT_CANDIDATE",
        "candidate_text": "decision: ship scoped memory projection with source refs",
        "source_refs": ["message:tenant:conversation:seq1"],
        "citations": ["message:tenant:conversation:seq1"],
        "confidence": 0.7,
    }


class WorkerRuntimeTests(unittest.TestCase):
    def test_valid_request_returns_hash_only_candidate(self) -> None:
        candidate, exit_code = run_worker_request(valid_request())

        self.assertEqual(exit_code, 0)
        self.assertEqual(candidate["status"], "CANDIDATE")
        self.assertEqual(candidate["worker_kind"], "MEMORY_EXTRACTION")
        self.assertEqual(len(candidate["output_sha256"]), 64)
        self.assertNotIn("candidate_text", candidate)

    def test_malformed_request_returns_failed_candidate(self) -> None:
        payload = valid_request()
        del payload["candidate_text"]

        candidate, exit_code = run_worker_request(payload)

        self.assertEqual(exit_code, 2)
        self.assertEqual(candidate["status"], "FAILED")
        self.assertEqual(candidate["error_class"], "MALFORMED_INPUT")

    def test_unsafe_request_returns_failed_candidate(self) -> None:
        payload = valid_request()
        payload["candidate_text"] = "Bearer secret-token-value"

        candidate, exit_code = run_worker_request(payload)

        self.assertEqual(exit_code, 2)
        self.assertEqual(candidate["status"], "FAILED")
        self.assertEqual(candidate["error_class"], "UNSAFE_INPUT")
        self.assertEqual(candidate["output_sha256"], "")

    def test_cli_smoke_outputs_json_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            request_path = Path(temp_dir) / "request.json"
            request_path.write_text(json.dumps(valid_request()), encoding="utf-8")

            result = subprocess.run(
                [
                    sys.executable,
                    "ai/python/scripts/run_candidate_worker.py",
                    str(request_path),
                ],
                check=True,
                capture_output=True,
                cwd=Path(__file__).resolve().parents[3],
                text=True,
            )

        candidate = json.loads(result.stdout)
        self.assertEqual(candidate["status"], "CANDIDATE")
        self.assertNotIn("candidate_text", candidate)


if __name__ == "__main__":
    unittest.main()
