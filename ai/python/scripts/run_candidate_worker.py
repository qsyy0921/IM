from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

from nexusim_ai_common.worker import build_failed_candidate, run_worker_request


def main() -> int:
    if len(sys.argv) != 2:
        print(
            json.dumps(
                build_failed_candidate({}, "MALFORMED_INPUT"),
                ensure_ascii=True,
                sort_keys=True,
            )
        )
        return 2

    try:
        payload: Any = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError):
        print(
            json.dumps(
                build_failed_candidate({}, "MALFORMED_INPUT"),
                ensure_ascii=True,
                sort_keys=True,
            )
        )
        return 2

    candidate, exit_code = run_worker_request(payload)
    print(json.dumps(candidate, ensure_ascii=True, sort_keys=True))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
