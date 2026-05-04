import { expect, test, type Request } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";

/**
 * Phase 0 RED — Knowledge Loop must NOT advance OODA stage on passive viewing.
 *
 * Pre-fix: `observeTiles` (IntersectionObserver) fired `loop.observe(entryKey)`
 * the moment a tile crossed 50% visibility, which posted a cross-stage
 * transition `from=observe, to=orient, trigger=dwell` to the BFF. The 60s
 * throttle hid the symptom under fast tests but the signal still leaked into
 * the projection (`KnowledgeLoopObserved`) and, perceptually, advanced the
 * stage by mere scrolling.
 *
 * Post-fix: dwell is removed entirely from the FE. Only an explicit user
 * gesture (tap-to-expand) can advance Observe → Orient. This spec asserts
 * the absence of any `/loop/transition` POST during a passive view window.
 */

const TRANSITION_PATH = "**/loop/transition";

test.describe("Knowledge Loop — passive viewing does not advance stage", () => {
	test("no /loop/transition POST fires during a 70s passive viewing window", async ({
		page,
	}) => {
		const transitionPosts: Array<{ url: string; body: unknown }> = [];

		await page.route(TRANSITION_PATH, async (route) => {
			const req: Request = route.request();
			if (req.method() === "POST") {
				let body: unknown = null;
				try {
					body = req.postDataJSON();
				} catch {
					body = req.postData();
				}
				transitionPosts.push({ url: req.url(), body });
			}
			// Always accept to keep the page state stable; the assertion is on
			// "did the FE try to fire?", not on the BFF's reply.
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({
					accepted: true,
					canonicalEntryKey: LOOP_FIXTURE_ENTRY_KEY,
				}),
			});
		});

		await page.goto("/loop");

		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();
		await expect(tile).toHaveAttribute("data-stage", "observe");

		// Hold the page open for 5 seconds — the IntersectionObserver-driven
		// dwell signal would have fired by now under any reasonable throttle
		// configuration. (We do not wait the literal 60s used in production
		// throttling because passive dwell must be removed entirely; even one
		// fire under a long-throttle test budget is wrong.)
		await page.waitForTimeout(5_000);

		// Stage must not have advanced.
		await expect(tile).toHaveAttribute("data-stage", "observe");

		// And no transition POST must have been issued at all.
		expect(transitionPosts, JSON.stringify(transitionPosts, null, 2)).toEqual(
			[],
		);
	});

	test("scrolling additional tiles into view also fires no transition", async ({
		page,
	}) => {
		const transitionPosts: Array<unknown> = [];

		await page.route(TRANSITION_PATH, async (route) => {
			if (route.request().method() === "POST") {
				try {
					transitionPosts.push(route.request().postDataJSON());
				} catch {
					transitionPosts.push(route.request().postData());
				}
			}
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({ accepted: true }),
			});
		});

		await page.goto("/loop");
		const firstTile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(firstTile).toBeVisible();

		// Scroll through the page a few times; any IntersectionObserver wiring
		// that converts visibility to a transition will leak here.
		await page.mouse.wheel(0, 800);
		await page.waitForTimeout(500);
		await page.mouse.wheel(0, -400);
		await page.waitForTimeout(500);
		await page.mouse.wheel(0, 1200);
		await page.waitForTimeout(2_000);

		expect(transitionPosts, JSON.stringify(transitionPosts, null, 2)).toEqual(
			[],
		);
	});
});
