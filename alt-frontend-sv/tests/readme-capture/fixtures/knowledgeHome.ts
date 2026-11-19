import type { Page } from "@playwright/test";
import {
	captureDate,
	daysAgo,
	fulfillJson,
	hoursAgo,
	minutesAgo,
} from "../helpers";

export const KNOWLEDGE_HOME_RPCS = {
	getKnowledgeHome:
		"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
	listLenses: "**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/ListLenses",
	trackHomeItemsSeen:
		"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeItemsSeen",
	trackHomeAction:
		"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction",
	streamUpdates:
		"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/StreamKnowledgeHomeUpdates",
	listSubscriptions: "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
} as const;

export const KNOWLEDGE_HOME_STREAM_ITEMS = [
	{
		itemKey: "article:019a9f00-1000-7000-8000-000000000001",
		itemType: "article",
		articleId: "019a9f00-1000-7000-8000-000000000001",
		title: "Zero-copy ring buffers for faster Rust services",
		publishedAt: minutesAgo(28),
		summaryExcerpt:
			"Lock-free ring buffers eliminate intermediate heap copies across socket boundaries under sustained traffic. Benchmarks show lower tail latency once batch thresholds align with cache-line sizes.",
		summaryState: "ready",
		tags: ["rust", "networking", "systems"],
		why: [{ code: "pulse_need_to_know" }, { code: "summary_completed" }],
		score: 0.96,
		url: "https://example.com/systems/zero-copy-ring-buffers",
	},
	{
		itemKey: "article:019a9f00-1000-7000-8000-000000000002",
		itemType: "article",
		articleId: "019a9f00-1000-7000-8000-000000000002",
		title: "Tuning PostgreSQL vacuum workers for heavy writes",
		publishedAt: hoursAgo(2),
		summaryExcerpt:
			"Aggressive autovacuum cost limits prevent transaction ID wraparound without starving foreground queries. Splitting hot append tables into shorter partitions keeps dead tuple bloat bounded.",
		summaryState: "ready",
		tags: ["postgresql", "databases", "storage"],
		why: [{ code: "tag_hotspot", tag: "postgresql" }],
		score: 0.91,
		url: "https://example.org/dev/postgresql-vacuum-workers",
	},
	{
		itemKey: "article:019a9f00-1000-7000-8000-000000000003",
		itemType: "article",
		articleId: "019a9f00-1000-7000-8000-000000000003",
		title: "Sparse attention routing for edge reasoning models",
		publishedAt: hoursAgo(4),
		summaryExcerpt:
			"Block-level token selection cuts key-value cache memory by half during multi-document synthesis. Accuracy stays within one percent on retrieval benchmarks while keeping local latency predictable.",
		summaryState: "ready",
		tags: ["ai-research", "inference", "attention"],
		why: [{ code: "in_weekly_recap" }],
		score: 0.87,
		url: "https://example.com/ai/sparse-attention-routing",
	},
	{
		itemKey: "article:019a9f00-1000-7000-8000-000000000004",
		itemType: "article",
		articleId: "019a9f00-1000-7000-8000-000000000004",
		title: "Designing editorial grids with variable display type",
		publishedAt: hoursAgo(6),
		summaryExcerpt:
			"Fluid measure constraints and optical font sizing preserve scanning rhythm across wide and compact viewports. High-contrast ink tokens keep dense multi-column layouts legible in daylight.",
		summaryState: "ready",
		tags: ["design", "typography", "css"],
		why: [{ code: "recent_interest_match" }],
		score: 0.82,
		url: "https://example.org/design/editorial-grids-variable-type",
	},
	{
		itemKey: "article:019a9f00-1000-7000-8000-000000000005",
		itemType: "article",
		articleId: "019a9f00-1000-7000-8000-000000000005",
		title: "Deep-ocean acoustic arrays map coastal methane plumes",
		publishedAt: daysAgo(1),
		summaryExcerpt:
			"Autonomous seafloor hydrophones track seasonal gas seeps across previously unmapped continental slopes. Multi-year recordings reveal how bottom currents redistribute dissolved plumes over winter.",
		summaryState: "ready",
		tags: ["science", "acoustics", "climate"],
		why: [{ code: "related_to_recent_search" }],
		score: 0.78,
		url: "https://example.com/science/deep-ocean-acoustic-arrays",
	},
];

