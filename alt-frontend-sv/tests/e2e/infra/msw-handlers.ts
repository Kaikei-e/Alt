/**
 * MSW Request Handlers
 *
 * These handlers use MSW's http and HttpResponse APIs for use with setupServer.
 * They provide the same mock responses as the http.createServer handlers,
 * but in MSW's declarative format for use in tests.
 */

import { http, HttpResponse } from "msw";
import {
	buildKratosSession,
	buildLoginFlow,
	buildRegistrationFlow,
	hasSessionCookie,
	KRATOS_SESSION_COOKIE_NAME,
	KRATOS_SESSION_COOKIE_VALUE,
	buildAuthHubSession,
	DEV_USER_ID,
	DEV_JWT_SECRET,
	DEV_JWT_ISSUER,
	DEV_JWT_AUDIENCE,
} from "./data/session";
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
} from "./data/feeds";
import {
	RECAP_RESPONSE,
	AUGUR_SSE_CHUNKS,
	AUGUR_CONNECT_MESSAGES,
} from "./data/recap";

// =============================================================================
// Kratos Handlers
// =============================================================================

export const kratosHandlers = [
	// Health check
	http.get("*/health", () => HttpResponse.text("OK")),

	// Session validation (whoami)
	http.get("*/sessions/whoami", ({ request }) => {
		const cookieHeader = request.headers.get("cookie");
		if (hasSessionCookie(cookieHeader || undefined)) {
			return HttpResponse.json(buildKratosSession());
		}
		return HttpResponse.json(
			{
				error: {
					code: "session_not_found",
					message: "No active Kratos session cookie was provided.",
				},
			},
			{ status: 401 },
		);
	}),

	// Login POST
	http.post("*/self-service/login", ({ request }) => {
		const url = new URL(request.url);
		const returnTo = url.searchParams.get("return_to") || "/sv/home";
		return new HttpResponse(null, {
			status: 303,
			headers: {
				Location: returnTo,
				"Set-Cookie": `${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}; Path=/; HttpOnly`,
			},
		});
	}),

	// Registration POST
	http.post("*/self-service/registration", ({ request }) => {
		const url = new URL(request.url);
		const returnTo = url.searchParams.get("return_to") || "/sv/home";
		return new HttpResponse(null, {
			status: 303,
			headers: {
				Location: returnTo,
				"Set-Cookie": `${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}; Path=/; HttpOnly`,
			},
		});
	}),

	// Login flow browser redirect
	http.get("*/self-service/login/browser", ({ request }) => {
		const url = new URL(request.url);
		const returnTo = url.searchParams.get("return_to") || "/sv/home";
		const returnToUrl = new URL(returnTo, request.url);
		const redirectUrl = `${returnToUrl.origin}/sv/auth/login?flow=flow-e2e-mock`;
		return new HttpResponse(null, {
			status: 303,
			headers: { Location: redirectUrl },
		});
	}),

	// Login flow GET
	http.get("*/self-service/login/flows", ({ request }) => {
		const host = new URL(request.url).host;
		return HttpResponse.json(buildLoginFlow(`http://${host}`));
	}),

	// Login flow with query param
	http.get("*/self-service/login", ({ request }) => {
		const host = new URL(request.url).host;
		return HttpResponse.json(buildLoginFlow(`http://${host}`));
	}),

	// Registration flow browser redirect
	http.get("*/self-service/registration/browser", ({ request }) => {
		const url = new URL(request.url);
		const returnTo = url.searchParams.get("return_to") || "/sv/home";
		const returnToUrl = new URL(returnTo, request.url);
		const redirectUrl = `${returnToUrl.origin}/sv/register?flow=flow-e2e-mock-reg`;
		return new HttpResponse(null, {
			status: 303,
			headers: { Location: redirectUrl },
		});
	}),

	// Registration flow GET
	http.get("*/self-service/registration/flows", ({ request }) => {
		const host = new URL(request.url).host;
		return HttpResponse.json(buildRegistrationFlow(`http://${host}`));
	}),

	// Registration flow with query param
	http.get("*/self-service/registration", ({ request }) => {
		const host = new URL(request.url).host;
		return HttpResponse.json(buildRegistrationFlow(`http://${host}`));
	}),
];

// =============================================================================
// AuthHub Handlers
// =============================================================================

// Note: JWT generation requires dynamic import for jsonwebtoken
// For MSW handlers, we return a static token for simplicity
const STATIC_DEV_TOKEN =
	"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImRldkBsb2NhbGhvc3QiLCJyb2xlIjoiYWRtaW4iLCJzaWQiOiJkZXYtc2Vzc2lvbiIsInN1YiI6IjAwMDAwMDAwLTAwMDAtMDAwMC0wMDAwLTAwMDAwMDAwMDAwMSIsImlzcyI6ImF1dGgtaHViIiwiYXVkIjoiYWx0LWJhY2tlbmQiLCJleHAiOjk5OTk5OTk5OTl9.mock-signature";

export const authHubHandlers = [
	// Health check
	http.get("*/health", () => HttpResponse.text("OK")),

	// Session endpoint
	http.get("*/session", () => {
		return HttpResponse.json(buildAuthHubSession(), {
			headers: {
				"X-Alt-Backend-Token": STATIC_DEV_TOKEN,
			},
		});
	}),

	// Auth session endpoint
	http.get("*/auth/session", () => {
		return HttpResponse.json(buildAuthHubSession(), {
			headers: {
				"X-Alt-Backend-Token": STATIC_DEV_TOKEN,
			},
		});
	}),
];

