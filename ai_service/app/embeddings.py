"""Embedding clients (Gemini or OpenAI), normalised to a fixed dimension."""

from __future__ import annotations

import logging
from typing import List, Optional

from .errors import NotConfiguredError, ProviderError
from .http_json import error_message, post_json

log = logging.getLogger("ai.embeddings")

GEMINI_BASE_URL = "https://generativelanguage.googleapis.com/v1beta"
OPENAI_URL = "https://api.openai.com/v1/embeddings"

# Providers cap input length; ~8k characters stays safely under both limits.
MAX_EMBED_CHARS = 8000


class EmbeddingClient:
    def __init__(
        self,
        provider: str,
        api_key: str,
        model: str,
        dimension: int = 1536,
        timeout: float = 45.0,
        gemini_base_url: str = GEMINI_BASE_URL,
        openai_url: str = OPENAI_URL,
    ):
        api_key = (api_key or "").strip()
        if not api_key:
            raise NotConfiguredError("an embedding API key is required")
        provider = (provider or "gemini").strip().lower()
        if provider not in ("gemini", "openai"):
            raise NotConfiguredError(f"unsupported embedding provider: {provider!r}")
        self.provider = provider
        self.api_key = api_key
        self.model = (model or "").strip() or (
            "text-embedding-3-small" if provider == "openai" else "gemini-embedding-001"
        )
        self.dimension = dimension
        self.timeout = timeout
        self.gemini_base_url = gemini_base_url.rstrip("/")
        self.openai_url = openai_url

    async def generate(self, text: str) -> List[float]:
        text = (text or "").strip()
        if not text:
            raise ProviderError("embedding input text is empty")
        text = text[:MAX_EMBED_CHARS]

        if self.provider == "openai":
            values = await self._openai(text)
        else:
            values = await self._gemini(text)
        if not values:
            raise ProviderError("embedding API returned no values")
        return self._fit(values)

    def _fit(self, values: List[float]) -> List[float]:
        """Truncate or zero-pad to the dimension the ``vector(N)`` column expects."""
        vec = [float(v) for v in values]
        if self.dimension > 0:
            if len(vec) > self.dimension:
                vec = vec[: self.dimension]
            elif len(vec) < self.dimension:
                vec = vec + [0.0] * (self.dimension - len(vec))
        return vec

    async def _gemini(self, text: str) -> List[float]:
        url = f"{self.gemini_base_url}/models/{self.model}:embedContent"
        payload = {
            "model": f"models/{self.model}",
            "content": {"parts": [{"text": text}]},
        }
        if self.dimension > 0:
            payload["outputDimensionality"] = self.dimension
        status, body = await self._post(url, payload, {"x-goog-api-key": self.api_key})
        if status != 200:
            raise ProviderError(
                f"gemini embeddings API error: {error_message(body, f'status {status}')}",
                status=status,
            )
        if isinstance(body, dict):
            embeddings = body.get("embeddings")
            if embeddings:
                return embeddings[0].get("values") or []
            return (body.get("embedding") or {}).get("values") or []
        raise ProviderError("decode gemini embeddings response: unexpected body")

    async def _openai(self, text: str) -> List[float]:
        payload = {"input": text, "model": self.model, "encoding_format": "float"}
        if self.dimension > 0:
            payload["dimensions"] = self.dimension
        status, body = await self._post(
            self.openai_url, payload, {"Authorization": f"Bearer {self.api_key}"}
        )
        if status != 200:
            raise ProviderError(
                f"openai embeddings API error: {error_message(body, f'status {status}')}",
                status=status,
            )
        data = (body or {}).get("data") if isinstance(body, dict) else None
        if not data:
            raise ProviderError("openai embeddings API returned no embedding")
        return data[0].get("embedding") or []

    async def _post(self, url: str, payload: dict, headers: dict):
        try:
            return await post_json(url, payload, headers=headers, timeout=self.timeout)
        except OSError as exc:
            raise ProviderError(f"call {self.provider} embeddings API: {exc}") from exc


def build_embedder(
    provider: str, api_key: str, model: str, dimension: int, timeout: float
) -> Optional[EmbeddingClient]:
    if not (api_key or "").strip():
        return None
    return EmbeddingClient(provider, api_key, model, dimension, timeout)


def to_vector_literal(values: List[float]) -> str:
    """Format a vector the way pgvector parses it: ``[0.1,0.2,...]``."""
    return "[" + ",".join(repr(float(v)) for v in values) + "]"
