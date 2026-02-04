import { env } from "$env/dynamic/private";
import type { FeedLink } from "$lib/schema/feedLink";
import { createServerTransport } from "$lib/connect/transport-server";
import {
	getUnreadFeeds as getUnreadFeedsConnect,
	getReadFeeds as getReadFeedsConnect,
	type ConnectFeedItem,
} from "$lib/connect/feeds";

// バックエンドAPIのベースURL（サーバーサイドからは内部URLを使用）
const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";
const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

/**
 * Get CSRF token from auth-hub session
 * V-004: CSRF protection for state-changing operations
 */
export async function getCSRFToken(
	cookieHeader: string | null,
): Promise<string | null> {
	if (!cookieHeader) return null;

	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: { Cookie: cookieHeader },
			cache: "no-store",
		});

		if (!response.ok) return null;

		const data = await response.json();
		return data.csrf_token ?? null;
	} catch {
		return null;
	}
}

export interface DetailedFeedStats {
	feed_amount: { amount: number };
	total_articles: { amount: number };
	unsummarized_articles: { amount: number };
}

export interface UnreadCount {
	count: number;
}

/**
 * auth-hubからバックエンドトークンを取得
 */
export async function getBackendToken(
	cookie: string | null,
): Promise<string | null> {
	if (!cookie) {
		console.warn("No cookie provided for backend token");
		return null;
	}

	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: {
				cookie: cookie,
			},
			cache: "no-store",
		});

		if (!response.ok) {
			console.warn(
				`Auth-hub session endpoint returned ${response.status}: ${response.statusText}`,
			);
			return null;
		}

		const token = response.headers.get("X-Alt-Backend-Token");
		if (!token) {
			console.warn("X-Alt-Backend-Token header not found in response");
		}
		return token;
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Failed to get backend token:", {
			message: errorMessage,
			stack: errorStack,
			authHubUrl: AUTH_HUB_URL,
			hasCookie: !!cookie,
		});
		return null;
	}
}

/**
 * バックエンドAPIを呼び出す
 */
