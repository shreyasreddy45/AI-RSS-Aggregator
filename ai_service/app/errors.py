"""Exception types and LLM-failure classification."""

from __future__ import annotations

from typing import Optional


class ApiError(Exception):
    """An error that maps directly onto an HTTP response: ``{"error": message}``."""

    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status
        self.message = message


class NotConfiguredError(Exception):
    """A required provider (LLM or embeddings) has no API key."""


class ProviderError(Exception):
    """An upstream LLM/embedding provider call failed."""

    def __init__(self, message: str, status: Optional[int] = None):
        super().__init__(message)
        self.status = status


_QUOTA_TOKENS = (
    "quota exceeded",
    "quota",
    "rate limit",
    "rate limited",
    "too many requests",
    "resource exhausted",
    "resource_exhausted",
    "429",
    "exhausted",
)

_UNAVAILABLE_TOKENS = (
    "service unavailable",
    "temporarily unavailable",
    "provider unavailable",
    "upstream unavailable",
    "timeout",
    "timed out",
    "deadline exceeded",
    "503",
)


def is_quota_error(err: BaseException) -> bool:
    if isinstance(err, ProviderError) and err.status == 429:
        return True
    text = str(err).lower()
    return any(token in text for token in _QUOTA_TOKENS)


def should_fallback_on_llm_failure(err: BaseException) -> bool:
    """True when the failure is transient (quota / timeout / outage), so callers
    can still return the retrieved sources instead of a hard error."""
    if is_quota_error(err):
        return True
    if isinstance(err, ProviderError) and err.status in (500, 502, 503, 504):
        return True
    text = str(err).lower()
    return any(token in text for token in _UNAVAILABLE_TOKENS)


def short_error(err: BaseException, limit: int = 300) -> str:
    """One-line, length-bounded error text that is safe to show to users."""
    text = " ".join(str(err).split())
    return text if len(text) <= limit else text[: limit - 1] + "…"
