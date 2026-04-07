import { expect, test } from "@playwright/test";
import { MobileKnowledgeHomePage } from "../../pages/mobile/MobileKnowledgeHomePage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	buildGetKnowledgeHomeResponse,
	KNOWLEDGE_HOME_ITEM_READY,
	KNOWLEDGE_HOME_ITEM_PENDING,
	RECALL_CANDIDATE_WITH_REASONS,
	FEATURE_FLAGS_RECALL_DISABLED,
} from "../../fixtures/factories/knowledgeHomeFactory";

const KH_GET =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome";
const KH_TRACK_ACTION =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction";
const KH_TRACK_SEEN =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeItemsSeen";
const KH_RECALL =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetRecallRail";
const KH_LIST_LENSES =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/ListLenses";
const KH_STREAM =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/StreamKnowledgeHomeUpdates";
const LIST_SUBS = "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";

function mockAllKnowledgeHomeRoutes(
	page: import("@playwright/test").Page,
	responseOverrides?: Parameters<typeof buildGetKnowledgeHomeResponse>[0],
) {
	const response = buildGetKnowledgeHomeResponse(responseOverrides);
	return Promise.all([
		page.route(KH_GET, (route) => fulfillJson(route, response)),
		page.route(KH_TRACK_SEEN, (route) => fulfillJson(route, {})),
		page.route(KH_TRACK_ACTION, (route) => fulfillJson(route, {})),
		page.route(KH_RECALL, (route) =>
			fulfillJson(route, {
				candidates: [RECALL_CANDIDATE_WITH_REASONS],
			}),
		),
		page.route(KH_LIST_LENSES, (route) =>
			fulfillJson(route, { lenses: [], activeLensId: "" }),
		),
		page.route(LIST_SUBS, (route) => fulfillJson(route, { sources: [] })),
		page.route(KH_STREAM, (route) => route.abort()),
	]);
}

test.describe("Mobile Knowledge Home", () => {
	let homePage: MobileKnowledgeHomePage;

	test.beforeEach(async ({ page }) => {
		homePage = new MobileKnowledgeHomePage(page);
		await mockAllKnowledgeHomeRoutes(page);
	});

	test("displays cards in mobile layout", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const readyCard = homePage.getCard(KNOWLEDGE_HOME_ITEM_READY.itemKey);
		await expect(readyCard).toBeVisible();

		const pendingCard = homePage.getCard(KNOWLEDGE_HOME_ITEM_PENDING.itemKey);
		await expect(pendingCard).toBeVisible();
	});

	test("shows summary excerpt for ready items", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const summary = homePage.getCardSummary(KNOWLEDGE_HOME_ITEM_READY.itemKey);
		await expect(summary).toContainText("非同期ランタイム");
	});

	test("shows Summarizing chip for pending items", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const chip = homePage.getSummarizingChip(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(chip).toBeVisible();
	});

	test("TodayBar renders Morning Letter link on mobile", async ({ page }) => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		await expect(page.getByText("Morning Letter")).toBeVisible();
	});

	test("recall candidate visible on mobile", async ({ page }) => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		await expect(page.getByText("Go Concurrency Patterns")).toBeVisible({
			timeout: 10000,
		});
	});
});

test.describe("Mobile Knowledge Home - Recall Disabled", () => {
	test("recall section hidden when flag is off", async ({ page }) => {
		const homePage = new MobileKnowledgeHomePage(page);
		await mockAllKnowledgeHomeRoutes(page, {
			featureFlags: FEATURE_FLAGS_RECALL_DISABLED,
			recallCandidates: [],
		});
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Cards should still render
		const readyCard = homePage.getCard(KNOWLEDGE_HOME_ITEM_READY.itemKey);
		await expect(readyCard).toBeVisible();

		// Recall candidate should NOT be visible
		await expect(page.getByText("Go Concurrency Patterns")).not.toBeVisible();
	});
});
