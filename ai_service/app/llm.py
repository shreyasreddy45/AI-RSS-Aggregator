"""Gemini text generation client."""

from __future__ import annotations

import asyncio
import logging
from typing import Optional

from .errors import NotConfiguredError, ProviderError
from .http_json import error_message, post_json

log = logging.getLogger("ai.llm")

GEMINI_BASE_URL = "https://generativelanguage.googleapis.com/v1beta"
# An alias that always points at Google's current Flash model.
DEFAULT_MODEL = "gemini-flash-latest"

# Transient server-side failures worth retrying. 429 (quota) is not retried:
# it normally lasts far longer than a short backoff.
_RETRY_STATUSES = frozenset({500, 502, 503, 504})


def build_summary_prompt(article: str) -> str:
    text = (article or "").strip() or "No article text was provided."
    return (
        "Summarize this article in a concise, readable format. Keep the summary short but "
        "informative. Focus on the main idea, key facts, and why it matters. Do not invent "
        f"details.\n\nArticle:\n{text}"
    )


class GeminiClient:
    def __init__(
        self,
        api_key: str,
        model: str,
        timeout: float = 30.0,
        base_url: str = GEMINI_BASE_URL,
        retries: int = 2,
        backoff: float = 1.5,
    ):
        api_key = (api_key or "").strip()
        if not api_key:
            raise NotConfiguredError("GEMINI_API_KEY is required")
        self.api_key = api_key
        self.model = (model or "").strip() or DEFAULT_MODEL
        self.timeout = timeout
        self.base_url = base_url.rstrip("/")
        self.retries = max(0, retries)
        self.backoff = backoff

    async def _pause(self, attempt: int) -> None:
        await asyncio.sleep(self.backoff * (2**attempt))

    async def generate_text(self, prompt: str) -> str:
        prompt = (prompt or "").strip()
        if not prompt:
            raise ProviderError("prompt content is empty")

        url = f"{self.base_url}/models/{self.model}:generateContent"
        payload = {"contents": [{"parts": [{"text": prompt}]}]}
        # The key goes in a header, not the URL, so it never lands in access logs.
        headers = {"x-goog-api-key": self.api_key}

        attempt = 0
        while True:
            try:
                status, body = await post_json(url, payload, headers=headers, timeout=self.timeout)
            except OSError as exc:
                if attempt < self.retries:
                    log.info("Gemini network error, retrying (%s): %s", attempt + 1, exc)
                    await self._pause(attempt)
                    attempt += 1
                    continue
                log.warning("Gemini unreachable model=%s: %s", self.model, exc)
                raise ProviderError(f"call gemini API: {exc}") from exc

            if status == 200:
                return self._extract_text(body)

            message = error_message(body, f"gemini API returned status {status}")
            if status in _RETRY_STATUSES and attempt < self.retries:
                log.info("Gemini returned %s, retrying (%s): %s", status, attempt + 1, message)
                await self._pause(attempt)
                attempt += 1
                continue

            log.warning("Gemini request failed status=%s model=%s: %s", status, self.model, message)
            if status == 404:
                message = (
                    f"model {self.model!r} was not found for this API key. "
                    f"Set GEMINI_MODEL to a valid model such as {DEFAULT_MODEL}. ({message})"
                )
            raise ProviderError(f"gemini API error: {message}", status=status)

    @staticmethod
    def _extract_text(body) -> str:
        if not isinstance(body, dict):
            raise ProviderError("decode gemini response: unexpected body")
        if isinstance(body.get("error"), dict) and body["error"].get("message"):
            raise ProviderError(f"gemini API error: {body['error']['message']}")

        candidates = body.get("candidates") or []
        if not candidates:
            reason = (body.get("promptFeedback") or {}).get("blockReason")
            raise ProviderError(
                f"gemini returned no content (blocked: {reason})" if reason else "gemini returned no content"
            )

        parts = ((candidates[0] or {}).get("content") or {}).get("parts") or []
        # Thinking models may emit reasoning parts flagged ``thought``; skip them.
        texts = [
            part.get("text", "")
            for part in parts
            if isinstance(part, dict) and not part.get("thought")
        ]
        content = "".join(texts).strip()
        if not content:
            raise ProviderError("gemini returned an empty response")
        return content

    async def summarize(self, article: str) -> str:
        article = (article or "").strip()
        if not article:
            raise ProviderError("article content is empty")
        return await self.generate_text(build_summary_prompt(article))


def build_llm(api_key: str, model: str, timeout: float) -> Optional[GeminiClient]:
    if not (api_key or "").strip():
        return None
    return GeminiClient(api_key, model, timeout)
