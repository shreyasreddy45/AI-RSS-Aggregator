"""Postgres access for the AI service.

``Repository`` is the interface the rest of the service depends on, which keeps
RAG, the agent and the pipeline unit-testable with an in-memory fake.
``PgRepository`` is the production implementation.

Every user-facing query is scoped to feeds the user follows (``feed_follows``),
so one user can never retrieve another user's reading list.
"""

from __future__ import annotations

import asyncio
from typing import Any, Dict, List, Optional, Protocol, Tuple

from .embeddings import to_vector_literal
from .models import Candidate, Post, iso
from .search import SearchParams

_POST_COLUMNS = (
    "p.id, p.title, p.description, p.content, p.url, p.feed_id, "
    "p.published_at, p.created_at, p.updated_at"
)

_FOLLOWED = (
    "EXISTS (SELECT 1 FROM feed_follows ff "
    "WHERE ff.feed_id = p.feed_id AND ff.user_id = %(user)s::uuid)"
)


class Repository(Protocol):
    async def ping(self) -> None: ...
    async def get_post(self, post_id: str) -> Optional[Post]: ...
    async def get_post_for_user(self, user_id: str, post_id: str) -> Optional[Post]: ...
    async def processing_needs(self, post_id: str) -> Tuple[bool, bool]: ...
    async def upsert_summary(self, post_id: str, summary: str, model: str) -> None: ...
    async def upsert_embedding(
        self, post_id: str, vector: List[float], provider: str, model: str
    ) -> None: ...
    async def pending_post_ids(
        self, limit: int, want_summary: bool, want_embedding: bool
    ) -> List[str]: ...
    async def recent_posts(
        self, user_id: str, limit: int, search: Optional[str] = None
    ) -> List[Post]: ...
    async def vector_candidates(
        self, user_id: str, vector: List[float], query_text: str, limit: int
    ) -> List[Candidate]: ...
    async def search_semantic(
        self, user_id: str, vector: List[float], params: SearchParams
    ) -> Tuple[int, List[Dict[str, Any]]]: ...
    async def search_hybrid(
        self, user_id: str, vector: List[float], params: SearchParams
    ) -> Tuple[int, List[Dict[str, Any]]]: ...


def _like_escape(value: str) -> str:
    return value.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")


def _row_to_post(row: Dict[str, Any]) -> Post:
    return Post(
        id=str(row["id"]),
        title=row["title"],
        description=row.get("description"),
        content=row.get("content"),
        url=row["url"],
        feed_id=str(row["feed_id"]),
        published_at=row["published_at"],
        created_at=row.get("created_at"),
        updated_at=row.get("updated_at"),
    )


def _row_to_result(row: Dict[str, Any]) -> Dict[str, Any]:
    result: Dict[str, Any] = {
        "id": str(row["id"]),
        "title": row["title"],
        "created_at": iso(row["created_at"]),
        "updated_at": iso(row["updated_at"]),
        "published_at": iso(row["published_at"]),
        "url": row["url"],
        "feed_id": str(row["feed_id"]),
        "rank": float(row["rank"] or 0.0),
    }
    if row.get("description") is not None:
        result["description"] = row["description"]
    return result


def _scope(user_id: str, params: SearchParams) -> Tuple[str, Dict[str, Any]]:
    """WHERE fragment (without the keyword) + params shared by the search queries."""
    sql = _FOLLOWED
    args: Dict[str, Any] = {"user": user_id}
    if params.feed_id:
        sql += " AND p.feed_id = %(feed)s::uuid"
        args["feed"] = params.feed_id
    if params.date_from:
        sql += " AND p.published_at >= %(date_from)s"
        args["date_from"] = params.date_from
    if params.date_to_exclusive:
        sql += " AND p.published_at < %(date_to)s"
        args["date_to"] = params.date_to_exclusive
    return sql, args


def _order_by(sort: str, relevance: str) -> str:
    if sort == "newest":
        return "p.published_at DESC"
    if sort == "oldest":
        return "p.published_at ASC"
    return relevance


