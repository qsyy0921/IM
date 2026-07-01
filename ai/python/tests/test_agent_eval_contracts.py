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
                "conflict_detected": True,
                "stale_evidence_used": False,
                "permission_abstain_required": False,
            }
        ]

        cases = validate_eval_suite(payload)

        self.assertEqual(cases[0].capability_family, "CONTEXT_EVIDENCE")
        self.assertEqual(cases[0].expected_source_coverage_refs, ["evidence:current"])
        self.assertTrue(cases[0].conflict_detected)
        self.assertFalse(cases[0].stale_evidence_used)


if __name__ == "__main__":
    unittest.main()
