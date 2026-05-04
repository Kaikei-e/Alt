import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * Knowledge Loop foreground exit-transition regression.
 *
 * User-reported visual bug (screenshot, 2026-04-26): clicking Dismiss on the
 * top foreground tile caused subsequent tiles to overlap in 2D — text bled
 * through, the OODA loop did not read as 3D-layered. ADR-000831 §11-13
 * mandates depth on tiles, no shadows, and a deeper-focus / return / changed
 * separation; the dismiss exit must therefore recede along Z and dissolve,
 * with neighbors sliding up smoothly into the freed grid track.
 *
 * Pre-fix: `.entry.dismissing` collapsed `max-height: 0` while the entry
 * stayed in the keyed #each, no `animate:flip` was wired, and `transform-style:
 * preserve-3d` lived on the plane root only — so neighbors stayed pinned and
 * mid-collapse text bled into the next row.
 *
 * Post-fix: dismissed entries are removed from the array after a short grace
 * window; the `#each` carries `animate:flip` for survivors and an `out:`
 * Z-recede for the leaver; reduced-motion drops Z and uses dissolve only.
 */

const KL_TRANSITION_PATH = "**/loop/transition";

test.describe("Knowledge Loop — dismiss exit transition (no overlap)", () => {
	test("dismissing the top tile does not cause Y-axis overlap with neighbors", async ({
		page,
	}) => {
		// Reject the IntersectionObserver dwell so the entry stays in `observe`
		// and the explicit Dismiss is the only state mutation we measure.
		await page.route(KL_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as Record<string, unknown>;
			if (body.trigger === "dwell") {
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
		await tile.click();
		const dismissCta = tile.locator("button.cta--dismiss").first();
		await expect(dismissCta).toBeVisible();

		// Grab the bounding boxes of every visible foreground tile *before*
		// dismiss so we have a baseline that proves no overlap exists at rest.
		const baselineBoxes = await page
			.getByTestId("loop-entry-tile")
			.evaluateAll((els) =>
				els
					.map((el) => el.getBoundingClientRect())
					.map((r) => ({ top: r.top, bottom: r.bottom, height: r.height })),
			);
		// Sanity: at rest, no two tiles overlap by more than 1 px vertically.
		for (let i = 0; i < baselineBoxes.length - 1; i += 1) {
			expect(baselineBoxes[i].bottom).toBeLessThanOrEqual(
				baselineBoxes[i + 1].top + 1,
			);
		}

		// Bypass animate:flip / loopRecede instability with a direct DOM click.
		await dismissCta.evaluate((el: Element) => (el as HTMLElement).click());

		// Allow the exit transition + grace window to complete (impl: 280 ms +
		// margin).
		await page.waitForTimeout(700);

		// Final state: the dismissed tile is gone; remaining neighbors keep
		// their non-overlap invariant. Each neighbor's `bottom` must be ≤ next
		// neighbor's `top` (within sub-pixel rounding from `animate:flip`).
		const remaining = await page
			.getByTestId("loop-entry-tile")
			.evaluateAll((els) =>
				els
					.map((el) => el.getBoundingClientRect())
					.map((r) => ({ top: r.top, bottom: r.bottom })),
			);
		expect(remaining.length).toBeLessThan(baselineBoxes.length);
		for (let i = 0; i < remaining.length - 1; i += 1) {
			expect(remaining[i].bottom).toBeLessThanOrEqual(remaining[i + 1].top + 1);
		}
	});

	test("dismissed tile is removed from the DOM (not left as a zero-height ghost)", async ({
		page,
	}) => {
		await page.route(KL_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as Record<string, unknown>;
			if (body.trigger === "dwell") {
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
		await tile.click();
		const dismissCta = tile.locator("button.cta--dismiss").first();
		await expect(dismissCta).toBeVisible();
		await dismissCta.evaluate((el: Element) => (el as HTMLElement).click());

		// Allow the exit transition + grace window to complete (impl uses 280 ms).
		await page.waitForTimeout(700);

		const stillPresent = await page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.count();
		expect(stillPresent).toBe(0);
	});

	test("foreground article does not carry a max-height clamp (regression for content overflow)", async ({
		page,
	}) => {
		await page.route(KL_TRANSITION_PATH, async (route) => {
			await fulfillJson(route, { error: "projection_stale" }, 409);
		});

		await page.goto("/loop");
		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();

		// Pre-fix the tile had `max-height: 640px` on `.entry`. With
		// content-overflow being the visible failure mode, the post-fix CSS must
		// not clamp the article at all (any clamp belongs on `.expand` only).
		const maxHeight = await tile.evaluate(
			(el) => window.getComputedStyle(el).maxHeight,
		);
		expect(maxHeight).toBe("none");
	});
});
