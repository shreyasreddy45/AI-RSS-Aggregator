# Load testing with k6

This directory contains a k6 script for exercising the application stack started by Docker Compose.

## Quick start

1. Start the app stack:

```bash
docker compose up -d --build
```

2. Wait for the API to become ready:

```bash
curl http://localhost:8080/v1/health
```

3. Run the load test:

```bash
k6 run loadtest/basic_load_test.js --env BASE_URL=http://localhost:8080
```

The script performs a single login once and reuses the resulting JWT for all subsequent requests. It exercises the main read-heavy routes with a ramp profile that includes both 50 VU and 100 VU phases.

## What the script measures

The workload mirrors the production route mix:

- 40% GET /v1/posts
- 25% GET /v1/search?q=...
- 20% GET /v1/recommendations
- 10% feed follow create/delete operations
- 5% POST /v1/agent/chat

The script includes pass/fail thresholds for p95 latency and error rate.

## Seeding realistic data first

If the database is empty, your numbers will mostly reflect cold-start and auth overhead rather than real application behavior. A realistic test should have a few users, a few feeds, and enough posts to populate the feed, recommendation, and search endpoints.

### Option A: seed through the API

Create a user and log in once:

```bash
curl -X POST http://localhost:8080/v1/users \
  -H 'Content-Type: application/json' \
  -d '{"name":"load tester","email":"loadtest@example.com","password":"StrongPass123!"}'

curl -X POST http://localhost:8080/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"loadtest@example.com","password":"StrongPass123!"}'
```

Then create a couple of feeds and follow them:

```bash
TOKEN="<jwt-from-login>"

curl -X POST http://localhost:8080/v1/feeds \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"AI News","url":"https://example.com/ai.xml"}'

curl -X POST http://localhost:8080/v1/feeds \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Startup Weekly","url":"https://example.com/startups.xml"}'

curl -X POST http://localhost:8080/v1/feedfollows \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"feed_id":"<feed-id-from-previous-call>"}'
```

### Option B: seed directly in Postgres

If you want a more realistic search and recommendation dataset, populate a few rows directly in the database:

```bash
docker compose exec postgres psql -U postgres -d rss <<'SQL'
INSERT INTO users (id, name, email, password_hash, created_at, updated_at, role)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'Tester 1', 'tester1@example.com', '$2a$10$fakehash', NOW(), NOW(), 'user'),
  ('22222222-2222-2222-2222-222222222222', 'Tester 2', 'tester2@example.com', '$2a$10$fakehash', NOW(), NOW(), 'user');

INSERT INTO feeds (id, name, created_at, updated_at, url, user_id)
VALUES
  ('33333333-3333-3333-3333-333333333333', 'AI News', NOW(), NOW(), 'https://example.com/ai.xml', '11111111-1111-1111-1111-111111111111'),
  ('44444444-4444-4444-4444-444444444444', 'Startup Weekly', NOW(), NOW(), 'https://example.com/startups.xml', '11111111-1111-1111-1111-111111111111'),
  ('55555555-5555-5555-5555-555555555555', 'Devops Digest', NOW(), NOW(), 'https://example.com/devops.xml', '22222222-2222-2222-2222-222222222222');

INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
VALUES
  ('66666666-6666-6666-6666-666666666666', NOW(), NOW(), '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333'),
  ('77777777-7777-7777-7777-777777777777', NOW(), NOW(), '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444');

WITH feed_list AS (
  SELECT id FROM feeds
)
SELECT 1;
SQL
```

For actual post data, insert enough rows to give search and recommendation endpoints meaningful bodies. For example:

```bash
docker compose exec postgres psql -U postgres -d rss <<'SQL'
WITH feed_rows AS (
  SELECT id FROM feeds
),
row_data AS (
  SELECT
    gen_random_uuid() AS id,
    'AI article #' || i AS title,
    'A realistic AI summary for load-testing search and recommendations ' || i AS description,
    NOW() - (i || ' minutes')::interval AS published_at,
    'https://example.com/posts/' || i AS url,
    (SELECT id FROM feeds ORDER BY id LIMIT 1) AS feed_id
  FROM generate_series(1, 200) AS i
)
INSERT INTO posts (id, title, description, created_at, updated_at, published_at, url, feed_id)
SELECT id, title, description, published_at, published_at, published_at, url, feed_id
FROM row_data;
SQL
```

This gives the main read-heavy endpoints enough rows to exercise indexing and filtering rather than a tiny empty table.

## Running with a custom user

If you want to reuse a specific seeded user instead of the default K6 user, set these before running:

```bash
export BASE_URL=http://localhost:8080
export TEST_EMAIL=loadtest@example.com
export TEST_PASSWORD=StrongPass123!
export TEST_NAME='k6 load user'

k6 run loadtest/basic_load_test.js --env BASE_URL="$BASE_URL" --env TEST_EMAIL="$TEST_EMAIL" --env TEST_PASSWORD="$TEST_PASSWORD" --env TEST_NAME="$TEST_NAME"
```

## Interpreting results

k6 prints:

- request counts
- throughput (requests/sec)
- p50 / p95 / p99 latency per request type
- total fail rate
- scenario-level ramp stats

The script includes thresholds so the run fails if the p95 latency exceeds 500 ms or the error rate exceeds 1%.
