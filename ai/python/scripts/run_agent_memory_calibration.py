from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.memory_calibration import (  # noqa: E402
    load_memory_calibration_payload,
    run_memory_admission_calibration,
)
from nexusim_ai_eval.reporting import ensure_not_same_path, write_json_artifact  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Run offline memory admission calibration over low-sensitive "
            "public-dataset-style samples."
        ),
    )
    parser.add_argument("payload_path", help="Path to memory calibration sample JSON")
    parser.add_argument("--report-out", help="Optional output path for calibration report")
    parser.add_argument(
        "--force",
        action="store_true",
        help="Allow overwriting the output report artifact",
    )
    args = parser.parse_args()

    payload_path = Path(args.payload_path)
    report_out = Path(args.report_out) if args.report_out else None

    try:
        if report_out is not None:
            ensure_not_same_path(report_out, payload_path, "report-out", "payload_path")
        payload = load_memory_calibration_payload(payload_path)
        report = run_memory_admission_calibration(payload)
        if report_out is not None:
            write_json_artifact(report_out, report, force=args.force)
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

    print(json.dumps(report, ensure_ascii=True, sort_keys=True))
    return 0 if report["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
