import { expect, test } from "@playwright/test";
import {
	CONNECT_KNOWLEDGE_LOOP_ACT_RESPONSE,
	LOOP_FIXTURE_ACT_ARTICLE_ID,
	LOOP_FIXTURE_ACT_ENTRY_KEY,
	LOOP_FIXTURE_ACT_SOURCE_URL,
} from "../../infra/data/knowledge-loop";

/**
 * Phase 0 RED — Knowledge Loop ACT "Open" must navigate the SPA reader with
 * the article's external HTTPS source URL as `?url=`, not just the projector's
 * internal `route` field. This is the regression where ACT Open landed in the
 * reader's "No article URL provided" placeholder because the FE was passing
 * `actTargets[].route` (an internal SPA path) directly to `goto()`.
 *
 * Backend mock returns CONNECT_KNOWLEDGE_LOOP_ACT_RESPONSE when lensModeId is
 * "e2e-act" (see tests/e2e/infra/handlers/backend.ts). The fixture seeds one
 * foreground entry with `currentEntryStage = ACT` so the workspace's Open
 * command renders directly without stage navigation.
 */

test.describe("Knowledge Loop — ACT Open lands in reader with content", () => {
	test("Open click on ACT entry navigates to /articles/<entryKey>?url=<source_url> and content loads", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-act");

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();
		await expect(workspace).toHaveAttribute("data-stage", "act");

		// The ACT workspace shows an Open command (and a Return command) per
		// the per-stage panel design; the OBSERVE/ORIENT/DECIDE branches do not.
		const openButton = workspace.getByRole("button", { name: /^open$/i });
		await expect(openButton).toBeVisible();
		await expect(openButton).toBeEnabled();

		await openButton.click();

		// `/articles/<entryKey>?url=<source_url>&title=<why_text>` per ADR-000875.
		// `entryKey` may contain ":" (e.g. `article:foo`) so it is encoded.
		const expectedSourceUrlEncoded = encodeURIComponent(
			LOOP_FIXTURE_ACT_SOURCE_URL,
		);
		const expectedEntryKeyEncoded = encodeURIComponent(
			LOOP_FIXTURE_ACT_ENTRY_KEY,
		);
		await expect(page).toHaveURL(
			new RegExp(
				`/articles/${expectedEntryKeyEncoded}\\?[^#]*\\burl=${expectedSourceUrlEncoded}\\b`,
			),
		);

		// The reader fetches via FetchArticleContent (mocked) and renders into
		// the article-content-surface region. Without the `?url=` handoff, the
		// reader bails at `if (!articleUrl) return` and renders the placeholder
		// "No article URL provided." — this is the Phase-0 RED assertion the
		// production projector + FE conflation makes fail.
		await expect(page.getByTestId("article-content-surface")).toBeVisible({
			timeout: 10_000,
		});

		// Negative regression guard: the reader must not be in its
		// "no URL" placeholder state.
		await expect(page.getByText("No article URL provided.")).toHaveCount(0);
	});

	test("ACT entry's actTargets carry source_url alongside the internal route", async ({
		page,
	}) => {
		// Pin the data shape FE expects: route is the internal SPA path
		// (display-only), source_url is the external HTTPS URL the reader
		// needs as ?url=. Drift in either direction reproduces the bug.
		const fixtureEntry =
			CONNECT_KNOWLEDGE_LOOP_ACT_RESPONSE.foregroundEntries[0];
		const articleTarget = fixtureEntry.actTargets.find(
			(t) => t.targetType === 1,
		);
		expect(articleTarget?.route).toBe(
			`/articles/${LOOP_FIXTURE_ACT_ARTICLE_ID}`,
		);
		expect(articleTarget?.sourceUrl).toBe(LOOP_FIXTURE_ACT_SOURCE_URL);
		expect(articleTarget?.sourceUrl).toMatch(/^https?:\/\//);

		await page.goto("/loop?lens=e2e-act");
		await expect(page.getByTestId("loop-ooda-workspace")).toBeVisible();
	});
});
