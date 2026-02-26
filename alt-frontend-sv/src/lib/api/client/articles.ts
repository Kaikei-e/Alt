import { callClientAPI } from "./core";
import { createClientTransport } from "$lib/connect/transport.client";

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
	article_id: string;
	og_image_url: string;
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
 * Connect-RPCを使用
 */
export async function getArticleSummaryClient(
	feedUrl: string,
): Promise<FetchArticleSummaryResponse> {
	const transport = createClientTransport();
	const { fetchArticleSummary } = await import("$lib/connect/articles");
	const response = await fetchArticleSummary(transport, [feedUrl]);

	// Convert camelCase to snake_case for API compatibility
	return {
		matched_articles: response.matchedArticles.map((item) => ({
			article_url: feedUrl,
			title: item.title,
			author: item.author || undefined,
			content: item.content,
			content_type: "text/html",
			published_at: item.publishedAt,
			fetched_at: item.fetchedAt,
			source_id: item.sourceId,
		})),
		total_matched: response.totalMatched,
		requested_count: response.requestedCount,
	};
}

/**
 * Get feed content on-the-fly (クライアントサイド)
 * Connect-RPC を使用
 */
export async function getFeedContentOnTheFlyClient(
	feedUrl: string,
	options?: { signal?: AbortSignal },
): Promise<FeedContentOnTheFlyResponse> {
	const transport = createClientTransport();
	const { fetchArticleContent } = await import("$lib/connect/articles");
	const response = await fetchArticleContent(
		transport,
		feedUrl,
		options?.signal,
	);

	return {
		content: response.content,
		article_id: response.articleId,
		og_image_url: response.ogImageUrl,
	};
}

/**
 * Archive content (クライアントサイド)
 * Connect-RPC を使用
 */
export async function archiveContentClient(
	feedUrl: string,
	title?: string,
): Promise<MessageResponse> {
	const transport = createClientTransport();
	const { archiveArticle } = await import("$lib/connect/articles");
	const response = await archiveArticle(transport, feedUrl, title?.trim());

	return {
		message: response.message,
	};
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
			if (
				completedDetectedAt !== null &&
				Date.now() - completedDetectedAt < pollInterval
			) {
				await new Promise((resolve) =>
					setTimeout(resolve, immediateRetryDelay),
				);
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
				console.log(
					`[SummarizeJob] Status changed: ${lastStatus} -> ${statusResponse.status}`,
					{
						jobId,
						attempt,
						pollDuration,
						elapsed: Date.now() - startTime,
					},
				);
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
						console.log(
							`[SummarizeJob] Completed status detected but summary not available, retrying immediately`,
							{
								jobId,
								attempt,
							},
						);
					}
					// Continue polling to get the summary
				}
			} else if (statusResponse.status === "failed") {
				throw new Error(statusResponse.error_message || "Summarization failed");
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
 * 内部的にConnect-RPC Server Streamingを使用し、全チャンクを収集してから返す
 */
export async function summarizeArticleClient(
	feedUrl: string,
): Promise<SummarizeArticleResponse> {
	const transport = createClientTransport();
	const { streamSummarize } = await import("$lib/connect/feeds");

	const result = await streamSummarize(transport, { feedUrl });

	return {
		success: true,
		summary: result.summary,
		article_id: result.articleId,
		feed_url: feedUrl,
	};
}

/**
 * Register favorite feed (クライアントサイド)
 * Connect-RPC を使用
 */
export async function registerFavoriteFeedClient(
	url: string,
): Promise<MessageResponse> {
	const transport = createClientTransport();
	const { registerFavoriteFeed } = await import("$lib/connect/rss");
	const response = await registerFavoriteFeed(transport, url);

	return {
		message: response.message,
	};
}
