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


if __name__ == "__main__":
    unittest.main()
