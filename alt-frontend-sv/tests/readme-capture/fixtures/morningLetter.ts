import type { Page } from "@playwright/test";
import {
	captureDate,
	daysAgo,
	fulfillJson,
	hoursFromNow,
	minutesAgo,
} from "../helpers";

export const MORNING_LETTER_ID = "019a9f00-7000-7000-8000-000000000001";

export const MORNING_LETTER_RPCS = {
	getLatestLetter:
		"**/api/v2/alt.morning_letter.v2.MorningLetterReadService/GetLatestLetter",
	getLetterByDate:
		"**/api/v2/alt.morning_letter.v2.MorningLetterReadService/GetLetterByDate",
	getLetterSources:
		"**/api/v2/alt.morning_letter.v2.MorningLetterReadService/GetLetterSources",
	getLetterEnrichment:
		"**/api/v2/alt.morning_letter.v2.MorningLetterReadService/GetLetterEnrichment",
} as const;

export const MORNING_LETTER_THROUGH_LINE =
	"Append-only storage, sparse edge inference, and readable grid systems converge.";

export const MORNING_LETTER_EXPECTED_TEXTS = [
	MORNING_LETTER_THROUGH_LINE,
	"Top Stories",
	"Worth a second look",
	"From your themes",
];

export const MORNING_LETTER_DOCUMENT_RESPONSE = {
	letter: {
		id: MORNING_LETTER_ID,
		targetDate: captureDate(),
		editionTimezone: "UTC",
		isDegraded: false,
		schemaVersion: 1,
		generationRevision: 1,
		model: "editorial-synthesizer",
		createdAt: minutesAgo(35),
		etag: '"rev-1"',
		body: {
			lead: "Today's edition traces how durable event logs simplify database rebuilds while edge runtimes and typographic layouts adapt to tighter latency budgets.",
			generatedAt: minutesAgo(35),
			sourceRecapWindowDays: 3,
			throughLine: MORNING_LETTER_THROUGH_LINE,
			sections: [
				{
					key: "top3",
					title: "Top stories",
					narrative:
						"Write-heavy services and bounded inference models lead this morning's engineering notes.",
					bullets: [
						"Zero-copy ring buffers cut tail latency across high-throughput Rust socket boundaries (Example Systems Blog).",
						"Aggressive PostgreSQL autovacuum limits keep dead tuple bloat bounded on hot append tables (Example Dev Weekly).",
						"Sparse block attention trims edge key-value cache memory by half without hurting retrieval recall (Example Science Desk).",
					],
					genre: "systems",
					whyReasons: [
						{ code: "pulse_need_to_know" },
						{ code: "tag_hotspot", tag: "postgresql" },
						{ code: "in_weekly_recap" },
					],
				},
				{
					key: "what_changed",
					title: "Worth a second look",
					narrative:
						"Design systems and seafloor telemetry studies expand two active reading threads.",
					bullets: [
						"Fluid editorial grids pair variable optical sizing with paper-first contrast tokens (Example Design Review).",
						"Deep-ocean acoustic arrays map seasonal coastal methane plumes across continental slopes (Example Science Desk).",
					],
					genre: "science",
					whyReasons: [
						{ code: "recent_interest_match" },
						{ code: "related_to_recent_search" },
					],
				},
				{
					key: "by_theme:systems-storage",
					title: "From your themes",
					narrative:
						"Recent reading clustered around replayable projections and deterministic cluster tests.",
					bullets: [
						"Incremental materialized views let shadow projections catch up before read traffic shifts (Example Dev Weekly).",
						"Deterministic clock simulation reproduces Kubernetes controller failovers in seconds (Example Systems Blog).",
					],
					genre: "storage",
					whyReasons: [
						{ code: "tag_hotspot", tag: "rust" },
						{ code: "in_weekly_recap" },
					],
				},
			],
		},
		surfaceStatus: {
			generatedAt: minutesAgo(35),
			windowStart: daysAgo(3),
			windowEnd: minutesAgo(35),
			isStale: false,
			staleAfter: hoursFromNow(12),
			latestRun: {
				jobId: "ml-job-2026-11-20",
				status: "completed",
				kickedAt: minutesAgo(40),
				updatedAt: minutesAgo(35),
			},
			degradationReasons: [],
		},
	},
};

export const MORNING_LETTER_SOURCES_RESPONSE = {
	sources: [
		{
			letterId: MORNING_LETTER_ID,
			sectionKey: "top3",
			articleId: "019a9f00-1000-7000-8000-000000000001",
			sourceType: 2,
			position: 0,
		},
		{
			letterId: MORNING_LETTER_ID,
			sectionKey: "top3",
			articleId: "019a9f00-1000-7000-8000-000000000002",
			sourceType: 1,
			position: 1,
		},
		{
			letterId: MORNING_LETTER_ID,
			sectionKey: "what_changed",
			articleId: "019a9f00-1000-7000-8000-000000000004",
			sourceType: 1,
			position: 0,
		},
		{
			letterId: MORNING_LETTER_ID,
			sectionKey: "by_theme:systems-storage",
			articleId: "019a9f00-3000-7000-8000-000000000002",
			sourceType: 1,
			position: 0,
		},
	],
};

export const MORNING_LETTER_ENRICHMENT_RESPONSE = {
	enrichments: [
		{
			sectionKey: "top3",
			articleId: "019a9f00-1000-7000-8000-000000000001",
			articleTitle: "Zero-copy ring buffers for faster Rust services",
			articleUrl: "https://example.com/systems/zero-copy-ring-buffers",
			articleAltHref: "/articles/019a9f00-1000-7000-8000-000000000001",
			feedTitle: "Example Systems Blog",
			tags: ["rust", "networking", "systems"],
			relatedArticles: [],
			summaryExcerpt:
				"Lock-free ring buffers eliminate intermediate heap copies across socket boundaries under sustained traffic.",
			chatHref: "/augur",
		},
		{
			sectionKey: "by_theme:systems-storage",
			articleId: "019a9f00-3000-7000-8000-000000000002",
			articleTitle:
				"Incremental materialized views over append-only event logs",
			articleUrl: "https://example.org/dev/incremental-views-event-logs",
			articleAltHref: "/articles/019a9f00-3000-7000-8000-000000000002",
			feedTitle: "Example Dev Weekly",
			tags: ["postgresql", "event-sourcing"],
			relatedArticles: [],
			summaryExcerpt:
				"Shadow projections replay immutable event records cleanly without taking live readers offline.",
			chatHref: "/augur",
		},
	],
};

export async function mockMorningLetterRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(MORNING_LETTER_RPCS.getLatestLetter, (route) =>
			fulfillJson(route, MORNING_LETTER_DOCUMENT_RESPONSE),
		),
		page.route(MORNING_LETTER_RPCS.getLetterByDate, (route) =>
			fulfillJson(route, MORNING_LETTER_DOCUMENT_RESPONSE),
		),
		page.route(MORNING_LETTER_RPCS.getLetterSources, (route) =>
			fulfillJson(route, MORNING_LETTER_SOURCES_RESPONSE),
		),
		page.route(MORNING_LETTER_RPCS.getLetterEnrichment, (route) =>
			fulfillJson(route, MORNING_LETTER_ENRICHMENT_RESPONSE),
		),
	]);
}
