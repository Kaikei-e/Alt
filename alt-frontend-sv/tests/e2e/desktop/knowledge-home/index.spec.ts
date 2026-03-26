import { expect, test } from "@playwright/test";
import { DesktopKnowledgeHomePage } from "../../pages/desktop/DesktopKnowledgeHomePage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	buildGetKnowledgeHomeResponse,
	KNOWLEDGE_HOME_ITEM_READY,
	KNOWLEDGE_HOME_ITEM_PENDING,
	RECALL_CANDIDATE_WITH_REASONS,
} from "../../fixtures/factories/knowledgeHomeFactory";

// Connect-RPC paths via SvelteKit proxy (/api/v2)
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
		page.route(LIST_SUBS, (route) =>
			fulfillJson(route, { sources: [] }),
		),
		page.route(KH_STREAM, (route) => route.abort()),
	]);
}

test.describe("Knowledge Home - Summary Display", () => {
	let homePage: DesktopKnowledgeHomePage;

	test.beforeEach(async ({ page }) => {
		homePage = new DesktopKnowledgeHomePage(page);
		await mockAllKnowledgeHomeRoutes(page);
	});

	test("displays summary excerpt when summary_state is ready", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const readyCard = homePage.getCard(KNOWLEDGE_HOME_ITEM_READY.itemKey);
		await expect(readyCard).toBeVisible();

		// Summary excerpt text should be visible
		const summary = homePage.getCardSummary(
			KNOWLEDGE_HOME_ITEM_READY.itemKey,
		);
		await expect(summary).toContainText("非同期ランタイム");

		// "Summarizing" chip should NOT be visible on ready items
		const chip = homePage.getSummarizingChip(
			KNOWLEDGE_HOME_ITEM_READY.itemKey,
		);
		await expect(chip).not.toBeVisible();
	});

	test("displays Summarizing chip when summary_state is pending", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const pendingCard = homePage.getCard(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(pendingCard).toBeVisible();

		// "Summarizing" chip should be visible
		const chip = homePage.getSummarizingChip(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(chip).toBeVisible();

		// Skeleton placeholder should be visible instead of summary text
		const skeleton = homePage.getCardSkeleton(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(skeleton.first()).toBeVisible();
	});

	test("transitions from pending to ready on data refresh", async ({
		page,
	}) => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Initially pending
		const chip = homePage.getSummarizingChip(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(chip).toBeVisible();

		// Simulate summary completion: re-route with ready state
		const updatedItem = {
			...KNOWLEDGE_HOME_ITEM_PENDING,
			summaryState: "ready",
			summaryExcerpt:
				"WebAssembly はブラウザ以外の環境でも広く使われるようになっている。",
		};
		const updatedResponse = buildGetKnowledgeHomeResponse({
			items: [KNOWLEDGE_HOME_ITEM_READY, updatedItem],
		});
		// Unroute and re-route with updated response
		await page.unroute(KH_GET);
		await page.route(KH_GET, (route) =>
			fulfillJson(route, updatedResponse),
		);

		// Trigger refresh by navigating away and back
		await page.goto("/");
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Now the item should show summary, not the chip
		const summary = homePage.getCardSummary(
			KNOWLEDGE_HOME_ITEM_PENDING.itemKey,
		);
		await expect(summary).toContainText("WebAssembly");
		await expect(chip).not.toBeVisible();
	});
});

test.describe("Knowledge Home - Tag Click Tracking", () => {
	let homePage: DesktopKnowledgeHomePage;

	test.beforeEach(async ({ page }) => {
		homePage = new DesktopKnowledgeHomePage(page);
		await mockAllKnowledgeHomeRoutes(page);
	});

	test("clicking a tag on a card fires tag_click tracking", async ({
		page,
	}) => {
		// Intercept tag navigation to prevent leaving the page
		await page.route("**/articles/by-tag**", (route) => route.abort());

		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Find the "rust" tag on the ready card
		const tags = homePage.getCardTags(KNOWLEDGE_HOME_ITEM_READY.itemKey);
		const rustTag = tags.getByText("rust");
		await expect(rustTag).toBeVisible();

		// Set up request promise BEFORE the click
		const trackRequestPromise = page.waitForRequest(
			(req) =>
				req.url().includes("TrackHomeAction") &&
				req.postDataJSON()?.actionType === "tag_click",
		);

		await rustTag.click();
		const trackRequest = await trackRequestPromise;

		expect(trackRequest.postDataJSON().itemKey).toBe(
			KNOWLEDGE_HOME_ITEM_READY.itemKey,
		);
	});

	test("clicking Open on a card fires open tracking", async ({ page }) => {
		// Intercept article navigation to prevent leaving the page
		await page.route("**/articles/**", (route) => route.abort());

		await homePage.goto();
		await homePage.waitForHomeLoaded();

		const openButton = homePage.getOpenButton(
			KNOWLEDGE_HOME_ITEM_READY.itemKey,
		);
		await expect(openButton).toBeVisible();

		// Set up request promise BEFORE the click
		const trackRequestPromise = page.waitForRequest(
			(req) =>
				req.url().includes("TrackHomeAction") &&
				req.postDataJSON()?.actionType === "open",
		);

		await openButton.click();
		const trackRequest = await trackRequestPromise;

		expect(trackRequest.postDataJSON().itemKey).toBe(
			KNOWLEDGE_HOME_ITEM_READY.itemKey,
		);
	});
});

test.describe("Knowledge Home - Recall Why Display", () => {
	let homePage: DesktopKnowledgeHomePage;

	test.beforeEach(async ({ page }) => {
		homePage = new DesktopKnowledgeHomePage(page);
		await mockAllKnowledgeHomeRoutes(page);
	});

	test("recall rail shows candidates with reason badges", async () => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Recall candidate title should be visible
		await expect(
			homePage.page.getByText("Go Concurrency Patterns"),
		).toBeVisible({ timeout: 10000 });

		// "Not revisited" reason badge should be visible
		await expect(
			homePage.page.getByText("Not revisited"),
		).toBeVisible();
	});

	test("recall Why panel shows detailed reasons including tag_interaction", async ({
		page,
	}) => {
		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Wait for recall candidate to appear
		const candidateTitle = page.getByText("Go Concurrency Patterns");
		await expect(candidateTitle).toBeVisible({ timeout: 10000 });

		// "Tag explored" badge should already be visible on the card (before expanding Why panel)
		await expect(page.getByText("Tag explored")).toBeVisible();

		// "Not revisited" badge should also be visible
		await expect(page.getByText("Not revisited")).toBeVisible();

		// Click "Why?" button to expand the detail panel
		const whyButton = homePage.getRecallWhyButton();
		await whyButton.click();

		// "Why recalled?" panel should appear with detailed descriptions
		await expect(page.getByText("Why recalled?")).toBeVisible();
		await expect(page.getByText("not revisited since")).toBeVisible();
	});

	test("recall candidate card click fires open tracking", async ({
		page,
	}) => {
		// Intercept article navigation
		await page.route("**/articles/**", (route) => route.abort());

		await homePage.goto();
		await homePage.waitForHomeLoaded();

		// Wait for recall candidate
		const recallCard = homePage.page.getByText("Go Concurrency Patterns");
		await expect(recallCard).toBeVisible({ timeout: 10000 });

		// Set up request promise BEFORE the click
		const trackRequestPromise = page.waitForRequest(
			(req) =>
				req.url().includes("TrackHomeAction") &&
				req.postDataJSON()?.actionType === "open",
		);

		await recallCard.click();
		const trackRequest = await trackRequestPromise;

		expect(trackRequest.postDataJSON().actionType).toBe("open");
	});
});
