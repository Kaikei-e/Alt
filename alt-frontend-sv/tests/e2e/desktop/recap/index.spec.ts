import {
	CONNECT_RECAP_EMPTY_RESPONSE,
	CONNECT_RECAP_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { DesktopRecapPage } from "../../pages/desktop/DesktopRecapPage";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";

test.describe("Desktop Recap", () => {
	let recapPage: DesktopRecapPage;

	test.beforeEach(async ({ page }) => {
		recapPage = new DesktopRecapPage(page);
	});

	test("renders page title and genre list", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify page title (PageHeader has static "Recap" title)
		await expect(recapPage.pageTitle).toBeVisible();
		await expect(recapPage.pageTitle).toContainText("Recap");

		// Verify window info is displayed (default is 3-day)
		await expect(page.getByText("3-day window")).toBeVisible();

		// Verify genre list is visible
		await expect(recapPage.genreList).toBeVisible();
	});

	test("displays genre items from API response", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Check genre buttons exist
		await expect(recapPage.getGenreByName("Technology")).toBeVisible();
		await expect(recapPage.getGenreByName("AI/ML")).toBeVisible();
	});

	test("auto-selects first genre on load", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// The first genre in the API response ("Technology") should be
		// auto-selected: its heading and summary should render, and the
		// genre list should mark it visually selected.
		await expect(recapPage.getRecapDetailHeading()).toHaveText("Technology");
		await expect(
			page.getByText("Major developments in technology this week."),
		).toBeVisible();
		await expect.poll(() => recapPage.isGenreSelected("Technology")).toBe(true);
	});

	test("switches genre when clicking another genre", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Sanity check on the pre-click state so the assertion below proves
		// a real transition rather than being trivially true either way.
		await expect(recapPage.getRecapDetailHeading()).toHaveText("Technology");

		// Click on AI/ML genre
		await recapPage.selectGenre("AI/ML");

		// Detail section should update to the newly selected genre's
		// content and drop the previous genre's content.
		await expect(recapPage.getRecapDetailHeading()).toHaveText("AI/ML");
		await expect(
			page.getByText("Latest papers and breakthroughs in ML."),
		).toBeVisible();
		await expect(
			page.getByText("Major developments in technology this week."),
		).not.toBeVisible();
		await expect.poll(() => recapPage.isGenreSelected("AI/ML")).toBe(true);
		await expect
			.poll(() => recapPage.isGenreSelected("Technology"))
			.toBe(false);
	});

	test("shows empty state when no recap data", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_EMPTY_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify empty state
		await expect(recapPage.emptyState).toBeVisible();
	});

	test("shows error state on API failure", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillConnectError(route, "Server error", "internal"),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify error message
		await expect(recapPage.errorMessage).toBeVisible();
	});

	test("shows loading spinner while fetching", async ({ page }) => {
		// Delay the response to observe loading state
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		await recapPage.goto();

		// Loading spinner should be visible initially
		await expect(recapPage.loadingSpinner).toBeVisible();

		// Then it should disappear
		await recapPage.waitForRecapLoaded();
		await expect(recapPage.loadingSpinner).not.toBeVisible();
	});
});

test.describe("Desktop Recap - Genre Selection", () => {
	test("genre list maintains selection state", async ({ page }) => {
		const recapPage = new DesktopRecapPage(page);

		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Select each genre in turn and verify the selection state settles
		// on that genre before moving to the next one — expect.poll
		// auto-retries against the real UI state instead of guessing a
		// fixed settle time.
		const genres = ["Technology", "AI/ML"];

		for (const genre of genres) {
			await recapPage.selectGenre(genre);
			await expect.poll(() => recapPage.isGenreSelected(genre)).toBe(true);
			await expect(recapPage.getRecapDetailHeading()).toHaveText(genre);
		}

		// Final selection should be AI/ML, and the previously selected
		// Technology genre should no longer show as selected.
		await expect.poll(() => recapPage.isGenreSelected("AI/ML")).toBe(true);
		await expect
			.poll(() => recapPage.isGenreSelected("Technology"))
			.toBe(false);
	});
});

test.describe("Desktop Recap - 7-Day Window", () => {
	test("switches to 7-day recap when clicking 7 Days button", async ({
		page,
	}) => {
		const recapPage = new DesktopRecapPage(page);

		// Mock both API endpoints
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify initial 3-day state
		await expect(page.getByText("3-day window")).toBeVisible();

		// Click 7 Days button
		await page.getByRole("button", { name: "7 Days" }).click();
		await recapPage.waitForRecapLoaded();

		// Verify window info updated to 7-day
		await expect(page.getByText("7-day window")).toBeVisible();
	});
});
