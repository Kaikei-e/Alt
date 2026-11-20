import type { Page } from "@playwright/test";
import { daysAgo, fulfillJson, hoursAgo } from "../helpers";

export const SWIPE_RPCS = {
	swipeData: "**/feeds/swipe/visual-preview/__data.json*",
	getUnreadFeeds: "**/api/v2/alt.feeds.v2.FeedService/GetUnreadFeeds",
	getReadFeeds: "**/api/v2/alt.feeds.v2.FeedService/GetReadFeeds",
	markAsRead: "**/api/v2/alt.feeds.v2.FeedService/MarkAsRead",
	listSubscriptions: "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
	resolveOgImages: "**/api/v2/alt.feeds.v2.FeedService/ResolveOgImages",
	fetchArticleContent:
		"**/api/v2/alt.articles.v2.ArticleService/FetchArticleContent",
	batchPrefetchImages:
		"**/api/v2/alt.articles.v2.ArticleService/BatchPrefetchImages",
	batchPrefetchArticleContent:
		"**/api/v2/alt.articles.v2.ArticleService/BatchPrefetchArticleContent",
	getReadTodayQueue:
		"**/api/v2/alt.reading.v1.ReadingService/GetReadTodayQueue",
	recordReadingEvents:
		"**/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
	imageProxy: "**/v1/images/proxy/**",
} as const;

export const SWIPE_DECK_ITEMS = [
	{
		id: "swipe-feed-0",
		feedId: "019a9f00-5000-7000-8000-000000000000",
		articleId: "019a9f00-5100-7000-8000-000000000000",
		title: "Structured concurrency patterns for Go worker pipelines",
		description:
			"Bounded worker pools with explicit context cancellation keep background ingestion predictable under upstream latency spikes.",
		link: "https://example.com/systems/structured-concurrency-go",
		published: "1 hour ago",
		createdAt: hoursAgo(1),
		author: "Example Systems Blog",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-0/cover-0",
	},
	{
		id: "swipe-feed-1",
		feedId: "019a9f00-5000-7000-8000-000000000001",
		articleId: "019a9f00-5100-7000-8000-000000000001",
		title: "Designing editorial grids with variable display type",
		description:
			"Fluid measure constraints and optical font sizing preserve scanning rhythm across wide desktop desks and compact mobile screens. Pairing high-contrast ink tokens with warm paper backgrounds reduces eye fatigue during long morning reading sessions. Column rules and monospace datelines create clear structural hierarchy without heavy borders or decorative cards. Consistent vertical spacing keeps dense multi-column headlines readable even on narrow portrait viewports.",
		link: "https://example.org/design/editorial-grids-variable-type",
		published: "2 hours ago",
		createdAt: hoursAgo(2),
		author: "Example Design Review",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-1/cover-1",
	},
	{
		id: "swipe-feed-2",
		feedId: "019a9f00-5000-7000-8000-000000000002",
		articleId: "019a9f00-5100-7000-8000-000000000002",
		title: "Deep-ocean acoustic arrays map coastal methane plumes",
		description:
			"Autonomous seafloor hydrophones track seasonal gas seeps across continental slopes and reveal how winter currents redistribute dissolved plumes.",
		link: "https://example.com/science/deep-ocean-acoustic-arrays",
		published: "5 hours ago",
		createdAt: hoursAgo(5),
		author: "Example Science Desk",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-2/cover-2",
	},
	{
		id: "swipe-feed-3",
		feedId: "019a9f00-5000-7000-8000-000000000003",
		articleId: "019a9f00-5100-7000-8000-000000000003",
		title: "Zero-copy ring buffers for faster Rust services",
		description:
			"Lock-free ring buffers eliminate intermediate heap copies across socket boundaries under sustained traffic and cut tail latency.",
		link: "https://example.com/systems/zero-copy-ring-buffers",
		published: "9 hours ago",
		createdAt: hoursAgo(9),
		author: "Example Systems Blog",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-3/cover-3",
	},
	{
		id: "swipe-feed-4",
		feedId: "019a9f00-5000-7000-8000-000000000004",
		articleId: "019a9f00-5100-7000-8000-000000000004",
		title: "Sparse attention routing for edge reasoning models",
		description:
			"Block-level token selection cuts key-value cache memory in half during multi-document synthesis while keeping local latency predictable.",
		link: "https://example.com/ai/sparse-attention-routing",
		published: "1 day ago",
		createdAt: daysAgo(1),
		author: "Example Science Desk",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-4/cover-4",
	},
	{
		id: "swipe-feed-5",
		feedId: "019a9f00-5000-7000-8000-000000000005",
		articleId: "019a9f00-5100-7000-8000-000000000005",
		title: "Tuning PostgreSQL vacuum workers for heavy writes",
		description:
			"Bounded autovacuum cost limits prevent transaction ID wraparound without starving foreground queries on busy append tables.",
		link: "https://example.org/dev/postgresql-vacuum-workers",
		published: "2 days ago",
		createdAt: daysAgo(2),
		author: "Example Dev Weekly",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-5/cover-5",
	},
	{
		id: "swipe-feed-6",
		feedId: "019a9f00-5000-7000-8000-000000000006",
		articleId: "019a9f00-5100-7000-8000-000000000006",
		title: "Deterministic replay testing for Kubernetes controllers",
		description:
			"Simulated network clocks reproduce rare split-brain failovers deterministically without waiting on live cluster timing.",
		link: "https://example.com/systems/deterministic-replay-kubernetes",
		published: "3 days ago",
		createdAt: daysAgo(3),
		author: "Example Systems Blog",
		ogImageProxyUrl: "/v1/images/proxy/readme-sig-6/cover-6",
	},
];

