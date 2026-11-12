import asyncio
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from app import config
from app.agent.agent import Agent
from app.errors import ApiError, ProviderError
from app.llm import GeminiClient
from app.pipeline import PostPipeline
from app.rag import RagService
from tests.fakes import NOW, FakeEmbedder, FakeLLM, FakeRepo, make_post


def run(c):
    return asyncio.run(c)


class SummaryOnDemandTests(unittest.TestCase):
    def test_creates_and_stores_summary(self):
        post = make_post(1, description="<p>Body text</p>")
        repo = FakeRepo([post])
        out = run(PostPipeline(repo, FakeLLM(), None).summarize_for_user("u", post.id))
        self.assertTrue(out["summary"].startswith("SUMMARY"))
        self.assertEqual(out["model"], "fake-model")
        self.assertIn(post.id, repo.summaries)

    def test_errors(self):
        post = make_post(1)
        with self.assertRaises(ApiError) as c:
            run(PostPipeline(FakeRepo([post]), None, None).summarize_for_user("u", post.id))
        self.assertEqual(c.exception.status, 503)
        with self.assertRaises(ApiError) as c:
            run(PostPipeline(FakeRepo(), FakeLLM(), None).summarize_for_user("u", "00000000-0000-0000-0000-000000000009"))
        self.assertEqual(c.exception.status, 404)
        pipe = PostPipeline(FakeRepo([post]), FakeLLM(fail=ProviderError("model x not found", 404)), None)
        with self.assertRaises(ApiError) as c:
            run(pipe.summarize_for_user("u", post.id))
        self.assertEqual(c.exception.status, 502)
        self.assertIn("model x not found", c.exception.message)


class DiagnosticsTests(unittest.TestCase):
    def test_hard_failure_message_includes_reason(self):
        rag = RagService(FakeRepo([make_post(1)]), FakeLLM(fail=ProviderError("bad key", 400)), None, clock=lambda: NOW)
        with self.assertRaises(ApiError) as c:
            run(rag.generate_answer("u", "q"))
        self.assertIn("bad key", c.exception.message)

    def test_degraded_answer_carries_llm_error(self):
        rag = RagService(FakeRepo([make_post(1)]), FakeLLM(fail=ProviderError("overloaded", 503)), None, clock=lambda: NOW)
        out = run(rag.generate_answer("u", "q"))
        self.assertEqual(out["llm_error"], "overloaded")
        self.assertEqual(len(out["sources"]), 1)

    def test_agent_reports_final_answer_failure(self):
        repo = FakeRepo([make_post(1, "Redis")])
        rag = RagService(repo, FakeLLM(fail=ProviderError("down")), None, clock=lambda: NOW)
        out = run(Agent(rag).run("u", "latest redis"))
        self.assertTrue(any(e.startswith("answer:") for e in out["errors"]))

    def test_default_model_is_an_alias(self):
        s = config.load_settings({"DB_URL": "x", "AI_SERVICE_TOKEN": "t"})
        self.assertEqual(s.gemini_model, "gemini-flash-latest")


class SeqHandler(BaseHTTPRequestHandler):
    queue = []
    hits = 0

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        SeqHandler.hits += 1
        status, payload = SeqHandler.queue.pop(0)
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *a):
        pass


class RetryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 0), SeqHandler)
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def client(self):
        return GeminiClient("K", "m", base_url=self.base, retries=2, backoff=0)

    def test_retries_503_then_succeeds(self):
        ok = {"candidates": [{"content": {"parts": [{"text": "fine"}]}}]}
        SeqHandler.queue, SeqHandler.hits = [(503, {"error": {"message": "overloaded"}}), (200, ok)], 0
        self.assertEqual(run(self.client().generate_text("hi")), "fine")
        self.assertEqual(SeqHandler.hits, 2)

    def test_gives_up_after_retries(self):
        SeqHandler.queue, SeqHandler.hits = [(503, {"error": {"message": "overloaded"}})] * 3, 0
        with self.assertRaises(ProviderError) as c:
            run(self.client().generate_text("hi"))
        self.assertEqual((c.exception.status, SeqHandler.hits), (503, 3))

    def test_no_retry_for_client_errors_and_clear_404(self):
        SeqHandler.queue, SeqHandler.hits = [(404, {"error": {"message": "not found"}})], 0
        with self.assertRaises(ProviderError) as c:
            run(self.client().generate_text("hi"))
        self.assertEqual(SeqHandler.hits, 1)
        self.assertIn("GEMINI_MODEL", str(c.exception))


if __name__ == "__main__":
    unittest.main()
