import { test as base } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { uuid } from "../../_shared/ids.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { env } from "./env.js";
import { extractTagsSchema } from "./schemas.js";

/**
 * Suite-wide fixtures.
 *
 * There is nothing to seed. tag-generator holds no rows: `/api/v1/extract-tags`
 * is a pure function of its request body (auth_service.py:543-573), and the
 * Redis Streams round trip creates only a reply stream that mq-hub deletes in
 * its own `defer` (generate_tags_usecase.go:81-93). So the isolation problem
 * the rest of the fleet solves with per-worker naming reduces here to one
 * thing: **the article id each round-trip test uses must be its own**, so two
 * concurrent requests cannot be confused for one another when a reply is
 * mis-routed. `articleId` below is that, minted per test.
 *
 * Both HTTP clients are worker-scoped. They hold no state worth isolating —
 * a test-scoped copy would just re-create a connection pool per test.
 */

type WorkerFixtures = {
	/** tag-generator's FastAPI listener on :9400. */
	api: APIRequestContext;
	/** mq-hub's Connect-RPC listener on :9500. */
	mqhub: APIRequestContext;
};

type TestFixtures = {
	/**
	 * A fresh article id for this test.
	 *
	 * A UUID rather than `testToken()`, and that is load-bearing:
	 * `TagGenerationRequestPayload.article_id` is `Field(max_length=36)`
	 * (tag_generator/handler/event_payload.py), so a token of the shape
	 * `<run>-w0-a1b2c3-some-test-title` would be rejected by the *validation*
	 * arm and every round-trip test would silently assert the error path
	 * instead of the happy one. 36 characters is exactly a UUID, which is also
	 * what alt-backend puts on the wire for a real article.
	 */
	articleId: string;
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

	mqhub: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.mqhubURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					// connect-go does not *require* this header (mq-hub mounts its
					// handler without `WithRequireConnectProtocolHeader`), but every
					// real client sends it and the Hurl scenario did too. Sending it
					// keeps the request identical to production traffic rather than
					// exercising a lenience the frontend never relies on.
					"Connect-Protocol-Version": "1",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	articleId: async ({}, use) => {
		await use(uuid());
	},
});

export { expect } from "@playwright/test";

/** The request body `/api/v1/extract-tags` accepts (auth_service.py:536-540). */
export type ExtractTagsBody = Record<string, unknown> & {
	title?: unknown;
	content?: unknown;
};

/** POSTs to `/api/v1/extract-tags` without asserting anything. */
export function postExtractTags(
	api: APIRequestContext,
	body: ExtractTagsBody | string,
): Promise<APIResponse> {
	return api.post("/api/v1/extract-tags", {
		headers: { "Content-Type": "application/json" },
		data: body,
	});
}

/** POSTs to `/api/v1/extract-tags` and asserts the full success envelope. */
export async function extractTags(api: APIRequestContext, body: ExtractTagsBody) {
	return expectJsonStatus(await postExtractTags(api, body), 200, extractTagsSchema);
}

/**
 * Calls mq-hub's synchronous tag-generation wrapper.
 *
 * `timeoutMs` is always passed explicitly. Omitting it makes mq-hub fall back
 * to `DefaultTagGenerationTimeoutMs` = 60_000 (generate_tags_usecase.go:19),
 * which is longer than this suite's per-test budget — so a dead consumer
 * would surface as an opaque Playwright timeout rather than as the assertion
 * that the reply never arrived.
 */
export function generateTagsForArticle(
	mqhub: APIRequestContext,
	request: Record<string, unknown> & { timeoutMs: number },
): Promise<APIResponse> {
	return mqhub.post("/services.mqhub.v1.MQHubService/GenerateTagsForArticle", {
		data: request,
	});
}

/**
 * Keyword-dense English text, carried over verbatim from
 * `e2e/fixtures/tag-generator/extract-tags-keyword.json`.
 *
 * Keeping the exact string matters: the assertions that depend on it (at
 * least one tag, confidence > 0, language "en") are only justified because
 * this text is *known* to have produced them in CI for the Hurl suite. New
 * prose would make those assertions guesses about a model's behaviour.
 */
export const KEYWORD_TITLE = "Machine learning tag extraction in Python";
export const KEYWORD_CONTENT =
	"KeyBERT uses sentence-transformers to extract semantic tags from text. " +
	"This article covers machine learning, natural language processing, and " +
	"Python tooling with FastAPI. The approach relies on contextual embeddings " +
	"computed by a multilingual transformer model.";

/** The minimal body from `e2e/fixtures/tag-generator/extract-tags-smoke.json`. */
export const SMOKE_TITLE = "hurl smoke";
export const SMOKE_CONTENT = "quick brown fox jumps over the lazy dog";
