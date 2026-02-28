/**
 * REST v1 Schema Validation Contract Tests
 *
 * Validates that REST v1 mock data in E2E tests conforms to Valibot schemas.
 * These tests catch shape mismatches between mock data and actual API contracts.
 */
import { describe, it, expect } from "vitest";
import * as v from "valibot";
import {
	FeedsResponseV1Schema,
	StatsResponseSchema,
	SearchResponseSchema,
	RecapResponseV1Schema,
	ArticleContentResponseSchema,
	RSSFeedLinksListResponseSchema,
	UnreadCountResponseSchema,
} from "$lib/schema/api-responses";
import {
	FEEDS_RESPONSE,
	STATS_RESPONSE,
	SEARCH_RESPONSE,
	RECAP_RESPONSE,
	ARTICLE_CONTENT_RESPONSE,
	RSS_FEED_LINKS_LIST_RESPONSE,
	UNREAD_COUNT_RESPONSE,
} from "../../../tests/e2e/fixtures/mockData";

describe("REST v1 Contract Validation", () => {
	it("FEEDS_RESPONSE conforms to FeedsResponseV1Schema", () => {
		const result = v.safeParse(FeedsResponseV1Schema, FEEDS_RESPONSE);
		if (!result.success) {
			console.error("Validation issues:", result.issues);
		}
		expect(result.success).toBe(true);
	});

	it("STATS_RESPONSE conforms to StatsResponseSchema", () => {
		const result = v.safeParse(StatsResponseSchema, STATS_RESPONSE);
		if (!result.success) {
			console.error("Validation issues:", result.issues);
		}
		expect(result.success).toBe(true);
	});

	it("UNREAD_COUNT_RESPONSE conforms to UnreadCountResponseSchema", () => {
		const result = v.safeParse(
			UnreadCountResponseSchema,
			UNREAD_COUNT_RESPONSE,
		);
		expect(result.success).toBe(true);
	});

	it("SEARCH_RESPONSE conforms to SearchResponseSchema", () => {
		const result = v.safeParse(SearchResponseSchema, SEARCH_RESPONSE);
		if (!result.success) {
			console.error("Validation issues:", result.issues);
		}
		expect(result.success).toBe(true);
	});

	it("RECAP_RESPONSE conforms to RecapResponseV1Schema", () => {
		const result = v.safeParse(RecapResponseV1Schema, RECAP_RESPONSE);
		if (!result.success) {
			console.error("Validation issues:", result.issues);
		}
		expect(result.success).toBe(true);
	});

	it("ARTICLE_CONTENT_RESPONSE conforms to ArticleContentResponseSchema", () => {
		const result = v.safeParse(
			ArticleContentResponseSchema,
			ARTICLE_CONTENT_RESPONSE,
		);
		expect(result.success).toBe(true);
	});

	it("RSS_FEED_LINKS_LIST_RESPONSE conforms to RSSFeedLinksListResponseSchema", () => {
		const result = v.safeParse(
			RSSFeedLinksListResponseSchema,
			RSS_FEED_LINKS_LIST_RESPONSE,
		);
		expect(result.success).toBe(true);
	});
});
