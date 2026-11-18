import {
	buildResolveOgImagesResponse,
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

const LIST_SUBSCRIPTIONS =
	"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";
const TRACK_HOME_ACTION =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction";
const BATCH_PREFETCH_IMAGES =
	"**/api/v2/alt.articles.v2.ArticleService/BatchPrefetchImages";

interface RecordedReadingEventJson {
	clientEventId?: string;
	occurredAt?: string;
	surfaceViewed?: {
		surface?: string;
	};
	reachedEnd?: {
		articleId?: string;
		surface?: string;
	};
	sessionEnded?: {
		articleId?: string;
		surface?: string;
		activeDwellMs?: string | number;
		maxScrollRatio?: number;
		bodyLengthChars?: number;
		reachedEnd?: boolean;
	};
}

interface RecordReadingEventsRequestJson {
	events?: RecordedReadingEventJson[];
}

const LONG_ARTICLE_HTML = Array.from(
	{ length: 28 },
	(_, index) =>
		`<p>Paragraph ${index + 1}: Detailed analytical prose covering modern feed architecture, client reading telemetry, scroll progression, and durable event projections across mobile viewports.</p>`,
).join("");

test.describe("Gallery reading events", () => {
	test("records surface view, reached end, and session ended when reading an article in the gallery modal", async ({
		page,
	}) => {
		const recordedRequests: RecordReadingEventsRequestJson[] = [];
		let openTracked = false;

		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(LIST_SUBSCRIPTIONS, (route) =>
			fulfillJson(route, { subscriptions: [] }),
		);
		await page.route(TRACK_HOME_ACTION, async (route) => {
			openTracked = true;
			await fulfillJson(route, {});
		});
		await page.route(BATCH_PREFETCH_IMAGES, (route) =>
			fulfillJson(route, { results: [] }),
		);
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			await fulfillJson(
				route,
				buildResolveOgImagesResponse({ settled: body?.feedIds ?? [] }),
			);
		});
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/ai-trends",
				content: LONG_ARTICLE_HTML,
				articleId: "article-1",
			}),
		);
		await page.route(CONNECT_RPC_PATHS.recordReadingEvents, async (route) => {
			const body = route
				.request()
				.postDataJSON() as RecordReadingEventsRequestJson;
			recordedRequests.push(body);
			await fulfillJson(route, {
				accepted: body.events?.length ?? 0,
				duplicates: 0,
			});
		});

		await gotoMobileRoute(page, "/feeds/visual-preview");

		await page.getByRole("button", { name: "Open AI Trends" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();
		await expect(dialog.getByText(/Paragraph 1:/)).toBeVisible();
		await expect.poll(() => openTracked).toBe(true);

		await page.waitForTimeout(60);
		expect(recordedRequests).toHaveLength(0);

		const modalBody = dialog.locator(".modal-body");
		await modalBody.evaluate((el) => {
			el.scrollTop = el.scrollHeight;
			el.dispatchEvent(new Event("scroll"));
		});
		await page.evaluate(
			() =>
				new Promise<void>((resolve) =>
					requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
				),
		);

		expect(recordedRequests).toHaveLength(0);

		await dialog.getByTestId("modal-close").click();
		await expect(dialog).toBeHidden();

		await expect.poll(() => recordedRequests.length).toBeGreaterThan(0);

		const events = recordedRequests.flatMap((req) => req.events ?? []);

		const surfaceViewed = events.find((e) => e.surfaceViewed !== undefined);
		expect(surfaceViewed?.surfaceViewed).toEqual({
			surface: "READING_SURFACE_GALLERY",
		});

		const reachedEnds = events.filter(
			(e) => e.reachedEnd?.articleId === "article-1",
		);
		expect(reachedEnds).toHaveLength(1);
		expect(reachedEnds[0]?.reachedEnd?.surface).toBe("READING_SURFACE_GALLERY");

		const sessionEndeds = events.filter(
			(e) => e.sessionEnded?.articleId === "article-1",
		);
		expect(sessionEndeds).toHaveLength(1);
		const sessionEnded = sessionEndeds[0]?.sessionEnded;
		expect(sessionEnded).toBeDefined();
		expect(sessionEnded?.surface).toBe("READING_SURFACE_GALLERY");
		expect(sessionEnded?.reachedEnd).toBe(true);
		expect(Number(sessionEnded?.activeDwellMs ?? 0)).toBeGreaterThan(0);
		expect(sessionEnded?.maxScrollRatio ?? 0).toBeGreaterThan(0);
		expect(sessionEnded?.bodyLengthChars ?? 0).toBeGreaterThan(0);
	});
});
