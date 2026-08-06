import { waitForReady, httpBody, httpOk } from "../../_shared/readiness.js";
import type { Probe } from "../../_shared/readiness.js";
import { env } from "../src/env.js";
import { KEYWORD_CONTENT, KEYWORD_TITLE } from "../src/fixtures.js";

/**
 * Readiness gate — the replacement for `00-setup.hurl`, and for the reason
 * the Hurl suite had to run `--jobs 1`.
 *
 * The old suite needed order for two separate reasons, and both die here:
 *
 *  1. `00-setup.hurl` polled `/health` before anything else ran. A readiness
 *     check that lives inside the suite is order-dependent by construction,
 *     and `fullyParallel` has no notion of "run this one first".
 *
 *  2. Scenario 03 existed partly as an **SBERT warm-up** so scenario 04's
 *     round trip would fit inside its timeout on a cold runner — a data
 *     dependency between two files that a parallel runner cannot honour. The
 *     warm-up is a property of the *stack*, not of a test, so it belongs
 *     here. Once this gate returns, every spec faces a warm model and can run
 *     in any order, on any worker.
 *
 * Probing here rather than inside a spec also means a stack that never comes
 * up fails **once**, naming the probe that never passed, instead of failing
 * every test with a connection error and leaving the reader to work out which
 * one was the cause.
 */

/** A healthy container is not a ready service; a cold ML image is why. */
const CONTAINER_TIMEOUT_MS = 90_000;

/**
 * The extractor warm-up gets its own, much larger budget.
 *
 * `/health` returns 200 as soon as FastAPI's lifespan has *spawned* the
 * background thread — it does not wait for `_background_tag_service` to
 * exist, so `/api/v1/extract-tags` answers 503 "Tag extraction service not
 * ready" (auth_service.py:555-556) for a window after the container is
 * healthy. On GitHub-hosted 2-core runners `TagExtractor.warmup()` has been
 * observed past 20s from cold, and the first real KeyBERT pass costs several
 * seconds more; the Hurl suite budgeted 180 × 1s for the same window.
 */
const WARMUP_TIMEOUT_MS = 300_000;

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

/**
 * Waits until `/api/v1/extract-tags` actually serves an inference.
 *
 * Deliberately requires a 200 with a well-formed body rather than "not 503":
 * a 500 from a broken MeCab/unidic or SBERT init path would otherwise satisfy
 * a laxer gate and every spec would then fail individually with the same
 * useless message.
 */
const extractorWarm: Probe = {
	label: `POST ${env.baseURL}/api/v1/extract-tags serves an inference`,
	run: async (api) => {
		const response = await api.post(`${env.baseURL}/api/v1/extract-tags`, {
			headers: { "Content-Type": "application/json" },
			data: { title: KEYWORD_TITLE, content: KEYWORD_CONTENT },
			// One request may legitimately take most of a minute on the very
			// first, model-loading call.
			timeout: 120_000,
		});
		if (response.status() !== 200) {
			throw new Error(`status ${response.status()}: ${(await response.text()).slice(0, 300)}`);
		}
		const body: unknown = await response.json();
		if (!isRecord(body) || body["success"] !== true) {
			throw new Error(`unexpected body ${JSON.stringify(body).slice(0, 300)}`);
		}
	},
};

/**
 * Waits until tag-generator's `alt:events:tags` consumer is joined and
 * replying.
 *
 * This is the window the Hurl suite covered with `retry: 3` on scenario 04:
 * `/health` can be 200 while the `redis-streams-tags-consumer` thread has not
 * yet reached `XGROUP CREATE`, and a request published before the group
 * exists is never delivered to it at all.
 *
 * The probe costs **no inference**. It sends a request with no `articleId`,
 * which `TagGenerationRequestPayload`'s `article_id_not_empty` validator
 * rejects before the extractor is touched (stream_event_handler.py:150-167),
 * so the reply comes back immediately. What is being established is only
 * that a reply comes back at all — i.e. that something is consuming the
 * stream — which is precisely what the round-trip specs need and nothing
 * more.
 */
const tagsConsumerJoined: Probe = {
	label: `${env.mqhubURL} round-trips to tag-generator's alt:events:tags consumer`,
	run: async (api) => {
		const response = await api.post(
			`${env.mqhubURL}/services.mqhub.v1.MQHubService/GenerateTagsForArticle`,
			{
				headers: { "Content-Type": "application/json", "Connect-Protocol-Version": "1" },
				data: { title: "readiness", content: "readiness", timeoutMs: 8_000 },
				timeout: 20_000,
			},
		);
		if (response.status() !== 200) {
			// 504/deadline_exceeded is the "no consumer replied" answer; keep polling.
			throw new Error(`status ${response.status()}: ${(await response.text()).slice(0, 300)}`);
		}
		const body: unknown = await response.json();
		if (!isRecord(body) || typeof body["errorMessage"] !== "string") {
			throw new Error(
				`a reply arrived but not from the validation arm: ` +
					`${JSON.stringify(body).slice(0, 300)}`,
			);
		}
	},
};

export default async function globalSetup(): Promise<void> {
	// Serial and in dependency order: mq-hub cannot round-trip before Redis is
	// up, and the consumer probe is meaningless before the extractor exists.
	await waitForReady(
		[
			httpBody(
				`${env.baseURL}/health`,
				(body) =>
					isRecord(body) && body["status"] === "healthy" && body["service"] === "tag-generator",
				`GET ${env.baseURL}/health reports {"status":"healthy","service":"tag-generator"}`,
			),
			httpOk(`${env.mqhubURL}/health`, `GET ${env.mqhubURL}/health (Redis reachable)`),
		],
		{ timeout: CONTAINER_TIMEOUT_MS, interval: 1_000 },
	);

	await waitForReady([extractorWarm], { timeout: WARMUP_TIMEOUT_MS, interval: 2_000 });

	await waitForReady([tagsConsumerJoined], { timeout: CONTAINER_TIMEOUT_MS, interval: 2_000 });
}
