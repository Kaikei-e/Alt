import { expect, test } from "@playwright/test";
import {
	LOOP_FIXTURE_NO_SOURCE_ARTICLE_ID,
	LOOP_FIXTURE_NO_SOURCE_ENTRY_KEY,
	LOOP_FIXTURE_NO_SOURCE_RECOVERED_URL,
} from "../../infra/data/knowledge-loop";

/**
 * Phase 0 RED — Open CTA is recoverable, never silently disabled.
 *
 * Pre-fix: when the projection's `actTargets[].sourceUrl` was missing (legacy
 * row before ADR-879 producer URL injection, or producer lookup miss), the FE
 * disabled the Open button with a generic aria-label. Users saw a dead control
 * with no way forward — flagged by NN/G as an anti-pattern: disabled hides
 * the reason and offers no recovery.
 *
 * Post-fix: Open stays enabled with a secondary label "Open · resolve url".
 * On click the FE issues a tenant-scoped lookup against the BFF
 * (`/loop/article-source-url?article_id=...`). Success → openHref + act
 * transition. Failure → inline error in the tile body explaining why
 * (404 / 403 / no-url). The pattern matches Linear's "primary action with
 * recovery" rather than "disabled with hint".
 */

const ACT_OPEN_PATH = "**/loop/article-source-url**";
const TRANSITION_PATH = "**/loop/transition";

test.describe("Knowledge Loop — Open CTA recoverable", () => {
	test("entry without sourceUrl shows enabled Open with 'resolve url' label", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-no-source");

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();
		await expect(workspace).toHaveAttribute("data-stage", "act");

		const openButton = workspace.getByRole("button", {
			name: /^open(\s*·\s*resolve url)?$/i,
		});
		await expect(openButton).toBeVisible();
		// The whole point: never silently disabled.
		await expect(openButton).toBeEnabled();
		// The recovery label is the visible affordance signaling that the URL
		// will be resolved on click.
		await expect(openButton).toHaveText(/resolve url/i);
	});

	test("clicking Open · resolve url calls BFF lookup and navigates on success", async ({
		page,
	}) => {
		const lookupCalls: string[] = [];

		await page.route(ACT_OPEN_PATH, async (route) => {
			lookupCalls.push(route.request().url());
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({
					sourceUrl: LOOP_FIXTURE_NO_SOURCE_RECOVERED_URL,
				}),
			});
		});

		// Allow the act transition to succeed; we are testing Open's lookup branch.
		await page.route(TRANSITION_PATH, async (route) => {
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({
					accepted: true,
					canonicalEntryKey: LOOP_FIXTURE_NO_SOURCE_ENTRY_KEY,
				}),
			});
		});

		await page.goto("/loop?lens=e2e-no-source");
		const openButton = page
			.getByTestId("loop-ooda-workspace")
			.getByRole("button", { name: /^open(\s*·\s*resolve url)?$/i });
		await expect(openButton).toBeEnabled();

		await openButton.click();

		await expect.poll(() => lookupCalls.length).toBeGreaterThanOrEqual(1);
		expect(lookupCalls[0]).toContain(
			`article_id=${LOOP_FIXTURE_NO_SOURCE_ARTICLE_ID}`,
		);

		// Reader navigation — same shape as the existing ADR-875 reader contract:
		// `/articles/<entryKey>?url=<recovered_source_url>&...`
		const expectedUrlEncoded = encodeURIComponent(
			LOOP_FIXTURE_NO_SOURCE_RECOVERED_URL,
		);
		const expectedEntryKeyEncoded = encodeURIComponent(
			LOOP_FIXTURE_NO_SOURCE_ENTRY_KEY,
		);
		await expect(page).toHaveURL(
			new RegExp(
				`/articles/${expectedEntryKeyEncoded}\\?[^#]*\\burl=${expectedUrlEncoded}\\b`,
			),
		);
	});

	test("BFF lookup failure surfaces inline error in tile body, no navigation", async ({
		page,
	}) => {
		await page.route(ACT_OPEN_PATH, async (route) => {
			await route.fulfill({
				status: 404,
				contentType: "application/json",
				body: JSON.stringify({ code: "not_found" }),
			});
		});
		await page.route(TRANSITION_PATH, async (route) => {
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({ accepted: true }),
			});
		});

		await page.goto("/loop?lens=e2e-no-source");
		const workspace = page.getByTestId("loop-ooda-workspace");
		const openButton = workspace.getByRole("button", {
			name: /^open(\s*·\s*resolve url)?$/i,
		});
		await openButton.click();

		// Inline error — exact wording is design-locked but must include the
		// failure code so the user can act on it (NN/G "explain why").
		await expect(
			workspace.getByTestId("loop-open-resolve-error"),
		).toBeVisible();
		await expect(workspace.getByTestId("loop-open-resolve-error")).toHaveText(
			/url unavailable|not[_ ]found|404/i,
		);

		// Still on /loop — no navigation on failed resolve.
		await expect(page).toHaveURL(/\/loop(\?|$)/);
	});
});
