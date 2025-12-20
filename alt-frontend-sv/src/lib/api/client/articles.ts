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
	const immediateRetryDelay = 100; // 100ms for immediate retry after detecting completion

	const startTime = Date.now();
	let lastStatus: string | null = null;
	let completedDetectedAt: number | null = null;

	for (let attempt = 0; attempt < maxAttempts; attempt++) {
		if (Date.now() - startTime > maxPollTime) {
			throw new Error("Summarization timeout");
		}

		// Wait before polling (except for first attempt and immediate retry after completion detection)
		if (attempt > 0) {
			// If we detected completion in previous attempt but didn't get summary, retry immediately
			if (completedDetectedAt !== null && Date.now() - completedDetectedAt < pollInterval) {
				await new Promise((resolve) => setTimeout(resolve, immediateRetryDelay));
			} else {
				await new Promise((resolve) => setTimeout(resolve, pollInterval));
			}
		}

		try {
			const pollStartTime = Date.now();

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

			const pollDuration = Date.now() - pollStartTime;

			// Log timing information for debugging
			if (statusResponse.status !== lastStatus) {
				console.log(`[SummarizeJob] Status changed: ${lastStatus} -> ${statusResponse.status}`, {
					jobId,
					attempt,
					pollDuration,
					elapsed: Date.now() - startTime,
				});
				lastStatus = statusResponse.status;
			}

			if (statusResponse.status === "completed") {
				// Check if we have the summary
				if (statusResponse.summary && statusResponse.summary.length > 0) {
					const totalDuration = Date.now() - startTime;
					console.log(`[SummarizeJob] Completed with summary`, {
						jobId,
						attempt,
						totalDuration,
						summaryLength: statusResponse.summary.length,
					});
					return {
						success: true,
						summary: statusResponse.summary,
						article_id: statusResponse.article_id || "",
						feed_url: feedUrl,
					};
				} else {
					// Status is completed but summary is not yet available
					// This might indicate a race condition - retry immediately
					if (completedDetectedAt === null) {
						completedDetectedAt = Date.now();
						console.log(`[SummarizeJob] Completed status detected but summary not available, retrying immediately`, {
							jobId,
							attempt,
						});
					}
					// Continue polling to get the summary
				}
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
			console.warn(`[SummarizeJob] Poll error (will retry)`, {
				jobId,
				attempt,
				error: error instanceof Error ? error.message : String(error),
			});
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

/**
 * Stream article summary (Client-side)
 * Returns a readable stream reader.
 */
export async function streamSummarizeArticleClient(
	feedUrl: string,
	articleId?: string,
	content?: string,
	title?: string,
	signal?: AbortSignal
): Promise<ReadableStreamDefaultReader<Uint8Array>> {
	const payload = {
		feed_url: feedUrl,
		article_id: articleId,
		content,
		title,
	};

	const response = await fetch("/sv/api/v1/feeds/summarize/stream", {
		method: "POST",
		signal,
		headers: {
			"Content-Type": "application/json",
		},
		credentials: "include", // 認証クッキーを送信
		body: JSON.stringify(payload),
	});

	if (!response.ok) {
		const errorText = await response.text().catch(() => "");
		const isAuthError = response.status === 401 || response.status === 403;
		console.error("Streaming request failed", {
			status: response.status,
			statusText: response.statusText,
			isAuthError,
			errorBody: errorText.substring(0, 200),
		});
		// Include status code in error message for better error handling
		const errorMsg = isAuthError
			? `Authentication failed: ${response.status} ${response.statusText}`
			: `Streaming failed: ${response.status} ${response.statusText}`;
		throw new Error(errorMsg);
	}

	if (!response.body) {
		throw new Error("Response body is empty");
	}

	return response.body.getReader();
}
