import fs from "fs";
import http from "http";
import path from "path";
import url from "url";

// Constants
const KRATOS_PORT = 4001;
const AUTH_HUB_PORT = 4002;
const BACKEND_PORT = 4003;

const KRATOS_SESSION_COOKIE_NAME = "ory_kratos_session";
const KRATOS_SESSION_COOKIE_VALUE = "e2e-session";

const buildKratosSessionPayload = () => {
	const now = new Date();
	return {
		id: "sess_e2e_fake",
		active: true,
		authenticated_at: now.toISOString(),
		expires_at: new Date(now.getTime() + 60 * 60 * 1000).toISOString(),
		issued_at: now.toISOString(),
		identity: {
			id: "user_e2e_fake",
			schema_id: "default",
			schema_url: "http://kratos/schemas/default",
			state: "active",
			traits: {
				email: "e2e@example.com",
				name: "E2E User",
			},
		},
		authentication_methods: [
			{
				method: "password",
				completed_at: now.toISOString(),
			},
		],
		metadata_public: {},
	};
};

const hasSessionCookie = (cookieHeader?: string) => {
	if (!cookieHeader) return false;
	return cookieHeader
		.split(";")
		.map((segment) => segment.trim())
		.includes(`${KRATOS_SESSION_COOKIE_NAME}=${KRATOS_SESSION_COOKIE_VALUE}`);
};

// TODO: replace bellow with dynamic path
const LOG_FILE = "mock-server.log";

function log(msg: string) {
	const line = `[${new Date().toISOString()}] ${msg}\n`;
	console.log(msg); // Keep console log for lightweight
	fs.appendFileSync(LOG_FILE, line);
}

// Mock Data from tests/e2e/mobile/feeds/index.spec.ts
const FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			url: "https://example.com/ai-trends",
			title: "AI Trends",
			description: "Deep dive into the ecosystem.",
			link: "https://example.com/ai-trends",
			published_at: "2025-12-20T10:00:00Z",
			tags: ["AI", "Tech"],
			author: { name: "Alice" },
			thumbnail: "https://example.com/thumb.jpg",
			feed_domain: "example.com",
			read_at: null,
			created_at: new Date().toISOString(),
			updated_at: new Date().toISOString(),
		},
		{
			id: "feed-2",
			url: "https://example.com/svelte-5",
			title: "Svelte 5 Tips",
			description: "Runes-first patterns for fast interfaces.",
			link: "https://example.com/svelte-5",
			published_at: "2025-12-19T09:00:00Z",
			tags: ["Svelte", "Web"],
			author: { name: "Bob" },
			thumbnail: null,
			feed_domain: "svelte.dev",
			read_at: null,
			created_at: new Date().toISOString(),
			updated_at: new Date().toISOString(),
		},
	],
	next_cursor: "next-cursor-123",
	has_more: true,
};

const VIEWED_FEEDS_EMPTY = {
	data: [],
	next_cursor: null,
	has_more: false,
};

// Connect-RPC response format (camelCase for JSON mapping)
const CONNECT_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			title: "AI Trends",
			description: "Deep dive into the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
		{
			id: "feed-2",
			title: "Svelte 5 Tips",
			description: "Runes-first patterns for fast interfaces.",
			link: "https://example.com/svelte-5",
			published: "1 day ago",
			createdAt: new Date().toISOString(),
			author: "Bob",
		},
	],
	nextCursor: "next-cursor-123",
	hasMore: true,
};

const CONNECT_READ_FEEDS_RESPONSE = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

const CONNECT_ARTICLE_CONTENT_RESPONSE = {
	url: "https://example.com/ai-trends",
	content: "<p>This is a mocked article content for E2E testing.</p>",
	articleId: "article-123",
};

// --- Kratos Server ---
const kratosServer = http.createServer((req, res) => {
	const parsedUrl = url.parse(req.url!, true);
	const path = parsedUrl.pathname;

	log(`[Mock Kratos] ${req.method} ${parsedUrl.pathname}`);

	res.setHeader("Access-Control-Allow-Origin", "*");
	res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
	res.setHeader("Access-Control-Allow-Headers", "Content-Type, Cookie");

	if (req.method === "OPTIONS") {
		res.writeHead(204);
		res.end();
		return;
	}

	if (path === "/sessions/whoami") {
		const cookieHeader = req.headers.cookie;
		if (hasSessionCookie(cookieHeader)) {
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(buildKratosSessionPayload()));
		} else {
			res.writeHead(401, { "Content-Type": "application/json" });
			res.end(
				JSON.stringify({
					error: {
						code: "session_not_found",
						message: "No active Kratos session cookie was provided.",
					},
				}),
			);
		}
		return;
	}

	res.writeHead(404);
	res.end("Not Found");
});

