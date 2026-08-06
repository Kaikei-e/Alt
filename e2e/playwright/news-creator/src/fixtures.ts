import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { testToken, uuid } from "../../_shared/ids.js";
import { env } from "./env.js";

/**
 * Suite-wide fixtures and request builders.
 *
 * news-creator is stateless — it holds no database and persists nothing — so
 * "seeding" here does not mean writing rows. It means every test minting the
 * identifiers it will later assert were echoed back: its own `article_id`, its
 * own `job_id`, its own query string. That is what replaces the Hurl suite's
 * `--jobs 1`.
 *
 * The Hurl suite ran serially because "HybridPrioritySemaphore state and queue
 * depth are shared" (e2e/hurl/news-creator/README.md). That is true of the
 * *counters*, and the one spec that reads them lives in its own single-worker
 * project. It was never true of the request/response pairs: nothing in
 * `03-summarize` could observe anything `05-recap-summary` did. Every other
 * spec here is order-free, and asserting the echo of a per-test identifier is
 * what proves it — a response assembled from a previous caller's request would
 * fail, where the Hurl suite's fixed `hurl-e2e-article-normal` could not tell.
 *
 * The HTTP client is worker-scoped: `APIRequestContext` is a connection pool,
 * and rebuilding one per test would add a TCP+TLS handshake to every
 * assertion for no isolation gain against a service that keeps no per-client
 * state.
 */

type WorkerFixtures = {
	/** The one news-creator listener, :11434. */
	api: APIRequestContext;
};

export type Seed = {
	/** Unique to (dispatch, worker, test). Safe in an id, a slug or a query. */
	readonly token: string;
	/** `article_id` for the summarize surface — echoed back in the response. */
	readonly articleId: string;
	/** A fresh RFC 4122 v4 UUID; `RecapSummaryRequest.job_id` demands one. */
	readonly jobId: string;
};

type TestFixtures = {
	seed: Seed;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	api: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: { "Content-Type": "application/json" },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	seed: async ({}, use, testInfo) => {
		const token = testToken(testInfo.workerIndex, testInfo.title);
		await use({ token, articleId: `e2e-${token}`, jobId: uuid() });
	},
});

export { expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

/**
 * The article body every summarize scenario posts.
 *
 * Carried over verbatim from `e2e/fixtures/news-creator/article-normal.json`
 * rather than regenerated, and for a specific reason: `summarize_usecase`
 * derives a token budget from the content length and refuses content whose
 * estimated prompt exceeds the context window, and the handler rejects
 * anything under 100 characters outright. This text is the one known to land
 * between those two guards, so a failure here is news-creator changing, not
 * the fixture drifting.
 */
export const ARTICLE_CONTENT =
	"OpenAI が新たな研究成果を発表した。 " +
	"大規模言語モデルの推論精度が改善され、長文ドキュメントの要約タスクで従来比 12% の品質向上を達成したと報告している。 " +
	"評価データセットには日本語と英語の両方が含まれており、多言語対応の進展が示唆されている。";

/** `SummarizeRequest` — `priority` is `pattern="^(high|low)$"`. */
export function summarizeBody(
	articleId: string,
	options: { stream?: boolean; priority?: "high" | "low" } = {},
): Record<string, unknown> {
	return {
		article_id: articleId,
		content: ARTICLE_CONTENT,
		stream: options.stream ?? false,
		priority: options.priority ?? "low",
	};
}

/** `GenerateRequest` — the Ollama pass-through. */
export function generateBody(prompt: string): Record<string, unknown> {
	return { prompt, model: env.stubModel, stream: false };
}

/** `RecapSummaryRequest` as recap-worker posts it. */
export function recapBody(jobId: string, genre: string): Record<string, unknown> {
	return {
		job_id: jobId,
		genre,
		clusters: [
			{
				cluster_id: 0,
				representative_sentences: [
					{
						text: "OpenAI が新しい大規模言語モデルを発表した。",
						published_at: "2026-04-17T09:00:00Z",
						source_url: "https://example.com/openai-announce",
						article_id: "art-recap-001",
						is_centroid: true,
					},
					{
						text: "推論性能が従来比で大幅に向上したと報告されている。",
						published_at: "2026-04-17T09:30:00Z",
						source_url: "https://example.com/openai-perf",
						article_id: "art-recap-002",
						is_centroid: false,
					},
				],
				top_terms: ["openai", "llm", "performance"],
			},
		],
		options: { max_bullets: 3, temperature: 0.3 },
	};
}

/** `MorningLetterRequest`. Omit `recap_summaries` to exercise the degraded path. */
export function morningLetterBody(
	targetDate: string,
	options: { withRecaps?: boolean } = {},
): Record<string, unknown> {
	const body: Record<string, unknown> = {
		target_date: targetDate,
		edition_timezone: "Asia/Tokyo",
		overnight_groups: [
			{
				group_id: uuid(),
				articles: [
					{ text: "Stub overnight article one for the morning letter test.", is_centroid: true },
					{ text: "Stub overnight article two providing additional content.", is_centroid: false },
				],
			},
		],
	};
	if (options.withRecaps ?? true) {
		body["recap_summaries"] = [
			{
				genre: "ai",
				title: "AI: stub recap title",
				bullets: ["stub bullet covering the AI genre overnight."],
				window_days: 3,
			},
		];
	}
	return body;
}

/** `ChatRequest` — `messages` is `min_length=1` and the model is strict+frozen. */
export function chatBody(content: string, stream: boolean): Record<string, unknown> {
	return {
		model: env.stubModel,
		messages: [{ role: "user", content }],
		stream,
	};
}
