import { test, expect } from "@playwright/test";

// E2E for the Knowledge Trail spine (Wave 2, read-only). Pull-only: the page
// loads on navigation and refreshes only on explicit action — there is no live
// channel to wait on (the deliberate PM-2026-039 / PM-2026-045 lesson).
test.describe("Knowledge Trail spine", () => {
	test("loads the trail page with an editorial header", async ({ page }) => {
		await page.goto("./knowledge/trail");
		await expect(page.getByRole("heading", { name: "Your Trail" })).toBeVisible({
			timeout: 15000,
		});
	});

	test("renders either footprints or the empty-state, never a spinner forever", async ({
		page,
	}) => {
		await page.goto("./knowledge/trail");

		const spine = page.getByTestId("trail-spine");
		await expect(spine).toBeVisible({ timeout: 15000 });

		const footprints = page.getByTestId("trail-footprint");
		const empty = page.getByTestId("trail-empty");
		await expect(footprints.first().or(empty)).toBeVisible({ timeout: 15000 });
	});

	test("exposes an explicit refresh affordance (pull-only)", async ({ page }) => {
		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-refresh")).toBeVisible({
			timeout: 15000,
		});
	});
});
