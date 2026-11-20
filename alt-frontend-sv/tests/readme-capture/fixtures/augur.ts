import type { Page } from "@playwright/test";
import { fulfillJson, hoursAgo, minutesAgo } from "../helpers";

export const AUGUR_CONVERSATION_ID = "019a9f00-4000-7000-8000-000000000001";

export const AUGUR_RPCS = {
	getConversation: "**/api/v2/alt.augur.v2.AugurService/GetConversation",
	listConversations: "**/api/v2/alt.augur.v2.AugurService/ListConversations",
} as const;

export const AUGUR_CITATIONS = [
	{
		url: "https://example.org/dev/incremental-views-event-logs",
		title: "Incremental materialized views over append-only event logs",
		publishedAt: hoursAgo(5),
		kind: "CITATION_KIND_WEB",
		refId: "",
	},
	{
		url: "https://example.org/dev/postgresql-vacuum-workers",
		title: "Tuning PostgreSQL vacuum workers for heavy writes",
		publishedAt: hoursAgo(2),
		kind: "CITATION_KIND_WEB",
		refId: "",
	},
	{
		url: "https://example.com/systems/structured-concurrency-go",
		title: "Structured concurrency patterns for Go worker pipelines",
		publishedAt: hoursAgo(3),
		kind: "CITATION_KIND_WEB",
		refId: "",
	},
	{
		url: "https://example.com/systems/zero-copy-ring-buffers",
		title: "Zero-copy ring buffers for faster Rust services",
		publishedAt: minutesAgo(28),
		kind: "CITATION_KIND_WEB",
		refId: "",
	},
];

export const AUGUR_EXPECTED_TEXTS = [
	"How do append-only event logs keep read models rebuildable?",
	...AUGUR_CITATIONS.map((c) => c.title),
];

export const AUGUR_CONVERSATION_RESPONSE = {
	id: AUGUR_CONVERSATION_ID,
	title: "How do append-only event logs keep read models rebuildable?",
	createdAt: minutesAgo(14),
	messages: [
		{
			role: "user",
			content: "How do append-only event logs keep read models rebuildable?",
			createdAt: minutesAgo(14),
			citations: [],
			relatedCitations: [],
		},
		{
			role: "assistant",
			content: [
				"By treating the event log as the durable source of truth, every projection stays a deterministic fold over immutable records [1]. When a schema or ranking rule changes, workers replay from offset zero without mutating historical state [2].",
				"Teams keep rebuilds fast by pairing incremental materialized views with bounded vacuum budgets on hot append tables [2] and structured worker pipelines that checkpoint progress cleanly [3].",
			].join("\n\n"),
			createdAt: minutesAgo(13),
			citations: AUGUR_CITATIONS.slice(0, 3),
			relatedCitations: [],
		},
		{
			role: "user",
			content: "Does replay require halting live writes during a rebuild?",
			createdAt: minutesAgo(8),
			citations: [],
			relatedCitations: [],
		},
		{
			role: "assistant",
			content:
				"No. You can build a shadow projection in parallel from the append-only stream [1], cap vacuum cost on active partitions [2], and switch readers once worker pipelines [3] and ring-buffered consumers [4] catch up to the log head.",
			createdAt: minutesAgo(7),
			citations: AUGUR_CITATIONS,
			relatedCitations: [],
		},
	],
};

export async function mockAugurRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(AUGUR_RPCS.getConversation, (route) =>
			fulfillJson(route, AUGUR_CONVERSATION_RESPONSE),
		),
		page.route(AUGUR_RPCS.listConversations, (route) =>
			fulfillJson(route, { conversations: [], nextPageToken: "" }),
		),
	]);
}
