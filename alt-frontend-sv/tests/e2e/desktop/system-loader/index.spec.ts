import { expect, test } from "@playwright/test";
import { DesktopRecapPage } from "../../pages/desktop/DesktopRecapPage";
import { fulfillJson } from "../../utils/mockHelpers";
import { RECAP_RESPONSE } from "../../fixtures/mockData";

test.describe("System Loader", () => {
	let recapPage: DesktopRecapPage;

	test.beforeEach(async ({ page }) => {
		recapPage = new DesktopRecapPage(page);
	});

	test("displays loading UI during CSR initialization", async ({ page }) => {
		// APIを遅延させてローディング状態をテスト
		await page.route("**/api/v1/recap/7days", async (route) => {
			await new Promise((r) => setTimeout(r, 1000));
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await recapPage.goto();

		// ローディング要素の確認
		const loader = page.getByTestId("system-loader");
		await expect(loader).toBeVisible();
		await expect(loader.getByText("Loading Alt")).toBeVisible();
		await expect(loader.locator(".animate-spin")).toBeVisible();
	});

	test("hides loader after content loads", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", async (route) => {
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await recapPage.goto();
		await expect(page.getByTestId("system-loader")).not.toBeVisible({
			timeout: 15000,
		});
	});

	test("displays loader with Alt logo", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", async (route) => {
			await new Promise((r) => setTimeout(r, 1000));
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await recapPage.goto();

		const loader = page.getByTestId("system-loader");
		await expect(loader).toBeVisible();

		// ロゴの確認
		const logo = loader.locator('img[alt="Alt Logo"]');
		await expect(logo).toBeVisible();
	});

	test("has correct accessibility attributes", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", async (route) => {
			await new Promise((r) => setTimeout(r, 1000));
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await recapPage.goto();

		const loader = page.getByTestId("system-loader");
		await expect(loader).toBeVisible();

		// アクセシビリティ属性の確認
		await expect(loader).toHaveAttribute("role", "status");
		await expect(loader).toHaveAttribute("aria-label", "Loading Alt");
	});
});
