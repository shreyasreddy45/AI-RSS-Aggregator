"""A tiny async JSON-over-HTTP helper built on the standard library.

Provider calls are small, infrequent POSTs, so ``urllib`` in a worker thread is
plenty and keeps the service free of extra runtime dependencies.
"""

from __future__ import annotations

import asyncio
import json
import urllib.error
import urllib.request
from typing import Any, Mapping, Optional, Tuple


def _post_json_sync(
    url: str, payload: Mapping[str, Any], headers: Mapping[str, str], timeout: float
) -> Tuple[int, Any]:
    data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url, data=data, method="POST")
    request.add_header("Content-Type", "application/json")
    for key, value in headers.items():
        request.add_header(key, value)

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, _decode(response.read())
    except urllib.error.HTTPError as exc:
        # Non-2xx responses still carry a useful JSON error body.
        try:
            body = exc.read()
        finally:
            exc.close()
        return exc.code, _decode(body)


def _decode(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return raw.decode("utf-8", errors="replace")


async def post_json(
    url: str,
    payload: Mapping[str, Any],
    *,
    headers: Optional[Mapping[str, str]] = None,
    timeout: float = 30.0,
) -> Tuple[int, Any]:
    """POST ``payload`` as JSON and return ``(status_code, decoded_body)``.

    Network-level failures (DNS, refused connection, timeout) raise ``OSError``.
    """
    try:
        return await asyncio.to_thread(_post_json_sync, url, payload, dict(headers or {}), timeout)
    except urllib.error.URLError as exc:  # wraps socket errors / timeouts
        raise OSError(str(exc.reason)) from exc


def error_message(body: Any, fallback: str) -> str:
    """Pull ``error.message`` out of a provider error body when present."""
    if isinstance(body, dict):
        err = body.get("error")
        if isinstance(err, dict) and err.get("message"):
            return str(err["message"])
        if isinstance(err, str) and err:
            return err
    if isinstance(body, str) and body.strip():
        return body.strip()[:300]
    return fallback


__all__ = ["post_json", "error_message"]
