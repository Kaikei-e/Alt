import type { Page } from "@playwright/test";
import { fulfillJson, hoursAgo, hoursFromNow } from "../helpers";

export const TODAY_RPCS = {
	getReadTodayQueue:
		"**/api/v2/alt.reading.v1.ReadingService/GetReadTodayQueue",
	recordReadingEvents:
		"**/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
} as const;

export const TODAY_QUEUE_ITEMS = [
	{
		articleId: "019a9f00-3000-7000-8000-000000000001",
		title: "Structured concurrency patterns for Go worker pipelines",
		url: "https://example.com/systems/structured-concurrency-go",
		sourceName: "Example Systems Blog",
		imageUrl: "",
		queuedAt: hoursAgo(2),
		expiresAt: hoursFromNow(46),
	},
	{
		articleId: "019a9f00-3000-7000-8000-000000000002",
		title: "Incremental materialized views over append-only event logs",
		url: "https://example.org/dev/incremental-views-event-logs",
		sourceName: "Example Dev Weekly",
		imageUrl: "",
		queuedAt: hoursAgo(5),
		expiresAt: hoursFromNow(43),
	},
	{
		articleId: "019a9f00-3000-7000-8000-000000000003",
		title: "Optical lattice clocks measure indoor gravitational redshift",
		url: "https://example.com/science/optical-lattice-clocks",
		sourceName: "Example Science Desk",
		imageUrl: "",
		queuedAt: hoursAgo(10),
		expiresAt: hoursFromNow(38),
	},
	{
		articleId: "019a9f00-3000-7000-8000-000000000004",
		title: "Paper-first color tokens for low-glare reading screens",
		url: "https://example.org/design/paper-first-color-tokens",
		sourceName: "Example Design Review",
		imageUrl: "",
		queuedAt: hoursAgo(16),
		expiresAt: hoursFromNow(32),
	},
	{
		articleId: "019a9f00-3000-7000-8000-000000000005",
		title: "Sandboxing untrusted plugins with WebAssembly memory keys",
		url: "https://example.org/systems/wasm-memory-keys",
		sourceName: "Example Systems Blog",
		imageUrl: "",
		queuedAt: hoursAgo(20),
		expiresAt: hoursFromNow(28),
	},
];

export const TODAY_QUEUE_TITLES = TODAY_QUEUE_ITEMS.map((item) => item.title);

export const TODAY_QUEUE_RESPONSE = {
	items: TODAY_QUEUE_ITEMS,
	cap: 7,
	readCountSince: 2,
};

export async function mockTodayRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(TODAY_RPCS.getReadTodayQueue, (route) =>
			fulfillJson(route, TODAY_QUEUE_RESPONSE),
		),
		page.route(TODAY_RPCS.recordReadingEvents, (route) =>
			fulfillJson(route, {
				accepted: 1,
				duplicates: 0,
				readArticleIds: [],
			}),
		),
	]);
}
