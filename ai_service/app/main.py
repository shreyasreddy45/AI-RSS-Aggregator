"""HTTP entry point for the AI service.

The service is internal: only the Go API calls it. Every request must carry the
shared ``X-Internal-Token``; user-scoped endpoints also need ``X-User-ID``, which
the Go API sets after it has validated the caller's JWT.

Run with:  uvicorn app.main:create_app --factory --host 0.0.0.0 --port 8000
"""

from __future__ import annotations

import hmac
import json
import logging
import uuid
from contextlib import asynccontextmanager
from typing import Any, Optional

from fastapi import APIRouter, Depends, FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from .agent.agent import require_query
from .config import Settings, load_settings
from .db import PgRepository
from .errors import ApiError
from .search import parse_search_params
from .services import Services, build_services
from .text import validate_query

log = logging.getLogger("ai.main")


def _services(request: Request) -> Services:
    return request.app.state.services


def require_internal(request: Request) -> None:
    token = _services(request).settings.internal_token
    if not token:  # only reachable when AI_SERVICE_ALLOW_NO_AUTH=true
        return
    supplied = request.headers.get("x-internal-token", "")
    if not hmac.compare_digest(supplied.encode("utf-8"), token.encode("utf-8")):
        raise ApiError(401, "invalid internal token")


def current_user(request: Request) -> str:
    raw = request.headers.get("x-user-id", "").strip()
    try:
        return str(uuid.UUID(raw))
    except ValueError:
        raise ApiError(401, "missing or invalid X-User-ID header") from None


def _uuid_param(value: str, name: str) -> str:
    try:
        return str(uuid.UUID(value))
    except ValueError:
        raise ApiError(400, f"{name} must be a valid UUID") from None


async def _json_body(request: Request, required: bool = True) -> Any:
    raw = await request.body()
    if not raw.strip():
        if required:
            raise ApiError(400, "invalid JSON body")
        return {}
    try:
        return json.loads(raw)
    except ValueError:
        raise ApiError(400, "invalid JSON body") from None


router = APIRouter(dependencies=[Depends(require_internal)])


@router.post("/internal/posts/{post_id}/process")
async def process_post(post_id: str, request: Request):
    """Generate the summary and embedding that a post is still missing."""
    return await _services(request).pipeline.process(_uuid_param(post_id, "postID"))


@router.get("/internal/posts/pending")
async def pending_posts(request: Request, limit: int = 500):
    ids = await _services(request).pipeline.pending(limit)
    return {"post_ids": ids}


@router.post("/posts/{post_id}/embeddings")
async def embed_post(post_id: str, request: Request, user_id: str = Depends(current_user)):
    return await _services(request).pipeline.embed_for_user(user_id, _uuid_param(post_id, "postID"))


@router.post("/posts/{post_id}/summary")
async def summarize_post(post_id: str, request: Request, user_id: str = Depends(current_user)):
    return await _services(request).pipeline.summarize_for_user(user_id, _uuid_param(post_id, "postID"))


@router.api_route("/ask", methods=["GET", "POST"])
async def ask(request: Request, user_id: str = Depends(current_user)):
    svc = _services(request)
    query = (request.query_params.get("q") or "").strip()
    if not query and request.method == "POST":
        payload = await _json_body(request)
        query = str(payload.get("query") or "").strip() if isinstance(payload, dict) else ""
    clean = validate_query(query)
    return await svc.rag.generate_answer(user_id, clean, svc.settings.rag_context_limit)


@router.post("/agent/chat")
async def agent_chat(request: Request, user_id: str = Depends(current_user)):
    payload = await _json_body(request, required=False)
    query = require_query(payload, request.query_params.get("q"))
    return await _services(request).agent.run(user_id, query)


@router.get("/search")
async def search(request: Request, user_id: str = Depends(current_user)):
    params = parse_search_params(request.query_params)
    return await _services(request).search.search(user_id, params)


def create_app(settings: Optional[Settings] = None, services: Optional[Services] = None) -> FastAPI:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        repo = None
        if services is not None:
            app.state.services = services
        else:
            cfg = settings or load_settings()
            repo = PgRepository(cfg.database_url, cfg.db_query_timeout)
            await repo.open()
            app.state.services = build_services(cfg, repo)
            log.info(
                "AI service ready (llm=%s, embeddings=%s)",
                app.state.services.llm is not None,
                app.state.services.embedder is not None,
            )
        try:
            yield
        finally:
            if repo is not None:
                await repo.close()

    # No public docs: this service is not meant to be reached by browsers.
    app = FastAPI(title="RSS AI service", lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)

    @app.get("/health")
    async def health(request: Request):
        svc = _services(request)
        return {"status": "ok", "llm": svc.llm is not None, "embeddings": svc.embedder is not None}

    @app.get("/health/ready")
    async def ready(request: Request):
        try:
            await _services(request).repo.ping()
        except Exception as exc:
            log.error("readiness check failed: %s", exc)
            return JSONResponse({"error": "database is not reachable"}, status_code=503)
        return {"status": "ready"}

    app.include_router(router)

    # Error bodies use the same {"error": "..."} shape as the Go API.
    @app.exception_handler(ApiError)
    async def _api_error(_: Request, exc: ApiError):
        return JSONResponse({"error": exc.message}, status_code=exc.status)

    @app.exception_handler(StarletteHTTPException)
    async def _http_error(_: Request, exc: StarletteHTTPException):
        return JSONResponse({"error": str(exc.detail)}, status_code=exc.status_code)

    @app.exception_handler(RequestValidationError)
    async def _validation_error(_: Request, exc: RequestValidationError):
        return JSONResponse({"error": "invalid request parameters"}, status_code=400)

    @app.exception_handler(Exception)
    async def _unhandled(_: Request, exc: Exception):
        log.exception("unhandled error: %s", exc)
        return JSONResponse({"error": "Internal Server Error"}, status_code=500)

    return app
