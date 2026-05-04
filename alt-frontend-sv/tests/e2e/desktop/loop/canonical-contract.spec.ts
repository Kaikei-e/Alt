import { expect, test } from "@playwright/test";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_KNOWLEDGE_LOOP_RESPONSE,
	LOOP_FIXTURE_ENTRY_KEY,
} from "../../infra/data/knowledge-loop";

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
 * These specs are the outside-in (E2E) tests that pin the contract against
 * the global mock fixture (tests/e2e/infra/data/knowledge-loop.ts) which
 * the in-process Playwright mock-server returns for
 * `/alt.knowledge.loop.v1.KnowledgeLoopService/GetKnowledgeLoop`. The
 * fixture itself was updated in lockstep with ADR-000844 so
 * decisionOptions = [revisit, ask, snooze] for the Observe foreground entry.
 *
 * Browser-side interception via `page.route()` is used for the BFF
 * endpoints `/loop/transition` and `/loop/ask` — those are called from
 * the browser after the SSR `+page.server.ts` load completes.
 */

const KL_TRANSITION_PATH = "**/loop/transition";
const KL_ASK_PATH = "**/loop/ask";

test.describe("Knowledge Loop — canonical contract (ADR-000844)", () => {
	test("card displays substantive why_text, never the 'New summary' placeholder", async ({
		page,
	}) => {
		await page.goto("/loop");

		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await expect(tile).toBeVisible();

		// The fixture's narrative pins §11 — substantive, not a placeholder.
		const expectedNarrative =
			CONNECT_KNOWLEDGE_LOOP_RESPONSE.foregroundEntries[0].whyPrimary.text;
		await expect(tile).toContainText(expectedNarrative);

		// Hard regression guard: the literal placeholder must not survive the
		// rewrite anywhere on the page.
		await expect(page.getByText(/^New summary$/)).toHaveCount(0);
	});

	test("Observe entry exposes §7-allowed CTAs as enabled buttons", async ({
		page,
	}) => {
		// The page's IntersectionObserver fires an observe → orient transition
		// the moment the tile is visible. Left unmocked, that POST hits the
		// shared mock backend, returns accepted, and applyLocalStage flips the
		// entry to orient — at which point Revisit (observe → orient) becomes
		// `disabled` because the allowlist forbids orient → orient. Reject the
		// dwell transition with 409 (mapped to "stale" by post()) so the entry
		// stays in observe and CTA enabled-state matches the §7 contract.
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
		await tile.click();

		// All three §7-allowed CTAs must render and be enabled. Earlier seeds
		// emitted open/save/snooze (→ act), which §7 forbids from observe;
		// those rendered disabled and only Ask was clickable.
		const revisitCta = tile.getByRole("button", { name: /^revisit$/i });
		const askCta = tile.getByRole("button", { name: /^ask$/i });
		const snoozeCta = tile.getByRole("button", { name: /^snooze$/i });

		await expect(revisitCta).toBeVisible();
		await expect(revisitCta).toBeEnabled();
		await expect(askCta).toBeVisible();
		await expect(askCta).toBeEnabled();
		await expect(snoozeCta).toBeVisible();
		await expect(snoozeCta).toBeEnabled();

		// Forbidden CTAs must NOT appear on Observe entries (would imply the
		// projector regressed to seeding act-stage actions).
		await expect(tile.getByRole("button", { name: /^open$/i })).toHaveCount(0);
		await expect(tile.getByRole("button", { name: /^save$/i })).toHaveCount(0);
	});

	test("tap on an observe tile fires the observe → orient user_tap transition", async ({
		page,
	}) => {
		// Auto-OODA suppression (Knowledge Loop 体験回復プラン Pillar 1):
		// the tile's tap-to-expand gesture is the explicit user_tap that
		// advances Observe → Orient. The pre-fix Revisit CTA path is folded
		// into the tap itself — Boyd's Orientation must be a conscious step,
		// not a side effect of the IntersectionObserver. Dwell is rejected
		// at the BFF; we still mock 409 for backwards-safety in case any
		// transitional code path briefly emits it.
		const captured: Array<Record<string, unknown>> = [];
		await page.route(KL_TRANSITION_PATH, async (route) => {
			const body = route.request().postDataJSON() as Record<string, unknown>;
			captured.push(body);
			if (body.trigger === "dwell") {
				await fulfillJson(route, { error: "invalid_argument" }, 400);
				return;
			}
			await fulfillJson(route, {
				accepted: true,
				canonicalEntryKey: LOOP_FIXTURE_ENTRY_KEY,
			});
		});

		await page.goto("/loop");

		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await tile.click();

		await expect
			.poll(() => captured.filter((c) => c.trigger === "user_tap").length)
			.toBeGreaterThan(0);

		const userTap = captured.find((c) => c.trigger === "user_tap");
		expect(userTap).toMatchObject({
			entryKey: LOOP_FIXTURE_ENTRY_KEY,
			fromStage: "observe",
			toStage: "orient",
		});
	});

	test("Ask CTA hands off to /loop/ask with the entry_key for Augur seeding", async ({
		page,
	}) => {
		// Same dwell-rejection trick: keeps the proposedStage at observe so the
		// Ask CTA is rendered before applyLocalStage can flip context.
		await page.route(KL_TRANSITION_PATH, async (route) => {
			await fulfillJson(route, { error: "projection_stale" }, 409);
		});

		const handshakePosts: Array<Record<string, unknown>> = [];
		await page.route(KL_ASK_PATH, async (route) => {
			handshakePosts.push(route.request().postDataJSON());
			await fulfillJson(route, { conversationId: "conv-e2e-1" });
		});

		await page.goto("/loop");

		const tile = page
			.locator(
				`[data-testid="loop-entry-tile"][data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`,
			)
			.first();
		await tile.click();
		await tile.getByRole("button", { name: /^ask$/i }).click();

		// The BFF re-fetches the entry from sovereign and forwards why_text +
		// evidence_refs to Augur (knowledge-loop-api.ts:54-82). The browser
		// only sees the entry_key + lensModeId + clientHandshakeId here.
		await expect.poll(() => handshakePosts.length).toBeGreaterThan(0);
		expect(handshakePosts[0]).toMatchObject({
			entryKey: LOOP_FIXTURE_ENTRY_KEY,
			lensModeId: "default",
		});
		expect(handshakePosts[0]).toHaveProperty("clientHandshakeId");

		// On a successful handshake the user is taken to /augur/{conversationId}
		// where the rag-orchestrator-rendered seed message carries the why_text.
		await expect(page).toHaveURL(/\/augur\/conv-e2e-1/);
	});
});
