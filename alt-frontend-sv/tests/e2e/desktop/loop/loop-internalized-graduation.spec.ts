import { expect, test } from "@playwright/test";
import {
	LOOP_FIXTURE_INTERNALIZED_ENTRY_KEY,
	LOOP_FIXTURE_INTERNALIZED_SIBLING_KEY,
} from "../../infra/data/knowledge-loop";

/**
 * ADR-000908 §Δ3 internalized graduation — read-path filter.
 *
 * Pins the user-visible contract that an entry with
 * `dismissState = DISMISS_STATE_INTERNALIZED` (proto enum 5) does NOT
 * render in the foreground or bucket planes of /loop, while a sibling
 * with the same shape and `dismissState = ACTIVE` continues to render
 * unchanged. The two-entry fixture catches the regression where the
 * filter is over-eager and hides healthy rows.
 *
 * Path covered: backend mock returns proto numeric 5 → connect/knowledge_loop
 * mapDismissFromProto translates to "internalized" → +page.svelte filters
 * dismissState==="internalized" from both `foreground` and `bucketEntries`.
 * Three seams; one user-visible assertion guards all of them.
 */

test.describe("Knowledge Loop — internalized graduation filter (ADR-000908 §Δ3)", () => {
	test("renders the active sibling but hides the internalized entry", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-internalized");

		// The active sibling must appear in the foreground. Wait on it
		// first so the assertion below targets a fully-loaded /loop page
		// rather than a pre-hydration empty state (which would always
		// satisfy the count=0 assertion vacuously).
		const sibling = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_INTERNALIZED_SIBLING_KEY}"]`,
			)
			.first();
		await expect(sibling).toBeVisible();

		// The internalized entry must NEVER be present in the DOM. We
		// assert count==0 rather than `not.toBeVisible()` because the
		// filter removes the row entirely (the Svelte `{#each}` does not
		// instantiate the tile component for filtered entries).
		const graduated = page.locator(
			`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_INTERNALIZED_ENTRY_KEY}"]`,
		);
		await expect(graduated).toHaveCount(0);
	});

	test("filter is stable across re-renders triggered by transition POSTs", async ({
		page,
	}) => {
		// A stream frame or refresh should not resurrect the filtered row.
		// We force a re-render by tapping the sibling (which fires a
		// transition POST and triggers the Loop hook's optimistic re-flow)
		// and then re-assert the graduated entry stays absent.
		await page.goto("/loop?lens=e2e-internalized");

		const sibling = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_INTERNALIZED_SIBLING_KEY}"]`,
			)
			.first();
		await expect(sibling).toBeVisible();
		await sibling.evaluate((el: Element) => (el as HTMLElement).click());

		// Settle long enough for the optimistic transition path to flow
		// through the Runes derivations before re-checking.
		await page.waitForTimeout(400);

		const graduated = page.locator(
			`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_INTERNALIZED_ENTRY_KEY}"]`,
		);
		await expect(graduated).toHaveCount(0);
	});
});
