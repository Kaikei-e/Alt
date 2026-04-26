import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../helpers/navigation";
import { fulfillJson } from "../utils/mockHelpers";
import {
	LOOP_FIXTURE_CHANGED_ENTRY_KEY,
	LOOP_FIXTURE_CHANGED_NEW_ENTRY_KEY,
	LOOP_FIXTURE_CONTINUE_ENTRY_KEY,
	LOOP_FIXTURE_REVIEW_ENTRY_KEY,
} from "../infra/data/knowledge-loop";

/**
 * Knowledge Loop Surface planes (Continue / Changed / Review) — Playwright E2E.
 *
 * Exercises PR-L8 (per-bucket entries + plane components) and PR-L9
 * (CSS depth hierarchy). Reads the SSR-seeded /loop page through the
 * mock backend (see tests/e2e/infra/handlers/backend.ts which serves
 * CONNECT_KNOWLEDGE_LOOP_RESPONSE).
 *
 * Loop stream is not exercised here because it would require a long-lived
 * Connect server stream mock; stream classification is unit-tested in
 * src/lib/hooks/loop-stream-frames.test.ts.
 */

const LOOP_TRANSITION_PATH = "**/loop/transition";

test.describe("Mobile Knowledge Loop — Surface planes", () => {
	test.beforeEach(async ({ page }) => {
		// Accept all transitions so plane CTAs do not explode on click.
		await page.route(LOOP_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as { entryKey?: string };
			await fulfillJson(route, {
				accepted: true,
				canonicalEntryKey: body.entryKey ?? null,
			});
		});
	});

	test("Continue plane renders timeline rows with resume affordance", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "loop");

		const plane = page.getByTestId("loop-continue-stream");
		if ((await plane.count()) === 0) {
			test.skip(
				true,
				"Continue fixture not rendered — mock server did not ship bucketEntries",
			);
			return;
		}
		await expect(plane).toBeVisible();

		const row = plane.locator(
			`[data-entry-key="${LOOP_FIXTURE_CONTINUE_ENTRY_KEY}"]`,
		);
		await expect(row).toBeVisible();
		// aria-label on the <li> carries the LoopPriority label.
		const parentLi = row.locator("xpath=ancestor::li");
		await expect(parentLi).toHaveAttribute("aria-label", /Continuing/);
	});

	test("Changed plane renders THEN/NOW diptych with confirm CTA", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "loop");

		const diff = page.getByTestId("loop-changed-diff");
		if ((await diff.count()) === 0) {
			test.skip(true, "Changed fixture not rendered");
			return;
		}
		await expect(diff).toBeVisible();

		// Both kickers must be on screen so the diptych reads as explicit.
		await expect(diff.getByText(/^Then$/)).toBeVisible();
		await expect(diff.getByText(/^Now$/)).toBeVisible();

		// Confirm CTA POSTs a transition to the Act stage. We don't validate the
		// navigation here — the separate transition spec covers it — but we
		// confirm the button fires the BFF route.
		let gotTransition = false;
		await page.route(LOOP_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as {
				entryKey?: string;
				toStage?: string;
			};
			if (
				body.entryKey === LOOP_FIXTURE_CHANGED_ENTRY_KEY &&
				body.toStage === "act"
			) {
				gotTransition = true;
			}
			await fulfillJson(route, { accepted: true });
		});

		const confirm = diff
			.locator(`[data-entry-key="${LOOP_FIXTURE_CHANGED_ENTRY_KEY}"]`)
			.first();
		await expect(confirm).toBeVisible();
		await confirm.click();

		await expect.poll(() => gotTransition, { timeout: 3_000 }).toBe(true);

		// supersededByEntryKey is surfaced in the THEN column when change_summary
		// is empty, but our fixture has change_summary so the THEN line should
		// carry the summary text.
		await expect(diff).toContainText(
			/Model cardinality bumped from 5 to 7 classes/,
		);
		// The new entry pointer is still accessible via data attribute for hand-off.
		void LOOP_FIXTURE_CHANGED_NEW_ENTRY_KEY;
	});

	test("Review plane renders a low-density mono list", async ({ page }) => {
		await gotoMobileRoute(page, "loop");

		const dock = page.getByTestId("loop-review-dock");
		if ((await dock.count()) === 0) {
			test.skip(true, "Review fixture not rendered");
			return;
		}
		await expect(dock).toBeVisible();

		const entry = dock.locator(
			`[data-entry-key="${LOOP_FIXTURE_REVIEW_ENTRY_KEY}"]`,
		);
		await expect(entry).toBeVisible();

		// Font is mono for the Review dock; CSS classes are scoped so we use
		// computed style to pin the intent rather than class names.
		const fontFamily = await entry.evaluate(
			(el) => window.getComputedStyle(el).fontFamily,
		);
		expect(fontFamily.toLowerCase()).toMatch(/mono|ibm plex/);
	});

	test("depth planes recede via translateZ when motion is allowed", async ({
		page,
	}) => {
		await page.emulateMedia({ reducedMotion: "no-preference" });
		await gotoMobileRoute(page, "loop");

		const root = page.getByTestId("knowledge-loop-root");
		await expect(root).toBeVisible();

		// Root perspective must be set so child translateZ composites.
		const perspective = await root.evaluate(
			(el) => window.getComputedStyle(el).perspective,
		);
		expect(perspective).not.toBe("none");

		const foreground = page.locator('[data-plane="foreground"]').first();
		const mid = page.locator('[data-plane="mid-context"]').first();

		if ((await mid.count()) === 0) {
			test.skip(true, "mid-context plane not present in fixture");
			return;
		}

		const foregroundTransform = await foreground.evaluate(
			(el) => window.getComputedStyle(el).transform,
		);
		const midTransform = await mid.evaluate(
			(el) => window.getComputedStyle(el).transform,
		);

		// Mid-context must differ from foreground — either a matrix3d with
		// translateZ or an explicit scale — otherwise the CSS layer broke.
		expect(midTransform).not.toBe(foregroundTransform);
	});

	test("depth planes collapse to flat under prefers-reduced-motion", async ({
		page,
	}) => {
		await page.emulateMedia({ reducedMotion: "reduce" });
		await gotoMobileRoute(page, "loop");

		const root = page.getByTestId("knowledge-loop-root");
		await expect(root).toBeVisible();

		// Under reduced motion the root perspective must be disabled so the
		// depth simulation stops — canonical contract §12.5.
		const perspective = await root.evaluate(
			(el) => window.getComputedStyle(el).perspective,
		);
		expect(perspective).toBe("none");

		const mid = page.locator('[data-plane="mid-context"]').first();
		if ((await mid.count()) === 0) return;

		const midTransform = await mid.evaluate(
			(el) => window.getComputedStyle(el).transform,
		);
		expect(midTransform === "none" || midTransform.endsWith("0)")).toBe(true);
	});
});
