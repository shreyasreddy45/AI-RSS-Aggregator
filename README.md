# Marginalia — AI RSS Aggregator

An RSS reader with a web frontend. Go handles accounts, feeds and scraping; a separate **Python service** handles everything AI: summaries, embeddings, question answering (RAG), the research agent, and semantic search.

```
 Browser ──► Go API (:8080) ───────────────► PostgreSQL (+ pgvector)
             • serves the web frontend         ▲
             • JWT auth, feeds, posts          │
             • scraper + job queue             │
             • rate limiting, Redis cache      │
                   │  X-Internal-Token          │
                   │  X-User-ID                 │
                   ▼                            │
             Python AI service (:8000, internal)┘
             • summaries + embeddings (Gemini / OpenAI)
             • /ask (RAG), /agent/chat, /search (semantic, hybrid)
```

The browser only ever talks to the Go API. Go validates the user's JWT, then forwards AI requests to the Python service over the private Docker network, authenticated with a shared secret. The Python service is never published to the host.

## What changed from the original

* **Frontend added.** A single-page app in `internal/web/static/`, embedded in the Go binary and served at `/`. Views: Today, For you, Search (keywords / meaning / both), Ask (quick answer or agent), Discover (add and follow feeds).
* **AI moved to Python** (`ai_service/`, FastAPI). The Go packages `summarizer`, `embeddings`, `rag` and `agent` are gone. Go keeps a thin client (`internal/aiclient`) and proxy handlers, so the public endpoints and paths are unchanged.
* **MCP removed:** `cmd/mcpserver`, `internal/mcpserver`, the MCP SDK dependency and its docs. The `cmd/evalrag` tool was removed with the Go RAG code it exercised.
* **Background jobs** now send only a post ID to the AI service, which creates whatever is missing (summary and/or embedding). It is idempotent, so retries are safe. Previously only summaries were generated automatically, so Ask and semantic search had nothing to search until you called the embeddings endpoint per post.
* **Fixes made along the way** are listed under [Notes](#notes).

## Quick start

```bash
cp .env.example .env
# edit .env: set JWT_SECRET, AI_SERVICE_TOKEN and GEMINI_API_KEY
docker compose up --build
```

Open <http://localhost:8080>, create an account, then add a feed in **Discover**. New posts appear within a minute, summaries and embeddings shortly after.

Without `GEMINI_API_KEY` the reader works normally; summaries, Ask, the agent and semantic search report that AI is unavailable. Keyword search always works.

## Configuration

| Variable | Used by | Purpose |
|---|---|---|
| `DB_URL`, `REDIS_ADDR`, `JWT_SECRET`, `PORT` | Go | As before |
| `SCRAPE_INTERVAL`, `MAX_CONCURRENCY`, `DB_QUERY_TIMEOUT` | Go | As before |
| `AI_SERVICE_URL` | Go | Where the Python service lives (`http://ai-service:8000`). Empty disables AI features |
| `AI_SERVICE_TOKEN` | Go + Python | Shared secret. The Python service refuses to start without it |
| `AI_REQUEST_TIMEOUT` | Go | Longest wait for an AI response (default `60s`) |
| `GEMINI_API_KEY`, `GEMINI_MODEL` | Python | Text generation. Default model is the `gemini-flash-latest` alias |
| `EMBEDDING_PROVIDER`, `EMBEDDING_MODEL`, `EMBEDDING_DIMENSION` | Python | `gemini` or `openai`; dimension must match the `vector(1536)` column |
| `EMBEDDING_API_KEY`, `OPENAI_API_KEY` | Python | Optional separate embedding key |
| `LLM_REQUEST_TIMEOUT` | Python | Per-call timeout for LLM requests |
| `AGENT_LLM_PLANNER` | Python | `true` lets the model pick the agent's tools; default is keyword rules |

## Python AI service

```
ai_service/app/
├── main.py          FastAPI routes, internal auth, error shape
├── services.py      builds the object graph
├── config.py        environment settings
├── llm.py           Gemini text generation
├── embeddings.py    Gemini / OpenAI embeddings
├── pipeline.py      per-post summary + embedding
├── rag.py           retrieval, hybrid ranking, answers
├── search.py        GET /search (semantic, hybrid)
├── db.py            Postgres access (every query scoped to the user's followed feeds)
└── agent/           planner, tools, executor, aggregator
```

Internal endpoints (not reachable from the browser):

| Endpoint | Called by |
|---|---|
| `POST /internal/posts/{id}/process` | Go job queue: create the missing summary and embedding |
| `GET /internal/posts/pending` | Go startup backfill |
| `GET\|POST /ask`, `POST /agent/chat`, `GET /search`, `POST /posts/{id}/embeddings` | Proxied from the matching `/v1/...` Go routes |
| `GET /health`, `GET /health/ready` | Compose healthcheck / readiness |

Hybrid ranking is unchanged: `0.6 × cosine similarity + 0.25 × full-text rank + 0.15 × recency` (7-day decay).

### Run locally without Docker

```bash
# Postgres with pgvector + Redis running locally, migrations applied
cd ai_service
python -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
export DB_URL=postgres://postgres:postgres@localhost:5432/rss?sslmode=disable
export AI_SERVICE_TOKEN=dev GEMINI_API_KEY=...
uvicorn app.main:create_app --factory --port 8000

# in another shell, the Go API
export AI_SERVICE_URL=http://localhost:8000 AI_SERVICE_TOKEN=dev
go run ./cmd/server
```

### Tests

```bash
cd ai_service && python -m unittest discover -s tests -t .   # standard library only
go test ./...
```

## Project layout

```
.
├── cmd/server/            Go entry point
├── internal/
│   ├── aiclient/          HTTP client for the Python service
│   ├── handlers/          REST handlers (ai.go proxies AI endpoints)
│   ├── web/static/        frontend (index.html, app.css, app.js)
│   └── ...                auth, scraper, queue, middleware, database (sqlc)
├── ai_service/            Python AI service
├── sql/                   goose migrations + sqlc queries
├── migration/             migration container
├── Dockerfile             Go API image
└── docker-compose.yml     postgres(pgvector), redis, migration, ai-service, app
```

## API Endpoints

### Users

| Method | Endpoint    | Description       |
| ------ | ----------- | ----------------- |
| POST   | `/v1/users` | Create a new user |

### Feeds

| Method | Endpoint    | Description   |
| ------ | ----------- | ------------- |
| POST   | `/v1/feeds` | Create a feed |
| GET    | `/v1/feeds` | Get all feeds |

### Feed Follows

| Method | Endpoint                | Description        |
| ------ | ----------------------- | ------------------ |
| POST   | `/v1/feed_follows`      | Follow a feed      |
| GET    | `/v1/feed_follows`      | Get followed feeds |
| DELETE | `/v1/feed_follows/{id}` | Unfollow a feed    |

### Posts

| Method | Endpoint    | Description                          |
| ------ | ----------- | ------------------------------------ |
| GET    | `/v1/posts` | Get posts for the authenticated user |

### Auth, search and AI

| Method | Endpoint | Description | Runs in |
|--------|----------|-------------|---------|
| POST | `/v1/login` | Email + password, returns `{ "token": ... }` | Go |
| GET | `/v1/search?q=&mode=text` | Keyword search | Go |
| GET | `/v1/search?q=&mode=semantic\|hybrid` | Vector / combined search | Python |
| GET | `/v1/recommendations` | Ranked recommendations | Go |
| GET | `/v1/posts/{id}/summary` | Stored AI summary (404 until generated) | Go (reads), Python (writes) |
| POST | `/v1/posts/{id}/summary` | Generate the summary now (used by the Summary button) | Python |
| POST | `/v1/posts/{id}/embeddings` | (Re)generate a post's embedding | Python |
| GET\|POST | `/v1/ask` | Answer a question from your articles | Python |
| POST | `/v1/agent/chat` | Tool-using agent | Python |

AI routes return `503` when the AI service or its provider key is not configured.

## Authentication

User-facing REST APIs use JWT bearer authentication.

Example:

```http
Authorization: Bearer <access_token>
```

API keys remain available for future internal service integrations only.

Example:

```http
Authorization: ApiKey <internal_service_key>
```

## Background Scraper

A background worker continuously checks for RSS feeds that need to be scraped. The scraping interval is configurable using the `SCRAPE_INTERVAL` environment variable.

## Graceful Shutdown

The application handles shutdown signals gracefully.

During shutdown it:

* Stops accepting new requests
* Allows active requests to complete
* Stops background workers
* Closes database connections
* Closes Redis connections
* Exits cleanly

## Docker

The application uses a multi-container Docker setup.

Services include:

* Go API (also serves the frontend)
* Python AI service (internal)
* PostgreSQL with pgvector
* Redis
* Migration service

Persistent Docker volumes are used so PostgreSQL data is preserved across container restarts.

## Notes

Things worth knowing, found while reading the original code:

* **Rotate your Gemini key.** The uploaded archive contained a `.env` with an API key. It is not included here; `.env` is now in `.gitignore`. If that key was ever committed or shared, revoke it and create a new one.
* **Post cache leaked between users.** `GET /v1/posts` was wrapped as `Cache(JWT(handler))`, so the cache key was built before the user was known and was shared across accounts. The generic cache wrapper is removed for that route; the handler already caches per user. Cache keys now share the invalidation prefix, and following/unfollowing/adding a feed clears them.
* **Agent tool names.** The original executor looked tools up as `search`, `rag`, … while the registry stored them as `SearchTool`, `RAGTool`, …, which would have missed every tool. The port registers tools under the names the planner emits.
* **Dates showing "Jan 1, 1".** The scraper only parsed RSS dates with a numeric zone (`+0000`), but many feeds (BBC) use `GMT`, so the date stayed at year 1. `internal/rss/dates.go` now accepts the common formats, and falls back to the fetch time. Repair rows scraped earlier with `UPDATE posts SET published_at = created_at WHERE published_at < '1971-01-01';`.
* **Summaries never appearing.** Posts that failed during an AI outage were only retried on restart; the backfill now runs every 15 minutes, provider errors are retried and shown in the UI instead of a generic message, and the Summary button generates one on demand.
* **Vector distance.** Queries now use cosine distance (`<=>`), matching the HNSW `vector_cosine_ops` index, instead of L2 (`<->`), which could not use it.
* **Postgres image.** `postgres:17` has no `vector` extension, so migration 011 would fail; compose now uses `pgvector/pgvector:pg17`.
* **Missing Dockerfile.** `docker-compose.yml` referenced a root `Dockerfile` that was not in the archive; one is included.

Not changed, but you may want to look at them:

* `POST /v1/users` and `GET /v1/users` return the raw database user, which includes the password hash and API key. The frontend ignores these fields, but the API should not send them.
* Login issues only a 15-minute access token (no refresh token), so the frontend sends you back to sign-in when it expires.
* `Login` compares against `pgx.ErrNoRows` while the code uses `database/sql`, so an unknown email probably returns 500 rather than 401.
* The RSS parser reads RSS 2.0 `<item>` entries only, not Atom.
* After pulling this change run `go mod tidy` once to drop the now-unused indirect dependencies, and `swag init -g cmd/server/main.go` if you regenerate the API docs.
* Neither the Go changes nor the SQL in `ai_service/app/db.py` could be compiled or run against Postgres in the environment where this was produced. The Python logic (44 tests) and the provider HTTP clients were run.

## Future Improvements

* GitHub Actions CI/CD
* Kubernetes deployment
* Prometheus metrics
* Grafana dashboards
* JWT authentication
* Rate limiting
* Unit and integration testing
* AWS deployment
* Terraform infrastructure

## License


This project is intended for learning and educational purposes.
