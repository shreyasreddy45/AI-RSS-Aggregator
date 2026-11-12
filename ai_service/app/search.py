"""Semantic / hybrid search: request parsing and the service behind GET /search."""

from __future__ import annotations

import uuid
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from typing import Any, Dict, List, Mapping, Optional

from .errors import ApiError, ProviderError


@dataclass(frozen=True)
class SearchParams:
    query: str
    limit: int = 20
    offset: int = 0
    sort: str = "relevance"
    mode: str = "semantic"
    feed_id: Optional[str] = None
    date_from: Optional[date] = None
    date_to: Optional[date] = None

    @property
    def date_to_exclusive(self) -> Optional[date]:
        return self.date_to + timedelta(days=1) if self.date_to else None


def _parse_date(value: Optional[str]) -> Optional[date]:
    value = (value or "").strip()
    if not value:
        return None
    try:
        return datetime.strptime(value, "%Y-%m-%d").date()
    except ValueError:
        raise ApiError(400, f"invalid date {value!r}: expected YYYY-MM-DD") from None


def _parse_int(value: Optional[str], name: str, low: int, high: Optional[int]) -> Optional[int]:
    value = (value or "").strip()
    if not value:
        return None
    try:
        number = int(value)
    except ValueError:
        number = None
    if number is None or number < low or (high is not None and number > high):
        bound = f"an integer between {low} and {high}" if high else "a non-negative integer"
        raise ApiError(400, f"{name} must be {bound}")
    return number


def parse_search_params(params: Mapping[str, str]) -> SearchParams:
    """Validate query-string parameters. Mirrors the rules the Go API applies."""
    query = " ".join((params.get("q") or "").split())
    if not query:
        raise ApiError(400, "q is required")
    if len(query) > 200:
        raise ApiError(400, "q must be 200 characters or less")

    limit = _parse_int(params.get("limit"), "limit", 1, 100) or 20
    offset = _parse_int(params.get("offset"), "offset", 0, None) or 0

    sort = (params.get("sort") or "relevance").strip().lower()
    if sort not in ("relevance", "newest", "oldest"):
        raise ApiError(400, "sort must be one of: relevance, newest, oldest")

    mode = (params.get("mode") or "semantic").strip().lower()
    if mode not in ("semantic", "hybrid"):
        raise ApiError(400, "mode must be one of: semantic, hybrid")

    feed_id = (params.get("feed_id") or "").strip() or None
    if feed_id:
        try:
            feed_id = str(uuid.UUID(feed_id))
        except ValueError:
            raise ApiError(400, "feed_id must be a valid UUID") from None

    date_from = _parse_date(params.get("from"))
    date_to = _parse_date(params.get("to"))
    if date_from and date_to and date_from > date_to:
        raise ApiError(400, "from must be before or equal to to")

    return SearchParams(
        query=query, limit=limit, offset=offset, sort=sort, mode=mode,
        feed_id=feed_id, date_from=date_from, date_to=date_to,
    )


class SearchService:
    def __init__(self, repo, embedder):
        self._repo = repo
        self._embedder = embedder

    async def search(self, user_id: str, params: SearchParams) -> Dict[str, Any]:
        if self._embedder is None:
            raise ApiError(503, f"{params.mode} search requires an embedding provider")
        try:
            vector = await self._embedder.generate(params.query)
        except ProviderError as exc:
            raise ApiError(502, f"failed to generate query embedding: {exc}") from exc

        if params.mode == "hybrid":
            total, rows = await self._repo.search_hybrid(user_id, vector, params)
        else:
            total, rows = await self._repo.search_semantic(user_id, vector, params)

        return {
            "query": params.query,
            "total": total,
            "limit": params.limit,
            "offset": params.offset,
            "sort": params.sort,
            "mode": params.mode,
            "results": rows,
        }
