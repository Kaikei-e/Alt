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
 * promise rejection and asserts that the reload-attempt counter in
 * sessionStorage was bumped — the scheduler bumps it just before
 * calling `location.reload()`, so we can verify intent without losing
 * the test page to an actual navigation.
 */
test.describe("client chunk-load failure self-recovers via reload", () => {
	test("hooks.client bumps the reload counter on a chunk-load rejection", async ({
		page,
	}) => {
		await page.goto("/");
		await page.waitForLoadState("domcontentloaded");

		await page.evaluate(() => {
			// Reset any prior state and stub the reload so the test page
			// does not navigate away — we only verify the schedule path.
			sessionStorage.removeItem("alt:chunk-reload-attempts");
			(
				window as unknown as { __reloadCalled: boolean }
			).__reloadCalled = false;
			Object.defineProperty(window.location, "reload", {
				configurable: true,
				value: () => {
					(
						window as unknown as { __reloadCalled: boolean }
					).__reloadCalled = true;
				},
			});

			queueMicrotask(() => {
				Promise.reject(
					new Error(
						"Failed to fetch dynamically imported module: /_app/immutable/chunks/synthetic-test.js",
					),
				);
			});
		});

		await expect
			.poll(
				async () =>
					await page.evaluate(() => ({
						counter: sessionStorage.getItem("alt:chunk-reload-attempts"),
						reloaded: (
							window as unknown as { __reloadCalled: boolean }
						).__reloadCalled,
					})),
				{ timeout: 5_000 },
			)
			.toMatchObject({ counter: "1", reloaded: true });
	});
});
