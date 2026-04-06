/**
 * Factory for Knowledge Home E2E test data.
 * Produces Connect-RPC response shapes matching GetKnowledgeHomeResponse.
 */

export const KNOWLEDGE_HOME_ITEM_READY = {
	itemKey: "article:e2e-ready-1",
	itemType: "article",
	articleId: "e2e-ready-1",
	title: "Rust Async Runtime Deep Dive",
	publishedAt: new Date(Date.now() - 3600_000).toISOString(),
	summaryExcerpt:
		"Rust の非同期ランタイムは tokio と async-std が主要な選択肢であり、それぞれ異なるトレードオフがある。",
	summaryState: "ready",
	tags: ["rust", "async", "runtime"],
	why: [
		{ code: "new_unread", refId: "", tag: "" },
		{ code: "summary_completed", refId: "", tag: "" },
	],
	score: 0.8,
	link: "https://example.com/rust-async",
};

export const KNOWLEDGE_HOME_ITEM_PENDING = {
	itemKey: "article:e2e-pending-1",
	itemType: "article",
	articleId: "e2e-pending-1",
	title: "WebAssembly の未来",
	publishedAt: new Date(Date.now() - 1800_000).toISOString(),
	summaryExcerpt: "",
	summaryState: "pending",
	tags: ["wasm", "web"],
	why: [{ code: "new_unread", refId: "", tag: "" }],
	score: 0.9,
	link: "https://example.com/wasm-future",
};

export const KNOWLEDGE_HOME_DIGEST = {
	date: new Date().toISOString().slice(0, 10),
	newArticles: 5,
	summarizedArticles: 3,
	unsummarizedArticles: 2,
	topTags: ["rust", "wasm", "ai"],
	weeklyRecapAvailable: false,
	eveningPulseAvailable: false,
	needToKnowCount: 0,
	digestFreshness: "fresh",
	lastProjectedAt: new Date().toISOString(),
};

export const RECALL_CANDIDATE_WITH_REASONS = {
	itemKey: "article:e2e-recall-1",
	recallScore: 0.55,
	reasons: [
		{
			type: "opened_before_but_not_revisited",
			description: "Opened 2 hours ago, not revisited since",
			sourceItemKey: "",
		},
		{
			type: "tag_interaction",
			description: 'You explored tag "rust" (1 hour ago)',
			sourceItemKey: "",
		},
	],
	firstEligibleAt: new Date(Date.now() - 3600_000).toISOString(),
	nextSuggestAt: new Date(Date.now() - 3600_000).toISOString(),
	item: {
		itemKey: "article:e2e-recall-1",
		itemType: "article",
		articleId: "e2e-recall-1",
		title: "Go Concurrency Patterns",
		publishedAt: new Date(Date.now() - 86400_000).toISOString(),
		summaryExcerpt:
			"Go の並行処理パターンは goroutine と channel を中心に構成される。",
		summaryState: "ready",
		tags: ["go", "concurrency"],
		why: [{ code: "new_unread", refId: "", tag: "" }],
		score: 0.1,
		link: "https://example.com/go-concurrency",
	},
};

export const KNOWLEDGE_HOME_ITEM_SUPERSEDED = {
	itemKey: "article:e2e-superseded-1",
	itemType: "article",
	articleId: "e2e-superseded-1",
	title: "GraphQL Federation Patterns",
	publishedAt: new Date(Date.now() - 7200_000).toISOString(),
	summaryExcerpt:
		"GraphQL Federation allows composing multiple subgraphs into a single unified API.",
	summaryState: "ready",
	tags: ["graphql", "federation", "api"],
	why: [
		{ code: "new_unread", refId: "", tag: "" },
		{ code: "summary_completed", refId: "", tag: "" },
	],
	score: 0.7,
	link: "https://example.com/graphql-federation",
	supersedeInfo: {
		state: "summary_updated",
		supersededAt: new Date(Date.now() - 600_000).toISOString(),
		previousSummaryExcerpt: "GraphQL は複数のスキーマを合成する仕組みとして Federation を提供している。",
		previousTags: ["graphql", "schema"],
		previousWhyCodes: ["new_unread"],
	},
};

export const FEATURE_FLAGS = [
	{ name: "enable_knowledge_home_page", enabled: true },
	{ name: "enable_knowledge_home_tracking", enabled: true },
	{ name: "enable_recall_rail", enabled: true },
	{ name: "enable_lens", enabled: false },
	{ name: "enable_stream_updates", enabled: false },
	{ name: "enable_supersede_ux", enabled: false },
];

export const FEATURE_FLAGS_WITH_LENS = FEATURE_FLAGS.map((f) =>
	f.name === "enable_lens" ? { ...f, enabled: true } : f,
);

export const FEATURE_FLAGS_RECALL_DISABLED = FEATURE_FLAGS.map((f) =>
	f.name === "enable_recall_rail" ? { ...f, enabled: false } : f,
);

export const FEATURE_FLAGS_WITH_SUPERSEDE = FEATURE_FLAGS.map((f) =>
	f.name === "enable_supersede_ux" ? { ...f, enabled: true } : f,
);

export const FEATURE_FLAGS_WITH_STREAM = FEATURE_FLAGS.map((f) =>
	f.name === "enable_stream_updates" ? { ...f, enabled: true } : f,
);

export const DIGEST_WITH_AVAILABILITY = {
	...KNOWLEDGE_HOME_DIGEST,
	weeklyRecapAvailable: true,
	eveningPulseAvailable: true,
};

export const DIGEST_WITHOUT_AVAILABILITY = {
	...KNOWLEDGE_HOME_DIGEST,
	weeklyRecapAvailable: false,
	eveningPulseAvailable: false,
};

export function buildListLensesResponse(
	lenses: { id: string; name: string; filterSummary: string }[] = [],
	activeLensId = "",
) {
	return { lenses, activeLensId };
}

export function buildGetKnowledgeHomeResponse(overrides?: {
	items?: unknown[];
	recallCandidates?: unknown[];
	featureFlags?: typeof FEATURE_FLAGS;
	todayDigest?: typeof KNOWLEDGE_HOME_DIGEST;
}) {
	return {
		todayDigest: overrides?.todayDigest ?? KNOWLEDGE_HOME_DIGEST,
		items: overrides?.items ?? [
			KNOWLEDGE_HOME_ITEM_READY,
			KNOWLEDGE_HOME_ITEM_PENDING,
		],
		nextCursor: "",
		hasMore: false,
		degradedMode: false,
		generatedAt: new Date().toISOString(),
		featureFlags: overrides?.featureFlags ?? FEATURE_FLAGS,
		serviceQuality: "full",
		recallCandidates: overrides?.recallCandidates ?? [
			RECALL_CANDIDATE_WITH_REASONS,
		],
	};
}

export function buildTrackHomeActionResponse() {
	return {};
}

export function buildTrackHomeItemsSeenResponse() {
	return {};
}