async function callBackendAPI<T>(
	endpoint: string,
	cookie: string | null,
): Promise<T> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	// JWTトークンはX-Alt-Backend-Tokenヘッダーで送信
	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_BASE_URL}${endpoint}`;
	try {
		const response = await fetch(url, {
			headers,
			cache: "no-store",
		});

		if (!response.ok) {
			const contentType = response.headers.get("content-type") || "";
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url,
					status: response.status,
					statusText: response.statusText,
					contentType,
					errorBody: errorText.substring(0, 200),
					hasToken: !!token,
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}

		// Check Content-Type before parsing JSON
		const contentType = response.headers.get("content-type") || "";
		const isJson = contentType.includes("application/json");

		if (!isJson) {
			const text = await response.text().catch(() => "");
			console.error("Backend API returned non-JSON response:", {
				url,
				contentType,
				status: response.status,
				bodyPreview: text.substring(0, 200),
			});
			throw new Error(
				`Backend API returned non-JSON response (${contentType}). Expected application/json.`,
			);
		}

		try {
			return await response.json();
		} catch (jsonError) {
			const errorMessage =
				jsonError instanceof Error ? jsonError.message : String(jsonError);
			console.error("Failed to parse JSON response from backend API:", {
				url,
				contentType,
				error: errorMessage,
			});
			throw new Error(
				`Failed to parse JSON response from backend: ${errorMessage}`,
			);
		}
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		if (
			error instanceof Error &&
			(error.message.includes("non-JSON response") ||
				error.message.includes("Failed to parse JSON"))
		) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			stack: errorStack,
			backendBaseUrl: BACKEND_BASE_URL,
			hasToken: !!token,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}

/**
 * 詳細統計を取得
 */
export async function getFeedStats(
	cookie: string | null,
): Promise<DetailedFeedStats> {
	return callBackendAPI<DetailedFeedStats>("/v1/feeds/stats/detailed", cookie);
}

/**
 * 今日の未読数を取得
 */
export async function getTodayUnreadCount(
	cookie: string | null,
	since: string,
): Promise<UnreadCount> {
	return callBackendAPI<UnreadCount>(
		`/v1/feeds/count/unreads?since=${encodeURIComponent(since)}`,
		cookie,
	);
}

/**
 * カーソルベースのレスポンス型
 */
export interface CursorResponse<T> {
	data: T[];
	next_cursor: string | null;
	has_more?: boolean;
}

/**
 * ConnectFeedItem を BackendFeedItem 互換形式に変換
 */
function connectFeedToBackendFormat(item: ConnectFeedItem): unknown {
	return {
		title: item.title,
		description: item.description,
		link: item.link,
		published: item.createdAt, // Use createdAt for RFC3339 format
		created_at: item.createdAt,
		author: item.author ? { name: item.author } : undefined,
		// Article ID in the articles table - required for mark-as-read functionality
		article_id: item.articleId,
	};
}

/**
 * カーソルベースでフィードを取得
 * Connect-RPC を使用
 */
export async function getFeedsWithCursor(
	cookie: string | null,
	cursor?: string,
	limit: number = 20,
): Promise<CursorResponse<unknown>> {
	const transport = await createServerTransport(cookie);
	const response = await getUnreadFeedsConnect(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToBackendFormat),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * フィードを既読にする
 */
export async function updateFeedReadStatus(
	cookie: string | null,
	feedUrl: string,
): Promise<void> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const url = `${BACKEND_BASE_URL}/v1/feeds/read`;
	try {
		const response = await fetch(url, {
			method: "POST",
			headers,
			body: JSON.stringify({ feed_url: feedUrl }),
			cache: "no-store",
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
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			backendBaseUrl: BACKEND_BASE_URL,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}

/**
 * カーソルベースで既読フィードを取得
 * Connect-RPC を使用
 */
export async function getReadFeedsWithCursor(
	cookie: string | null,
	cursor?: string,
	limit: number = 32,
): Promise<CursorResponse<unknown>> {
	const transport = await createServerTransport(cookie);
	const response = await getReadFeedsConnect(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToBackendFormat),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * RSSフィードリンク一覧を取得
 */
export async function getFeedLinks(cookie: string | null): Promise<FeedLink[]> {
	return callBackendAPI<FeedLink[]>("/v1/rss-feed-link/list", cookie);
}

/**
 * RSSフィードリンクを登録
 */
export async function registerRssFeed(
	cookie: string | null,
	url: string,
): Promise<void> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const endpoint = "/v1/rss-feed-link/register";
	const fullUrl = `${BACKEND_BASE_URL}${endpoint}`;

	try {
		const response = await fetch(fullUrl, {
			method: "POST",
			headers,
			body: JSON.stringify({ url }),
			cache: "no-store",
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url: fullUrl,
					status: response.status,
					statusText: response.statusText,
					errorBody: errorText.substring(0, 200),
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling backend API:", {
			url: fullUrl,
			message: errorMessage,
			backendBaseUrl: BACKEND_BASE_URL,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}

/**
 * RSSフィードリンクを削除
 */
export async function deleteFeedLink(
	cookie: string | null,
	id: string,
): Promise<void> {
	const token = await getBackendToken(cookie);

	const headers: HeadersInit = {
		"Content-Type": "application/json",
	};

	if (token) {
		headers["X-Alt-Backend-Token"] = token;
	}

	const endpoint = `/v1/rss-feed-link/${encodeURIComponent(id)}`;
	const fullUrl = `${BACKEND_BASE_URL}${endpoint}`;

	try {
		const response = await fetch(fullUrl, {
			method: "DELETE",
			headers,
			cache: "no-store",
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`API call failed: ${response.status} ${response.statusText}`,
				{
					url: fullUrl,
					status: response.status,
					statusText: response.statusText,
					errorBody: errorText.substring(0, 200),
				},
			);
			throw new Error(
				`API call failed: ${response.status} ${response.statusText}`,
			);
		}
	} catch (error) {
		if (error instanceof Error && error.message.includes("API call failed")) {
			throw error;
		}
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Network error calling backend API:", {
			url: fullUrl,
			message: errorMessage,
			backendBaseUrl: BACKEND_BASE_URL,
		});
		throw new Error(`Failed to connect to backend: ${errorMessage}`);
	}
}

// ============================================
// Tag Trail API functions
// ============================================

export interface RandomFeedResponse {
	feed: {
		id: string;
		url: string;
		title?: string;
		description?: string;
	} | null;
}

export interface ArticlesByTagResponse {
	articles: Array<{
		id: string;
		title: string;
		link: string;
		published_at: string;
		feed_title?: string;
	}>;
	next_cursor?: string;
	has_more: boolean;
}

export interface ArticleTagsResponse {
	article_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
}

/**
 * Get a random subscription feed for Tag Trail (server-side)
 */
export async function getRandomSubscription(
	cookie: string | null,
): Promise<RandomFeedResponse> {
	return callBackendAPI<RandomFeedResponse>("/v1/rss-feed-link/random", cookie);
}

/**
 * Get articles by tag ID for Tag Trail (server-side)
 */
export async function getArticlesByTag(
	cookie: string | null,
	tagId: string,
	cursor?: string,
	limit = 20,
): Promise<ArticlesByTagResponse> {
	const params = new URLSearchParams({ tag_id: tagId, limit: limit.toString() });
	if (cursor) {
		params.append("cursor", cursor);
	}
	return callBackendAPI<ArticlesByTagResponse>(
		`/v1/articles/by-tag?${params.toString()}`,
		cookie,
	);
}

/**
 * Get tags for a specific article (server-side)
 */
export async function getArticleTags(
	cookie: string | null,
	articleId: string,
): Promise<ArticleTagsResponse> {
	return callBackendAPI<ArticleTagsResponse>(
		`/v1/articles/${articleId}/tags`,
		cookie,
	);
}

export interface FeedTagsResponse {
	feed_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
	next_cursor?: string;
}

/**
 * Get tags for a specific feed by ID (server-side)
 * Used by Tag Trail to get tags for a random feed
 */
export async function getFeedTagsById(
	cookie: string | null,
	feedId: string,
	limit = 20,
): Promise<FeedTagsResponse> {
	return callBackendAPI<FeedTagsResponse>(
		`/v1/feeds/${feedId}/tags?limit=${limit}`,
		cookie,
	);
}
