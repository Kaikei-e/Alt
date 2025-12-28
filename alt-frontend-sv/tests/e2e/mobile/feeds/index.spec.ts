import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

test.describe("mobile feeds routes", () => {
	test("feeds list renders and supports mark-as-read", async ({ page }) => {
		// Changing approach: Use page.waitForRequest for verification

		await gotoMobileRoute(page, "feeds");

		const cards = page.getByTestId("feed-card");
		await expect(cards).toHaveCount(2);

		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Set up the request promise BEFORE clicking the button
		const requestPromise = page.waitForRequest(
			(req) =>
				req.url().includes("/api/v1/feeds/read") && req.method() === "POST",
		);

		await firstCard
			.getByRole("button", { name: /mark .* as read/i })
			.click({ force: true });

		await expect(firstCard).toHaveCount(0);

		// I can do:
		// const requestPromise = page.waitForRequest(req => req.url().includes('/api/v1/feeds/read') && req.method() === 'POST');
		// await firstCard...click()
		// const request = await requestPromise;
		// expect(request.postDataJSON())...

		// BUT I am inside a replacement block. I need to write valid code.
	});
});
