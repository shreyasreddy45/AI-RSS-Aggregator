import asyncio
import unittest

from app.agent import planner as pl
from app.agent.agent import Agent, build_fallback_answer
from app.agent.runtime import Aggregator
from app.agent.tools import best_matching_post, detect_search_mode
from app.agent.types import CombinedContext, ToolResult
from app.errors import ApiError, ProviderError
from app.models import Candidate, build_sources
from app.pipeline import PostPipeline
from app.rag import (HybridWeights, RagService, build_rag_prompt, rank_candidates, recency_score,
                     score_candidate)
from tests.fakes import NOW, FakeEmbedder, FakeLLM, FakeRepo, make_post


def run(c):
    return asyncio.run(c)


class RankingTests(unittest.TestCase):
    def test_recency_decays(self):
        self.assertEqual(recency_score(NOW, 100, NOW), 1.0)
        self.assertLess(recency_score(NOW.replace(day=1), 7 * 86400, NOW), 0.05)

    def test_rank_prefers_similar_then_recent(self):
        a = Candidate(make_post(1, days_old=30), cosine=0.9, text_rank=0)
        b = Candidate(make_post(2, days_old=0), cosine=0.2, text_rank=0)
        c = Candidate(make_post(3, days_old=0), cosine=0.9, text_rank=0)
        order = [x.post.id[-1] for x in rank_candidates([a, b, c], HybridWeights(), NOW)]
        self.assertEqual(order, ["3", "1", "2"])
        self.assertGreater(score_candidate(c, HybridWeights(), NOW), score_candidate(a, HybridWeights(), NOW))

    def test_prompt_contains_sources(self):
        s = build_sources([make_post(1, description="<p>Hi</p>")])
        p = build_rag_prompt("why?", s)
        self.assertIn("https://example.com/1", p)
        self.assertIn("Hi", p)
        self.assertNotIn("<p>", p)
        self.assertIn("none available", build_rag_prompt("q", []))


class RagServiceTests(unittest.TestCase):
    def svc(self, repo, llm=None, emb=None):
        return RagService(repo, llm, emb, clock=lambda: NOW)

    def test_answer_hybrid(self):
        posts = [make_post(1, "Redis news"), make_post(2, "Go news")]
        repo = FakeRepo(posts, [Candidate(posts[0], 0.9, 1.0), Candidate(posts[1], 0.1, 0)])
        out = run(self.svc(repo, FakeLLM("grounded"), FakeEmbedder()).generate_answer("u", "redis?", 1))
        self.assertEqual(out["answer"], "grounded")
        self.assertEqual([s["title"] for s in out["sources"]], ["Redis news"])
        self.assertEqual(out["retrieval"], "hybrid")

    def test_no_llm_is_503(self):
        with self.assertRaises(ApiError) as c:
            run(self.svc(FakeRepo(), None, None).generate_answer("u", "q"))
        self.assertEqual(c.exception.status, 503)

    def test_falls_back_to_recent_without_embedder(self):
        repo = FakeRepo([make_post(1)])
        r = run(self.svc(repo).retrieve_hybrid("u", "q", 5))
        self.assertEqual((r.method, len(r.posts)), ("recent", 1))

    def test_keyword_fallback_when_nothing_embedded(self):
        repo = FakeRepo([make_post(1, "Kubernetes tips")], candidates=[])
        r = run(self.svc(repo, None, FakeEmbedder()).retrieve_hybrid("u", "show me kubernetes", 5))
        self.assertEqual(r.method, "keyword")
        self.assertEqual(repo.searches[-1], "kubernetes")

    def test_quota_failure_returns_sources_with_notice(self):
        repo = FakeRepo([make_post(1)])
        out = run(self.svc(repo, FakeLLM(fail=ProviderError("quota", 429))).generate_answer("u", "q"))
        self.assertIn("quota", out["answer"])
        self.assertEqual(len(out["sources"]), 1)

    def test_hard_llm_failure_is_502(self):
        repo = FakeRepo([make_post(1)])
        with self.assertRaises(ApiError) as c:
            run(self.svc(repo, FakeLLM(fail=ProviderError("bad key", 400))).generate_answer("u", "q"))
        self.assertEqual(c.exception.status, 502)

    def test_empty_corpus(self):
        out = run(self.svc(FakeRepo(), FakeLLM()).generate_answer("u", "q"))
        self.assertEqual(out["sources"], [])


class PlannerTests(unittest.TestCase):
    def plan(self, q):
        return [s.name for s in run(pl.Planner().plan(q))]

    def test_routes(self):
        self.assertEqual(self.plan("latest news"), ["search"])
        self.assertEqual(self.plan("recommend something interesting"), ["recommendation"])
        self.assertEqual(self.plan("explain how redis is implemented"), ["rag", "semantic_search"])
        self.assertEqual(self.plan("summarize the latest"), ["search", "summary"])
        self.assertEqual(self.plan("hello"), ["search"])
        self.assertEqual(self.plan(""), ["search"])

    def test_llm_plan_parsing(self):
        steps = pl.parse_llm_plan('```json\n[{"tool_name":"RAGTool","reason":"r"},{"tool_name":"nope"},{"tool_name":"rag"}]\n```')
        self.assertEqual([s.name for s in steps], ["rag"])
        self.assertEqual(pl.parse_llm_plan("not json"), [])

    def test_llm_planner_falls_back(self):
        p = pl.LLMPlanner(FakeLLM(answer="garbage"), {"search": "d"})
        self.assertEqual([s.name for s in run(p.plan("latest news"))], ["search"])
        p = pl.LLMPlanner(FakeLLM(fail=ProviderError("x")), {"search": "d"})
        self.assertEqual([s.name for s in run(p.plan("latest news"))], ["search"])


