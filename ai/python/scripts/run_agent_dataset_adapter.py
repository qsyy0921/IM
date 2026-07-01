from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_eval.adapter_runner import convert_adapter_payload, run_adapter_payload  # noqa: E402
from nexusim_ai_eval.evaluator import eval_report_to_payload  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Convert a local low-sensitive dataset adapter sample to EvalSuite JSON.",
    )
    parser.add_argument("payload_path", help="Path to adapter sample JSON")
    parser.add_argument(
        "--run",
        action="store_true",
        help="Run the converted EvalSuite and emit EvalReport JSON",
    )
    args = parser.parse_args()

    try:
        payload = _load_json(Path(args.payload_path))
        if args.run:
            report = run_adapter_payload(payload)
            print(json.dumps(eval_report_to_payload(report), ensure_ascii=True, sort_keys=True))
            return 0 if report.status == "PASS" else 1
        suite = convert_adapter_payload(payload)
        print(json.dumps(suite, ensure_ascii=True, sort_keys=True))
        return 0
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


def _load_json(path: Path) -> dict[str, Any]:
    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load adapter sample") from exc
    if not isinstance(payload, dict):
        raise ValueError("adapter sample must be an object")
    return payload


if __name__ == "__main__":
    raise SystemExit(main())
