import { expect, test } from "../../fixtures/pomFixtures";

/**
 * The subscribe funnel, end to end on the frontend side.
 *
 * A push subscription is the first genuine per-user preference in this system —
 * there is no preferences table, no per-user config RPC, and no preferences
 * store to hang it on. So this spec pins the contract the rest of the feature
 * is built against: turning a notification type on must (a) obtain a browser
 * subscription and (b) register it with the backend, and the UI must not claim
 * "enabled" until the registration actually lands.
 *
 * The browser side is stubbed rather than driven for real: Chromium will not
 * mint a real PushSubscription without a live push service and a valid VAPID
 * key, and this spec is about the app's wiring, not about FCM. `page.route`
 * intercepts the RPC for the same reason every other spec here does — the
 * mock backend at :4003 has no push service.
 *
 * Deliberately NOT asserted here:
 *  - the request body shape. That is the job of a schema-conformance test in
 *    `src/test/contracts/`, where a proto change breaks compilation rather
 *    than producing a confusing E2E diff.
 *  - actual push delivery. There is no delivery receipt in Web Push at all,
 *    so no test can assert it; a 201 only means the push service accepted the
 *    message.
 */

const REGISTER_RPC = "**/api/v2/alt.push.v1.PushService/RegisterSubscription";

/** Matches the four notification kinds the settings page exposes. */
const RECAP_TOGGLE = "notification-toggle-recap-ready";

/**
 * Stands in for a browser that has a service worker and will grant permission.
 * Installed before any app code runs so the settings page sees it during init.
 */
async function stubPushCapableBrowser(page: import("@playwright/test").Page) {
	await page.addInitScript(() => {
		const fakeSubscription = {
			endpoint: "https://fcm.googleapis.com/fcm/send/e2e-fake-endpoint",
			expirationTime: null,
			toJSON() {
				return {
					endpoint: this.endpoint,
					expirationTime: null,
					keys: {
						p256dh: "BE2Ee2E2e2E2e2E2e2E2e2E2e2E2e2E2e2E2e2E2e2E",
						auth: "e2Ee2Ee2Ee2Ee2Ee2Ee2A",
					},
				};
			},
		};

		Object.defineProperty(Notification, "permission", {
			configurable: true,
			get: () => "granted",
		});
		Notification.requestPermission = async () => "granted";

		const pushManager = {
			getSubscription: async () => null,
			subscribe: async () => fakeSubscription,
			permissionState: async () => "granted",
		};

		Object.defineProperty(navigator.serviceWorker, "ready", {
			configurable: true,
			get: async () => ({ pushManager }),
		});
	});
}

test.describe("notification settings", () => {
	test("enabling a notification type registers the subscription with the backend", async ({
		page,
	}) => {
		await stubPushCapableBrowser(page);

		let registrationCalls = 0;
		await page.route(REGISTER_RPC, async (route) => {
			registrationCalls += 1;
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: "{}",
			});
		});

		await page.goto("/settings/notifications");
		await page.waitForLoadState("domcontentloaded");

		const recapToggle = page.getByTestId(RECAP_TOGGLE);
		await expect(recapToggle).toBeVisible();
		await expect(
			recapToggle,
			"a freshly loaded page with no stored subscription starts off",
		).toHaveAttribute("aria-checked", "false");

		await recapToggle.click();

		await expect
			.poll(() => registrationCalls, {
				timeout: 10_000,
				message:
					"turning a notification type on must register the subscription with the backend",
			})
			.toBeGreaterThan(0);

		await expect(
			recapToggle,
			"the toggle may only read as enabled once registration succeeded",
		).toHaveAttribute("aria-checked", "true");
	});

	test("keeps the toggle off when registration fails", async ({ page }) => {
		await stubPushCapableBrowser(page);

		await page.route(REGISTER_RPC, async (route) => {
			await route.fulfill({ status: 500, body: "" });
		});

		await page.goto("/settings/notifications");
		await page.waitForLoadState("domcontentloaded");

		const recapToggle = page.getByTestId(RECAP_TOGGLE);
		await recapToggle.click();

		// A toggle that flips optimistically and never reconciles is how a user
		// ends up believing notifications are on while nothing is subscribed —
		// the silent-failure shape this feature is specifically trying to avoid.
		await expect(recapToggle).toHaveAttribute("aria-checked", "false");
		await expect(page.getByTestId("notification-settings-error")).toBeVisible();
	});

	test("explains the Home Screen requirement when not installed", async ({
		page,
	}) => {
		await stubPushCapableBrowser(page);

		await page.goto("/settings/notifications");
		await page.waitForLoadState("domcontentloaded");

		// iOS delivers Web Push only to a Home Screen web app, and there is no
		// beforeinstallprompt on iOS to automate it — the user has to walk the
		// share sheet themselves, so the page has to say so. A regular browser
		// tab (which is what this project runs in) is the not-installed case.
		await expect(page.getByTestId("install-instructions")).toBeVisible();
	});
});
