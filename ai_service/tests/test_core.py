import asyncio
import unittest

from app import config, text
from app.errors import ApiError, ProviderError, is_quota_error, should_fallback_on_llm_failure
from app.search import SearchService, parse_search_params


def run(coro):
    return asyncio.run(coro)


class TextTests(unittest.TestCase):
    def test_validate_query(self):
        self.assertEqual(text.validate_query("  hello   world "), "hello world")
        with self.assertRaises(ApiError):
            text.validate_query("   ")
        with self.assertRaises(ApiError):
            text.validate_query("x" * 501)

    def test_normalize_search_query_drops_filler(self):
        self.assertEqual(text.normalize_search_query("Show me the latest AI news!"), "ai")
        self.assertEqual(text.normalize_search_query("what is redis?"), "is redis")
        self.assertEqual(text.normalize_search_query("   "), "")

    def test_strip_html(self):
        html = "<p>Hello <b>world</b> &amp; friends</p><script>x()</script><p>Second</p>"
        self.assertEqual(text.strip_html(html), "Hello world & friends\nSecond")
        self.assertEqual(text.strip_html(None), "")

    def test_source_text_prefers_richest(self):
        self.assertEqual(text.summary_source_text("T", None, None), "T")
        self.assertEqual(text.summary_source_text("T", "<p>desc</p>", None), "desc")
        self.assertEqual(text.summary_source_text("T", "d", "much longer content"), "much longer content")

    def test_embedding_text_does_not_duplicate_content(self):
        out = text.embedding_source_text("T", "same", "same")
        self.assertEqual(out, "Title:\nT\n\nDescription:\nsame")

    def test_excerpt_truncates(self):
        out = text.excerpt("a" * 600)
        self.assertEqual(len(out), 500)
        self.assertTrue(out.endswith("..."))
        self.assertEqual(text.excerpt(""), "No description available.")


class ConfigTests(unittest.TestCase):
    def test_parse_duration(self):
        self.assertEqual(config.parse_duration("30s", 1), 30)
        self.assertEqual(config.parse_duration("1m30s", 1), 90)
        self.assertEqual(config.parse_duration("500ms", 1), 0.5)
        self.assertEqual(config.parse_duration("", 7), 7)
        with self.assertRaises(ValueError):
            config.parse_duration("abc", 1)

    def test_requires_token_unless_allowed(self):
        with self.assertRaises(ValueError):
            config.load_settings({"DB_URL": "postgres://x"})
        s = config.load_settings({"DB_URL": "postgres://x", "AI_SERVICE_ALLOW_NO_AUTH": "true"})
        self.assertFalse(s.llm_enabled)

    def test_embedding_key_resolution(self):
        base = {"DB_URL": "x", "AI_SERVICE_TOKEN": "t", "GEMINI_API_KEY": "g"}
        s = config.load_settings(base)
        self.assertEqual((s.embedding_provider, s.embedding_api_key, s.embedding_model),
                         ("gemini", "g", "gemini-embedding-001"))
        s = config.load_settings({**base, "EMBEDDING_PROVIDER": "openai", "OPENAI_API_KEY": "o"})
        self.assertEqual((s.embedding_api_key, s.embedding_model), ("o", "text-embedding-3-small"))
        s = config.load_settings({**base, "EMBEDDING_API_KEY": "e"})
        self.assertEqual(s.embedding_api_key, "e")


class ErrorTests(unittest.TestCase):
    def test_classification(self):
        self.assertTrue(is_quota_error(ProviderError("x", status=429)))
        self.assertTrue(is_quota_error(ProviderError("Resource exhausted")))
        self.assertFalse(is_quota_error(ProviderError("bad request")))
        self.assertTrue(should_fallback_on_llm_failure(ProviderError("x", status=503)))
        self.assertFalse(should_fallback_on_llm_failure(ProviderError("invalid api key", status=400)))


class SearchTests(unittest.TestCase):
    def test_parse_ok(self):
        p = parse_search_params({"q": " a  b ", "limit": "5", "mode": "hybrid", "sort": "newest",
                                 "from": "2026-01-01", "to": "2026-01-31",
                                 "feed_id": "123e4567-e89b-12d3-a456-426614174000"})
        self.assertEqual((p.query, p.limit, p.mode, p.sort), ("a b", 5, "hybrid", "newest"))
        self.assertEqual(str(p.date_to_exclusive), "2026-02-01")

    def test_parse_rejects(self):
        for bad in ({}, {"q": "x", "limit": "0"}, {"q": "x", "sort": "z"}, {"q": "x", "mode": "text"},
                    {"q": "x", "feed_id": "nope"}, {"q": "x", "from": "01/01/2026"},
                    {"q": "x", "from": "2026-02-01", "to": "2026-01-01"}):
            with self.assertRaises(ApiError, msg=str(bad)):
                parse_search_params(bad)

    def test_service_modes_and_missing_embedder(self):
        from tests.fakes import FakeEmbedder, FakeRepo
        svc = SearchService(FakeRepo(), FakeEmbedder())
        out = run(svc.search("u", parse_search_params({"q": "x", "mode": "hybrid"})))
        self.assertEqual((out["total"], out["mode"], out["results"][0]["id"]), (2, "hybrid", "y"))
        out = run(svc.search("u", parse_search_params({"q": "x"})))
        self.assertEqual(out["results"][0]["id"], "x")
        with self.assertRaises(ApiError) as ctx:
            run(SearchService(FakeRepo(), None).search("u", parse_search_params({"q": "x"})))
        self.assertEqual(ctx.exception.status, 503)
        with self.assertRaises(ApiError) as ctx:
            run(SearchService(FakeRepo(), FakeEmbedder(fail=True)).search("u", parse_search_params({"q": "x"})))
        self.assertEqual(ctx.exception.status, 502)


if __name__ == "__main__":
    unittest.main()
