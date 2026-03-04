import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_EVENING_PULSE_PATH,
	CONNECT_EVENING_PULSE_RESPONSE,
	CONNECT_EVENING_PULSE_QUIET_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Mobile Evening Pulse", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE),
		);
	});

	test("shows loading skeleton", async ({ page, mobileEveningPulsePage }) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE);
		});

		await mobileEveningPulsePage.goto();
		await expect(mobileEveningPulsePage.skeleton).toBeVisible();
	});

	test("renders pulse data", async ({ page, mobileEveningPulsePage }) => {
		await mobileEveningPulsePage.goto();
		await mobileEveningPulsePage.waitForPulseLoaded();

		await expect(page.getByText("AI Breakthrough")).toBeVisible();
	});

	test("shows quiet day state", async ({ page, mobileEveningPulsePage }) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillJson(route, CONNECT_EVENING_PULSE_QUIET_RESPONSE),
		);

		await mobileEveningPulsePage.goto();
		await mobileEveningPulsePage.waitForPulseLoaded();
		await expect(mobileEveningPulsePage.quietDayMessage).toBeVisible();
	});

	test("shows error state with retry", async ({
		page,
		mobileEveningPulsePage,
	}) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await mobileEveningPulsePage.goto();
		await expect(mobileEveningPulsePage.errorState).toBeVisible({
			timeout: 10000,
		});
		await expect(mobileEveningPulsePage.retryButton).toBeVisible();
	});

	test("retry triggers re-fetch", async ({ page, mobileEveningPulsePage }) => {
		let requestCount = 0;
		await page.route(CONNECT_EVENING_PULSE_PATH, async (route) => {
			requestCount++;
			if (requestCount === 1) {
				await fulfillError(route, "Server error", 500);
			} else {
				await fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE);
			}
		});

		await mobileEveningPulsePage.goto();
		await expect(mobileEveningPulsePage.errorState).toBeVisible({
			timeout: 10000,
		});
		await mobileEveningPulsePage.retryButton.click();
		await expect(mobileEveningPulsePage.errorState).not.toBeVisible({
			timeout: 10000,
		});
	});
});
