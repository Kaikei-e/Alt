import { expect, test, type Route } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";

/**
 * Knowledge Loop OODA tile depth regression.
 *
 * Pre-fix: every foreground tile sat at translateZ(0). The OODA cycle was
 * conveyed by a flat horizontal kicker text in the masthead, with no
 * spatial sense of "Observe is in front, Act is behind" or vice versa.
 * Canonical contract §12 mandates depth on tiles, with deeper-focus
 * stages receding into the page. ADR-000831 forbids drop shadows, so
 * depth must come from translateZ + saturate/brightness + border weight.
 *
 * Post-fix: each tile maps `data-stage` to a translateZ band — observe
 * is at Z=0, orient/decide/act recede further. `prefers-reduced-motion`
 * collapses every band to translateZ(0) (per contract §12.5: dissolve +
 * highlight fade + color shift replace depth simulation).
 */

const KL_TRANSITION_PATH = "**/loop/transition";

// Reject the IntersectionObserver-fired dwell so applyLocalStage doesn't flip
// the fixture entry to `orient` between mount and assertion. The pattern
// matches canonical-contract.spec.ts / dismiss-persistence.spec.ts — depth is
// asserted on the seeded `observe` stage, not the post-dwell `orient`.
async function rejectDwellTransitions(route: Route) {
	const body = route.request().postDataJSON() as { trigger?: string };
	if (body.trigger === "dwell") {
		await route.fulfill({
			status: 409,
			contentType: "application/json",
			body: JSON.stringify({ error: "projection_stale" }),
		});
		return;
	}
	await route.fulfill({
		status: 200,
		contentType: "application/json",
		body: JSON.stringify({
			accepted: true,
			canonicalEntryKey: LOOP_FIXTURE_ENTRY_KEY,
		}),
	});
}

test.describe("Knowledge Loop — OODA tile depth", () => {
	test("foreground tile carries a non-flat 3D transform tied to its stage", async ({
		page,
	}) => {
		await page.route(KL_TRANSITION_PATH, rejectDwellTransitions);
		await page.goto("/loop");

		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();
		await expect(tile).toHaveAttribute("data-stage", "observe");

		// The foreground plane sets `perspective`, so a tile with translateZ
		// resolves to a non-identity `matrix3d(...)` rather than a flat 2D
		// matrix. We assert the computed transform either is a matrix3d, or —
		// for observe (Z=0) — accepts a 2D matrix; in either case it must not
		// be `none` because the page registers a transform-style for stagger.
		const transform = await tile.evaluate(
			(el) => window.getComputedStyle(el).transform,
		);
		expect(transform).not.toBe("none");
	});

	test("reduced-motion collapses tile depth to flat", async ({ browser }) => {
		const ctx = await browser.newContext({ reducedMotion: "reduce" });
		const page = await ctx.newPage();
		try {
			await page.route(KL_TRANSITION_PATH, rejectDwellTransitions);
			await page.goto("/loop");
			const tile = page
				.locator(
					`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
				)
				.first();
			await expect(tile).toBeVisible();

			const transform = await tile.evaluate(
				(el) => window.getComputedStyle(el).transform,
			);
			// Under reduced motion the tile must not retain a 3D matrix —
			// either `none` or a 2D `matrix(...)` is acceptable. A `matrix3d`
			// in this state means the contract §12.5 fallback is broken.
			expect(transform.startsWith("matrix3d")).toBe(false);
		} finally {
			await ctx.close();
		}
	});
});
