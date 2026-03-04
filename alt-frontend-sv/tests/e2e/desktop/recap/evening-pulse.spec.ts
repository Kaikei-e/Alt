import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_EVENING_PULSE_PATH,
	CONNECT_EVENING_PULSE_RESPONSE,
	CONNECT_EVENING_PULSE_QUIET_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Desktop Evening Pulse", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE),
		);
	});

	test("shows loading skeleton initially", async ({
		page,
		desktopEveningPulsePage,
	}) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE);
		});

		await desktopEveningPulsePage.goto();
		await expect(desktopEveningPulsePage.skeleton).toBeVisible();
	});

	test("renders pulse data with topic cards", async ({
		page,
		desktopEveningPulsePage,
	}) => {
		await desktopEveningPulsePage.goto();
		await desktopEveningPulsePage.waitForPulseLoaded();

		await expect(page.getByText("AI Breakthrough")).toBeVisible();
		await expect(page.getByText("Web Standards Update")).toBeVisible();
	});

	test("shows quiet day state", async ({ page, desktopEveningPulsePage }) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillJson(route, CONNECT_EVENING_PULSE_QUIET_RESPONSE),
		);

		await desktopEveningPulsePage.goto();
		await desktopEveningPulsePage.waitForPulseLoaded();
		await expect(desktopEveningPulsePage.quietDayMessage).toBeVisible();
	});

	test("shows error state with retry", async ({
		page,
		desktopEveningPulsePage,
	}) => {
		await page.route(CONNECT_EVENING_PULSE_PATH, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await desktopEveningPulsePage.goto();
		await expect(desktopEveningPulsePage.errorState).toBeVisible({
			timeout: 10000,
		});
		await expect(desktopEveningPulsePage.retryButton).toBeVisible();
	});

	test("retry triggers re-fetch", async ({ page, desktopEveningPulsePage }) => {
		let requestCount = 0;
		await page.route(CONNECT_EVENING_PULSE_PATH, async (route) => {
			requestCount++;
			if (requestCount === 1) {
				await fulfillError(route, "Server error", 500);
			} else {
				await fulfillJson(route, CONNECT_EVENING_PULSE_RESPONSE);
			}
		});

		await desktopEveningPulsePage.goto();
		await expect(desktopEveningPulsePage.errorState).toBeVisible({
			timeout: 10000,
		});
		await desktopEveningPulsePage.retryButton.click();

		await expect(desktopEveningPulsePage.errorState).not.toBeVisible({
			timeout: 10000,
		});
	});
});
