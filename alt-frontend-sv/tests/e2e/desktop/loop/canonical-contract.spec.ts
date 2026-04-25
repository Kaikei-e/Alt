import { expect, test } from "@playwright/test";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	buildGetKnowledgeLoopResponse,
	KL_GET,
	KL_STREAM,
	KL_TRANSITION,
	KL_ASK_HANDSHAKE,
	LOOP_ENTRY_OBSERVE_FRESH,
	LOOP_ENTRY_OBSERVE_NO_TITLE,
} from "../../fixtures/factories/knowledgeLoopFactory";

/**
 * Knowledge Loop canonical-contract regressions (ADR-000844).
 *
 * The user-visible bug was a triple failure on /loop:
 *   1. Every card showed the literal placeholder "New summary".
 *   2. Clicking a card revealed only the Ask CTA — open/save/snooze were
 *      rendered but disabled because §7 forbids observe → act.
 *   3. Augur transition surfaced a stub conversation seed (because whyText
 *      was the placeholder).
 *
 * These specs are the outside-in (E2E) RED that pins the contract:
 *   - whyPrimary.text MUST NOT match /^New summary/.
 *   - Observe entries MUST seed §7-allowed CTAs (revisit/ask/snooze) so all
 *     three render *enabled*.
 *   - The Augur handshake MUST receive the entry_key + lensModeId so the
 *     conversation seed message can carry the why_text downstream.
 *
 * The specs mock the Knowledge Loop Connect-RPC endpoints; they do not
 * require the docker stack. Mock data is stage-appropriate per the contract
 * — if the FE ever re-introduces an observe → act CTA, the §7 assertion
 * here fails before the user sees disabled buttons.
 */

function mockAllLoopRoutes(
	page: import("@playwright/test").Page,
	overrides?: Parameters<typeof buildGetKnowledgeLoopResponse>[0],
) {
	const response = buildGetKnowledgeLoopResponse(overrides);
	return Promise.all([
		page.route(KL_GET, (route) => fulfillJson(route, response)),
		page.route(KL_TRANSITION, (route) =>
			fulfillJson(route, { accepted: true, message: "" }),
		),
		page.route(KL_STREAM, (route) => route.abort()),
	]);
}

test.describe("Knowledge Loop — canonical contract (ADR-000844)", () => {
	test("card displays substantive why_text, never the 'New summary' placeholder", async ({
		page,
	}) => {
		await mockAllLoopRoutes(page);
		await page.goto("/loop");

		const tile = page
			.getByTestId("loop-entry-tile")
			.filter({ hasText: "fresh summary ready to read" })
			.first();
		await expect(tile).toBeVisible();

		// The narrative must include the article title — that's the whole point
		// of the v3 enricher: payload-derived context, not an event-name stub.
		await expect(tile).toContainText(LOOP_ENTRY_OBSERVE_FRESH.whyPrimary.text);

		// Hard regression guard: the literal placeholder must not survive the
		// rewrite anywhere on the page.
		await expect(page.getByText(/^New summary$/)).toHaveCount(0);
	});

	test("fallback narrative (no article_title) still avoids the placeholder", async ({
		page,
	}) => {
		await mockAllLoopRoutes(page, {
			foreground: [LOOP_ENTRY_OBSERVE_NO_TITLE],
		});
		await page.goto("/loop");

		const tile = page.getByTestId("loop-entry-tile").first();
		await expect(tile).toBeVisible();
		await expect(tile).toContainText(
			LOOP_ENTRY_OBSERVE_NO_TITLE.whyPrimary.text,
		);
		await expect(page.getByText(/New summary/)).toHaveCount(0);
	});

	test("Observe entry exposes §7-allowed CTAs as enabled buttons", async ({
		page,
	}) => {
		await mockAllLoopRoutes(page);
		await page.goto("/loop");

		const tile = page.getByTestId("loop-entry-tile").first();
		await expect(tile).toBeVisible();

		// Click the tile to expand and reveal CTAs.
		await tile.click();

		// All three §7-allowed CTAs must render and be enabled. Earlier seeds
		// emitted open/save/snooze (→ act), which §7 forbids from observe;
		// those rendered disabled and only Ask was clickable.
		const revisitCta = tile.getByRole("button", { name: /Revisit/i });
		const askCta = tile.getByRole("button", { name: /Ask/i });
		const snoozeCta = tile.getByRole("button", { name: /Snooze/i });

		await expect(revisitCta).toBeVisible();
		await expect(revisitCta).toBeEnabled();
		await expect(askCta).toBeVisible();
		await expect(askCta).toBeEnabled();
		await expect(snoozeCta).toBeVisible();
		await expect(snoozeCta).toBeEnabled();

		// Forbidden CTAs must NOT appear on Observe entries (would imply the
		// projector regressed to seeding act-stage actions).
		await expect(tile.getByRole("button", { name: /^Open$/i })).toHaveCount(0);
		await expect(tile.getByRole("button", { name: /^Save$/i })).toHaveCount(0);
	});

	test("Revisit CTA fires observe → orient transition", async ({ page }) => {
		await mockAllLoopRoutes(page);

		const transitionCalls: Array<Record<string, unknown>> = [];
		await page.route(KL_TRANSITION, async (route) => {
			const post = route.request().postDataJSON();
			transitionCalls.push(post);
			await fulfillJson(route, { accepted: true, message: "" });
		});

		await page.goto("/loop");

		const tile = page.getByTestId("loop-entry-tile").first();
		await tile.click();
		await tile.getByRole("button", { name: /Revisit/i }).click();

		await expect.poll(() => transitionCalls.length).toBeGreaterThan(0);

		const call = transitionCalls[0];
		expect(call).toMatchObject({
			entryKey: LOOP_ENTRY_OBSERVE_FRESH.entryKey,
			// proto enum encodes as numeric on the wire; OBSERVE=1, ORIENT=2.
			fromStage: 1,
			toStage: 2,
		});
	});

	test("Ask CTA hands off to /loop/ask with the entry_key for Augur seeding", async ({
		page,
	}) => {
		await mockAllLoopRoutes(page);

		const handshakePosts: Array<Record<string, unknown>> = [];
		await page.route(KL_ASK_HANDSHAKE, async (route) => {
			handshakePosts.push(route.request().postDataJSON());
			await fulfillJson(route, { conversationId: "conv-e2e-1" });
		});

		await page.goto("/loop");

		const tile = page.getByTestId("loop-entry-tile").first();
		await tile.click();
		await tile.getByRole("button", { name: /^Ask$/i }).click();

		// The BFF re-fetches the entry from sovereign and forwards why_text +
		// evidence_refs to Augur (knowledge-loop-api.ts:54-82). The browser
		// only sees the entry_key + lensModeId + clientHandshakeId here.
		await expect.poll(() => handshakePosts.length).toBeGreaterThan(0);
		expect(handshakePosts[0]).toMatchObject({
			entryKey: LOOP_ENTRY_OBSERVE_FRESH.entryKey,
			lensModeId: "default",
		});
		expect(handshakePosts[0]).toHaveProperty("clientHandshakeId");

		// On a successful handshake the user is taken to /augur/{conversationId}
		// where the rag-orchestrator-rendered seed message carries the why_text.
		await expect(page).toHaveURL(/\/augur\/conv-e2e-1/);
	});
});
