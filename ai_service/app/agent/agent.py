"""The agent: plan -> run tools -> aggregate -> compose a grounded answer."""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List, Optional

from ..errors import ApiError, short_error
from ..rag import RagService
from ..text import excerpt, validate_query
from .planner import LLMPlanner, Planner
from .runtime import Aggregator, Executor
from .tools import build_tools
from .types import CombinedContext, ToolResult

log = logging.getLogger("ai.agent")

MAX_PROMPT_SOURCES = 8
MAX_NOTE_CHARS = 1500


def build_final_prompt(query: str, context: CombinedContext, results: List[ToolResult]) -> str:
    lines = [
        "You are a grounded RSS assistant. Answer the user question using the retrieved articles "
        "only. Do not invent facts. Cite the article titles and URLs when referencing sources.\n",
        f"User question: {query}",
        "\nRetrieved context:",
    ]
    for i, source in enumerate(context.sources[:MAX_PROMPT_SOURCES], start=1):
        lines.append(f"{i}. {source.title} ({source.url})")
        if source.description:
            lines.append(f"Description: {excerpt(source.description)}")

    notes = [
        (r.name, str(r.data["answer"])[:MAX_NOTE_CHARS])
        for r in results
        if r.data.get("answer") and not r.data.get("llm_fallback")
    ]
    if notes:
        lines.append("\nDraft notes from tools (verify against the articles above):")
        lines.extend(f"- {name}: {note}" for name, note in notes)

    lines.append("\nFinal answer should be concise, useful, and grounded in the provided sources.")
    return "\n".join(lines)


def build_fallback_answer(query: str, context: CombinedContext) -> str:
    if not context.sources:
        return "I could not find enough relevant context for that query."
    items = [f"- {s.title} ({s.url})" for s in context.sources if s.url]
    if not items:
        return "I found relevant content, but no source URLs were available to cite."
    return f'Based on the available articles, here is the most relevant answer to "{query}":\n' + "\n".join(items)


class Agent:
    def __init__(self, rag: RagService, use_llm_planner: bool = False, max_chars: int = 12000):
        self._rag = rag
        tools = build_tools(rag, max_chars)
        self._executor = Executor(tools)
        self._aggregator = Aggregator()
        if use_llm_planner:
            descriptions = {name: tool.description for name, tool in tools.items()}
            self._planner = LLMPlanner(rag.llm, descriptions)
        else:
            self._planner = Planner()

    async def run(self, user_id: str, raw_query: str) -> Dict[str, Any]:
        query = validate_query(raw_query)
        started = time.monotonic()

        plan = await self._planner.plan(query)
        results, errors = await self._executor.run(user_id, query, plan)
        context = self._aggregator.aggregate(results)
        answer, note = await self._finalize(query, context, results)
        if note:
            errors.append(note)

        return {
            "answer": answer,
            "sources": [s.to_dict() for s in context.sources],
            "plan": [step.name for step in plan],
            "tools_used": [r.name for r in results],
            "execution_time_ms": int((time.monotonic() - started) * 1000),
            "tool_results": len(results),
            "errors": errors,
        }

    async def _finalize(self, query: str, context: CombinedContext, results: List[ToolResult]):
        """Returns ``(answer, note)``; ``note`` explains why the plain fallback was used."""
        llm: Optional[Any] = self._rag.llm
        note = None
        if llm is None:
            note = "answer: AI generation is not configured"
        elif context.sources:
            try:
                answer = await llm.generate_text(build_final_prompt(query, context, results))
                if answer.strip():
                    return answer, None
            except Exception as exc:
                log.warning("final answer generation failed: %s", exc)
                note = f"answer: {short_error(exc)}"
        return build_fallback_answer(query, context), note


def require_query(payload: Any, query_string_value: Optional[str]) -> str:
    """Accept the question from the JSON body or ``?q=``."""
    query = ""
    if isinstance(payload, dict):
        query = str(payload.get("query") or "").strip()
    if not query:
        query = (query_string_value or "").strip()
    if not query:
        raise ApiError(400, "query is required")
    return query
