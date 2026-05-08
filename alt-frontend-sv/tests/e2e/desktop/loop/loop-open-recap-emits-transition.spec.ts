import { expect, test } from "@playwright/test";
import {
	LOOP_FIXTURE_RECAP_ENTRY_KEY,
	LOOP_FIXTURE_RECAP_TOPIC_SNAPSHOT_ID,
} from "../../infra/data/knowledge-loop";
import {
	field,
	installTransitionCapture,
} from "../../utils/transition-capture";

/**
 * Knowledge Loop Phase 2 — Open Recap CTA must emit a semantic transition
 * before navigation, not be a plain `<a>` link. The recap target is seeded
 * by Surface Planner v2 from a RecapTopicSnapshotted event (canonical
 * contract §6.4.1); clicking it is a deliberate "open recap topic" act, so
 * it must round-trip back to Loop as `acted_intent=open` /
 * `target_type=recap` / `continue_flag=true`.
 *
 * Reference: docs/plan/knowledge-loop-completion-02-semantic-decide-act.md §4
 */

test.describe("Knowledge Loop — Open Recap fires semantic transition before navigation", () => {
	test("clicking Open Recap sends acted_intent=open / target_type=recap and only then navigates", async ({
		page,
	}) => {
		const capture = await installTransitionCapture(page);

		await page.goto("/loop?lens=e2e-recap");

		// The tile's `.cta--recap` button only renders when the tile is
		// expanded (LoopEntryTile.svelte's `{#if expanded}` guard). Locate the
		// recap-fixture tile by its data-entry-key, expand it via tile click,
		// then click the recap button. Existing Loop e2e specs note that an
		// ancestor's pointer-event capture sometimes swallows the synthetic
		// Playwright click; dispatching the click directly on the element
		// (matching dismiss-persistence.spec.ts) avoids that.
		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_RECAP_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();
		await tile.evaluate((el: Element) => (el as HTMLElement).click());

		const recapCta = tile.locator(".cta--recap").first();
		await expect(recapCta).toBeVisible();
		await recapCta.evaluate((el: Element) => (el as HTMLElement).click());

		await expect.poll(() => capture.posts.length).toBeGreaterThanOrEqual(1);
		const post = capture.posts[0];
		expect(field<string>(post, "actedIntent", "acted_intent")).toBe("open");
		expect(field<string>(post, "targetType", "target_type")).toBe("recap");
		expect(field<string>(post, "targetRef", "target_ref")).toBe(
			LOOP_FIXTURE_RECAP_TOPIC_SNAPSHOT_ID,
		);
		expect(field<boolean>(post, "continueFlag", "continue_flag")).toBe(true);

		// SPA navigation has left /loop. The exact destination route
		// (`/recap/topic/<id>`) is not implemented in this app yet; SvelteKit
		// falls back to a default route. The semantic guarantee this spec
		// pins is that the transition POST fires *before* navigation, not
		// the existence of the recap topic page.
		await expect(page).not.toHaveURL(/\/loop\?/);
	});
});
