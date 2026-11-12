"""Builds the object graph once at start-up."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any, Optional

from .agent.agent import Agent
from .config import Settings
from .embeddings import EmbeddingClient, build_embedder
from .llm import GeminiClient, build_llm
from .pipeline import PostPipeline
from .rag import RagService
from .search import SearchService

log = logging.getLogger("ai.services")


@dataclass
class Services:
    settings: Settings
    repo: Any
    llm: Optional[GeminiClient]
    embedder: Optional[EmbeddingClient]
    rag: RagService
    search: SearchService
    pipeline: PostPipeline
    agent: Agent


def build_services(settings: Settings, repo: Any) -> Services:
    llm = build_llm(settings.gemini_api_key, settings.gemini_model, settings.llm_timeout)
    embedder = build_embedder(
        settings.embedding_provider,
        settings.embedding_api_key,
        settings.embedding_model,
        settings.embedding_dimension,
        settings.embedding_timeout,
    )
    if llm is None:
        log.warning("GEMINI_API_KEY is not set: summaries, answers and the agent's LLM steps are disabled")
    if embedder is None:
        log.warning("No embedding API key: embeddings and semantic search are disabled")

    rag = RagService(repo, llm, embedder)
    return Services(
        settings=settings,
        repo=repo,
        llm=llm,
        embedder=embedder,
        rag=rag,
        search=SearchService(repo, embedder),
        pipeline=PostPipeline(repo, llm, embedder, settings.max_input_chars),
        agent=Agent(rag, settings.agent_llm_planner, settings.max_input_chars),
    )
