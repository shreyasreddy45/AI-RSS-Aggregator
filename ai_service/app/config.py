"""Runtime configuration, read from environment variables.

The Go API and this service share one .env file, so names and duration formats
(e.g. ``30s``, ``5m``) deliberately match what the Go side already uses.
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass
from typing import Mapping, Optional

_DURATION_PART = re.compile(r"(\d+(?:\.\d+)?)(ms|s|m|h)")
_UNIT_SECONDS = {"ms": 0.001, "s": 1.0, "m": 60.0, "h": 3600.0}


def parse_duration(value: str, default: float) -> float:
    """Parse ``"30s"``, ``"1m30s"``, ``"500ms"`` or a bare number of seconds."""
    text = (value or "").strip().lower()
    if not text:
        return default
    try:
        return float(text)
    except ValueError:
        pass
    parts = _DURATION_PART.findall(text)
    if not parts or "".join(n + u for n, u in parts) != text:
        raise ValueError(f"invalid duration: {value!r}")
    return sum(float(n) * _UNIT_SECONDS[u] for n, u in parts)


def _truthy(value: str) -> bool:
    return (value or "").strip().lower() in {"1", "true", "yes", "on"}


def _int(env: Mapping[str, str], key: str, default: int) -> int:
    raw = (env.get(key) or "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError as exc:
        raise ValueError(f"{key} must be an integer, got {raw!r}") from exc


@dataclass(frozen=True)
class Settings:
    database_url: str
    internal_token: str
    allow_no_auth: bool
    gemini_api_key: str
    gemini_model: str
    embedding_provider: str
    embedding_model: str
    embedding_api_key: str
    embedding_dimension: int
    llm_timeout: float
    embedding_timeout: float
    db_query_timeout: float
    agent_llm_planner: bool
    rag_context_limit: int
    max_input_chars: int

    @property
    def llm_enabled(self) -> bool:
        return bool(self.gemini_api_key)

    @property
    def embeddings_enabled(self) -> bool:
        return bool(self.embedding_api_key)

    def validate(self) -> None:
        if not self.database_url:
            raise ValueError("DB_URL is not set")
        if not self.internal_token and not self.allow_no_auth:
            raise ValueError(
                "AI_SERVICE_TOKEN is not set. Set the same secret for the Go API and this "
                "service, or set AI_SERVICE_ALLOW_NO_AUTH=true for local development only."
            )
        if self.embedding_dimension <= 0:
            raise ValueError("EMBEDDING_DIMENSION must be greater than 0")


def load_settings(env: Optional[Mapping[str, str]] = None) -> Settings:
    env = os.environ if env is None else env

    gemini_key = (env.get("GEMINI_API_KEY") or "").strip()
    provider = (env.get("EMBEDDING_PROVIDER") or "").strip().lower()
    if not provider:
        provider = "gemini" if gemini_key else "openai"

    # EMBEDDING_API_KEY wins; otherwise use the provider's own key.
    embedding_key = (env.get("EMBEDDING_API_KEY") or "").strip()
    if not embedding_key:
        if provider == "openai":
            embedding_key = (env.get("OPENAI_API_KEY") or "").strip()
        else:
            embedding_key = gemini_key

    model = (env.get("EMBEDDING_MODEL") or "").strip()
    if not model:
        model = "text-embedding-3-small" if provider == "openai" else "gemini-embedding-001"

    settings = Settings(
        database_url=(env.get("DB_URL") or "").strip(),
        internal_token=(env.get("AI_SERVICE_TOKEN") or "").strip(),
        allow_no_auth=_truthy(env.get("AI_SERVICE_ALLOW_NO_AUTH", "")),
        gemini_api_key=gemini_key,
        gemini_model=(env.get("GEMINI_MODEL") or "").strip() or "gemini-flash-latest",
        embedding_provider=provider,
        embedding_model=model,
        embedding_api_key=embedding_key,
        embedding_dimension=_int(env, "EMBEDDING_DIMENSION", 1536),
        llm_timeout=parse_duration(env.get("LLM_REQUEST_TIMEOUT", ""), 30.0),
        embedding_timeout=parse_duration(env.get("EMBEDDING_REQUEST_TIMEOUT", ""), 45.0),
        db_query_timeout=parse_duration(env.get("DB_QUERY_TIMEOUT", ""), 5.0),
        agent_llm_planner=_truthy(env.get("AGENT_LLM_PLANNER", "")),
        rag_context_limit=_int(env, "RAG_CONTEXT_LIMIT", 5),
        max_input_chars=_int(env, "AI_MAX_INPUT_CHARS", 12000),
    )
    settings.validate()
    return settings
