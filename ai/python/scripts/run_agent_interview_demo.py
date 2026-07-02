from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.interview_demo import (  # noqa: E402
    OutputPaths,
    interview_demo_result_to_payload,
    load_interview_demo_fixture,
    run_interview_demo,
    write_interview_demo_outputs,
)
from nexusim_ai_eval.reporting import ensure_not_same_path  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Run the backend-isolated NexusIM Agent interview demo from a "
            "synthetic ref-only fixture."
        ),
    )
    parser.add_argument("fixture_path", help="Path to interview Agent demo fixture JSON")
    parser.add_argument("--summary-out", help="Optional output path for summary JSON")
    parser.add_argument("--report-out", help="Optional output path for EvalReport JSON")
    parser.add_argument("--replay-out", help="Optional output path for ReplayBundle JSON")
    parser.add_argument(
        "--force",
        action="store_true",
        help="Allow overwriting existing output artifacts",
    )
    args = parser.parse_args()

    fixture_path = Path(args.fixture_path)
    paths = OutputPaths(
        summary_out=Path(args.summary_out) if args.summary_out else None,
        report_out=Path(args.report_out) if args.report_out else None,
        replay_out=Path(args.replay_out) if args.replay_out else None,
    )

    try:
        _ensure_outputs_do_not_overwrite_fixture(fixture_path, paths)
        fixture = load_interview_demo_fixture(fixture_path)
        result = run_interview_demo(fixture)
        write_interview_demo_outputs(result, paths, force=args.force)
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

    summary = interview_demo_result_to_payload(result)
    print(json.dumps(summary, ensure_ascii=True, sort_keys=True))
    return 0 if result.status == "PASS" else 1


def _ensure_outputs_do_not_overwrite_fixture(
    fixture_path: Path,
    paths: OutputPaths,
) -> None:
    for label, path in (
        ("summary-out", paths.summary_out),
        ("report-out", paths.report_out),
        ("replay-out", paths.replay_out),
    ):
        if path is not None:
            ensure_not_same_path(path, fixture_path, label, "fixture_path")


if __name__ == "__main__":
    raise SystemExit(main())
