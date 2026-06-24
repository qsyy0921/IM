from __future__ import annotations

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
CONTRACT_PATH = ROOT / "contracts" / "worker-candidate.schema.json"

from nexusim_ai_common.contracts import validate_worker_candidate  # noqa: E402


def main() -> int:
    document = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    if document.get("schema_version") != 1:
        raise SystemExit("worker candidate contract schema_version must be 1")
    if "low-sensitive" not in document.get("scope", ""):
        raise SystemExit("worker candidate contract scope must state low-sensitive")

    candidate = {
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
        "confidence": 0.7,
    }
    validate_worker_candidate(candidate)
    print(f"OK   Python AI worker contracts validated: {CONTRACT_PATH}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
