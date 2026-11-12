"""Executes a plan and merges the tools' output."""

from __future__ import annotations

import asyncio
import logging
from typing import Any, Dict, List, Mapping, Tuple

from ..models import Source
from .types import CombinedContext, ExecutionStep, ToolError, ToolResult

log = logging.getLogger("ai.agent.runtime")


class Executor:
    def __init__(self, tools: Mapping[str, Any]):
        self._tools = tools

    async def run(
        self, user_id: str, query: str, plan: List[ExecutionStep]
    ) -> Tuple[List[ToolResult], List[str]]:
        """Run every planned tool concurrently. Returns (results, error messages).

        One failing tool never sinks the others: its error is reported and the
        remaining results are still used.
        """
        runnable: List[Tuple[ExecutionStep, Any]] = []
        errors: List[str] = []
        for step in plan:
            tool = self._tools.get(step.name)
            if tool is None:
                errors.append(f"{step.name}: tool not found")
                continue
            runnable.append((step, tool))

        outcomes = await asyncio.gather(
            *(tool.execute(user_id, query) for _, tool in runnable), return_exceptions=True
        )

        results: List[ToolResult] = []
        for (step, _), outcome in zip(runnable, outcomes):
            if isinstance(outcome, ToolError):
                errors.append(f"{step.name}: {outcome}")
            elif isinstance(outcome, asyncio.CancelledError):
                raise outcome
            elif isinstance(outcome, BaseException):
                log.warning("tool %s failed: %s", step.name, outcome)
                errors.append(f"{step.name}: {outcome}")
            else:
                results.append(outcome)
        return results, errors


class Aggregator:
    """De-duplicates sources by URL and orders them newest first."""

    def aggregate(self, results: List[ToolResult]) -> CombinedContext:
        seen = set()
        sources: List[Source] = []
        metadata: Dict[str, Any] = {}
        for result in results:
            for key, value in result.metadata.items():
                if value is not None:
                    metadata[key] = value
            for source in result.sources:
                if not source.url or source.url in seen:
                    continue
                seen.add(source.url)
                sources.append(source)

        sources.sort(key=lambda s: s.title)  # stable secondary key
        sources.sort(key=lambda s: _sort_time(s), reverse=True)
        return CombinedContext(sources=sources, metadata=metadata)


def _sort_time(source: Source) -> float:
    if source.published_at is None:
        return 0.0
    value = source.published_at
    if value.tzinfo is None:
        from datetime import timezone

        value = value.replace(tzinfo=timezone.utc)
    return value.timestamp()
