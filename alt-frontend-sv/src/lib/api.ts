import { env } from "$env/dynamic/private";

// バックエンドAPIのベースURL（サーバーサイドからは内部URLを使用）
const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";
const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

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
async function getBackendToken(cookie: string | null): Promise<string | null> {
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
		console.error("Failed to get backend token:", {
			message: errorMessage,
			authHubUrl: AUTH_HUB_URL,
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
		console.error("Network error calling backend API:", {
			url,
			message: errorMessage,
			backendBaseUrl: BACKEND_BASE_URL,
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
