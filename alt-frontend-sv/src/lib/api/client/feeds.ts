import type { CursorResponse } from "$lib/api";
import type { BackendFeedItem, SanitizedFeed } from "$lib/schema/feed";
import { sanitizeFeed } from "$lib/schema/feed";
import type {
	CursorSearchResponse,
	FeedSearchResult,
	SearchFeedItem,
} from "$lib/schema/search";
import type {
	DetailedFeedStatsSummary,
	FeedStatsSummary,
	UnreadCountResponse,
} from "$lib/schema/stats";
import { callClientAPI } from "./core";

/**
 * カーソルベースでフィードを取得（クライアントサイド）
 */
export async function getFeedsWithCursorClient(
	cursor?: string,
	limit: number = 20,
): Promise<CursorResponse<SanitizedFeed>> {
	const params = new URLSearchParams();
	params.set("limit", limit.toString());
	if (cursor) {
		params.set("cursor", cursor);
	}

	const response = await callClientAPI<CursorResponse<BackendFeedItem>>(
		`/v1/feeds/fetch/cursor?${params.toString()}`,
	);

	// Transform backend items to sanitized feeds
	const sanitizedData = response.data.map((item) => sanitizeFeed(item));

	return {
		data: sanitizedData,
		next_cursor: response.next_cursor,
		has_more: response.has_more ?? response.next_cursor !== null,
	};
}

/**
 * フィードを既読にする（クライアントサイド）
 */
export async function updateFeedReadStatusClient(
	feedUrl: string,
): Promise<void> {
	await callClientAPI("/v1/feeds/read", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ feed_url: feedUrl }),
	});
}

/**
 * カーソルベースで既読フィードを取得（クライアントサイド）
 */
export async function getReadFeedsWithCursorClient(
	cursor?: string,
	limit: number = 32,
): Promise<CursorResponse<SanitizedFeed>> {
	const params = new URLSearchParams();
	params.set("limit", limit.toString());
	if (cursor) {
		params.set("cursor", cursor);
	}

	const response = await callClientAPI<CursorResponse<BackendFeedItem>>(
		`/v1/feeds/fetch/viewed/cursor?${params.toString()}`,
	);

	// Transform backend items to sanitized feeds
	const sanitizedData = response.data.map((item) => sanitizeFeed(item));

	return {
		data: sanitizedData,
		next_cursor: response.next_cursor,
		has_more: response.has_more ?? response.next_cursor !== null,
	};
}

/**
 * フィードを検索（クライアントサイド）
 * カーソルベースのページネーションをサポート（offsetベース）
 */
export async function searchFeedsClient(
	query: string,
	cursor?: number, // Offset for pagination (integer)
	limit: number = 20,
): Promise<FeedSearchResult> {
	try {
		// Prepare request payload
		const payload: { query: string; cursor?: number; limit?: number } = {
			query,
		};

		if (cursor !== undefined && cursor !== null) {
			payload.cursor = cursor;
		}
		if (limit !== undefined) {
			payload.limit = limit;
		}

		const response = await callClientAPI<
			SearchFeedItem[] | FeedSearchResult | CursorSearchResponse
		>("/v1/feeds/search", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(payload),
		});

		// Handle cursor-based response (new format)
		if (
			typeof response === "object" &&
			response !== null &&
			"data" in response &&
			"next_cursor" in response
		) {
			const cursorResponse = response as CursorSearchResponse;
			return {
				results: cursorResponse.data,
				error: null,
				next_cursor: cursorResponse.next_cursor,
				has_more: cursorResponse.has_more ?? cursorResponse.next_cursor !== null,
			};
		}

		// Handle array response (backward compatibility)
		if (Array.isArray(response)) {
			return {
				results: response,
				error: null,
				next_cursor: null,
				has_more: false,
			};
		}

		// Handle FeedSearchResult response
		const result = response as FeedSearchResult;
		return {
			results: result.results || [],
			error: result.error || null,
			next_cursor: result.next_cursor ?? null,
			has_more: result.has_more ?? false,
		};
	} catch (error) {
		const errorMessage =
			error instanceof Error ? error.message : "Search failed";
		return {
			results: [],
			error: errorMessage,
			next_cursor: null,
			has_more: false,
		};
	}
}

/**
 * フィードの統計情報を取得（クライアントサイド）
 */
export async function getFeedStatsClient(): Promise<FeedStatsSummary> {
	return callClientAPI<FeedStatsSummary>("/v1/feeds/stats");
}

/**
 * フィードの詳細統計情報を取得（クライアントサイド）
 */
export async function getDetailedFeedStatsClient(): Promise<DetailedFeedStatsSummary> {
	return callClientAPI<DetailedFeedStatsSummary>("/v1/feeds/stats/detailed");
}

/**
 * 未読記事数を取得（クライアントサイド）
 */
export async function getUnreadCountClient(): Promise<UnreadCountResponse> {
	return callClientAPI<UnreadCountResponse>("/v1/feeds/count/unreads");
}