// --- Auth Hub Server ---
const authHubServer = http.createServer((req, res) => {
	const parsedUrl = url.parse(req.url!, true);
	const path = parsedUrl.pathname;

	log(`[Mock AuthHub] ${req.method} ${parsedUrl.pathname}`);

	if (path === "/session" || path === "/auth/session") {
		res.writeHead(200, {
			"Content-Type": "application/json",
			"X-Alt-Backend-Token": "mock-backend-token",
		});
		res.end(
			JSON.stringify({
				user_id: "user_e2e_fake",
				email: "e2e@example.com",
			}),
		);
		return;
	}

	res.writeHead(404);
	res.end("Not Found");
});

// --- Backend Server ---
const backendServer = http.createServer((req, res) => {
	const parsedUrl = url.parse(req.url!, true);
	const path = parsedUrl.pathname;

	log(`[Mock Backend] ${req.method} ${parsedUrl.pathname}`);

	res.setHeader("Content-Type", "application/json");

	if (
		path === "/api/v1/feeds/fetch/cursor" ||
		path === "/v1/feeds/fetch/cursor"
	) {
		res.writeHead(200);
		res.end(JSON.stringify(FEEDS_RESPONSE));
		return;
	}

	if (
		path === "/api/v1/feeds/fetch/viewed/cursor" ||
		path === "/v1/feeds/fetch/viewed/cursor"
	) {
		res.writeHead(200);
		res.end(JSON.stringify(VIEWED_FEEDS_EMPTY));
		return;
	}

	// RSS Feed Link List mock
	if (
		path === "/api/v1/rss-feed-link/list" ||
		path === "/v1/rss-feed-link/list"
	) {
		res.writeHead(200);
		res.end(JSON.stringify([]));
		return;
	}

	// Stats mock
	if (path === "/api/v1/feeds/stats" || path === "/v1/feeds/stats") {
		res.writeHead(200);
		res.end(
			JSON.stringify({
				total_feeds: 12,
				total_reads: 345,
				unread_count: 7,
			}),
		);
		return;
	}

	// Stats detailed mock
	if (
		path === "/api/v1/feeds/stats/detailed" ||
		path === "/v1/feeds/stats/detailed"
	) {
		res.writeHead(200);
		res.end(
			JSON.stringify({
				feed_amount: { amount: 10 },
				total_articles: { amount: 50 },
				unsummarized_articles: { amount: 5 },
			}),
		);
		return;
	}

	// Unread count mock
	if (
		path === "/api/v1/feeds/count/unreads" ||
		path === "/v1/feeds/count/unreads"
	) {
		res.writeHead(200);
		res.end(JSON.stringify({ count: 5 }));
		return;
	}

	// Mark as read mock
	if (path === "/api/v1/feeds/read" || path === "/v1/feeds/read") {
		res.writeHead(200);
		res.end(JSON.stringify({ ok: true }));
		return;
	}

	// Recap 7-days mock - matches RecapGenre schema
	if (path === "/api/v1/recap/7days" || path === "/v1/recap/7days") {
		res.writeHead(200);
		res.end(
			JSON.stringify({
				genres: [
					{
						genre: "Technology",
						summary: "Major developments in technology this week.",
						topTerms: ["AI", "Web", "Frameworks"],
						articleCount: 2,
						clusterCount: 1,
						evidenceLinks: [
							{ articleId: "art-1", title: "GPT-5 Announced", sourceUrl: "https://example.com/gpt5", publishedAt: "2025-12-20T10:00:00Z", lang: "en" },
							{ articleId: "art-2", title: "Claude Updates", sourceUrl: "https://example.com/claude", publishedAt: "2025-12-20T09:00:00Z", lang: "en" },
						],
						bullets: ["AI advances continue"],
					},
					{
						genre: "AI/ML",
						summary: "Latest papers and breakthroughs in ML.",
						topTerms: ["ML", "Research"],
						articleCount: 1,
						clusterCount: 1,
						evidenceLinks: [
							{ articleId: "art-3", title: "New Architecture", sourceUrl: "https://example.com/arch", publishedAt: "2025-12-19T10:00:00Z", lang: "en" },
						],
						bullets: ["New architecture proposed"],
					},
				],
			}),
		);
		return;
	}

	// Augur chat mock (streaming response) - uses SSE with event types
	if (path === "/api/v1/augur/chat" || path === "/v1/augur/chat") {
		res.writeHead(200, {
			"Content-Type": "text/event-stream",
			"Cache-Control": "no-cache",
			Connection: "keep-alive",
		});
		// Use proper SSE format with `event: delta` for text chunks
		res.write("event: delta\ndata: Based on your recent feeds, \n\n");
		res.write("event: delta\ndata: here are the key trends: \n\n");
		res.write("event: delta\ndata: AI development is accelerating.\n\n");
		res.write("event: done\ndata: {}\n\n");
		res.end();
		return;
	}

	// Article content mock
	if (path === "/api/v1/articles/content" || path === "/v1/articles/content") {
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
	// Connect-RPC v2 Endpoints (server-side SSR calls)
	// =============================================================================

	// Connect-RPC: GetUnreadFeeds
	if (path === "/alt.feeds.v2.FeedService/GetUnreadFeeds") {
		res.setHeader("Content-Type", "application/json");
		res.writeHead(200);
		res.end(JSON.stringify(CONNECT_FEEDS_RESPONSE));
		return;
	}

	// Connect-RPC: GetReadFeeds
	if (path === "/alt.feeds.v2.FeedService/GetReadFeeds") {
		res.setHeader("Content-Type", "application/json");
		res.writeHead(200);
		res.end(JSON.stringify(CONNECT_READ_FEEDS_RESPONSE));
		return;
	}

	// Connect-RPC: MarkAsRead
	if (path === "/alt.feeds.v2.FeedService/MarkAsRead") {
		res.setHeader("Content-Type", "application/json");
		res.writeHead(200);
		res.end(JSON.stringify({ message: "Feed marked as read" }));
		return;
	}

	// Connect-RPC: FetchArticleContent
	if (path === "/alt.articles.v2.ArticleService/FetchArticleContent") {
		res.setHeader("Content-Type", "application/json");
		res.writeHead(200);
		res.end(JSON.stringify(CONNECT_ARTICLE_CONTENT_RESPONSE));
		return;
	}

	// Connect-RPC: StreamChat (Augur) - streaming response
	if (path === "/alt.augur.v2.AugurService/StreamChat") {
		res.setHeader("Content-Type", "application/connect+json");
		res.setHeader("Connect-Content-Encoding", "identity");
		res.setHeader("Connect-Accept-Encoding", "identity");
		res.writeHead(200);
		// Connect-RPC streaming format: newline-delimited JSON
		const messages = [
			{ result: { kind: "delta", payload: { case: "delta", value: "Based on your recent feeds, " } } },
			{ result: { kind: "delta", payload: { case: "delta", value: "here are the key trends: " } } },
			{ result: { kind: "delta", payload: { case: "delta", value: "AI development is accelerating." } } },
			{
				result: {
					kind: "done",
					payload: {
						case: "done",
						value: {
							answer: "Based on your recent feeds, here are the key trends: AI development is accelerating.",
							citations: [
								{ url: "https://example.com/ai", title: "AI News", publishedAt: "2025-12-20T10:00:00Z" },
							],
						},
					},
				},
			},
			{ result: {} },
		];
		res.end(messages.map((m) => JSON.stringify(m)).join("\n") + "\n");
		return;
	}

	res.writeHead(200); // Default to 200 OK empty json for others to prevent crashes, or 404?
	// Ideally testing other endpoints will "fail" if they get empty json, but for SSR robustness empty is safer.
	res.end(JSON.stringify({}));
});

// Start servers
export async function startMockServers() {
	return new Promise<void>((resolve) => {
		let started = 0;
		const onStart = () => {
			started++;
			if (started === 3) resolve();
		};

		kratosServer.listen(KRATOS_PORT, "0.0.0.0", () => {
			console.log(`Mock Kratos running on port ${KRATOS_PORT}`);
			onStart();
		});
		authHubServer.listen(AUTH_HUB_PORT, "0.0.0.0", () => {
			console.log(`Mock AuthHub running on port ${AUTH_HUB_PORT}`);
			onStart();
		});
		backendServer.listen(BACKEND_PORT, "0.0.0.0", () => {
			console.log(`Mock Backend running on port ${BACKEND_PORT}`);
			onStart();
		});
	});
}

export async function stopMockServers() {
	return new Promise<void>((resolve) => {
		let closed = 0;
		const onClose = () => {
			closed++;
			if (closed === 3) resolve();
		};
		kratosServer.close(onClose);
		authHubServer.close(onClose);
		backendServer.close(onClose);
	});
}

// Graceful shutdown handlers for local development
process.on("SIGINT", async () => {
	console.log("\nShutting down mock servers gracefully...");
	await stopMockServers();
	process.exit(0);
});

process.on("SIGTERM", async () => {
	console.log("\nShutting down mock servers gracefully...");
	await stopMockServers();
	process.exit(0);
});
