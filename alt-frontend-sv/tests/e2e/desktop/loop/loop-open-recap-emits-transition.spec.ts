import { expect, test } from "@playwright/test";
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
		// Hold the BFF reply so we can prove the transition is fired before the
		// goto fully completes — the request reaches the BFF route handler before
		// the navigation observably progresses.
		const capture = await installTransitionCapture(page, { hold: true });

		await page.goto("/loop?lens=e2e-recap");

		const tile = page
			.locator('[data-testid="loop-entry-tile"]')
			.filter({ has: page.locator(".cta--recap") })
			.first();
		await expect(tile).toBeVisible();

		// Expand the tile to reveal the CTA row.
		await tile.click();

		const recapCta = tile.locator(".cta--recap").first();
		await expect(recapCta).toBeVisible();
		await recapCta.click();

		await expect.poll(() => capture.posts.length).toBeGreaterThanOrEqual(1);
		const post = capture.posts[0];
		expect(field<string>(post, "actedIntent", "acted_intent")).toBe("open");
		expect(field<string>(post, "targetType", "target_type")).toBe("recap");
		expect(typeof field<string>(post, "targetRef", "target_ref")).toBe(
			"string",
		);
		expect(field<boolean>(post, "continueFlag", "continue_flag")).toBe(true);

		// Release the held reply and assert SPA navigation lands on the
		// recap topic page.
		capture.release();
		await expect(page).toHaveURL(/\/recap\/topic\//);
	});
});
