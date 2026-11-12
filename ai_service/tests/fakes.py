from datetime import datetime, timedelta, timezone

from app.errors import ProviderError
from app.models import Candidate, Post

NOW = datetime(2026, 10, 29, 12, 0, tzinfo=timezone.utc)


def make_post(i, title=None, days_old=1, description=None, content=None):
    return Post(
        id=f"00000000-0000-0000-0000-{i:012d}",
        title=title or f"Post {i}",
        url=f"https://example.com/{i}",
        feed_id="11111111-1111-1111-1111-111111111111",
        published_at=NOW - timedelta(days=days_old),
        description=description,
        content=content,
    )


class FakeLLM:
    model = "fake-model"

    def __init__(self, answer="LLM answer", fail=None):
        self.answer, self.fail, self.prompts = answer, fail, []

    async def generate_text(self, prompt):
        self.prompts.append(prompt)
        if self.fail:
            raise self.fail
        return self.answer

    async def summarize(self, text):
        self.prompts.append(text)
        if self.fail:
            raise self.fail
        return "SUMMARY: " + text[:20]


class FakeEmbedder:
    provider, model = "fake", "fake-embed"

    def __init__(self, fail=False):
        self.fail, self.calls = fail, []

    async def generate(self, text):
        self.calls.append(text)
        if self.fail:
            raise ProviderError("embed down")
        return [0.1, 0.2, 0.3]


class FakeRepo:
    def __init__(self, posts=None, candidates=None):
        self.posts = {p.id: p for p in (posts or [])}
        self.candidates = candidates or []
        self.summaries, self.embeddings = {}, {}
        self.searches = []

    async def ping(self):
        pass

    async def get_post(self, post_id):
        return self.posts.get(post_id)

    async def get_post_for_user(self, user_id, post_id):
        return self.posts.get(post_id)

    async def processing_needs(self, post_id):
        return post_id not in self.summaries, post_id not in self.embeddings

    async def upsert_summary(self, post_id, summary, model):
        self.summaries[post_id] = summary

    async def upsert_embedding(self, post_id, vector, provider, model):
        self.embeddings[post_id] = vector

    async def pending_post_ids(self, limit, want_summary, want_embedding):
        return [
            i for i in self.posts
            if (want_summary and i not in self.summaries) or (want_embedding and i not in self.embeddings)
        ][:limit]

    async def recent_posts(self, user_id, limit, search=None):
        self.searches.append(search)
        posts = sorted(self.posts.values(), key=lambda p: p.published_at, reverse=True)
        if search:
            posts = [p for p in posts if search.lower() in p.title.lower()]
        return posts[:limit]

    async def vector_candidates(self, user_id, vector, query_text, limit):
        return self.candidates[:limit]

    async def search_semantic(self, user_id, vector, params):
        return 1, [{"id": "x", "rank": 0.9}]

    async def search_hybrid(self, user_id, vector, params):
        return 2, [{"id": "y", "rank": 0.5}]
