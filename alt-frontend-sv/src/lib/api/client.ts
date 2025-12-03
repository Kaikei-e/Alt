import { browser } from "$app/environment";
import type { CursorResponse } from "$lib/api";
import type { BackendFeedItem, SanitizedFeed } from "$lib/schema/feed";
import { sanitizeFeed } from "$lib/schema/feed";

/**
 * クライアントサイドからバックエンドAPIを呼び出す
 */
async function callClientAPI<T>(
	endpoint: string,
	options?: RequestInit,
): Promise<T> {
	if (!browser) {
		throw new Error("This function can only be called from the client");
	}

	const url = `/api${endpoint}`;
	try {
		const response = await fetch(url, {
			...options,
			credentials: "include",
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url,
					status: response.status,
					statusText: response.statusText,
					errorBody: errorText.substring(0, 200),
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}

		return response.json();
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling API:", {
			url,
			message: errorMessage,
		});
		throw new Error(`Failed to connect to API: ${errorMessage}`);
	}
}

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
