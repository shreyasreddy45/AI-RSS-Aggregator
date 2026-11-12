"""Exercise the provider clients against a real local HTTP server."""
import asyncio
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from app.embeddings import EmbeddingClient, to_vector_literal
from app.errors import NotConfiguredError, ProviderError
from app.llm import GeminiClient


class Handler(BaseHTTPRequestHandler):
    seen = []
    responses = {}

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        Handler.seen.append((self.path, {k.lower(): v for k, v in self.headers.items()}, body))
        status, payload = Handler.responses.get(self.path.split("?")[0], (404, {"error": {"message": "nope"}}))
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *a):
        pass


class ClientTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 0), Handler)
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def setUp(self):
        Handler.seen, Handler.responses = [], {}

    def test_gemini_success_uses_header_key_and_skips_thought(self):
        Handler.responses["/models/m:generateContent"] = (200, {"candidates": [{"content": {"parts": [
            {"text": "thinking", "thought": True}, {"text": "Hello "}, {"text": "world"}]}}]})
        c = GeminiClient("KEY", "m", base_url=self.base)
        self.assertEqual(asyncio.run(c.generate_text("hi")), "Hello world")
        path, headers, body = Handler.seen[0]
        self.assertNotIn("KEY", path)
        self.assertEqual(headers["x-goog-api-key"], "KEY")
        self.assertEqual(body["contents"][0]["parts"][0]["text"], "hi")

    def test_gemini_errors(self):
        Handler.responses["/models/m:generateContent"] = (429, {"error": {"message": "Quota exceeded"}})
        c = GeminiClient("K", "m", base_url=self.base)
        with self.assertRaises(ProviderError) as ctx:
            asyncio.run(c.generate_text("hi"))
        self.assertEqual(ctx.exception.status, 429)
        self.assertIn("Quota exceeded", str(ctx.exception))
        Handler.responses["/models/m:generateContent"] = (200, {"candidates": []})
        with self.assertRaises(ProviderError):
            asyncio.run(c.generate_text("hi"))

    def test_gemini_unreachable_and_unconfigured(self):
        with self.assertRaises(ProviderError):
            asyncio.run(GeminiClient("K", "m", base_url="http://127.0.0.1:1", timeout=2).generate_text("x"))
        with self.assertRaises(NotConfiguredError):
            GeminiClient("", "m")

    def test_gemini_embedding_pads_and_truncates(self):
        Handler.responses["/models/e:embedContent"] = (200, {"embedding": {"values": [1, 2]}})
        c = EmbeddingClient("gemini", "K", "e", dimension=4, gemini_base_url=self.base)
        self.assertEqual(asyncio.run(c.generate("x")), [1.0, 2.0, 0.0, 0.0])
        self.assertEqual(Handler.seen[0][2]["outputDimensionality"], 4)
        c2 = EmbeddingClient("gemini", "K", "e", dimension=1, gemini_base_url=self.base)
        self.assertEqual(asyncio.run(c2.generate("x")), [1.0])

    def test_openai_embedding(self):
        Handler.responses["/v1/embeddings"] = (200, {"data": [{"embedding": [0.5, 0.25]}]})
        c = EmbeddingClient("openai", "K", "m", dimension=2, openai_url=self.base + "/v1/embeddings")
        self.assertEqual(asyncio.run(c.generate("x")), [0.5, 0.25])
        self.assertEqual(Handler.seen[0][1]["authorization"], "Bearer K")

    def test_embedding_error_and_literal(self):
        Handler.responses["/models/e:embedContent"] = (400, {"error": {"message": "bad"}})
        c = EmbeddingClient("gemini", "K", "e", dimension=2, gemini_base_url=self.base)
        with self.assertRaises(ProviderError):
            asyncio.run(c.generate("x"))
        with self.assertRaises(ProviderError):
            asyncio.run(c.generate("   "))
        self.assertEqual(to_vector_literal([1, 0.5]), "[1.0,0.5]")


if __name__ == "__main__":
    unittest.main()
