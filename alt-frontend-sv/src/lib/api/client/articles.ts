import { callClientAPI } from "./core";

/**
 * Safe HTML string type (server-sanitized)
 */
export type SafeHtmlString = string;

/**
 * Article summary item from API
 */
export interface ArticleSummaryItem {
	article_url: string;
	title: string;
	author?: string;
	content: SafeHtmlString;
	content_type: string;
	published_at: string;
	fetched_at: string;
	source_id: string;
}

/**
 * Fetch article summary response
 */
export interface FetchArticleSummaryResponse {
	matched_articles: ArticleSummaryItem[];
	total_matched: number;
	requested_count: number;
}

/**
 * Feed content on-the-fly response
 */
export interface FeedContentOnTheFlyResponse {
	content: SafeHtmlString;
}

/**
 * Message response from API
 */
export interface MessageResponse {
	message: string;
}

/**
 * Summarize article response
 */
export interface SummarizeArticleResponse {
	success: boolean;
	summary: string;
	article_id: string;
	feed_url: string;
}

/**
 * Get article summary (クライアントサイド)
 */
export async function getArticleSummaryClient(
	feedUrl: string,
): Promise<FetchArticleSummaryResponse> {
	return callClientAPI<FetchArticleSummaryResponse>("/v1/articles/summary", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ feed_urls: [feedUrl] }),
	});
}

/**
 * Get feed content on-the-fly (クライアントサイド)
 */
export async function getFeedContentOnTheFlyClient(
	feedUrl: string,
): Promise<FeedContentOnTheFlyResponse> {
	return callClientAPI<FeedContentOnTheFlyResponse>("/v1/articles/content", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ url: feedUrl }),
	});
}

/**
 * Archive content (クライアントサイド)
 */
export async function archiveContentClient(
	feedUrl: string,
	title?: string,
): Promise<MessageResponse> {
	const payload: Record<string, unknown> = { feed_url: feedUrl };
	if (title?.trim()) {
		payload.title = title.trim();
	}

	return callClientAPI<MessageResponse>("/v1/articles/archive", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(payload),
	});
}

/**
 * Summarize article response with job info (for async operations)
 */
interface SummarizeJobResponse {
	success: boolean;
	job_id?: string;
	status_url?: string;
	article_id?: string;
	feed_url?: string;
	message?: string;
	summary?: string;
}

/**
 * Poll for summarize job status
 */
async function pollSummarizeJobStatus(
	jobId: string,
	statusUrl: string,
	feedUrl: string,
): Promise<SummarizeArticleResponse> {
	const maxAttempts = 60;
	const pollInterval = 2000; // 2 seconds
	const maxPollTime = 300000; // 5 minutes

	const startTime = Date.now();

	for (let attempt = 0; attempt < maxAttempts; attempt++) {
		if (Date.now() - startTime > maxPollTime) {
			throw new Error("Summarization timeout");
		}

		if (attempt > 0) {
			await new Promise((resolve) => setTimeout(resolve, pollInterval));
		}

		try {
			// Handle both absolute and relative URLs
			let statusResponse: {
				status: string;
				summary?: string;
				article_id?: string;
				error_message?: string;
			};

			if (statusUrl.startsWith("http")) {
				// Absolute URL - use fetch directly
				const response = await fetch(statusUrl, {
					method: "GET",
					credentials: "include",
				});

				if (!response.ok && response.status !== 202) {
					throw new Error(`Status check failed: ${response.status}`);
				}

				const contentType = response.headers.get("content-type") || "";
				if (!contentType.includes("application/json")) {
					throw new Error("Status response is not JSON");
				}

				statusResponse = await response.json();
			} else {
				// Relative URL - remove /api prefix if present (callClientAPI adds /sv/api)
				const normalizedStatusUrl = statusUrl.startsWith("/api/")
					? statusUrl.replace(/^\/api/, "")
					: statusUrl.startsWith("/")
						? statusUrl
						: `/${statusUrl}`;

				statusResponse = await callClientAPI<{
					status: string;
					summary?: string;
					article_id?: string;
					error_message?: string;
				}>(normalizedStatusUrl, {
					method: "GET",
				});
			}

			if (statusResponse.status === "completed") {
				return {
					success: true,
					summary: statusResponse.summary || "",
					article_id: statusResponse.article_id || "",
					feed_url: feedUrl,
				};
			} else if (statusResponse.status === "failed") {
				throw new Error(
					statusResponse.error_message || "Summarization failed",
				);
			}
			// Continue polling for "pending" or "running"
		} catch (error) {
			if (attempt === maxAttempts - 1) {
				throw error;
			}
			// Continue polling on errors (except on last attempt)
		}
	}

	throw new Error("Summarization timeout");
}

/**
 * Summarize article (クライアントサイド)
 */
export async function summarizeArticleClient(
	feedUrl: string,
): Promise<SummarizeArticleResponse> {
	const response = await callClientAPI<SummarizeJobResponse>(
		"/v1/feeds/summarize",
		{
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ feed_url: feedUrl }),
		},
	);

	// Check if this is an async job (202 Accepted with job_id)
	if (response.job_id && response.status_url) {
		// Poll for job completion
		return await pollSummarizeJobStatus(
			response.job_id,
			response.status_url,
			feedUrl,
		);
	}

	// Immediate result (200 OK) - return as SummarizeArticleResponse
	return response as SummarizeArticleResponse;
}

/**
 * Register favorite feed (クライアントサイド)
 */
export async function registerFavoriteFeedClient(
	url: string,
): Promise<MessageResponse> {
	return callClientAPI<MessageResponse>("/v1/feeds/register/favorite", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ url }),
	});
}
