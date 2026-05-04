import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * Knowledge Loop dismiss persistence regression.
 *
 * Post-Auto-OODA-suppression contract:
 *   1. Clicking the tile expands it AND fires an explicit observe → orient
 *      `user_tap` transition. Allow that to settle before clicking Dismiss
 *      so the dismiss goes through cleanly (loop.dismiss is independent of
 *      transitionTo's inFlight gate, but the assertion below filters on
 *      trigger=defer to ignore the user_tap).
 *   2. Clicking Dismiss fires `POST /loop/transition` with
 *      `trigger=defer` and `fromStage === toStage`.
 *   3. Reloading the page after dismiss does not re-render the tile, so
 *      long as the projector treats the row as `dismiss_state=deferred`
 *      and the read filter excludes non-active entries.
 */

const KL_TRANSITION_PATH = "**/loop/transition";

test.describe("Knowledge Loop — dismiss persistence", () => {
	test("dismissing an observe-stage tile posts a DEFER transition", async ({
		page,
	}) => {
		const transitionBodies: Array<Record<string, unknown>> = [];

		await page.route(KL_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as Record<string, unknown>;
			transitionBodies.push(body);
			// Reject dwell observes so the entry stays in `observe` and the only
			// transition we measure is the explicit Dismiss.
			if (
				body.trigger === "dwell" ||
				body.trigger === "TRANSITION_TRIGGER_DWELL"
			) {
				await fulfillJson(route, { error: "projection_stale" }, 409);
				return;
			}
			await fulfillJson(route, {
				accepted: true,
				canonicalEntryKey: body.entryKey,
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

		await tile.click();
		// Wait for the tap-to-orient user_tap transition to land before
		// clicking Dismiss; otherwise the optimistic stage flip + concurrent
		// post can race the dismiss button mid-render.
		await expect
			.poll(
				() =>
					transitionBodies.filter((b) => b.trigger === "user_tap").length,
			)
			.toBeGreaterThan(0);

		const dismissCta = tile.locator("button.cta--dismiss").first();
		await expect(dismissCta).toBeVisible();
		await expect(dismissCta).toBeEnabled();
		// Bypass any ancestor pointer-event capture by dispatching a click
		// directly on the element.
		await dismissCta.evaluate((el: Element) => (el as HTMLElement).click());

		// Allow the optimistic exit transition to settle.
		await page.waitForTimeout(700);

		// At least one POST went out for dismiss, and at least one of those
		// non-dwell calls carries the DEFER trigger with same-stage from/to.
		const deferCalls = transitionBodies.filter(
			(b) =>
				b.trigger === "defer" ||
				b.trigger === "TRANSITION_TRIGGER_DEFER" ||
				b.intent === "defer",
		);
		expect(deferCalls.length).toBeGreaterThan(0);
		const call = deferCalls[0];
		expect(call.entryKey).toBe(LOOP_FIXTURE_ENTRY_KEY);
		expect(call.fromStage).toBe(call.toStage);
	});

	test("dismissed tile does not return after reload", async ({ page }) => {
		// Track the dismissed entryKey across the BFF GET path so the second
		// __data.json fetch returns a foreground without it.
		let observedDefer = false;

		await page.route(KL_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as Record<string, unknown>;
			if (
				body.trigger === "dwell" ||
				body.trigger === "TRANSITION_TRIGGER_DWELL"
			) {
				await fulfillJson(route, { error: "projection_stale" }, 409);
				return;
			}
			if (
				body.trigger === "defer" ||
				body.trigger === "TRANSITION_TRIGGER_DEFER" ||
				body.intent === "defer"
			) {
				observedDefer = true;
			}
			await fulfillJson(route, {
				accepted: true,
				canonicalEntryKey: body.entryKey,
			});
		});

		// On reload, after a defer has been observed, the page-data fetch must
		// not include the dismissed entry. This mimics the projector having
		// flipped dismiss_state and the read filter `dismiss_state='active'`
		// excluding the row from the foreground.
		await page.route("**/loop/__data.json*", async (route) => {
			const response = await route.fetch();
			const text = await response.text();
			if (!observedDefer) {
				await route.fulfill({ response });
				return;
			}
			// SvelteKit __data.json is a JSON-streaming format; rather than
			// parse it strictly, drop any reference to the dismissed entry key.
			const stripped = text
				.split("\n")
				.filter((line) => !line.includes(LOOP_FIXTURE_ENTRY_KEY))
				.join("\n");
			await route.fulfill({
				status: response.status(),
				headers: response.headers(),
				body: stripped,
			});
		});

		await page.goto("/loop");
		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();
		await tile.click();
		// The tap-to-orient transition runs concurrently with the upcoming
		// dismiss; the optimistic stage flip + animate:flip make the tile
		// briefly unstable for Playwright's pointer-based click. Resolve the
		// race by dispatching the click directly on the DOM element.
		const dismissCta = tile.locator("button.cta--dismiss").first();
		await expect(dismissCta).toBeVisible();
		await dismissCta.evaluate((el: Element) => (el as HTMLElement).click());

		await page.waitForTimeout(700);
		expect(observedDefer).toBe(true);

		await page.reload();
		const stillPresent = await page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.count();
		expect(stillPresent).toBe(0);
	});
});
