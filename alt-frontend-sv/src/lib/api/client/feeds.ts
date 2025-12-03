import type { CursorResponse } from "$lib/api";
import type { BackendFeedItem, SanitizedFeed } from "$lib/schema/feed";
import { sanitizeFeed } from "$lib/schema/feed";
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

