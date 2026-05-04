import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";

/**
 * Phase 0 RED — A tap on the entry tile is the explicit gesture that advances
 * Observe → Orient.
 *
 * The Auto-OODA suppression removes IntersectionObserver-driven dwell. To
 * preserve the OODA experience the user must still be able to enter Orient
 * with a single gesture; the tile's existing tap-to-expand affordance is
 * upgraded to also fire a `(observe, orient, user_tap)` transition.
 *
 * The transition POST must:
 *   - have `from_stage: "observe"`, `to_stage: "orient"`, `trigger: "user_tap"`
 *   - be optimistic: the tile's `data-stage` flips to "orient" without waiting
 *     for the server's `accepted` reply
 */

const TRANSITION_PATH = "**/loop/transition";

test.describe("Knowledge Loop — tap advances stage with user_tap", () => {
	test("clicking a tile fires observe→orient with trigger=user_tap and flips data-stage optimistically", async ({
		page,
	}) => {
		const transitionPosts: Array<Record<string, unknown>> = [];

		// Hold the BFF response so we can prove the FE flips before the server
		// reply lands (optimistic patch).
		let release: () => void = () => {};
		const released = new Promise<void>((resolve) => {
			release = resolve;
		});

		await page.route(TRANSITION_PATH, async (route) => {
			if (route.request().method() === "POST") {
				try {
					transitionPosts.push(
						route.request().postDataJSON() as Record<string, unknown>,
					);
				} catch {
					/* ignore */
				}
			}
			// Wait until the test releases — proves FE optimism.
			await released;
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

		await tile.click();

		// Optimistic UI: data-stage flips to orient before the held BFF reply.
		await expect(tile).toHaveAttribute("data-stage", "orient");

		// Now release the BFF so the page can reconcile.
		release();

		// Exactly one transition POST issued, with the canonical fields.
		await expect.poll(() => transitionPosts.length).toBeGreaterThanOrEqual(1);
		const post = transitionPosts[0];
		expect(post.entryKey ?? post.entry_key).toBe(LOOP_FIXTURE_ENTRY_KEY);
		expect(post.fromStage ?? post.from_stage).toBe("observe");
		expect(post.toStage ?? post.to_stage).toBe("orient");
		expect(post.trigger).toBe("user_tap");
	});
});
