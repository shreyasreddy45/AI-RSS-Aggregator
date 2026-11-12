"""Retrieval-augmented answering over the articles a user follows."""

from __future__ import annotations

import logging
import math
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Callable, Dict, List, Optional

from .errors import (
    ApiError,
    ProviderError,
    is_quota_error,
    short_error,
    should_fallback_on_llm_failure,
)
from .models import Candidate, Post, RetrievalResult, Source, build_sources, iso
from .text import excerpt, normalize_search_query

log = logging.getLogger("ai.rag")

QUOTA_ANSWER = (
    "Context retrieved successfully but answer generation failed because the LLM provider "
    "quota was exceeded."
)
UNAVAILABLE_ANSWER = (
    "Context retrieved successfully but answer generation failed because the LLM provider "
    "is temporarily unavailable."
)

# Retrieval modes (kept identical to the original Go implementation).
KEYWORD = "keyword"
VECTOR = "vector"
HYBRID = "hybrid"
RECOMMENDED = "recommended"
RECENT = "recent"
_MODES = {KEYWORD, VECTOR, HYBRID, RECOMMENDED, RECENT}

# How many nearest neighbours to pull per requested result before re-ranking.
CANDIDATE_MULTIPLIER = 10


# --------------------------------------------------------------------------
# prompt
# --------------------------------------------------------------------------


def build_rag_prompt(query: str, sources: List[Source]) -> str:
    if not sources:
        return (
            "You are a grounded RSS assistant. Answer the user's question using the available "
            "context only. If the sources do not contain the answer, say so clearly and do not "
            f"invent facts.\n\nQuestion: {query}\n\nRelevant sources: none available."
        )

    lines = [
        "You are a grounded RSS assistant. Use only the provided sources to answer the user's "
        "question. Do not invent facts. If the sources do not provide a clear answer, say so "
        "and explain the gap. Cite the source titles and URLs when referencing information.\n",
        "Question:",
        query,
        "\nRelevant sources:",
    ]
    for i, source in enumerate(sources, start=1):
        lines.append(
            f"{i}. Title: {source.title}\n"
            f"   URL: {source.url}\n"
            f"   Published: {iso(source.published_at)}\n"
            f"   Content excerpt: {excerpt(source.description)}\n"
        )
    lines.append("\nAnswer in a concise, helpful way and include a short list of source URLs at the end.")
    return "\n".join(lines)


# --------------------------------------------------------------------------
# ranking
# --------------------------------------------------------------------------


@dataclass(frozen=True)
class HybridWeights:
    cosine: float = 0.6
    full_text: float = 0.25
    recency: float = 0.15
    half_life_seconds: float = 7 * 24 * 3600.0


def _aware(value: datetime) -> datetime:
    return value if value.tzinfo else value.replace(tzinfo=timezone.utc)


def recency_score(published_at: Optional[datetime], half_life_seconds: float, now: datetime) -> float:
    if published_at is None:
        return 0.0
    if half_life_seconds <= 0:
        return 1.0
    age = (_aware(now) - _aware(published_at)).total_seconds()
    if age <= 0:
        return 1.0
    return math.exp(-age / half_life_seconds)


def normalize_text_rank(rank: float) -> float:
    return 0.0 if rank <= 0 else min(rank / 10.0, 1.0)


def score_candidate(c: Candidate, weights: HybridWeights, now: datetime) -> float:
    return (
        weights.cosine * max(c.cosine, 0.0)
        + weights.full_text * normalize_text_rank(c.text_rank)
        + weights.recency * recency_score(c.post.published_at, weights.half_life_seconds, now)
    )


def rank_candidates(
    candidates: List[Candidate], weights: HybridWeights, now: datetime
) -> List[Candidate]:
    scored = [(score_candidate(c, weights, now), c) for c in candidates]
    scored.sort(key=lambda pair: (pair[0], _aware(pair[1].post.published_at)), reverse=True)
    return [c for _, c in scored]


# --------------------------------------------------------------------------
# service
# --------------------------------------------------------------------------


def _now() -> datetime:
    return datetime.now(timezone.utc)