export const KNOWLEDGE_HOME_RECALL_CANDIDATES = [
	{
		itemKey: "article:019a9f00-2000-7000-8000-000000000001",
		recallScore: 0.84,
		reasons: [
			{
				type: "opened_before_but_not_revisited",
				description: "Opened 2 days ago, not revisited since",
				sourceItemKey: "",
			},
			{
				type: "tag_interaction",
				description: 'You explored tag "kubernetes" (3 hours ago)',
				sourceItemKey: "",
			},
		],
		firstEligibleAt: hoursAgo(18),
		nextSuggestAt: hoursAgo(2),
		item: {
			itemKey: "article:019a9f00-2000-7000-8000-000000000001",
			itemType: "article",
			articleId: "019a9f00-2000-7000-8000-000000000001",
			title: "Deterministic replay testing for Kubernetes controllers",
			publishedAt: daysAgo(2),
			summaryExcerpt:
				"Simulated network clocks reproduce rare split-brain failovers in seconds without cluster flakiness.",
			summaryState: "ready",
			tags: ["kubernetes", "testing"],
			why: [{ code: "new_unread" }],
			score: 0.72,
			url: "https://example.com/systems/deterministic-replay-kubernetes",
		},
	},
	{
		itemKey: "article:019a9f00-2000-7000-8000-000000000002",
		recallScore: 0.76,
		reasons: [
			{
				type: "recap_context_unfinished",
				description: "Part of your active Weekly Recap thread",
				sourceItemKey: "",
			},
		],
		firstEligibleAt: daysAgo(1),
		nextSuggestAt: hoursAgo(4),
		item: {
			itemKey: "article:019a9f00-2000-7000-8000-000000000002",
			itemType: "article",
			articleId: "019a9f00-2000-7000-8000-000000000002",
			title: "Optical lattice clocks measure indoor gravitational redshift",
			publishedAt: daysAgo(3),
			summaryExcerpt:
				"Millimeter-scale elevation shifts register clear frequency offsets in portable atomic clock assemblies.",
			summaryState: "ready",
			tags: ["science", "physics"],
			why: [{ code: "in_weekly_recap" }],
			score: 0.68,
			url: "https://example.com/science/optical-lattice-clocks",
		},
	},
	{
		itemKey: "article:019a9f00-2000-7000-8000-000000000003",
		recallScore: 0.71,
		reasons: [
			{
				type: "related_to_recent_search",
				description: "Matches your recent search on inference quality",
				sourceItemKey: "",
			},
		],
		firstEligibleAt: daysAgo(2),
		nextSuggestAt: hoursAgo(6),
		item: {
			itemKey: "article:019a9f00-2000-7000-8000-000000000003",
			itemType: "article",
			articleId: "019a9f00-2000-7000-8000-000000000003",
			title: "Evaluating calibration drift in quantized language models",
			publishedAt: daysAgo(4),
			summaryExcerpt:
				"Four-bit weight compression preserves top-line accuracy but skews confidence scores on rare domains.",
			summaryState: "ready",
			tags: ["ai-research", "evaluation"],
			why: [{ code: "related_to_recent_search" }],
			score: 0.64,
			url: "https://example.com/ai/quantized-calibration-drift",
		},
	},
];

export const KNOWLEDGE_HOME_TITLES = KNOWLEDGE_HOME_STREAM_ITEMS.map(
	(item) => item.title,
);

export const KNOWLEDGE_HOME_RESPONSE = {
	todayDigest: {
		date: captureDate(),
		newArticles: 14,
		summarizedArticles: 11,
		unsummarizedArticles: 3,
		topTags: ["rust", "postgresql", "design", "science", "ai-research"],
		weeklyRecapAvailable: true,
		needToKnowCount: 1,
		digestFreshness: "fresh",
		lastProjectedAt: minutesAgo(4),
	},
	items: KNOWLEDGE_HOME_STREAM_ITEMS,
	nextCursor: "",
	hasMore: false,
	degradedMode: false,
	generatedAt: minutesAgo(1),
	featureFlags: [
		{ name: "enable_knowledge_home_page", enabled: true },
		{ name: "enable_knowledge_home_tracking", enabled: true },
		{ name: "enable_lens", enabled: true },
		{ name: "enable_stream_updates", enabled: true },
		{ name: "enable_supersede_ux", enabled: true },
	],
	serviceQuality: "full",
	recallCandidates: KNOWLEDGE_HOME_RECALL_CANDIDATES,
};

export const KNOWLEDGE_HOME_LENSES_RESPONSE = {
	lenses: [
		{
			lensId: "lens-systems-rust",
			name: "Systems & Rust",
			description: "Low-level runtimes, storage engines, and networking",
			createdAt: daysAgo(12),
			updatedAt: daysAgo(2),
			currentVersion: {
				versionId: "ver-systems-1",
				queryText: "",
				tagIds: ["rust", "postgresql", "webassembly"],
				sourceIds: [],
				timeWindow: "7d",
				includeRecap: true,
				includePulse: true,
				sortMode: "relevance",
			},
		},
		{
			lensId: "lens-ai-research",
			name: "AI Research",
			description: "Model architecture, quantization, and evaluation",
			createdAt: daysAgo(9),
			updatedAt: daysAgo(1),
			currentVersion: {
				versionId: "ver-ai-1",
				queryText: "",
				tagIds: ["ai-research", "inference"],
				sourceIds: [],
				timeWindow: "7d",
				includeRecap: false,
				includePulse: true,
				sortMode: "relevance",
			},
		},
	],
	activeLensId: "",
};

export async function mockKnowledgeHomeRoutes(
	page: Page,
): Promise<() => Promise<void>> {
	const pendingStreamResolvers: Array<() => void> = [];

	await Promise.all([
		page.route(KNOWLEDGE_HOME_RPCS.getKnowledgeHome, (route) =>
			fulfillJson(route, KNOWLEDGE_HOME_RESPONSE),
		),
		page.route(KNOWLEDGE_HOME_RPCS.listLenses, (route) =>
			fulfillJson(route, KNOWLEDGE_HOME_LENSES_RESPONSE),
		),
		page.route(KNOWLEDGE_HOME_RPCS.trackHomeItemsSeen, (route) =>
			fulfillJson(route, {}),
		),
		page.route(KNOWLEDGE_HOME_RPCS.trackHomeAction, (route) =>
			fulfillJson(route, {}),
		),
		page.route(KNOWLEDGE_HOME_RPCS.listSubscriptions, (route) =>
			fulfillJson(route, { sources: [] }),
		),
		// Hold the streaming RPC pending during capture so useStreamUpdates stays
		// idle without flashing a "Reconnecting..." status bar.
		page.route(
			KNOWLEDGE_HOME_RPCS.streamUpdates,
			(route) =>
				new Promise<void>((resolve) => {
					pendingStreamResolvers.push(() => {
						void route
							.abort()
							.catch(() => {})
							.finally(resolve);
					});
				}),
		),
	]);

	return async () => {
		while (pendingStreamResolvers.length > 0) {
			pendingStreamResolvers.pop()?.();
		}
	};
}
