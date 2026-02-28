/**
 * Valibot schemas for REST v1 API response validation.
 * Used in contract tests to verify mock data matches actual API shape.
 */
import * as v from "valibot";

// =============================================================================
// Feed Schemas (REST v1)
// =============================================================================

export const FeedAuthorSchema = v.object({
	name: v.string(),
});

export const FeedItemV1Schema = v.object({
	id: v.string(),
	url: v.string(),
	title: v.string(),
	description: v.string(),
	link: v.string(),
	published_at: v.string(),
	tags: v.array(v.string()),
	author: FeedAuthorSchema,
	thumbnail: v.nullable(v.string()),
	feed_domain: v.string(),
	read_at: v.nullable(v.string()),
	created_at: v.string(),
	updated_at: v.string(),
});

export const FeedsResponseV1Schema = v.object({
	data: v.array(FeedItemV1Schema),
	next_cursor: v.nullable(v.string()),
	has_more: v.boolean(),
});

// =============================================================================
// Stats Schemas
// =============================================================================

export const StatsResponseSchema = v.object({
	feed_amount: v.object({ amount: v.number() }),
	total_articles: v.object({ amount: v.number() }),
	unsummarized_articles: v.object({ amount: v.number() }),
});

export const UnreadCountResponseSchema = v.object({
	count: v.number(),
});

// =============================================================================
// Search Schemas
// =============================================================================

export const SearchResultItemSchema = v.object({
	title: v.string(),
	description: v.string(),
	link: v.string(),
	published: v.string(),
	author: FeedAuthorSchema,
});

export const SearchResponseSchema = v.object({
	data: v.array(SearchResultItemSchema),
	next_cursor: v.nullable(v.string()),
	has_more: v.boolean(),
});

// =============================================================================
// Recap Schemas (REST v1)
// =============================================================================

export const EvidenceLinkV1Schema = v.object({
	article_id: v.string(),
	title: v.string(),
	source_url: v.string(),
	published_at: v.string(),
	lang: v.string(),
});

export const RecapGenreV1Schema = v.object({
	genre: v.string(),
	summary: v.string(),
	top_terms: v.array(v.string()),
	article_count: v.number(),
	cluster_count: v.number(),
	evidence_links: v.array(EvidenceLinkV1Schema),
	bullets: v.array(v.string()),
});

export const RecapResponseV1Schema = v.object({
	job_id: v.string(),
	executed_at: v.string(),
	window_start: v.string(),
	window_end: v.string(),
	total_articles: v.number(),
	genres: v.array(RecapGenreV1Schema),
});

// =============================================================================
// Article Schemas
// =============================================================================

export const ArticleContentResponseSchema = v.object({
	content: v.string(),
});

// =============================================================================
// RSS Feed Link Schemas
// =============================================================================

export const RSSFeedLinkSchema = v.object({
	id: v.string(),
	url: v.string(),
});

export const RSSFeedLinksListResponseSchema = v.object({
	links: v.array(RSSFeedLinkSchema),
});
