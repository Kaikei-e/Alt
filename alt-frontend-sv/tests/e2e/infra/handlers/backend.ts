/**
 * Backend Mock Handlers
 * Handles REST v1 and Connect-RPC v2 API endpoints
 */

import http from "node:http";
import {
	FEEDS_RESPONSE,
	VIEWED_FEEDS_RESPONSE,
	FEED_STATS,
	DETAILED_FEED_STATS,
	UNREAD_COUNT,
	RSS_FEED_LINKS,
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_RESPONSE,
	CONNECT_DETAILED_STATS,
	CONNECT_UNREAD_COUNT,
	CONNECT_ARTICLE_CONTENT,
} from "../data/feeds";
import {
	RECAP_RESPONSE,
	AUGUR_SSE_CHUNKS,
	AUGUR_CONNECT_MESSAGES,
	CONNECT_RECAP_RESPONSE,
} from "../data/recap";
import { JOB_PROGRESS_RESPONSE } from "../../fixtures/mockData";

export const BACKEND_PORT = 4003;

/**
 * Log helper for Backend server
 */
function log(msg: string) {
	console.log(`[Mock Backend] ${msg}`);
}

/**
 * Create the Backend mock server
 */
export function createBackendServer(): http.Server {
	return http.createServer((req, res) => {
		const url = new URL(req.url!, `http://${req.headers.host}`);
		const path = url.pathname;

		log(`${req.method} ${path}`);

		// Health check
		if (path === "/health") {
			res.writeHead(200, { "Content-Type": "text/plain" });
			res.end("OK");
			return;
		}

		res.setHeader("Content-Type", "application/json");

		// =============================================================================
		// REST v1 Endpoints
		// =============================================================================

		// Feeds - cursor-based pagination
		if (
			path === "/api/v1/feeds/fetch/cursor" ||
			path === "/v1/feeds/fetch/cursor"
		) {
			res.writeHead(200);
			res.end(JSON.stringify(FEEDS_RESPONSE));
			return;
		}

		// Viewed feeds
		if (
			path === "/api/v1/feeds/fetch/viewed/cursor" ||
			path === "/v1/feeds/fetch/viewed/cursor"
		) {
			res.writeHead(200);
			res.end(JSON.stringify(VIEWED_FEEDS_RESPONSE));
			return;
		}

		// RSS Feed Link List
		if (
			path === "/api/v1/rss-feed-link/list" ||
			path === "/v1/rss-feed-link/list"
		) {
			res.writeHead(200);
			res.end(JSON.stringify(RSS_FEED_LINKS));
			return;
		}

		// Stats
		if (path === "/api/v1/feeds/stats" || path === "/v1/feeds/stats") {
			res.writeHead(200);
			res.end(JSON.stringify(FEED_STATS));
			return;
		}

		// Stats detailed
		if (
			path === "/api/v1/feeds/stats/detailed" ||
			path === "/v1/feeds/stats/detailed"
		) {
			res.writeHead(200);
			res.end(JSON.stringify(DETAILED_FEED_STATS));
			return;
		}

		// Unread count
		if (
			path === "/api/v1/feeds/count/unreads" ||
			path === "/v1/feeds/count/unreads"
		) {
			res.writeHead(200);
			res.end(JSON.stringify(UNREAD_COUNT));
			return;
		}

		// Mark as read
		if (path === "/api/v1/feeds/read" || path === "/v1/feeds/read") {
			res.writeHead(200);
			res.end(JSON.stringify({ ok: true }));
			return;
		}

		// Recap 7-days
		if (path === "/api/v1/recap/7days" || path === "/v1/recap/7days") {
			res.writeHead(200);
			res.end(JSON.stringify(RECAP_RESPONSE));
			return;
		}

		// Job progress dashboard (handles both /api/v1 and /v1 paths for recap-worker)
		if (
			path === "/api/v1/dashboard/job-progress" ||
			path.startsWith("/api/v1/dashboard/job-progress") ||
			path === "/v1/dashboard/job-progress" ||
			path.startsWith("/v1/dashboard/job-progress")
		) {
			res.writeHead(200);
			res.end(JSON.stringify(JOB_PROGRESS_RESPONSE));
			return;
		}

		// Augur chat (streaming SSE)
		if (path === "/api/v1/augur/chat" || path === "/v1/augur/chat") {
			res.writeHead(200, {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache",
				Connection: "keep-alive",
			});
			for (const chunk of AUGUR_SSE_CHUNKS) {
				res.write(chunk);
			}
			res.end();
			return;
		}

		// Article content
		if (
			path === "/api/v1/articles/content" ||
			path === "/v1/articles/content"
		) {
			res.writeHead(200);
			res.end(
				JSON.stringify({
					content: "<p>This is a mocked article content.</p>",
					article_id: "mock-article-id",
				}),
			);
			return;
		}

		// =============================================================================
		// Connect-RPC v2 Endpoints
		// =============================================================================

		// GetUnreadFeeds
		if (path === "/alt.feeds.v2.FeedService/GetUnreadFeeds") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_FEEDS_RESPONSE));
			return;
		}

		// GetReadFeeds
		if (path === "/alt.feeds.v2.FeedService/GetReadFeeds") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_READ_FEEDS_RESPONSE));
			return;
		}

		// MarkAsRead
		if (path === "/alt.feeds.v2.FeedService/MarkAsRead") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify({ message: "Feed marked as read" }));
			return;
		}

		// GetDetailedFeedStats
		if (path === "/alt.feeds.v2.FeedService/GetDetailedFeedStats") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_DETAILED_STATS));
			return;
		}

		// GetUnreadCount
		if (path === "/alt.feeds.v2.FeedService/GetUnreadCount") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_UNREAD_COUNT));
			return;
		}

		// FetchArticleContent
		if (path === "/alt.articles.v2.ArticleService/FetchArticleContent") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_ARTICLE_CONTENT));
			return;
		}

		// StreamChat (Augur) - Connect-RPC streaming
		if (path === "/alt.augur.v2.AugurService/StreamChat") {
			res.setHeader("Content-Type", "application/connect+json");
			res.setHeader("Connect-Content-Encoding", "identity");
			res.setHeader("Connect-Accept-Encoding", "identity");
			res.writeHead(200);
			// Connect-RPC streaming format: newline-delimited JSON
			res.end(
				AUGUR_CONNECT_MESSAGES.map((m) => JSON.stringify(m)).join("\n") + "\n",
			);
			return;
		}

		// GetSevenDayRecap (Connect-RPC)
		if (path === "/alt.recap.v2.RecapService/GetSevenDayRecap") {
			res.setHeader("Content-Type", "application/json");
			res.writeHead(200);
			res.end(JSON.stringify(CONNECT_RECAP_RESPONSE));
			return;
		}

		// Default response for unknown endpoints
		res.writeHead(200);
		res.end(JSON.stringify({}));
	});
}
