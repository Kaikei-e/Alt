import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";
import { fulfillConnectStream } from "../../utils/mockHelpers";

/**
 * Knowledge Loop fetch-storm regression.
 *
 * Live nginx + alt-butterfly-facade + alt-backend logs (2026-04-26 04:30 UTC)
 * showed dozens of `GET /loop/__data.json?x-sveltekit-invalidated=1` requests
 * per second from one client, the browser hitting `ERR_INSUFFICIENT_RESOURCES`
 * after the per-origin connection ceiling, plus ~50 `stream_jwt_expired` log
 * lines firing in lockstep waves. Root cause: the page's
 * `useKnowledgeLoopStream` callback wired `onFrame` and `onExpired` to
 * `invalidateAll()` unconditionally — every non-heartbeat frame and every JWT
 * expiry triggered an SSR `__data.json` refetch, which in turn churned the
 * stream `$effect`'s `data`-keyed dependency, opening yet more streams.
 *
 * The fix coalesces stream-driven refresh: at most one `__data.json` refetch
 * per debounce window regardless of how many frames arrive. This spec pins
 * that contract.
 */

const STREAM_PATH =
	"**/alt.knowledge.loop.v1.KnowledgeLoopService/StreamKnowledgeLoopUpdates";
const DATA_JSON_PATH = "**/loop/__data.json*";

test.describe("Knowledge Loop — fetch-storm regression (live logs 2026-04-26)", () => {
	test("a burst of stream frames does NOT trigger one __data.json per frame", async ({
		page,
	}) => {
		let dataJsonHits = 0;
		await page.route(DATA_JSON_PATH, async (route) => {
			dataJsonHits += 1;
			// Forward to the real handler so the page receives a normal SSR payload.
			await route.continue();
		});

		// Mock the streaming RPC to emit 12 non-heartbeat frames in rapid
		// succession plus a terminal `stream_expired`. Pre-fix this would fire
		// `invalidateAll()` 12+1 times. Post-fix the coalescer collapses them
		// into at most one trailing refresh.
		await page.route(STREAM_PATH, async (route) => {
			const frames = [
				// 12 "appended" frames — each non-heartbeat, so each would have
				// triggered the old onFrame → invalidateAll path.
				...Array.from({ length: 12 }, (_, i) => ({
					projectionSeqHiwater: String(100 + i),
					update: {
						case: "appended",
						value: {
							entryKey: `loop-storm-${i}`,
							revision: String(i + 1),
						},
					},
				})),
				// Terminal expiry — pre-fix this drove the second invalidation
				// wave that produced the lockstep `stream_jwt_expired` logs.
				{
					projectionSeqHiwater: "200",
					update: {
						case: "streamExpired",
						value: { reason: "jwt_exp" },
					},
				},
			];
			await fulfillConnectStream(route, frames);
		});

		await page.goto("/loop");
		await expect(
			page.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			),
		).toBeVisible();

		// Give the coalescer's trailing window time to fire (impl uses a 600 ms
		// debounce; we allow 2 s so this isn't tight against config drift).
		await page.waitForTimeout(2_000);

		// SSR initial render is 1. A single coalesced refresh is allowed. Pre-fix
		// this routinely climbed to 12+ inside the same 2 s window.
		expect(dataJsonHits).toBeLessThanOrEqual(2);
	});

	test("an immediate stream_expired does not loop into infinite refetches", async ({
		page,
	}) => {
		let dataJsonHits = 0;
		await page.route(DATA_JSON_PATH, async (route) => {
			dataJsonHits += 1;
			await route.continue();
		});

		// First connect: send `stream_expired` straight away (mirrors the prod
		// pattern where the SSR-issued JWT was already near-expiry).
		await page.route(STREAM_PATH, async (route) => {
			await fulfillConnectStream(route, [
				{
					projectionSeqHiwater: "1",
					update: { case: "streamExpired", value: { reason: "jwt_exp" } },
				},
			]);
		});

		await page.goto("/loop");
		await page.waitForTimeout(2_000);

		// One initial SSR + at most one coalesced refresh. Pre-fix the page
		// reconnected, hit immediate expiry again, called invalidateAll, and the
		// cycle compounded across the AbortController teardown delay.
		expect(dataJsonHits).toBeLessThanOrEqual(2);
	});
});
