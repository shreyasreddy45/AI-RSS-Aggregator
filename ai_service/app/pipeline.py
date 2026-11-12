"""Per-post AI work: summaries and embeddings."""

from __future__ import annotations

import logging
from typing import Any, Dict, List

from .errors import ApiError, NotConfiguredError, ProviderError, short_error
from .text import clip, embedding_source_text, summary_source_text

log = logging.getLogger("ai.pipeline")

DEFAULT_PENDING_LIMIT = 500
MAX_PENDING_LIMIT = 5000


class PostPipeline:
    def __init__(self, repo, llm, embedder, max_input_chars: int = 12000):
        self._repo = repo
        self._llm = llm
        self._embedder = embedder
        self._max_chars = max_input_chars

    async def process(self, post_id: str) -> Dict[str, Any]:
        """Generate whatever is still missing for a post. Idempotent.

        Raises ``ApiError(502)`` if a needed step failed, so the Go job queue
        retries the job with backoff.
        """
        post = await self._repo.get_post(post_id)
        if post is None:
            raise ApiError(404, "post not found")

        needs_summary, needs_embedding = await self._repo.processing_needs(post_id)
        outcome = {"post_id": post_id, "summary": "skipped", "embedding": "skipped"}
        failures: List[str] = []

        if needs_summary:
            if self._llm is None:
                outcome["summary"] = "disabled"
            else:
                text = clip(
                    summary_source_text(post.title, post.description, post.content), self._max_chars
                )
                if not text:
                    outcome["summary"] = "empty"
                else:
                    try:
                        summary = await self._llm.summarize(text)
                        await self._repo.upsert_summary(post_id, summary, self._llm.model)
                        outcome["summary"] = "created"
                    except ProviderError as exc:
                        log.warning("summary failed post=%s: %s", post_id, exc)
                        outcome["summary"] = "failed"
                        failures.append(f"summary: {exc}")

        if needs_embedding:
            if self._embedder is None:
                outcome["embedding"] = "disabled"
            else:
                text = embedding_source_text(post.title, post.description, post.content)
                if not text:
                    outcome["embedding"] = "empty"
                else:
                    try:
                        vector = await self._embedder.generate(text)
                        await self._repo.upsert_embedding(
                            post_id, vector, self._embedder.provider, self._embedder.model
                        )
                        outcome["embedding"] = "created"
                    except ProviderError as exc:
                        log.warning("embedding failed post=%s: %s", post_id, exc)
                        outcome["embedding"] = "failed"
                        failures.append(f"embedding: {exc}")

        if failures:
            raise ApiError(502, "; ".join(failures))
        return outcome

    async def embed_for_user(self, user_id: str, post_id: str) -> Dict[str, Any]:
        """(Re)generate one post's embedding on request; the post must be visible to the user."""
        if self._embedder is None:
            raise ApiError(503, "embedding provider is not configured")

        post = await self._repo.get_post_for_user(user_id, post_id)
        if post is None:
            raise ApiError(404, "post not found")

        text = embedding_source_text(post.title, post.description, post.content)
        if not text:
            raise ApiError(400, "post has no content to embed")

        try:
            vector = await self._embedder.generate(text)
        except (ProviderError, NotConfiguredError) as exc:
            log.error("embedding failed post=%s: %s", post_id, exc)
            raise ApiError(502, "failed to generate embedding") from exc

        await self._repo.upsert_embedding(
            post_id, vector, self._embedder.provider, self._embedder.model
        )
        return {
            "post_id": post_id,
            "provider": self._embedder.provider,
            "model": self._embedder.model,
            "dimension": len(vector),
        }

    async def summarize_for_user(self, user_id: str, post_id: str) -> Dict[str, Any]:
        """Generate (or regenerate) one post's summary on request."""
        if self._llm is None:
            raise ApiError(503, "AI generation is not configured")

        post = await self._repo.get_post_for_user(user_id, post_id)
        if post is None:
            raise ApiError(404, "post not found")

        text = clip(summary_source_text(post.title, post.description, post.content), self._max_chars)
        if not text:
            raise ApiError(400, "post has no content to summarize")

        try:
            summary = await self._llm.summarize(text)
        except ProviderError as exc:
            log.warning("on-demand summary failed post=%s: %s", post_id, exc)
            raise ApiError(502, f"AI provider error: {short_error(exc)}") from exc

        await self._repo.upsert_summary(post_id, summary, self._llm.model)
        return {"post_id": post_id, "summary": summary, "model": self._llm.model}

    async def pending(self, limit: int = DEFAULT_PENDING_LIMIT) -> List[str]:
        limit = max(1, min(limit, MAX_PENDING_LIMIT))
        return await self._repo.pending_post_ids(
            limit, want_summary=self._llm is not None, want_embedding=self._embedder is not None
        )
