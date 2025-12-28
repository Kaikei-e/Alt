import http from "http";
import url from "url";
import fs from "fs";
import path from "path";

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
