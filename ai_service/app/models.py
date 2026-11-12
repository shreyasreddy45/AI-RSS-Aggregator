"""Plain data types passed between the repository, RAG and agent layers."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional


def iso(value: Optional[datetime]) -> Optional[str]:
    """ISO-8601 in UTC with a ``Z`` suffix, matching what the Go API emits.

    Posts are stored in ``TIMESTAMP`` (no zone) columns, so naive values are UTC.
    """
    if value is None:
        return None
    if value.tzinfo is None:
        value = value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


@dataclass
class Post:
    id: str
    title: str
    url: str
    feed_id: str
    published_at: datetime
    description: Optional[str] = None
    content: Optional[str] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


@dataclass
class Source:
    """A post cited in an answer."""

    id: str
    title: str
    url: str
    published_at: Optional[datetime]
    feed_id: str
    description: str = ""

    def to_dict(self) -> Dict[str, Any]:
        data: Dict[str, Any] = {
            "id": self.id,
            "title": self.title,
            "url": self.url,
            "published_at": iso(self.published_at),
            "feed_id": self.feed_id,
        }
        if self.description:
            data["description"] = self.description
        return data


def build_sources(posts: List[Post]) -> List[Source]:
    from .text import strip_html  # local import keeps this module dependency-free

    return [
        Source(
            id=p.id,
            title=p.title,
            url=p.url,
            published_at=p.published_at,
            feed_id=p.feed_id,
            description=strip_html(p.description),
        )
        for p in posts
    ]


@dataclass
class Candidate:
    """A post retrieved by vector search, with the signals used for ranking."""

    post: Post
    cosine: float
    text_rank: float


@dataclass
class RetrievalResult:
    posts: List[Post] = field(default_factory=list)
    method: str = ""
    documents_scanned: int = 0
    warnings: List[str] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
