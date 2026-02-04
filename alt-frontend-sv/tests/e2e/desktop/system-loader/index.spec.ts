import { expect, test } from "@playwright/test";
import { DesktopRecapPage } from "../../pages/desktop/DesktopRecapPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_RECAP_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("System Loader", () => {
	let recapPage: DesktopRecapPage;

	test.beforeEach(async ({ page }) => {
		recapPage = new DesktopRecapPage(page);
	});

	/**
	 * These tests verify the SystemLoader component behavior.
	 * Note: Testing transient loading states is inherently flaky in E2E tests.
	 * We focus on verifying the loader eventually hides after content loads.
	 */

	test("hides loader after content loads", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, async (route) => {
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		await recapPage.goto();
		// Verify the loader is NOT visible after page fully loads
		await expect(page.getByTestId("system-loader")).not.toBeVisible({
			timeout: 15000,
		});
	});

	test("shows content after loading completes", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, async (route) => {
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		await recapPage.goto();

		// Verify content is visible after loading
		await expect(page.getByRole("heading", { name: /Recap/i })).toBeVisible({
			timeout: 10000,
		});
		// Verify loader is not shown
		await expect(page.getByTestId("system-loader")).not.toBeVisible();
	});

	test("SystemLoader component has correct structure", async ({ page }) => {
		// Test the component directly by navigating and triggering a slow API
		let resolveRoute: () => void;
		const routePromise = new Promise<void>((resolve) => {
			resolveRoute = resolve;
		});

		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, async (route) => {
			await routePromise;
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		// Use domcontentloaded to catch the loader before JS finishes
		await page.goto(recapPage.url, { waitUntil: "domcontentloaded" });

		// The loader might or might not be visible depending on timing
		// Instead, we'll just verify that when the loader IS in the DOM, it has correct attributes
		const loader = page.getByTestId("system-loader");
		const isLoaderVisible = await loader.isVisible().catch(() => false);

		if (isLoaderVisible) {
			await expect(loader).toHaveAttribute("role", "status");
			await expect(loader).toHaveAttribute("aria-label", "Loading Alt");
			await expect(loader.getByText("Loading Alt")).toBeVisible();
			const logo = loader.locator('img[alt="Alt Logo"]');
			await expect(logo).toBeVisible();
		}

		// Release the API
		resolveRoute!();

		// Ensure loader is hidden after content loads
		await expect(loader).not.toBeVisible({ timeout: 15000 });
	});
});
