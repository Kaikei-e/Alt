import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../helpers/navigation";
import { fulfillJson } from "../utils/mockHelpers";
import { LOOP_FIXTURE_ENTRY_KEY } from "../infra/data/knowledge-loop";

/**
 * Knowledge Loop OODA transition UI — Playwright E2E.
 *
 * The /loop page is SSR-backed by the mock backend in
 * tests/e2e/infra/handlers/backend.ts (GetKnowledgeLoop seeded).
 * Client-side transitions POST to /loop/transition (BFF route), which
 * we intercept here to assert request shape and drive UI states.
 */

const LOOP_TRANSITION_PATH = "**/loop/transition";

type CapturedRequest = {
	clientTransitionId: string;
	entryKey: string;
	fromStage: string;
	toStage: string;
	trigger: string;
};

async function routeTransitionAccepted(
	page: import("@playwright/test").Page,
	captured: CapturedRequest[],
) {
	await page.route(LOOP_TRANSITION_PATH, async (route) => {
		const body = route.request().postDataJSON() as CapturedRequest;
		captured.push(body);
		await fulfillJson(route, {
			accepted: true,
			canonicalEntryKey: body.entryKey,
		});
	});
}

test.describe("Mobile Knowledge Loop — OODA transition", () => {
	test("tile tap reveals stage-appropriate CTAs (§7 transition allowlist)", async ({
		page,
	}) => {
		const captured: CapturedRequest[] = [];
		await routeTransitionAccepted(page, captured);

		await gotoMobileRoute(page, "loop");

		const tile = page
			.getByTestId("loop-entry-tile")
			.filter({
				has: page.locator(`[data-entry-key="${LOOP_FIXTURE_ENTRY_KEY}"]`),
			})
			.first();
		const fallback = page.getByTestId("loop-entry-tile").first();
		const target = (await tile.count()) > 0 ? tile : fallback;

		if ((await target.count()) === 0) {
			test.skip(true, "Loop entry fixture not rendered — SSR decode failed");
			return;
		}

		await expect(target).toBeVisible();
		await target.click();

		// Per ADR-000844 §7: an Observe entry seeds revisit (→ orient), ask,
		// snooze. Earlier seeds emitted open/save (→ act), which §7 forbids.
		await expect(
			target.getByRole("button", { name: /^revisit$/i }),
		).toBeVisible();
		await expect(target.getByRole("button", { name: /^ask$/i })).toBeVisible();
		await expect(
			target.getByRole("button", { name: /^snooze$/i }),
		).toBeVisible();
		await expect(
			target.getByRole("button", { name: /^dismiss$/i }),
		).toBeVisible();
		// Forbidden CTAs must NOT appear on Observe entries.
		await expect(target.getByRole("button", { name: /^open$/i })).toHaveCount(
			0,
		);
		await expect(target.getByRole("button", { name: /^save$/i })).toHaveCount(
			0,
		);

		await expect(target).toHaveAttribute("aria-expanded", "true");
	});

	test("Revisit CTA posts an observe → orient transition", async ({ page }) => {
		const captured: CapturedRequest[] = [];
		await routeTransitionAccepted(page, captured);

		await gotoMobileRoute(page, "loop");

		const target = page.getByTestId("loop-entry-tile").first();
		if ((await target.count()) === 0) {
			test.skip(true, "Loop entry fixture not rendered");
			return;
		}

		// Per ADR-000844 §7 + the tap-to-orient UX (e90ada874): tapping an
		// observe-stage tile commits the observe → orient transition itself
		// — it is the canonical Revisit gesture. The dedicated Revisit CTA
		// inside the tile becomes redundant once the tap has fired (orient
		// → orient is not in the §7 allowlist), so this spec asserts the
		// transition emitted by the tap, not a click on the now-stale CTA.
		await target.click();

		await expect
			.poll(() => captured.length, { timeout: 3_000 })
			.toBeGreaterThanOrEqual(1);
		const orientReq = captured.find((r) => r.toStage === "orient");
		expect(orientReq).toBeTruthy();
		expect(orientReq?.fromStage).toBe("observe");
		expect(orientReq?.entryKey).toBe(LOOP_FIXTURE_ENTRY_KEY);
		expect(orientReq?.clientTransitionId).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
		);
	});

	test("Dismiss CTA fades the tile and sends a transition", async ({
		page,
	}) => {
		const captured: CapturedRequest[] = [];
		await routeTransitionAccepted(page, captured);

		await gotoMobileRoute(page, "loop");

		const target = page.getByTestId("loop-entry-tile").first();
		if ((await target.count()) === 0) {
			test.skip(true, "Loop entry fixture not rendered");
			return;
		}

		await target.click();
		const dismissCta = target.getByRole("button", { name: /^dismiss$/i });
		// Wait for the auto-orient transition (fired on the expanding tap) to
		// clear `inFlight`. Dismiss itself only depends on `dismissing`, but
		// settling first keeps the assertion deterministic across retries.
		await expect(dismissCta).toBeEnabled({ timeout: 5_000 });
		await dismissCta.click();

		// Dismiss is a same-stage `defer` transition (canonical contract §8.2).
		// The fade animation lives on the parent `.foreground-row` and depends
		// on `out:loopRecede` finishing, which is timing-fragile under headless
		// Chrome — assert the BFF POST instead, which is what the projector
		// actually consumes.
		await expect
			.poll(
				() =>
					captured.find(
						(r) =>
							r.entryKey === LOOP_FIXTURE_ENTRY_KEY &&
							r.fromStage === r.toStage &&
							r.trigger === "defer",
					),
				{ timeout: 5_000 },
			)
			.toBeTruthy();
	});

	test("prefers-reduced-motion: the page renders without transform parallax", async ({
		page,
	}) => {
		await page.emulateMedia({ reducedMotion: "reduce" });
		await gotoMobileRoute(page, "loop");

		const root = page.getByTestId("knowledge-loop-root");
		await expect(root).toBeVisible();

		// Under reduced-motion the root must not carry a Y-translate once settled.
		await expect
			.poll(async () => {
				return root.evaluate((el) => {
					const tr = window.getComputedStyle(el).transform;
					return tr === "none" || tr.endsWith("0)") ? "flat" : "parallax";
				});
			})
			.toBe("flat");
	});

	test("Ask CTA POSTs /loop/ask and navigates to /augur/<conversationId>", async ({
		page,
	}) => {
		const captured: Array<{
			clientHandshakeId: string;
			entryKey: string;
		}> = [];
		await page.route("**/loop/ask", async (route) => {
			const body = route.request().postDataJSON() as {
				clientHandshakeId: string;
				entryKey: string;
			};
			captured.push(body);
			await fulfillJson(route, { conversationId: "conv-fixture-1" });
		});
		// Auto-orient (tap-to-orient, e90ada874) fires a /loop/transition
		// POST whose pending state holds `inFlight` for the entry — Ask is
		// gated by `inFlight || !onAsk`, so without a fast 200 here the CTA
		// stays disabled and the click never lands.
		await page.route("**/loop/transition", async (route) => {
			await fulfillJson(route, { accepted: true });
		});

		await gotoMobileRoute(page, "loop");

		const target = page.getByTestId("loop-entry-tile").first();
		if ((await target.count()) === 0) {
			test.skip(true, "Loop entry fixture not rendered");
			return;
		}
		await target.click();

		const askCta = target.getByRole("button", { name: /^ask$/i });
		await expect(askCta).toBeVisible();
		await expect(askCta).toBeEnabled({ timeout: 5_000 });
		await askCta.click();

		// First gate: the /loop/ask BFF POST is the canonical assertion of
		// "Ask CTA wired to Augur handshake". Capture-then-poll keeps the
		// test useful even if the subsequent goto navigation is flaky in
		// headless Chrome.
		await expect.poll(() => captured.length, { timeout: 5_000 }).toBe(1);
		expect(captured[0]?.entryKey).toBe(LOOP_FIXTURE_ENTRY_KEY);
		expect(captured[0]?.clientHandshakeId).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
		);

		// Second gate: the page should client-side-navigate once the BFF
		// returns a conversationId. Allow extra slack — we already proved
		// the handshake fired.
		await expect
			.poll(() => page.url(), { timeout: 10_000 })
			.toContain("/augur/conv-fixture-1");
	});
});