class PgRepository:
    def __init__(self, dsn: str, statement_timeout: float = 5.0, max_size: int = 10):
        self._dsn = dsn
        self._statement_timeout_ms = int(statement_timeout * 1000)
        self._max_size = max_size
        self._pool = None

    async def open(self) -> None:
        # Imported lazily so the rest of the package (and its tests) does not
        # need psycopg installed.
        #
        # This uses psycopg's *synchronous* pool and runs queries in worker
        # threads. Async psycopg needs a selector event loop, but uvicorn on
        # Windows always uses the proactor loop, so the async API cannot connect
        # there. Threads work identically on Windows, macOS and Linux.
        from psycopg.rows import dict_row
        from psycopg_pool import ConnectionPool

        self._pool = ConnectionPool(
            conninfo=self._dsn,
            min_size=1,
            max_size=self._max_size,
            kwargs={
                "row_factory": dict_row,
                "options": f"-c statement_timeout={self._statement_timeout_ms}",
            },
            open=False,
        )
        await asyncio.to_thread(self._pool.open, True, 15)

    async def close(self) -> None:
        if self._pool is not None:
            await asyncio.to_thread(self._pool.close)

    # -- low-level helpers (blocking work runs in a thread) -----------------

    def _all_sync(self, sql: str, params: Dict[str, Any]) -> List[Dict[str, Any]]:
        with self._pool.connection() as conn:
            return conn.execute(sql, params).fetchall()

    def _one_sync(self, sql: str, params: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        with self._pool.connection() as conn:
            return conn.execute(sql, params).fetchone()

    def _run_sync(self, sql: str, params: Dict[str, Any]) -> None:
        with self._pool.connection() as conn:
            conn.execute(sql, params)

    async def _all(self, sql: str, params: Dict[str, Any]) -> List[Dict[str, Any]]:
        return await asyncio.to_thread(self._all_sync, sql, params)

    async def _one(self, sql: str, params: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        return await asyncio.to_thread(self._one_sync, sql, params)

    async def _run(self, sql: str, params: Dict[str, Any]) -> None:
        await asyncio.to_thread(self._run_sync, sql, params)

    # -- Repository --------------------------------------------------------

    async def ping(self) -> None:
        await self._one("SELECT 1 AS ok", {})

    async def get_post(self, post_id: str) -> Optional[Post]:
        row = await self._one(
            f"SELECT {_POST_COLUMNS} FROM posts p WHERE p.id = %(id)s::uuid", {"id": post_id}
        )
        return _row_to_post(row) if row else None

    async def get_post_for_user(self, user_id: str, post_id: str) -> Optional[Post]:
        row = await self._one(
            f"SELECT {_POST_COLUMNS} FROM posts p WHERE p.id = %(id)s::uuid AND {_FOLLOWED}",
            {"id": post_id, "user": user_id},
        )
        return _row_to_post(row) if row else None

    async def processing_needs(self, post_id: str) -> Tuple[bool, bool]:
        row = await self._one(
            """
            SELECT (s.post_id IS NULL) AS needs_summary, (e.post_id IS NULL) AS needs_embedding
            FROM posts p
            LEFT JOIN post_summaries s ON s.post_id = p.id
            LEFT JOIN post_embeddings e ON e.post_id = p.id
            WHERE p.id = %(id)s::uuid
            """,
            {"id": post_id},
        )
        if not row:
            return False, False
        return bool(row["needs_summary"]), bool(row["needs_embedding"])

    async def upsert_summary(self, post_id: str, summary: str, model: str) -> None:
        await self._run(
            """
            INSERT INTO post_summaries (id, post_id, summary, model, created_at, updated_at)
            VALUES (gen_random_uuid(), %(post_id)s::uuid, %(summary)s, %(model)s, NOW(), NOW())
            ON CONFLICT (post_id) DO UPDATE
               SET summary = EXCLUDED.summary, model = EXCLUDED.model, updated_at = NOW()
            """,
            {"post_id": post_id, "summary": summary, "model": model[:128]},
        )

    async def upsert_embedding(
        self, post_id: str, vector: List[float], provider: str, model: str
    ) -> None:
        await self._run(
            """
            INSERT INTO post_embeddings (id, post_id, embedding, provider, model, created_at, updated_at)
            VALUES (gen_random_uuid(), %(post_id)s::uuid, %(vec)s::vector, %(provider)s, %(model)s, NOW(), NOW())
            ON CONFLICT (post_id) DO UPDATE
               SET embedding = EXCLUDED.embedding, provider = EXCLUDED.provider,
                   model = EXCLUDED.model, updated_at = NOW()
            """,
            {
                "post_id": post_id,
                "vec": to_vector_literal(vector),
                "provider": provider[:32],
                "model": model[:128],
            },
        )

    async def pending_post_ids(
        self, limit: int, want_summary: bool, want_embedding: bool
    ) -> List[str]:
        if not (want_summary or want_embedding):
            return []
        rows = await self._all(
            """
            SELECT p.id
            FROM posts p
            LEFT JOIN post_summaries s ON s.post_id = p.id
            LEFT JOIN post_embeddings e ON e.post_id = p.id
            WHERE (%(ws)s AND s.post_id IS NULL) OR (%(we)s AND e.post_id IS NULL)
            ORDER BY p.published_at DESC
            LIMIT %(limit)s
            """,
            {"ws": want_summary, "we": want_embedding, "limit": limit},
        )
        return [str(r["id"]) for r in rows]

    async def recent_posts(
        self, user_id: str, limit: int, search: Optional[str] = None
    ) -> List[Post]:
        sql = f"SELECT {_POST_COLUMNS} FROM posts p WHERE {_FOLLOWED}"
        params: Dict[str, Any] = {"user": user_id, "limit": limit}
        if search:
            sql += (
                " AND (p.title ILIKE %(like)s"
                " OR p.search_document @@ websearch_to_tsquery('english', %(q)s))"
            )
            params["like"] = f"%{_like_escape(search)}%"
            params["q"] = search
        sql += " ORDER BY p.published_at DESC LIMIT %(limit)s"
        return [_row_to_post(r) for r in await self._all(sql, params)]

    async def vector_candidates(
        self, user_id: str, vector: List[float], query_text: str, limit: int
    ) -> List[Candidate]:
        rows = await self._all(
            f"""
            SELECT {_POST_COLUMNS},
                   1 - (pe.embedding <=> %(vec)s::vector) AS cosine,
                   ts_rank_cd(p.search_document, websearch_to_tsquery('english', %(q)s)) AS text_rank
            FROM post_embeddings pe
            JOIN posts p ON p.id = pe.post_id
            WHERE {_FOLLOWED}
            ORDER BY pe.embedding <=> %(vec)s::vector
            LIMIT %(limit)s
            """,
            {"user": user_id, "vec": to_vector_literal(vector), "q": query_text, "limit": limit},
        )
        return [
            Candidate(
                post=_row_to_post(r),
                cosine=float(r["cosine"] or 0.0),
                text_rank=float(r["text_rank"] or 0.0),
            )
            for r in rows
        ]

    async def search_semantic(
        self, user_id: str, vector: List[float], params: SearchParams
    ) -> Tuple[int, List[Dict[str, Any]]]:
        scope, args = _scope(user_id, params)
        count = await self._one(
            f"SELECT COUNT(*) AS n FROM post_embeddings pe JOIN posts p ON p.id = pe.post_id WHERE {scope}",
            args,
        )
        order = _order_by(
            params.sort, "pe.embedding <=> %(vec)s::vector ASC, p.published_at DESC"
        )
        rows = await self._all(
            f"""
            SELECT p.id, p.title, p.description, p.created_at, p.updated_at, p.published_at,
                   p.url, p.feed_id,
                   1.0 / (1.0 + (pe.embedding <=> %(vec)s::vector)) AS rank
            FROM post_embeddings pe
            JOIN posts p ON p.id = pe.post_id
            WHERE {scope}
            ORDER BY {order}
            LIMIT %(limit)s OFFSET %(offset)s
            """,
            {**args, "vec": to_vector_literal(vector), "limit": params.limit, "offset": params.offset},
        )
        return int(count["n"]) if count else 0, [_row_to_result(r) for r in rows]

    async def search_hybrid(
        self, user_id: str, vector: List[float], params: SearchParams
    ) -> Tuple[int, List[Dict[str, Any]]]:
        scope, args = _scope(user_id, params)
        match = "p.search_document @@ websearch_to_tsquery('english', %(q)s)"
        args = {**args, "q": params.query}
        count = await self._one(
            f"SELECT COUNT(*) AS n FROM posts p WHERE {scope} AND {match}", args
        )
        order = _order_by(params.sort, "rank DESC, p.published_at DESC")
        rows = await self._all(
            f"""
            SELECT p.id, p.title, p.description, p.created_at, p.updated_at, p.published_at,
                   p.url, p.feed_id,
                   (
                     0.35 * COALESCE(ts_rank_cd(p.search_document, websearch_to_tsquery('english', %(q)s)), 0.0)
                     + 0.65 * CASE WHEN pe.embedding IS NULL THEN 0.0
                                   ELSE 1.0 / (1.0 + (pe.embedding <=> %(vec)s::vector)) END
                   ) AS rank
            FROM posts p
            LEFT JOIN post_embeddings pe ON pe.post_id = p.id
            WHERE {scope} AND {match}
            ORDER BY {order}
            LIMIT %(limit)s OFFSET %(offset)s
            """,
            {**args, "vec": to_vector_literal(vector), "limit": params.limit, "offset": params.offset},
        )
        return int(count["n"]) if count else 0, [_row_to_result(r) for r in rows]
