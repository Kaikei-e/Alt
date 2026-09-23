import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../fixtures/factories";
import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";

test.describe("Desktop 3-Day Topic Cards", () => {
	test("renders job window, rank, headline, what, why, genre, continues marker, and sources", async ({
		page,
		desktopRecapCardsPage,
	}) => {
		const mockData = buildMockRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecapCards, (route) =>
			fulfillJson(route, mockData),
		);

		await desktopRecapCardsPage.goto();
		await desktopRecapCardsPage.waitForLoaded();

		// Page title
		await expect(desktopRecapCardsPage.pageTitle).toBeVisible();

		// Job window: rendered with local date format (e.g., Sep 19, 2026 – Sep 22, 2026)
		await expect(desktopRecapCardsPage.jobWindow).toBeVisible();
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
		await expect(desktopRecapCardsPage.jobWindow).toContainText(expectedFrom);
		await expect(desktopRecapCardsPage.jobWindow).toContainText(expectedTo);

		// Card 1: rank 1, headline, what, why, genre
		const card1 = desktopRecapCardsPage.getCardByRank(1);
		await expect(card1).toBeVisible();
		await expect(desktopRecapCardsPage.getCardHeadline(1)).toHaveText(
			"大規模推論モデルの国内展開が加速",
		);
		await expect(desktopRecapCardsPage.getCardWhat(1)).toContainText(
			"最新の推論モデルが国内クラウドで提供開始された。",
		);
		await expect(desktopRecapCardsPage.getCardWhy(1)).toBeVisible();
		await expect(desktopRecapCardsPage.getCardWhy(1)).toContainText(
			"機密データ保護と低遅延運用の両立が期待される。",
		);
		await expect(desktopRecapCardsPage.getCardGenre(1)).toBeVisible();
		await expect(desktopRecapCardsPage.getCardGenre(1)).toHaveText(
			"Technology",
		);
		await expect(desktopRecapCardsPage.getCardContinues(1)).not.toBeVisible();

		// Sources on card 1: links opening in new tab with rel=noopener
		const sources1 = desktopRecapCardsPage.getCardSources(1);
		await expect(sources1).toHaveCount(2);
		const firstSource = sources1.first();
		await expect(firstSource).toHaveAttribute("target", "_blank");
		const rel = await firstSource.getAttribute("rel");
		expect(rel).toContain("noopener");
		await expect(firstSource).toContainText("[1]");
		await expect(firstSource).toContainText("国内オープンソースAIの動向 1");
		await expect(firstSource).toContainText("example.com");

		// Card 2: why_ja is null (not visible), continues_card_id is set ("Continues" visible)
		const card2 = desktopRecapCardsPage.getCardByRank(2);
		await expect(card2).toBeVisible();
		await expect(desktopRecapCardsPage.getCardWhy(2)).not.toBeVisible();
		await expect(desktopRecapCardsPage.getCardContinues(2)).toBeVisible();
		await expect(desktopRecapCardsPage.getCardContinues(2)).toContainText(
			"Continues",
		);
		await expect(desktopRecapCardsPage.getCardGenre(2)).toHaveText("Web");

		// Card 3: genre is null (not visible), why_ja is present, continues is absent
		const card3 = desktopRecapCardsPage.getCardByRank(3);
		await expect(card3).toBeVisible();
		await expect(desktopRecapCardsPage.getCardGenre(3)).not.toBeVisible();
		await expect(desktopRecapCardsPage.getCardWhy(3)).toBeVisible();
		await expect(desktopRecapCardsPage.getCardContinues(3)).not.toBeVisible();
	});

	test("shows empty state when job is null", async ({
		page,
		desktopRecapCardsPage,
	}) => {
		const emptyData = buildMockEmptyRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecapCards, (route) =>
			fulfillJson(route, emptyData),
		);

		await desktopRecapCardsPage.goto();
		await desktopRecapCardsPage.waitForLoaded();

		await expect(desktopRecapCardsPage.emptyState).toBeVisible();
		await expect(desktopRecapCardsPage.emptyState).toContainText(
			"No topic cards yet",
		);
		// Plus one sentence
		await expect(desktopRecapCardsPage.emptyState).toContainText(
			"Three-day topic recap cards will appear here once generated.",
		);
		await expect(desktopRecapCardsPage.cards).toHaveCount(0);
	});

	test("shows degraded notice when job.degraded is true", async ({
		page,
		desktopRecapCardsPage,
	}) => {
		const degradedData = buildMockDegradedRecapCardsResponse();
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecapCards, (route) =>
			fulfillJson(route, degradedData),
		);

		await desktopRecapCardsPage.goto();
		await desktopRecapCardsPage.waitForLoaded();

		await expect(desktopRecapCardsPage.degradedNotice).toBeVisible();
		await expect(desktopRecapCardsPage.degradedNotice).toContainText(
			/degraded/i,
		);
		await expect(desktopRecapCardsPage.cards).toHaveCount(1);
	});

	test("shows error state when RPC fails", async ({
		page,
		desktopRecapCardsPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecapCards, (route) =>
			fulfillConnectError(route, "Internal server error", "internal"),
		);

		await desktopRecapCardsPage.goto();
		await desktopRecapCardsPage.waitForLoaded();

		await expect(desktopRecapCardsPage.errorMessage).toBeVisible();
		await expect(desktopRecapCardsPage.retryButton).toBeVisible();
	});

	test("links from legacy recap page to /recap/cards", async ({
		page,
		desktopRecapPage,
	}) => {
		await desktopRecapPage.goto();

		const topicCardsLink = desktopRecapPage.topicCardsLink;
		await expect(topicCardsLink).toBeVisible();
		await expect(topicCardsLink).toHaveAttribute("href", "/recap/cards");

		await topicCardsLink.click();
		await expect(page).toHaveURL(/\/recap\/cards$/);
	});
});
