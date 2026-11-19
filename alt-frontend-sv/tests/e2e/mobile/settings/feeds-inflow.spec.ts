import { CONNECT_FEED_INFLOW_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

test.describe("mobile settings/feeds — feed inflow and daily caps", () => {
	test("renders 30-day inflow figures, legacy opened count only when > 0, and en-dash for zero inflow", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "settings/feeds");

		await expect(
			page.getByRole("heading", { name: "Inflow, last 30 days" }),
		).toBeVisible();
		await expect(
			page.getByText(
				"A daily cap limits how many articles from a feed appear in Swipe and Gallery each day. Search still finds the rest.",
			),
		).toBeVisible();

		const rows = page.getByTestId("inflow-row");
		await expect(rows).toHaveCount(3);

		// Default order: Most inflow (420, 180, 0)
		const engagedRow = rows.nth(0);
		await expect(engagedRow).toContainText("engaged.example");
		await expect(engagedRow).toContainText("/atom.xml");
		await expect(engagedRow.getByTestId("inflow-count")).toHaveText("420");
		await expect(engagedRow.getByTestId("inflow-read-count")).toHaveText("210");
		await expect(engagedRow.getByTestId("inflow-read-rate")).toHaveText("50%");
		await expect(engagedRow.getByTestId("inflow-legacy-opened")).toHaveText(
			"Opened (older): 8",
		);

		const noisyRow = rows.nth(1);
		await expect(noisyRow).toContainText("noisy.example");
		await expect(noisyRow).toContainText("/feed.xml");
		await expect(noisyRow.getByTestId("inflow-read-rate")).toHaveText("5%");
		await expect(noisyRow.getByTestId("inflow-legacy-opened")).toHaveCount(0);

		const quietRow = rows.nth(2);
		await expect(quietRow).toContainText("alpha-quiet.example");
		await expect(quietRow).toContainText("/rss.xml");
		await expect(quietRow.getByTestId("inflow-read-rate")).toHaveText("–");
		await expect(quietRow.getByTestId("inflow-legacy-opened")).toHaveCount(0);
	});

	test("sorting by Lowest read rate reorders feeds and puts zero-inflow last", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "settings/feeds");

		const rows = page.getByTestId("inflow-row");
		await expect(rows).toHaveCount(3);
		await expect(rows.nth(0)).toContainText("engaged.example");

		await page
			.getByTestId("inflow-sort-select")
			.selectOption("lowest-read-rate");

		await expect(rows.nth(0)).toContainText("noisy.example");
		await expect(rows.nth(1)).toContainText("engaged.example");
		await expect(rows.nth(2)).toContainText("alpha-quiet.example");
	});

	test("setting a daily cap persists across reload and clearing it removes the cap", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "settings/feeds");

		const noisyRow = page
			.getByTestId("inflow-row")
			.filter({ hasText: "noisy.example" });
		await expect(noisyRow.getByTestId("inflow-cap-value")).toHaveText("No cap");

		await noisyRow.getByTestId("inflow-cap-edit").click();
		const input = noisyRow.getByTestId("inflow-cap-input");
		await input.fill("25");
		await noisyRow.getByTestId("inflow-cap-save").click();

		await expect(noisyRow.getByTestId("inflow-cap-value")).toHaveText(
			"25 per day",
		);

		// Reload and confirm the mock backend persisted the cap
		await page.reload();
		await expect(noisyRow.getByTestId("inflow-cap-value")).toHaveText(
			"25 per day",
		);

		// Clear cap and confirm it persists across reload
		await noisyRow.getByTestId("inflow-cap-edit").click();
		await noisyRow.getByTestId("inflow-cap-clear").click();
		await expect(noisyRow.getByTestId("inflow-cap-value")).toHaveText("No cap");

		await page.reload();
		await expect(noisyRow.getByTestId("inflow-cap-value")).toHaveText("No cap");
	});

	test("failed save rolls back optimistic cap and shows inline alert", async ({
		page,
	}) => {
		await page.route(
			CONNECT_FEED_INFLOW_PATHS.setSubscriptionDailyCap,
			(route) =>
				fulfillJson(
					route,
					{ code: "internal", message: "Failed to save cap" },
					500,
				),
		);

		await gotoMobileRoute(page, "settings/feeds");

		const engagedRow = page
			.getByTestId("inflow-row")
			.filter({ hasText: "engaged.example" });
		await expect(engagedRow.getByTestId("inflow-cap-value")).toHaveText(
			"No cap",
		);

		await engagedRow.getByTestId("inflow-cap-edit").click();
		await engagedRow.getByTestId("inflow-cap-input").fill("30");
		await engagedRow.getByTestId("inflow-cap-save").click();

		await expect(engagedRow.getByTestId("inflow-cap-value")).toHaveText(
			"No cap",
		);
		await expect(engagedRow.getByRole("alert")).toBeVisible();
	});

	test("has no horizontal overflow and meets 44px touch target floor at 360px", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 360, height: 800 });
		await gotoMobileRoute(page, "settings/feeds");

		const rows = page.getByTestId("inflow-row");
		await expect(rows).toHaveCount(3);

		const overflow = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}));
		expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

		const editBox = await rows
			.first()
			.getByTestId("inflow-cap-edit")
			.boundingBox();
		expect(editBox).not.toBeNull();
		expect(editBox?.height).toBeGreaterThanOrEqual(44);

		const sortBox = await page.getByTestId("inflow-sort-select").boundingBox();
		expect(sortBox).not.toBeNull();
		expect(sortBox?.height).toBeGreaterThanOrEqual(44);
	});
});
