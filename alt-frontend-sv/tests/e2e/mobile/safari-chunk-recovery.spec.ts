import { expect, test } from "@playwright/test";

/**
 * Phase 5 regression: deploys rotate /_app/immutable/* hashes. When an
 * iOS Safari tab returns from suspend with a stale module reference it
 * hits 404 on the chunk fetch and surfaces "Cannot Open the Page" in
 * Chromium-style soft-fail (re-nav) or full WebKit-style hard-fail. The
 * hooks.client error handler must intercept the resulting
 * `Failed to fetch dynamically imported module` rejection and force a
 * full reload, breaking out of the broken state without user action.
 *
 * The test triggers the failure path synthetically via an unhandled
 * promise rejection and lets the real reload fire — `sessionStorage`
 * survives that reload, so the bumped `alt:chunk-reload-attempts`
 * counter persists and proves the scheduler took both legs (bump + go).
 * `window.location.reload` is non-configurable on Chromium, so stubbing
 * it cross-browser is not viable; observing the persisted counter is.
 */
test.describe("client chunk-load failure self-recovers via reload", () => {
	test("hooks.client bumps the reload counter on a chunk-load rejection", async ({
		page,
	}) => {
		await page.goto("/");
		await page.waitForLoadState("domcontentloaded");

		await page.evaluate(() => {
			sessionStorage.removeItem("alt:chunk-reload-attempts");
		});

		// Arm a load-event waiter *before* we trigger the rejection; the next
		// load event must come from the scheduler-driven location.reload().
		const reloaded = page.waitForEvent("load", { timeout: 8_000 });

		await page.evaluate(() => {
			queueMicrotask(() => {
				Promise.reject(
					new Error(
						"Failed to fetch dynamically imported module: /_app/immutable/chunks/synthetic-test.js",
					),
				);
			});
		});

		await reloaded;
		await page.waitForLoadState("domcontentloaded");

		await expect
			.poll(
				async () =>
					await page.evaluate(() =>
						sessionStorage.getItem("alt:chunk-reload-attempts"),
					),
				{ timeout: 5_000 },
			)
			.toBe("1");
	});
});
