"""The agent's tools. Each one retrieves articles and/or calls the LLM."""

from __future__ import annotations

import logging
import time
from typing import Dict, List, Optional

from ..errors import ProviderError, is_quota_error, should_fallback_on_llm_failure
from ..models import Post, RetrievalResult, build_sources
from ..rag import (
    HYBRID,
    KEYWORD,
    QUOTA_ANSWER,
    RECOMMENDED,
    UNAVAILABLE_ANSWER,
    VECTOR,
    RagService,
    build_rag_prompt,
)
from ..text import clip, normalize_search_query, strip_html
from .types import RAG, RECOMMENDATION, SEARCH, SEMANTIC_SEARCH, SUMMARY, ToolError, ToolResult

log = logging.getLogger("ai.agent.tools")

_QUESTION_WORDS = (
    "how", "what", "why", "when", "where", "which", "explain", "summarize",
    "recommend", "suggest", "compare", "tell me", "best",
)


def detect_search_mode(query: str) -> str:
    """Short queries are keyword lookups; question-like ones benefit from hybrid."""
    trimmed = (query or "").strip().lower()
    if not trimmed or len(trimmed.split()) <= 3:
        return KEYWORD
    if any(token in trimmed for token in _QUESTION_WORDS):
        return HYBRID
    return KEYWORD


def best_matching_post(posts: List[Post], query: str) -> Optional[Post]:
    """Pick the post whose title/description/content mentions the query terms most."""
    if not posts:
        return None
    keys = (normalize_search_query(query) or (query or "").strip().lower()).split()
    if not keys:
        return posts[0]

    def score(post: Post) -> float:
        title = (post.title or "").lower()
        desc = strip_html(post.description).lower()
        body = strip_html(post.content).lower()
        total = 0.0
        for key in keys:
            if key in title:
                total += 3.0
            if key in desc:
                total += 2.0
            if key in body:
                total += 1.5
        return total or 0.1

    # Highest score first; newest wins ties.
    return max(posts, key=lambda p: (score(p), p.published_at))


def _result(
    name: str,
    retrieval: Optional[RetrievalResult],
    posts: List[Post],
    started: float,
    data: Optional[Dict] = None,
) -> ToolResult:
    sources = build_sources(posts)
    result = ToolResult(
        name=name,
        data=dict(data or {}),
        sources=sources,
        metadata={"status": "ok"},
        duration_ms=int((time.monotonic() - started) * 1000),
        documents_returned=len(sources),
    )
    if retrieval is not None:
        result.retrieval_method = retrieval.method
        result.documents_scanned = retrieval.documents_scanned
        result.warnings = list(retrieval.warnings)
        result.metadata["retrieval_method"] = retrieval.method
        result.metadata["documents_scanned"] = retrieval.documents_scanned
    result.metadata["documents_returned"] = len(sources)
    return result


def _require_posts(retrieval: RetrievalResult) -> List[Post]:
    if not retrieval.posts:
        raise ToolError((retrieval.errors or ["No indexed articles available."])[0])
    return retrieval.posts


class SearchTool:
    name = SEARCH
    description = "Search articles using keyword, semantic, or hybrid retrieval."

    def __init__(self, rag: RagService):
        self._rag = rag

    async def execute(self, user_id: str, query: str) -> ToolResult:
        started = time.monotonic()
        normalized = normalize_search_query(query)
        mode = detect_search_mode(query)
        if self._rag.embedder is not None and normalized:
            mode = HYBRID
        retrieval = await self._rag.retrieve(user_id, normalized or query, 5, mode)
        posts = _require_posts(retrieval)
        return _result(
            self.name, retrieval, posts, started,
            {"count": len(posts), "search_mode": mode, "normalized_query": normalized},
        )


class SemanticSearchTool:
    name = SEMANTIC_SEARCH
    description = "Semantic retrieval using vector similarity and ranked article matches."

    def __init__(self, rag: RagService):
        self._rag = rag

    async def execute(self, user_id: str, query: str) -> ToolResult:
        started = time.monotonic()
        normalized = normalize_search_query(query)
        retrieval = await self._rag.retrieve(user_id, normalized or query, 5, VECTOR)
        posts = _require_posts(retrieval)
        return _result(
            self.name, retrieval, posts, started,
            {"count": len(posts), "normalized_query": normalized},
        )


class RecommendationTool:
    name = RECOMMENDATION
    description = "Recommend the top matching articles for a user query using semantic ranking."

    def __init__(self, rag: RagService):
        self._rag = rag

    async def execute(self, user_id: str, query: str) -> ToolResult:
        started = time.monotonic()
        normalized = normalize_search_query(query)
        retrieval = await self._rag.retrieve(user_id, normalized or query, 8, RECOMMENDED)
        posts = _require_posts(retrieval)
        return _result(
            self.name, retrieval, posts, started,
            {"count": len(posts), "recommended": True, "normalized_query": normalized},
        )


class RAGTool:
    name = RAG
    description = "Ground a response using the most relevant articles for the query."

    def __init__(self, rag: RagService):
        self._rag = rag

    async def execute(self, user_id: str, query: str) -> ToolResult:
        started = time.monotonic()
        normalized = normalize_search_query(query)
        retrieval = await self._rag.retrieve(user_id, normalized or query, 5, HYBRID)
        posts = _require_posts(retrieval)
        llm = self._rag.llm
        if llm is None:
            raise ToolError("LLM unavailable.")

        sources = build_sources(posts)
        data = {"normalized_query": normalized}
        try:
            data["answer"] = await llm.generate_text(build_rag_prompt(query, sources))
        except ProviderError as exc:
            if not should_fallback_on_llm_failure(exc):
                raise ToolError(f"answer generation failed: {exc}") from exc
            data["answer"] = QUOTA_ANSWER if is_quota_error(exc) else UNAVAILABLE_ANSWER
            data["llm_fallback"] = True
        return _result(self.name, retrieval, posts, started, data)


class SummaryTool:
    name = SUMMARY
    description = "Summarize the best matching article for a user query."

    def __init__(self, rag: RagService, max_chars: int = 12000):
        self._rag = rag
        self._max_chars = max_chars

    async def execute(self, user_id: str, query: str) -> ToolResult:
        started = time.monotonic()
        llm = self._rag.llm
        if llm is None:
            raise ToolError("LLM unavailable.")

        normalized = normalize_search_query(query)
        retrieval = await self._rag.retrieve(user_id, normalized or query, 10, RECOMMENDED)
        posts = _require_posts(retrieval)
        post = best_matching_post(posts, query) or posts[0]

        parts = [post.title]
        for extra in (strip_html(post.description), strip_html(post.content)):
            if extra and extra not in parts:
                parts.append(extra)
        text = clip("\n\n".join(parts), self._max_chars)

        data = {
            "selected_post_id": post.id,
            "selected_title": post.title,
            "model_used": llm.model,
            "normalized_query": normalized,
        }
        try:
            data["answer"] = await llm.summarize(text)
        except ProviderError as exc:
            if not should_fallback_on_llm_failure(exc):
                raise ToolError(f"summary failed: {exc}") from exc
            data["answer"] = QUOTA_ANSWER if is_quota_error(exc) else UNAVAILABLE_ANSWER
            data["llm_fallback"] = True
        return _result(self.name, retrieval, [post], started, data)


def build_tools(rag: RagService, max_chars: int = 12000) -> Dict[str, object]:
    tools = [
        SearchTool(rag),
        SemanticSearchTool(rag),
        RecommendationTool(rag),
        RAGTool(rag),
        SummaryTool(rag, max_chars),
    ]
    return {tool.name: tool for tool in tools}
