"""Low-sensitive safety checks for Python AI worker candidate payloads."""

from __future__ import annotations

import re
from typing import Any


SENSITIVE_TEXT_PATTERNS = [
    re.compile(r"sk-[A-Za-z0-9_-]{12,}"),
    re.compile(r"Bearer\s+[A-Za-z0-9._-]{12,}", re.IGNORECASE),
    re.compile(r"-----BEGIN\s+(RSA\s+)?PRIVATE KEY-----"),
    re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}"),
    re.compile(r"\b1[3-9]\d{9}\b"),
]

SENSITIVE_KEY_FRAGMENTS = {
    "authorization",
    "cookie",
    "credential",
    "email",
    "id_token",
    "password",
    "phone",
    "private_key",
    "provider_body",
    "prompt",
    "refresh_token",
    "secret",
    "session",
    "token",
}


def assert_low_sensitive_value(value: Any, context: str) -> None:
    """Reject obvious secrets, PII and raw prompt/provider fields."""

    if isinstance(value, dict):
        for key, nested in value.items():
            normalized_key = str(key).strip().lower().replace("-", "_")
            if any(fragment in normalized_key for fragment in SENSITIVE_KEY_FRAGMENTS):
                raise ValueError(f"sensitive-looking key in {context}: {key}")
            assert_low_sensitive_value(nested, f"{context}.{key}")
        return

    if isinstance(value, list):
        for index, nested in enumerate(value):
            assert_low_sensitive_value(nested, f"{context}[{index}]")
        return

    if isinstance(value, str):
        for pattern in SENSITIVE_TEXT_PATTERNS:
            if pattern.search(value):
                raise ValueError(f"sensitive-looking text in {context}")