class ToolHelperTests(unittest.TestCase):
    def test_detect_mode(self):
        self.assertEqual(detect_search_mode("redis"), "keyword")
        self.assertEqual(detect_search_mode("how does redis persistence work"), "hybrid")

    def test_best_match(self):
        a, b = make_post(1, "Cooking", days_old=0), make_post(2, "Redis internals", days_old=5)
        self.assertEqual(best_matching_post([a, b], "redis").id, b.id)

    def test_aggregator_dedupes_and_sorts(self):
        p1, p2 = make_post(1, days_old=5), make_post(2, days_old=1)
        r1 = ToolResult("a", sources=build_sources([p1, p2]))
        r2 = ToolResult("b", sources=build_sources([p1]))
        ctx = Aggregator().aggregate([r1, r2])
        self.assertEqual([s.id[-1] for s in ctx.sources], ["2", "1"])


class AgentTests(unittest.TestCase):
    def test_end_to_end(self):
        posts = [make_post(1, "Redis caching guide"), make_post(2, "Other")]
        repo = FakeRepo(posts, [Candidate(posts[0], 0.9, 0.5)])
        rag = RagService(repo, FakeLLM("final answer"), FakeEmbedder(), clock=lambda: NOW)
        out = run(Agent(rag).run("u", "explain redis caching"))
        self.assertEqual(out["answer"], "final answer")
        self.assertEqual(out["plan"], ["rag", "semantic_search"])
        self.assertEqual(sorted(out["tools_used"]), ["rag", "semantic_search"])
        self.assertTrue(out["sources"])
        self.assertEqual(out["errors"], [])

    def test_tool_failure_is_reported_not_fatal(self):
        rag = RagService(FakeRepo(), None, None, clock=lambda: NOW)  # nothing indexed, no LLM
        out = run(Agent(rag).run("u", "latest news"))
        self.assertEqual(out["tools_used"], [])
        self.assertTrue(out["errors"])
        self.assertIn("could not find", out["answer"])

    def test_fallback_when_final_llm_fails(self):
        posts = [make_post(1, "Redis")]
        repo = FakeRepo(posts)
        rag = RagService(repo, FakeLLM(fail=ProviderError("down")), None, clock=lambda: NOW)
        out = run(Agent(rag).run("u", "latest redis"))
        self.assertIn("https://example.com/1", out["answer"])

    def test_empty_query_rejected(self):
        with self.assertRaises(ApiError):
            run(Agent(RagService(FakeRepo(), None, None)).run("u", "  "))

    def test_fallback_answer(self):
        self.assertIn("could not find", build_fallback_answer("q", CombinedContext()))


class PipelineTests(unittest.TestCase):
    def test_process_creates_missing_and_is_idempotent(self):
        post = make_post(1, description="<p>desc</p>")
        repo = FakeRepo([post])
        pipe = PostPipeline(repo, FakeLLM(), FakeEmbedder())
        out = run(pipe.process(post.id))
        self.assertEqual((out["summary"], out["embedding"]), ("created", "created"))
        self.assertIn(post.id, repo.summaries)
        self.assertEqual(len(repo.embeddings[post.id]), 3)
        out = run(pipe.process(post.id))
        self.assertEqual((out["summary"], out["embedding"]), ("skipped", "skipped"))

    def test_failure_raises_502_but_keeps_other_step(self):
        post = make_post(1)
        repo = FakeRepo([post])
        pipe = PostPipeline(repo, FakeLLM(fail=ProviderError("quota", 429)), FakeEmbedder())
        with self.assertRaises(ApiError) as c:
            run(pipe.process(post.id))
        self.assertEqual(c.exception.status, 502)
        self.assertIn(post.id, repo.embeddings)
        self.assertNotIn(post.id, repo.summaries)

    def test_missing_post_404_and_disabled_providers(self):
        with self.assertRaises(ApiError) as c:
            run(PostPipeline(FakeRepo(), None, None).process("00000000-0000-0000-0000-000000000009"))
        self.assertEqual(c.exception.status, 404)
        post = make_post(1)
        out = run(PostPipeline(FakeRepo([post]), None, None).process(post.id))
        self.assertEqual((out["summary"], out["embedding"]), ("disabled", "disabled"))

    def test_pending_and_embed_for_user(self):
        post = make_post(1)
        repo = FakeRepo([post])
        pipe = PostPipeline(repo, FakeLLM(), FakeEmbedder())
        self.assertEqual(run(pipe.pending(10)), [post.id])
        out = run(pipe.embed_for_user("u", post.id))
        self.assertEqual((out["dimension"], out["provider"]), (3, "fake"))
        with self.assertRaises(ApiError) as c:
            run(PostPipeline(repo, None, None).embed_for_user("u", post.id))
        self.assertEqual(c.exception.status, 503)


if __name__ == "__main__":
    unittest.main()
