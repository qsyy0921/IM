from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from nexusim_ai_memory.extractor import run_memory_extraction


def valid_batch() -> dict[str, object]:
    return {
        "schema_version": 1,
        "task_id": "memory-extract-task-01",
        "tenant_id": "tenant-a",
        "conversation_id": "conv-alpha",
        "messages": [
            {
                "message_id": "msg-1",
                "conversation_seq": 7,
                "speaker_id": "user-a",
                "text": "decision: ship source-backed group memory\nordinary follow up",
            },
            {
                "message_id": "msg-2",
                "conversation_seq": 8,
                "speaker_id": "user-b",
                "text": "profile_signal: user-a coordinates cross-group launches",
            },
            {
                "message_id": "msg-3",
                "conversation_seq": 9,
                "speaker_id": "user-c",
                "text": "this is ordinary chat and must not become memory",
            },
        ],
    }


class MemoryExtractionCandidateTests(unittest.TestCase):
    def test_extracts_explicit_cues_without_raw_text(self) -> None:
        result, exit_code = run_memory_extraction(valid_batch())

        self.assertEqual(exit_code, 0)
        self.assertEqual(result["status"], "COMPLETED")
        self.assertEqual(result["candidate_count"], 2)
        self.assertEqual(result["ordinary_message_count"], 1)
        candidates = result["candidates"]
        self.assertEqual(candidates[0]["memory_event_type"], "DECISION")
        self.assertEqual(candidates[0]["source_refs"], ["message:tenant-a:conv-alpha:7"])
        self.assertEqual(candidates[0]["worker_kind"], "MEMORY_EXTRACTION")
        self.assertEqual(candidates[0]["output_type"], "MEMORY_EVENT_CANDIDATE")
        self.assertEqual(len(candidates[0]["output_sha256"]), 64)
        serialized = json.dumps(result, ensure_ascii=False)
        self.assertNotIn("ship source-backed group memory", serialized)
        self.assertNotIn("ordinary follow up", serialized)
        self.assertFalse(result["report"]["raw_text_returned"])
        self.assertFalse(result["report"]["final_memory_persisted"])
        self.assertTrue(result["report"]["requires_go_validation"])

    def test_profile_signal_requires_review(self) -> None:
        result, exit_code = run_memory_extraction(valid_batch())

        self.assertEqual(exit_code, 0)
        profile_candidate = result["candidates"][1]
        self.assertEqual(profile_candidate["memory_event_type"], "PROFILE_SIGNAL")
        self.assertEqual(profile_candidate["review_state"], "NEEDS_REVIEW")
        self.assertIn("GROUP_SCOPE_PROFILE_SIGNAL", profile_candidate["safety_flags"])
        self.assertIn("NEEDS_REVIEW", profile_candidate["safety_flags"])

    def test_ordinary_chat_returns_zero_candidates(self) -> None:
        payload = valid_batch()
        payload["messages"] = [
            {
                "message_id": "msg-ordinary",
                "conversation_seq": 1,
                "speaker_id": "user-a",
                "text": "hello everyone, this should stay ordinary chat",
            }
        ]

        result, exit_code = run_memory_extraction(payload)

        self.assertEqual(exit_code, 0)
        self.assertEqual(result["candidate_count"], 0)
        self.assertEqual(result["ordinary_message_count"], 1)
        self.assertEqual(result["candidates"], [])

    def test_rejects_sensitive_input(self) -> None:
        payload = valid_batch()
        payload["messages"] = [
            {
                "message_id": "msg-secret",
                "conversation_seq": 1,
                "speaker_id": "user-a",
                "text": "decision: use Bearer secret-token-value",
            }
        ]

        result, exit_code = run_memory_extraction(payload)

        self.assertEqual(exit_code, 2)
        self.assertEqual(result["status"], "FAILED")
        self.assertEqual(result["error_class"], "UNSAFE_INPUT")
        self.assertEqual(result["candidate_count"], 0)

    def test_sensitive_failed_task_id_is_not_echoed(self) -> None:
        payload = valid_batch()
        payload["task_id"] = "Bearer secret-token-value"

        result, exit_code = run_memory_extraction(payload)

        self.assertEqual(exit_code, 2)
        self.assertEqual(result["status"], "FAILED")
        self.assertEqual(result["task_id"], "unknown_task")

    def test_rejects_malformed_batch(self) -> None:
        payload = valid_batch()
        del payload["conversation_id"]

        result, exit_code = run_memory_extraction(payload)

        self.assertEqual(exit_code, 2)
        self.assertEqual(result["status"], "FAILED")
        self.assertEqual(result["error_class"], "MALFORMED_INPUT")

    def test_cli_outputs_low_sensitive_result(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            request_path = Path(temp_dir) / "memory-request.json"
            request_path.write_text(json.dumps(valid_batch()), encoding="utf-8")

            result = subprocess.run(
                [
                    sys.executable,
                    "ai/python/scripts/run_memory_extraction_candidate.py",
                    str(request_path),
                ],
                check=True,
                capture_output=True,
                cwd=Path(__file__).resolve().parents[3],
                text=True,
            )

        output = json.loads(result.stdout)
        self.assertEqual(output["status"], "COMPLETED")
        self.assertEqual(output["candidate_count"], 2)
        self.assertNotIn("source-backed group memory", result.stdout)


if __name__ == "__main__":
    unittest.main()
