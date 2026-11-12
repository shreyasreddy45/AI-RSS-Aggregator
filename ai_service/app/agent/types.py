"""Shared types for the agent."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List

from ..models import Source

# Tool identifiers. The planner emits these names and the registry is keyed by them.
SEARCH = "search"
SEMANTIC_SEARCH = "semantic_search"
RECOMMENDATION = "recommendation"
RAG = "rag"
SUMMARY = "summary"
ALL_TOOLS = (SEARCH, SEMANTIC_SEARCH, RECOMMENDATION, RAG, SUMMARY)


class ToolError(Exception):
    """A tool could not produce a result (e.g. nothing indexed, LLM unavailable)."""


@dataclass
class ExecutionStep:
    name: str
    reason: str = ""


@dataclass
class ToolResult:
    name: str
    data: Dict[str, Any] = field(default_factory=dict)
    sources: List[Source] = field(default_factory=list)
    metadata: Dict[str, Any] = field(default_factory=dict)
    warnings: List[str] = field(default_factory=list)
    duration_ms: int = 0
    documents_scanned: int = 0
    documents_returned: int = 0
    retrieval_method: str = ""


@dataclass
class CombinedContext:
    sources: List[Source] = field(default_factory=list)
    metadata: Dict[str, Any] = field(default_factory=dict)
