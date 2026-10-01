import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../fixtures/factories";
import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";

test.describe("Mobile Topic Cards", () => {
	test("renders job window, freshness line, rank, headline, what, why, genre, continues marker, and sources", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		const mockData = buildMockRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillJson(route, mockData),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		// Page title & subtitle
		await expect(mobileRecapCardsPage.pageTitle).toBeVisible();
		await expect(page.locator("header").first()).toContainText(
			"Daily cross-source topic cards",
		);

		// Freshness line
		await expect(page.getByTestId("recap-cards-freshness")).toBeVisible();
		await expect(page.getByTestId("recap-cards-freshness")).toContainText(
			"Updated",
		);

		// Job window
		await expect(mobileRecapCardsPage.jobWindow).toBeVisible();
		await expect(mobileRecapCardsPage.jobWindow).toContainText(
			"72-hour window",
		);
		if (!mockData.job) {
			throw new Error("mockData.job is required for this test");
		}
		const expectedFrom = new Date(mockData.job.from).toLocaleDateString(
			"en-US",
			{ month: "short", day: "numeric" },
		);
		const expectedTo = new Date(mockData.job.to).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
		});
		await expect(mobileRecapCardsPage.jobWindow).toContainText(expectedFrom);
		await expect(mobileRecapCardsPage.jobWindow).toContainText(expectedTo);

		// Card 1: rank 1, headline, what, why, genre
		const card1 = mobileRecapCardsPage.getCardByRank(1);
		await expect(card1).toBeVisible();
		await expect(mobileRecapCardsPage.getCardHeadline(1)).toHaveText(
			"大規模推論モデルの国内展開が加速",
		);
		await expect(mobileRecapCardsPage.getCardWhat(1)).toContainText(
			"最新の推論モデルが国内クラウドで提供開始された。",
		);
		await expect(mobileRecapCardsPage.getCardWhy(1)).toBeVisible();
		await expect(mobileRecapCardsPage.getCardGenre(1)).toHaveText("Technology");
		await expect(mobileRecapCardsPage.getCardContinues(1)).not.toBeVisible();

		// Sources on card 1
		const sources1 = mobileRecapCardsPage.getCardSources(1);
		await expect(sources1).toHaveCount(2);
		const firstSource = sources1.first();
		await expect(firstSource).toHaveAttribute("target", "_blank");
		const rel = await firstSource.getAttribute("rel");
		expect(rel).toContain("noopener");
		await expect(firstSource).toContainText("[1]");
		await expect(firstSource).toContainText("国内オープンソースAIの動向 1");

		// Card 2: why_ja is null, continues_card_id is set
		const card2 = mobileRecapCardsPage.getCardByRank(2);
		await expect(card2).toBeVisible();
		await expect(mobileRecapCardsPage.getCardWhy(2)).not.toBeVisible();
		await expect(mobileRecapCardsPage.getCardContinues(2)).toBeVisible();
		await expect(mobileRecapCardsPage.getCardContinues(2)).toContainText(
			"Continues",
		);

		// Card 3: genre is null
		const card3 = mobileRecapCardsPage.getCardByRank(3);
		await expect(card3).toBeVisible();
		await expect(mobileRecapCardsPage.getCardGenre(3)).not.toBeVisible();
	});

	test("shows empty state when job is null and no active run", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		const emptyData = buildMockEmptyRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillJson(route, emptyData),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		await expect(mobileRecapCardsPage.emptyState).toBeVisible();
		await expect(mobileRecapCardsPage.emptyState).toContainText(
			"No topic cards yet",
		);
		await expect(mobileRecapCardsPage.emptyState).toContainText(
			"Topic cards will appear here after the first daily run.",
		);
		await expect(mobileRecapCardsPage.cards).toHaveCount(0);
	});

	for (const status of ["running", "pending"] as const) {
		test(`shows running status notice when latestRun is ${status}`, async ({
			page,
			mobileRecapCardsPage,
		}) => {
			const mockData = buildMockRecapCardsResponse({
				latestRun: {
					jobId: `run-${status}-001`,
					status,
					kickedAt: "2026-09-22T19:00:00Z",
					updatedAt: "2026-09-22T19:01:00Z",
				},
			});
			await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
				fulfillJson(route, mockData),
			);

			await mobileRecapCardsPage.goto();
			await mobileRecapCardsPage.waitForLoaded();

			await expect(
				page.getByText(/Update in progress \(started /),
			).toBeVisible();
			await expect(page.getByRole("status")).toContainText(
				"Update in progress",
			);
			await expect(mobileRecapCardsPage.cards).toHaveCount(3);
		});
	}

	test("shows failed status notice with job context when latestRun is failed and job exists", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		const mockData = buildMockRecapCardsResponse({
			latestRun: {
				jobId: "run-failed-001",
				status: "failed",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:02:00Z",
			},
		});
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillJson(route, mockData),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		await expect(
			page.getByText(/The latest update failed at .* Showing cards from /),
		).toBeVisible();
		await expect(page.getByRole("alert")).toContainText(
			"The latest update failed",
		);
		await expect(mobileRecapCardsPage.cards).toHaveCount(3);
	});

	test("shows failed status notice without job context when latestRun is failed and job is null", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		const mockData = buildMockEmptyRecapCardsResponse({
			latestRun: {
				jobId: "run-failed-002",
				status: "failed",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:02:00Z",
			},
		});
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillJson(route, mockData),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		await expect(page.getByText(/The latest update failed at /)).toBeVisible();
		await expect(page.getByRole("alert")).toContainText(
			"The latest update failed",
		);
		await expect(page.getByText(/Showing cards from/)).not.toBeVisible();
		await expect(mobileRecapCardsPage.cards).toHaveCount(0);
	});

	test("shows degraded notice when job.degraded is true", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		const degradedData = buildMockDegradedRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillJson(route, degradedData),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		await expect(mobileRecapCardsPage.degradedNotice).toBeVisible();
		await expect(mobileRecapCardsPage.degradedNotice).toContainText(
			/degraded/i,
		);
		await expect(mobileRecapCardsPage.cards).toHaveCount(1);
	});

	test("shows error state when RPC fails", async ({
		page,
		mobileRecapCardsPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getTopicCards, (route) =>
			fulfillConnectError(route, "Internal server error", "internal"),
		);

		await mobileRecapCardsPage.goto();
		await mobileRecapCardsPage.waitForLoaded();

		await expect(mobileRecapCardsPage.errorMessage).toBeVisible();
		await expect(mobileRecapCardsPage.retryButton).toBeVisible();
	});

	test("links from legacy recap page to /recap/cards", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await mobile3DayRecapPage.goto();

		const topicCardsLink = mobile3DayRecapPage.topicCardsLink;
		await expect(topicCardsLink).toBeVisible();
		await expect(topicCardsLink).toHaveAttribute("href", "/recap/cards");

		await topicCardsLink.click();
		await expect(page).toHaveURL(/\/recap\/cards$/);
	});
});
