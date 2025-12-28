import http from "http";
import url from "url";

const PORT = 4435;

const sessionPayload = {
	id: "sess_e2e",
	active: true,
	expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
	issued_at: new Date().toISOString(),
	identity: {
		id: "user_e2e",
		schema_id: "default",
		schema_url: "http://kratos/schemas/default",
		state: "active",
		state_changed_at: new Date().toISOString(),
		traits: {
			email: "e2e@example.com",
			name: "E2E User",
		},
		verifiable_addresses: [],
		recovery_addresses: [],
		metadata_public: null,
		metadata_admin: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
	},
};

const server = http.createServer((req, res) => {
	const parsedUrl = url.parse(req.url, true);
	const path = parsedUrl.pathname;

	// Add CORS headers
	res.setHeader("Access-Control-Allow-Origin", "*");
	res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
	res.setHeader("Access-Control-Allow-Headers", "Content-Type, Cookie");

	if (req.method === "OPTIONS") {
		res.writeHead(204);
		res.end();
		return;
	}

	console.log(`[kratos-mock] Request: ${req.method} ${path}`);

	if (path === "/sessions/whoami") {
		const cookie = req.headers.cookie;
		if (cookie && cookie.includes("ory_kratos_session=e2e-session")) {
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(sessionPayload));
		} else {
			res.writeHead(401, { "Content-Type": "application/json" });
			res.end(
				JSON.stringify({
					error: { code: 401, message: "No active session found" },
				}),
			);
		}
		return;
	}

	// Auth Hub Mock
	if (path === "/session") {
		res.writeHead(200, {
			"Content-Type": "application/json",
			"X-Alt-Backend-Token": "mock-backend-token",
		});
		res.end(JSON.stringify({ status: "ok" }));
		return;
	}

	// Backend API Mock: /v1/feeds/fetch/cursor
	if (path === "/v1/feeds/fetch/cursor") {
		const mockFeeds = {
			data: [
				{
					id: "feed-1",
					url: "https://example.com/feed1",
					title: "AI Trends",
					description: "Latest news on AI",
					published_at: new Date().toISOString(),
					tags: ["AI", "Tech"],
					thumbnail: "https://example.com/thumb.jpg",
					feed_domain: "example.com",
					read_at: null,
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString(),
				},
				{
					id: "feed-2",
					url: "https://example.com/feed2",
					title: "SvelteKit Updates",
					description: "New features in SvelteKit",
					published_at: new Date().toISOString(),
					tags: ["Svelte", "Web"],
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
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(JSON.stringify(mockFeeds));
		return;
	}

	// Backend API Mock: /v1/feeds/stats/detailed
	if (path === "/v1/feeds/stats/detailed") {
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(
			JSON.stringify({
				feed_amount: { amount: 10 },
				total_articles: { amount: 50 },
				unsummarized_articles: { amount: 5 },
			}),
		);
		return;
	}

	// Backend API Mock: /v1/feeds/count/unreads
	if (path === "/v1/feeds/count/unreads") {
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(JSON.stringify({ count: 5 }));
		return;
	}

	// Health check
	if (path === "/health/ready" || path === "/health/alive") {
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(JSON.stringify({ status: "ok" }));
		return;
	}

	res.writeHead(404, { "Content-Type": "application/json" });
	res.end(JSON.stringify({ error: "Not Found" }));
});

server.on("error", (e) => {
	if (e.code === "EADDRINUSE") {
		console.log(
			`[kratos-mock] Port ${PORT} already in use, assuming server is running.`,
		);
		process.exit(0);
	} else {
		console.error("[kratos-mock] Server error:", e);
		process.exit(1);
	}
});

server.listen(PORT, "127.0.0.1", () => {
	console.log(
		`[kratos-mock] specialized Kratos mock listening on port ${PORT}`,
	);
});
