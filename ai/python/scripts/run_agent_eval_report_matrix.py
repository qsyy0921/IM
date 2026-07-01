from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.reporting import (  # noqa: E402
    ensure_not_same_path,
    load_report_matrix_plan,
    run_report_matrix_plan,
    write_json_artifact,
)


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Generate low-sensitive multi-suite Agent EvalReport matrix and "
            "baseline refresh approval manifest artifacts."
        ),
    )
    parser.add_argument("plan_path", help="Path to report matrix plan JSON")
    parser.add_argument("--matrix-out", required=True, help="Output path for report matrix")
    parser.add_argument(
        "--approval-manifest-out",
        required=True,
        help="Output path for baseline refresh approval manifest",
    )
    parser.add_argument(
        "--base-dir",
        default=".",
        help="Base directory for relative paths in the matrix plan",
    )
    parser.add_argument(
        "--score-tolerance",
        type=float,
        default=0.0,
        help="Allowed numeric score decrease before reporting regression",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Allow overwriting report, review, matrix and manifest artifacts",
    )
    args = parser.parse_args()

    plan_path = Path(args.plan_path)
    matrix_out = Path(args.matrix_out)
    approval_manifest_out = Path(args.approval_manifest_out)
    base_dir = Path(args.base_dir)

    try:
        ensure_not_same_path(matrix_out, approval_manifest_out, "matrix-out", "approval-manifest-out")
        plan_payload = load_report_matrix_plan(plan_path)
        matrix_payload, approval_manifest = run_report_matrix_plan(
            plan_payload,
            base_dir=base_dir,
            score_tolerance=args.score_tolerance,
            force=args.force,
        )
        write_json_artifact(matrix_out, matrix_payload, force=args.force)
        write_json_artifact(approval_manifest_out, approval_manifest, force=args.force)
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

    response = {
        "schema_version": 1,
        "status": "PASS"
        if matrix_payload["status"] == "PASS" and approval_manifest["status"] == "PASS"
        else "FAIL",
        "matrix": matrix_payload,
        "approval_manifest": approval_manifest,
    }
    print(json.dumps(response, ensure_ascii=True, sort_keys=True))
    return 0 if response["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