class RagService:
    def __init__(
        self,
        repo,
        llm,
        embedder,
        weights: Optional[HybridWeights] = None,
        clock: Callable[[], datetime] = _now,
    ):
        self._repo = repo
        self._llm = llm
        self._embedder = embedder
        self._weights = weights or HybridWeights()
        self._clock = clock

    @property
    def llm(self):
        return self._llm

    @property
    def embedder(self):
        return self._embedder

    # -- retrieval ---------------------------------------------------------

    async def retrieve_hybrid(self, user_id: str, query: str, limit: int = 5) -> RetrievalResult:
        """Vector search re-ranked with full-text relevance and recency.

        Degrades gracefully: no embedder / embedding failure -> recent posts;
        nothing embedded yet -> keyword search.
        """
        limit = limit if limit > 0 else 5

        if self._embedder is None:
            posts = await self._repo.recent_posts(user_id, limit)
            return RetrievalResult(
                posts=posts, method=RECENT, documents_scanned=len(posts),
                warnings=["embedding provider unavailable; using recent posts"],
            )

        try:
            vector = await self._embedder.generate(query)
        except ProviderError as exc:
            log.warning("query embedding failed; falling back to recent posts: %s", exc)
            posts = await self._repo.recent_posts(user_id, limit)
            return RetrievalResult(
                posts=posts, method=RECENT, documents_scanned=len(posts),
                warnings=["embedding generation failed; using recent posts"],
            )

        candidates = await self._repo.vector_candidates(
            user_id, vector, query, limit * CANDIDATE_MULTIPLIER
        )
        if not candidates:
            keyword_query = normalize_search_query(query) or query
            posts = await self._repo.recent_posts(user_id, limit, search=keyword_query)
            if posts:
                return RetrievalResult(
                    posts=posts, method=KEYWORD, documents_scanned=len(posts),
                    warnings=["no embedded articles yet; used keyword search"],
                )
            return RetrievalResult(method=HYBRID, errors=["No indexed articles available."])

        ranked = rank_candidates(candidates, self._weights, self._clock())
        posts = [c.post for c in ranked[:limit]]
        return RetrievalResult(posts=posts, method=HYBRID, documents_scanned=len(candidates))

    async def retrieve(
        self, user_id: str, query: str, limit: int = 5, mode: str = RECOMMENDED
    ) -> RetrievalResult:
        """Mode-driven retrieval used by the agent's tools."""
        limit = limit if limit > 0 else 5
        mode = mode if mode in _MODES else RECOMMENDED
        normalized = normalize_search_query(query)
        if not normalized and mode in (RECOMMENDED, HYBRID, VECTOR):
            mode = RECENT

        if mode == RECENT:
            return await self._recent(user_id, limit)
        if mode == KEYWORD:
            return await self._keyword(user_id, normalized, limit)
        if mode == VECTOR:
            return await self._vector_only(user_id, normalized, limit)

        # hybrid / recommended
        if self._embedder is None:
            result = await self._keyword(user_id, normalized, limit)
            result.warnings.append("embedding provider unavailable; using keyword fallback")
            return result
        try:
            return await self.retrieve_hybrid(user_id, normalized, limit)
        except Exception as exc:  # DB or other failure in the vector path
            log.warning("hybrid retrieval failed; using keyword results: %s", exc)
            result = await self._keyword(user_id, normalized, limit)
            result.errors.append(str(exc))
            result.warnings.append("hybrid retrieval failed; returned keyword results")
            return result

    async def _recent(self, user_id: str, limit: int) -> RetrievalResult:
        posts = await self._repo.recent_posts(user_id, limit)
        if not posts:
            return RetrievalResult(method=RECENT, errors=["No subscribed feeds found."])
        return RetrievalResult(posts=posts, method=RECENT, documents_scanned=len(posts))

    async def _keyword(self, user_id: str, query: str, limit: int) -> RetrievalResult:
        posts = await self._repo.recent_posts(user_id, limit, search=query or None)
        if not posts:
            return RetrievalResult(method=KEYWORD, errors=["No indexed articles available."])
        return RetrievalResult(posts=posts, method=KEYWORD, documents_scanned=len(posts))

    async def _vector_only(self, user_id: str, query: str, limit: int) -> RetrievalResult:
        if self._embedder is None:
            return RetrievalResult(method=VECTOR, errors=["Embedding generation failed."])
        try:
            vector = await self._embedder.generate(query)
        except ProviderError as exc:
            return RetrievalResult(method=VECTOR, errors=[f"Embedding generation failed: {exc}"])
        candidates = await self._repo.vector_candidates(user_id, vector, query, limit)
        if not candidates:
            return RetrievalResult(method=VECTOR, errors=["No semantic matches found."])
        posts = [c.post for c in candidates]
        return RetrievalResult(posts=posts, method=VECTOR, documents_scanned=len(posts))

    # -- answering ---------------------------------------------------------

    async def generate_answer(self, user_id: str, query: str, limit: int = 5) -> Dict:
        if self._llm is None:
            raise ApiError(503, "AI generation is not configured")

        result = await self.retrieve_hybrid(user_id, query, limit)
        model = self._llm.model
        generated_at = iso(self._clock())

        if not result.posts:
            return {
                "answer": "No indexed documents are available for retrieval.",
                "sources": [],
                "query": query,
                "model": model,
                "generated_at": generated_at,
                "retrieval": result.method,
            }

        sources = build_sources(result.posts)
        llm_error = None
        try:
            answer = await self._llm.generate_text(build_rag_prompt(query, sources))
        except ProviderError as exc:
            detail = short_error(exc)
            if not should_fallback_on_llm_failure(exc):
                log.error("answer generation failed: %s", detail)
                raise ApiError(502, f"AI provider error: {detail}") from exc
            log.warning("answer generation degraded: %s", detail)
            answer = QUOTA_ANSWER if is_quota_error(exc) else UNAVAILABLE_ANSWER
            llm_error = detail

        response = {
            "answer": answer,
            "sources": [s.to_dict() for s in sources],
            "query": query,
            "model": model,
            "generated_at": generated_at,
            "retrieval": result.method,
        }
        if llm_error:
            response["llm_error"] = llm_error
        return response
