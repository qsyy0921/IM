from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.comparison import compare_eval_reports  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Compare two low-sensitive Agent EvalReport JSON files.",
    )
    parser.add_argument("baseline_report_path", help="Path to baseline EvalReport JSON")
    parser.add_argument("current_report_path", help="Path to current EvalReport JSON")
    parser.add_argument(
        "--score-tolerance",
        type=float,
        default=0.0,
        help="Allowed numeric score decrease before reporting regression",
    )
    args = parser.parse_args()

    try:
        baseline = _load_json(Path(args.baseline_report_path), "baseline report")
        current = _load_json(Path(args.current_report_path), "current report")
        comparison = compare_eval_reports(
            baseline,
            current,
            score_tolerance=args.score_tolerance,
        )
    except ValueError as exc:
        print(
            json.dumps(
                {
                    "schema_version": 1,
                    "status": "FAILED",
                    "error_class": "MALFORMED_INPUT",
                    "message": str(exc),
                },
                ensure_ascii=True,
                sort_keys=True,
            )
        )
        return 2

    print(json.dumps(comparison, ensure_ascii=True, sort_keys=True))
    return 0 if comparison["status"] == "PASS" else 1


def _load_json(path: Path, context: str) -> dict[str, Any]:
    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"failed to load {context}") from exc
    if not isinstance(payload, dict):
        raise ValueError(f"{context} must be an object")
    return payload


if __name__ == "__main__":
    raise SystemExit(main())