export const SWIPE_EXPECTED_TITLES = SWIPE_DECK_ITEMS[1]
	? [SWIPE_DECK_ITEMS[1].title]
	: [];

/**
 * Generates an edge-to-edge abstract geometric SVG cover in the Alt-Paper palette:
 * #faf9f7 paper, #2f4f4f slate, #8b2e2e oxblood accent, and warm greys.
 */
function buildGeometricCoverSvg(variant: number): string {
	const accentShift = (variant % 3) * 44;
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 480" width="800" height="480" preserveAspectRatio="xMidYMid slice">
  <rect width="800" height="480" fill="#ebe7df"/>
  <rect x="0" y="0" width="276" height="480" fill="#2f4f4f"/>
  <rect x="276" y="0" width="524" height="22" fill="#d0c8bb"/>
  <circle cx="${505 + accentShift}" cy="240" r="168" fill="#8b2e2e" opacity="0.92"/>
  <circle cx="348" cy="184" r="102" fill="#faf9f7" opacity="0.88"/>
  <rect x="276" y="318" width="440" height="14" fill="#8a8273"/>
  <rect x="276" y="350" width="310" height="7" fill="#2f4f4f" opacity="0.78"/>
  <line x1="64" y1="60" x2="220" y2="420" stroke="#faf9f7" stroke-width="3" opacity="0.5"/>
  <circle cx="144" cy="176" r="42" fill="none" stroke="#faf9f7" stroke-width="3"/>
</svg>`;
}

export async function mockSwipeRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(SWIPE_RPCS.swipeData, async (route) => {
			const res = await route.fetch();
			const json = (await res.json()) as {
				type: string;
				nodes: Array<Record<string, unknown>>;
			};
			if (Array.isArray(json.nodes) && json.nodes.length > 0) {
				const last = json.nodes[json.nodes.length - 1];
				json.nodes[json.nodes.length - 1] = {
					type: "data",
					data: [
						{ initialFeeds: 1, nextCursor: 2, articleData: 3 },
						[],
						null,
						{
							firstArticleImageUrl: 4,
							firstArticleContent: 4,
							firstArticleId: 4,
						},
						null,
					],
					uses: last?.uses ?? {},
				};
			}
			await route.fulfill({ response: res, json });
		}),
		page.route(SWIPE_RPCS.getUnreadFeeds, (route) =>
			fulfillJson(route, {
				data: SWIPE_DECK_ITEMS,
				nextCursor: "",
				hasMore: false,
			}),
		),
		page.route(SWIPE_RPCS.getReadFeeds, (route) =>
			fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		),
		page.route(SWIPE_RPCS.markAsRead, (route) =>
			fulfillJson(route, { message: "Feed marked as read" }),
		),
		page.route(SWIPE_RPCS.listSubscriptions, (route) =>
			fulfillJson(route, { sources: [] }),
		),
		page.route(SWIPE_RPCS.getReadTodayQueue, (route) =>
			fulfillJson(route, { items: [], cap: 7, readCountSince: 0 }),
		),
		page.route(SWIPE_RPCS.recordReadingEvents, (route) =>
			fulfillJson(route, {
				accepted: 1,
				duplicates: 0,
				readArticleIds: [],
			}),
		),
		page.route(SWIPE_RPCS.resolveOgImages, (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			const feedIds = Array.isArray(body?.feedIds) ? body.feedIds : [];
			const images = feedIds
				.map((feedId) => {
					const match = SWIPE_DECK_ITEMS.find((item) => item.feedId === feedId);
					return match ? { feedId, proxyUrl: match.ogImageProxyUrl } : null;
				})
				.filter((entry): entry is { feedId: string; proxyUrl: string } =>
					Boolean(entry),
				);
			return fulfillJson(route, { images, unresolved: [] });
		}),
		page.route(SWIPE_RPCS.fetchArticleContent, (route) => {
			const body = route.request().postDataJSON() as { feedUrl?: string };
			const match =
				SWIPE_DECK_ITEMS.find((item) => item.link === body?.feedUrl) ??
				SWIPE_DECK_ITEMS[0];
			if (!match) {
				return fulfillJson(route, {});
			}
			return fulfillJson(route, {
				url: match.link,
				content: `<p>${match.description}</p>`,
				articleId: match.articleId,
				ogImageUrl: "",
				ogImageProxyUrl: match.ogImageProxyUrl,
			});
		}),
		page.route(SWIPE_RPCS.batchPrefetchImages, (route) => {
			const body = route.request().postDataJSON() as {
				articleIds?: string[];
			};
			const ids = Array.isArray(body?.articleIds) ? body.articleIds : [];
			const images = ids
				.map((articleId) => {
					const match = SWIPE_DECK_ITEMS.find(
						(item) => item.articleId === articleId,
					);
					return match
						? {
								articleId,
								proxyUrl: match.ogImageProxyUrl,
								isCached: true,
							}
						: null;
				})
				.filter(
					(
						entry,
					): entry is {
						articleId: string;
						proxyUrl: string;
						isCached: boolean;
					} => Boolean(entry),
				);
			return fulfillJson(route, { images });
		}),
		page.route(SWIPE_RPCS.batchPrefetchArticleContent, (route) =>
			fulfillJson(route, { warmedCount: 0, skippedCount: 0 }),
		),
		page.route(SWIPE_RPCS.imageProxy, (route) => {
			const url = route.request().url();
			const match = /cover-(\d+)/.exec(url);
			const variant = match ? Number(match[1]) : 1;
			return route.fulfill({
				status: 200,
				contentType: "image/svg+xml",
				headers: {
					"Cache-Control": "public, max-age=604800, immutable",
				},
				body: buildGeometricCoverSvg(variant),
			});
		}),
	]);
}
