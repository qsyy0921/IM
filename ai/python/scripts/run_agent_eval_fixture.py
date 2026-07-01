from __future__ import annotations

import json
import sys
from pathlib import Path

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.evaluator import eval_report_to_payload, run_eval_suite  # noqa: E402
from nexusim_ai_eval.fixtures import load_eval_suite  # noqa: E402


def main() -> int:
    if len(sys.argv) != 2:
        print(
            json.dumps(
                {
                    "schema_version": 1,
                    "status": "FAILED",
                    "error_class": "MALFORMED_INPUT",
                    "message": "expected one eval suite JSON path",
                },
                ensure_ascii=True,
                sort_keys=True,
            )
        )
        return 2

    try:
        payload = load_eval_suite(Path(sys.argv[1]))
        report = run_eval_suite(payload)
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

    print(json.dumps(eval_report_to_payload(report), ensure_ascii=True, sort_keys=True))
    return 0 if report.status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
