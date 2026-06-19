from __future__ import annotations

import unittest

from nexusim_ai_common.contracts import validate_worker_candidate


def valid_candidate() -> dict[str, object]:
    return {
        "schema_version": 1,
        "task_id": "task_01",
        "candidate_id": "cand_01",
        "worker_kind": "MEMORY_EXTRACTION",
        "status": "CANDIDATE",
        "output_type": "MEMORY_EVENT_CANDIDATE",
        "output_sha256": "a" * 64,
        "source_refs": ["message:tenant:conversation:seq1"],
        "citations": ["message:tenant:conversation:seq1"],
        "safety_flags": ["LOW_SENSITIVE"],
        "confidence": 0.8,
    }


class WorkerCandidateContractTests(unittest.TestCase):
    def test_valid_candidate(self) -> None:
        candidate = validate_worker_candidate(valid_candidate())
        self.assertEqual(candidate.worker_kind, "MEMORY_EXTRACTION")
        self.assertEqual(candidate.status, "CANDIDATE")

    def test_rejects_raw_prompt_field(self) -> None:
        payload = valid_candidate()
        payload["raw_prompt"] = "full prompt"
        with self.assertRaises(ValueError):
            validate_worker_candidate(payload)

    def test_rejects_sensitive_text(self) -> None:
        payload = valid_candidate()
        payload["notes"] = "Bearer secret-token-value"
        with self.assertRaises(ValueError):
            validate_worker_candidate(payload)

    def test_rejects_final_control_fields(self) -> None:
        payload = valid_candidate()
        payload["approval_id"] = "approval_01"
        with self.assertRaises(ValueError):
            validate_worker_candidate(payload)

    def test_rejects_missing_output_hash_for_candidate(self) -> None:
        payload = valid_candidate()
        payload["output_sha256"] = ""
        with self.assertRaises(ValueError):
            validate_worker_candidate(payload)

    def test_rejects_unknown_worker_kind(self) -> None:
        payload = valid_candidate()
        payload["worker_kind"] = "BUSINESS_BACKEND"
        with self.assertRaises(ValueError):
            validate_worker_candidate(payload)


if __name__ == "__main__":
    unittest.main()
