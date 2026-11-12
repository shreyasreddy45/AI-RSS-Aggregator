"""Decides which tools to run for a query."""

from __future__ import annotations

import json
import logging
from typing import Dict, List, Optional, Tuple

from .types import (
    ALL_TOOLS,
    RAG,
    RECOMMENDATION,
    SEARCH,
    SEMANTIC_SEARCH,
    SUMMARY,
    ExecutionStep,
)

log = logging.getLogger("ai.agent.planner")


def _contains_any(text: str, needles: Tuple[str, ...]) -> bool:
    return any(n in text for n in needles)


_SEARCH_WORDS = ("latest", "news", "breaking", "feed", "search", "find", "today", "announcement", "posts")
_RECOMMEND_WORDS = ("recommend", "suggest", "best", "top picks", "discover", "interesting")
_SEMANTIC_WORDS = (
    "similar", "related", "how is", "used in", "implemented", "architecture", "redis",
    "vector", "embedding", "semantic", "kubernetes", "nvidia", "openai",
)
_RAG_WORDS = ("what is", "explain", "how does", "why", "tell me about", "what are", "today's", "announcement")
_SUMMARY_WORDS = ("summarize", "summary", "briefly", "tl;dr", "overview")


class Planner:
    """Deterministic keyword router. Safe default: needs no LLM and cannot fail."""

    async def plan(self, raw: str) -> List[ExecutionStep]:
        query = (raw or "").strip().lower()
        if not query:
            return [ExecutionStep(SEARCH, "empty query falls back to search")]

        steps: List[ExecutionStep] = []
        if _contains_any(query, _SEARCH_WORDS):
            steps.append(ExecutionStep(SEARCH, "search fetches likely relevant articles"))
        if _contains_any(query, _RECOMMEND_WORDS):
            steps.append(ExecutionStep(RECOMMENDATION, "recommendation complements search"))
        if _contains_any(query, _RAG_WORDS):
            steps.append(ExecutionStep(RAG, "RAG grounds the answer with article evidence"))
        if _contains_any(query, _SEMANTIC_WORDS):
            steps.append(
                ExecutionStep(SEMANTIC_SEARCH, "semantic retrieval finds conceptually related articles")
            )
        if _contains_any(query, _SUMMARY_WORDS):
            steps.append(ExecutionStep(SUMMARY, "summary tool condenses the matched article content"))

        if not steps:
            return [ExecutionStep(SEARCH, "default fallback plan")]

        return _unique(steps)


class LLMPlanner:
    """Optional model-backed router; falls back to the keyword planner on any problem."""

    def __init__(self, llm, tool_descriptions: Dict[str, str], fallback: Optional[Planner] = None):
        self._llm = llm
        self._descriptions = tool_descriptions
        self._fallback = fallback or Planner()

    async def plan(self, raw: str) -> List[ExecutionStep]:
        if self._llm is None:
            return await self._fallback.plan(raw)
        try:
            response = await self._llm.generate_text(
                f"{build_llm_planner_prompt(self._descriptions)}\n\nUser query: {(raw or '').strip()}"
            )
        except Exception as exc:  # any provider failure -> deterministic plan
            log.info("LLM planner unavailable, using keyword planner: %s", exc)
            return await self._fallback.plan(raw)

        steps = parse_llm_plan(response)
        return steps if steps else await self._fallback.plan(raw)


def build_llm_planner_prompt(descriptions: Dict[str, str]) -> str:
    tools = "\n".join(f"- {name}: {desc}" for name, desc in descriptions.items())
    return (
        "You are a tool router. Select the best tool(s) for the query. Available tools:\n"
        f"{tools}\n"
        'Return only valid JSON in the format [{"tool_name":"search","reason":"latest news retrieval"}] '
        "and use only tool names from the list."
    )


_ALIASES = {
    "searchtool": SEARCH, "search": SEARCH,
    "recommendationtool": RECOMMENDATION, "recommendation": RECOMMENDATION,
    "ragtool": RAG, "rag": RAG,
    "summarytool": SUMMARY, "summary": SUMMARY,
    "semanticsearchtool": SEMANTIC_SEARCH, "semantic_search": SEMANTIC_SEARCH,
    "semanticsearch": SEMANTIC_SEARCH,
}


def parse_llm_plan(raw: str) -> List[ExecutionStep]:
    text = (raw or "").strip()
    if text.startswith("```"):
        text = text.strip("`")
        if text.lower().startswith("json"):
            text = text[4:]
        text = text.strip()
    try:
        items = json.loads(text)
    except (ValueError, TypeError):
        return []
    if not isinstance(items, list):
        return []

    steps: List[ExecutionStep] = []
    for item in items:
        if not isinstance(item, dict):
            continue
        name = _ALIASES.get(str(item.get("tool_name", "")).strip().lower())
        if name in ALL_TOOLS:
            steps.append(ExecutionStep(name, str(item.get("reason", ""))))
    return _unique(steps)


def _unique(steps: List[ExecutionStep]) -> List[ExecutionStep]:
    seen = set()
    out = []
    for step in steps:
        if step.name not in seen:
            seen.add(step.name)
            out.append(step)
    return out
