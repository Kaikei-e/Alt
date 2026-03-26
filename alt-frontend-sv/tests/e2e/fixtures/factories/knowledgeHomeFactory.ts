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
		summaryExcerpt: "Go の並行処理パターンは goroutine と channel を中心に構成される。",
		summaryState: "ready",
		tags: ["go", "concurrency"],
		why: [{ code: "new_unread", refId: "", tag: "" }],
		score: 0.1,
		link: "https://example.com/go-concurrency",
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

export function buildGetKnowledgeHomeResponse(overrides?: {
	items?: unknown[];
	recallCandidates?: unknown[];
}) {
	return {
		todayDigest: KNOWLEDGE_HOME_DIGEST,
		items: overrides?.items ?? [
			KNOWLEDGE_HOME_ITEM_READY,
			KNOWLEDGE_HOME_ITEM_PENDING,
		],
		nextCursor: "",
		hasMore: false,
		degradedMode: false,
		generatedAt: new Date().toISOString(),
		featureFlags: FEATURE_FLAGS,
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
