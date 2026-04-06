import { expect, test } from "@playwright/test";

test.describe("Desktop Tag Verse", () => {
	test("renders Tag Verse page title", async ({ page }) => {
		await page.goto("./feeds/tag-verse");
		await page.waitForLoadState("domcontentloaded");

		// Page title should contain "Tag Verse"
		await expect(page).toHaveTitle(/Tag Verse/);
	});

	test("shows tag verse screen on desktop viewport", async ({ page }) => {
		await page.goto("./feeds/tag-verse");
		await page.waitForLoadState("domcontentloaded");

		// Desktop viewport should show the TagVerseScreen component
		// Not redirected to /home
		expect(page.url()).toContain("tag-verse");
	});
});
