from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from nexusim_ai_memory.extractor import run_memory_extraction  # noqa: E402


def main() -> int:
    if len(sys.argv) != 2:
        result, exit_code = run_memory_extraction({})
        print(json.dumps(result, ensure_ascii=True, sort_keys=True))
        return exit_code

    try:
        payload: Any = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError):
        result, exit_code = run_memory_extraction({})
        print(json.dumps(result, ensure_ascii=True, sort_keys=True))
        return exit_code

    result, exit_code = run_memory_extraction(payload)
    print(json.dumps(result, ensure_ascii=True, sort_keys=True))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
