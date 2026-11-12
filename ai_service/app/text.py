"""Pure text helpers shared by summarization, embeddings, RAG and the agent."""

from __future__ import annotations

import re
from html import unescape
from html.parser import HTMLParser
from typing import Optional

from .errors import ApiError

MAX_QUERY_CHARS = 500

# Words that carry no search signal when a user phrases a request as a sentence.
FILLER_WORDS = frozenset(
    {
        "latest", "news", "articles", "article", "about", "show", "give", "find", "me",
        "recommend", "recommendation", "summarize", "summary", "explain", "tell", "what",
        "who", "why", "when", "where", "which", "how", "the", "a", "an", "and", "or", "to",
        "for", "from", "with", "please",
    }
)

_STRIP_CHARS = "\t\n\r.,!?;:'\"()[]{}<>/\\"


def validate_query(raw: Optional[str]) -> str:
    """Trim and collapse whitespace; reject empty or oversized questions."""
    query = " ".join((raw or "").split())
    if not query:
        raise ApiError(400, "query is required")
    if len(query) > MAX_QUERY_CHARS:
        raise ApiError(400, f"query must be {MAX_QUERY_CHARS} characters or less")
    return query


def normalize_search_query(raw: Optional[str]) -> str:
    """Drop filler words so ``"show me the latest AI news"`` searches for ``"ai"``."""
    words = []
    for word in (raw or "").strip().lower().split():
        word = word.strip(_STRIP_CHARS)
        if word and word not in FILLER_WORDS:
            words.append(word)
    return " ".join(words)


class _TextExtractor(HTMLParser):
    _BLOCK_TAGS = {"p", "br", "div", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "tr"}
    _SKIP_TAGS = {"script", "style"}

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self._chunks: list[str] = []
        self._skip_depth = 0

    def handle_starttag(self, tag, attrs):  # noqa: D401 - HTMLParser API
        if tag in self._SKIP_TAGS:
            self._skip_depth += 1
        elif tag in self._BLOCK_TAGS:
            self._chunks.append("\n")

    def handle_endtag(self, tag):
        if tag in self._SKIP_TAGS and self._skip_depth:
            self._skip_depth -= 1
        elif tag in self._BLOCK_TAGS:
            self._chunks.append("\n")

    def handle_data(self, data):
        if not self._skip_depth:
            self._chunks.append(data)

    def text(self) -> str:
        return "".join(self._chunks)


def strip_html(value: Optional[str]) -> str:
    """Turn the HTML found in RSS descriptions into readable plain text."""
    if not value:
        return ""
    parser = _TextExtractor()
    try:
        parser.feed(value)
        parser.close()
        text = parser.text()
    except Exception:  # malformed markup: fall back to a crude tag strip
        text = re.sub(r"<[^>]+>", " ", value)
    text = unescape(text)
    lines = (" ".join(line.split()) for line in text.splitlines())
    return "\n".join(line for line in lines if line)


def clip(text: str, limit: int) -> str:
    if limit > 0 and len(text) > limit:
        return text[:limit].rstrip()
    return text


def excerpt(description: Optional[str], limit: int = 500) -> str:
    text = " ".join(strip_html(description).split())
    if not text:
        return "No description available."
    if len(text) > limit:
        return text[: limit - 3].rstrip() + "..."
    return text


def summary_source_text(title: str, description: Optional[str], content: Optional[str]) -> str:
    """Pick the richest available text for a post: content > description > title."""
    source = (title or "").strip()
    desc = strip_html(description)
    if desc:
        source = desc
    body = strip_html(content)
    if body and len(body) > len(source):
        source = body
    return source


def embedding_source_text(title: str, description: Optional[str], content: Optional[str]) -> str:
    """Structured text that is embedded for a post."""
    sections = []
    if (title or "").strip():
        sections.append("Title:\n" + title.strip())
    desc = strip_html(description)
    if desc:
        sections.append("Description:\n" + desc)
    body = strip_html(content)
    # Content defaults to the description for RSS posts; don't embed it twice.
    if body and body != desc:
        sections.append("Content:\n" + body)
    return "\n\n".join(sections)
