"""Fixture loading for the offline Agent eval harness."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from nexusim_ai_eval.contracts import validate_eval_suite


def load_eval_suite(path: Path) -> dict[str, Any]:
    """Load a UTF-8 JSON eval suite and validate its low-sensitive structure."""

    try:
        payload: Any = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("failed to load eval suite") from exc
    if not isinstance(payload, dict):
        raise ValueError("eval suite must be an object")
    validate_eval_suite(payload)
    return payload
