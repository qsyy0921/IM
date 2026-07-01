from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.reporting import (  # noqa: E402
    build_baseline_refresh_review,
    ensure_not_same_path,
    generate_current_report_payload,
    load_report_payload,
    write_json_artifact,
)


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Generate a low-sensitive current Agent EvalReport and optional "
            "baseline refresh review."
        ),
    )
    parser.add_argument("fixture_path", help="Path to synthetic EvalSuite JSON")
    parser.add_argument("--report-out", required=True, help="Output path for current EvalReport")
    parser.add_argument("--baseline", help="Optional baseline EvalReport JSON path")
    parser.add_argument("--review-out", help="Optional output path for baseline refresh review")
    parser.add_argument(
        "--score-tolerance",
        type=float,
        default=0.0,
        help="Allowed numeric score decrease before reporting regression",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Allow overwriting report/review output artifacts",
    )
    args = parser.parse_args()

    fixture_path = Path(args.fixture_path)
    report_out = Path(args.report_out)
    baseline_path = Path(args.baseline) if args.baseline else None
    review_out = Path(args.review_out) if args.review_out else None

    try:
        if baseline_path is not None:
            ensure_not_same_path(report_out, baseline_path, "report-out", "baseline")
            if review_out is not None:
                ensure_not_same_path(review_out, baseline_path, "review-out", "baseline")
        if review_out is not None:
            ensure_not_same_path(report_out, review_out, "report-out", "review-out")

        current_report = generate_current_report_payload(fixture_path)
        baseline_report = (
            load_report_payload(baseline_path, "baseline report") if baseline_path else None
        )
        review = build_baseline_refresh_review(
            current_report_payload=current_report,
            current_report_path=report_out,
            baseline_report_payload=baseline_report,
            baseline_report_path=baseline_path,
            score_tolerance=args.score_tolerance,
        )
        write_json_artifact(report_out, current_report, force=args.force)
        if review_out is not None:
            write_json_artifact(review_out, review, force=args.force)
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

    print(json.dumps(review, ensure_ascii=True, sort_keys=True))
    return 0 if review["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
