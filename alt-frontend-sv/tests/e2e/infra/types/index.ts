/**
 * Mock Server Type Definitions
 * Shared types for mock data and handlers
 */

// =============================================================================
// Kratos Types
// =============================================================================

export interface KratosIdentity {
	id: string;
	schema_id: string;
	schema_url: string;
	state: string;
	traits: {
		email: string;
		name: string;
	};
}

export interface KratosSession {
	id: string;
	active: boolean;
	authenticated_at: string;
	expires_at: string;
	issued_at: string;
	identity: KratosIdentity;
	authentication_methods: Array<{
		method: string;
		completed_at: string;
	}>;
	metadata_public: Record<string, unknown>;
}

export interface KratosFlowNode {
	type: string;
	group: string;
	attributes: {
		name: string;
		type: string;
		value?: string;
		required?: boolean;
	};
	messages: unknown[];
	meta: {
		label?: {
			id: number;
			text: string;
			type: string;
		};
	};
}

export interface KratosFlow {
	id: string;
	type: string;
	expires_at: string;
	issued_at: string;
	request_url: string;
	ui: {
		action: string;
		method: string;
		nodes: KratosFlowNode[];
		messages: unknown[];
	};
}

// =============================================================================
// AuthHub Types
// =============================================================================

export interface AuthHubSessionResponse {
	user_id: string;
	email: string;
}

// =============================================================================
// Feed Types (REST v1)
// =============================================================================

export interface FeedAuthor {
	name: string;
}

export interface Feed {
	id: string;
	url: string;
	title: string;
	description: string;
	link: string;
	published_at: string;
	tags: string[];
	author: FeedAuthor;
	thumbnail: string | null;
	feed_domain: string;
	read_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface FeedsResponse {
	data: Feed[];
	next_cursor: string | null;
	has_more: boolean;
}

export interface FeedStats {
	total_feeds: number;
	total_reads: number;
	unread_count: number;
}

export interface DetailedFeedStats {
	feed_amount: { amount: number };
	total_articles: { amount: number };
	unsummarized_articles: { amount: number };
}

// =============================================================================
// Feed Types (Connect-RPC v2)
// =============================================================================

export interface ConnectFeed {
	id: string;
	articleId: string;
	title: string;
	description: string;
	link: string;
	published: string;
	createdAt: string;
	author: string;
}

export interface ConnectFeedsResponse {
	data: ConnectFeed[];
	nextCursor: string;
	hasMore: boolean;
}

export interface ConnectDetailedStats {
	feedAmount: number;
	articleAmount: number;
	unsummarizedFeedAmount: number;
}

export interface ConnectArticleContent {
	url: string;
	content: string;
	articleId: string;
}

// =============================================================================
// Recap Types
// =============================================================================

export interface RecapEvidenceLink {
	articleId: string;
	title: string;
	sourceUrl: string;
	publishedAt: string;
	lang: string;
}

export interface RecapGenre {
	genre: string;
	summary: string;
	topTerms: string[];
	articleCount: number;
	clusterCount: number;
	evidenceLinks: RecapEvidenceLink[];
	bullets: string[];
}

export interface RecapResponse {
	genres: RecapGenre[];
}

// =============================================================================
// Augur Types
// =============================================================================

export interface AugurCitation {
	url: string;
	title: string;
	publishedAt: string;
}

export interface AugurStreamDelta {
	result: {
		kind: "delta";
		payload: {
			case: "delta";
			value: string;
		};
	};
}

export interface AugurStreamDone {
	result: {
		kind: "done";
		payload: {
			case: "done";
			value: {
				answer: string;
				citations: AugurCitation[];
			};
		};
	};
}

// =============================================================================
// RSS Feed Link Types
// =============================================================================

export interface RssFeedLink {
	id: string;
	url: string;
	title: string;
	description: string;
}