// =============================================================================
// Backend Handlers
// =============================================================================

export const backendHandlers = [
	// Health check
	http.get("*/health", () => HttpResponse.text("OK")),

	// =============================================================================
	// REST v1 Endpoints
	// =============================================================================

	// Feeds - cursor-based pagination
	http.get("*/v1/feeds/fetch/cursor", () => HttpResponse.json(FEEDS_RESPONSE)),
	http.get("*/api/v1/feeds/fetch/cursor", () =>
		HttpResponse.json(FEEDS_RESPONSE),
	),

	// Viewed feeds
	http.get("*/v1/feeds/fetch/viewed/cursor", () =>
		HttpResponse.json(VIEWED_FEEDS_RESPONSE),
	),
	http.get("*/api/v1/feeds/fetch/viewed/cursor", () =>
		HttpResponse.json(VIEWED_FEEDS_RESPONSE),
	),

	// RSS Feed Link List
	http.get("*/v1/rss-feed-link/list", () => HttpResponse.json(RSS_FEED_LINKS)),
	http.get("*/api/v1/rss-feed-link/list", () =>
		HttpResponse.json(RSS_FEED_LINKS),
	),

	// Stats
	http.get("*/v1/feeds/stats", () => HttpResponse.json(FEED_STATS)),
	http.get("*/api/v1/feeds/stats", () => HttpResponse.json(FEED_STATS)),

	// Stats detailed
	http.get("*/v1/feeds/stats/detailed", () =>
		HttpResponse.json(DETAILED_FEED_STATS),
	),
	http.get("*/api/v1/feeds/stats/detailed", () =>
		HttpResponse.json(DETAILED_FEED_STATS),
	),

	// Unread count
	http.get("*/v1/feeds/count/unreads", () => HttpResponse.json(UNREAD_COUNT)),
	http.get("*/api/v1/feeds/count/unreads", () =>
		HttpResponse.json(UNREAD_COUNT),
	),

	// Mark as read
	http.post("*/v1/feeds/read", () => HttpResponse.json({ ok: true })),
	http.post("*/api/v1/feeds/read", () => HttpResponse.json({ ok: true })),

	// Recap 7-days
	http.get("*/v1/recap/7days", () => HttpResponse.json(RECAP_RESPONSE)),
	http.get("*/api/v1/recap/7days", () => HttpResponse.json(RECAP_RESPONSE)),

	// Augur chat (streaming SSE)
	http.post("*/v1/augur/chat", () => {
		const encoder = new TextEncoder();
		const stream = new ReadableStream({
			start(controller) {
				for (const chunk of AUGUR_SSE_CHUNKS) {
					controller.enqueue(encoder.encode(chunk));
				}
				controller.close();
			},
		});
		return new HttpResponse(stream, {
			headers: {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache",
				Connection: "keep-alive",
			},
		});
	}),
	http.post("*/api/v1/augur/chat", () => {
		const encoder = new TextEncoder();
		const stream = new ReadableStream({
			start(controller) {
				for (const chunk of AUGUR_SSE_CHUNKS) {
					controller.enqueue(encoder.encode(chunk));
				}
				controller.close();
			},
		});
		return new HttpResponse(stream, {
			headers: {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache",
				Connection: "keep-alive",
			},
		});
	}),

	// Article content
	http.get("*/v1/articles/content", () =>
		HttpResponse.json({
			content: "<p>This is a mocked article content.</p>",
			article_id: "mock-article-id",
		}),
	),
	http.get("*/api/v1/articles/content", () =>
		HttpResponse.json({
			content: "<p>This is a mocked article content.</p>",
			article_id: "mock-article-id",
		}),
	),

	// =============================================================================
	// Connect-RPC v2 Endpoints
	// =============================================================================

	// GetUnreadFeeds
	http.post("*/alt.feeds.v2.FeedService/GetUnreadFeeds", () =>
		HttpResponse.json(CONNECT_FEEDS_RESPONSE),
	),

	// GetReadFeeds
	http.post("*/alt.feeds.v2.FeedService/GetReadFeeds", () =>
		HttpResponse.json(CONNECT_READ_FEEDS_RESPONSE),
	),

	// MarkAsRead
	http.post("*/alt.feeds.v2.FeedService/MarkAsRead", () =>
		HttpResponse.json({ message: "Feed marked as read" }),
	),

	// GetDetailedFeedStats
	http.post("*/alt.feeds.v2.FeedService/GetDetailedFeedStats", () =>
		HttpResponse.json(CONNECT_DETAILED_STATS),
	),

	// GetUnreadCount
	http.post("*/alt.feeds.v2.FeedService/GetUnreadCount", () =>
		HttpResponse.json(CONNECT_UNREAD_COUNT),
	),

	// FetchArticleContent
	http.post("*/alt.articles.v2.ArticleService/FetchArticleContent", () =>
		HttpResponse.json(CONNECT_ARTICLE_CONTENT),
	),

	// StreamChat (Augur) - Connect-RPC streaming
	http.post("*/alt.augur.v2.AugurService/StreamChat", () => {
		const body =
			AUGUR_CONNECT_MESSAGES.map((m) => JSON.stringify(m)).join("\n") + "\n";
		return new HttpResponse(body, {
			headers: {
				"Content-Type": "application/connect+json",
				"Connect-Content-Encoding": "identity",
				"Connect-Accept-Encoding": "identity",
			},
		});
	}),
];

// =============================================================================
// All Handlers
// =============================================================================

export const handlers = [
	...kratosHandlers,
	...authHubHandlers,
	...backendHandlers,
];
