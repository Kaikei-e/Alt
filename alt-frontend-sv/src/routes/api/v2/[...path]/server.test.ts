import net from "node:net";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createBackendServer } from "../../../../../tests/e2e/infra/handlers/backend";
import { fallback } from "./+server";

function makeEvent(path: string, token: string | null = "backend-token") {
	return {
		request: new Request(`http://localhost/api/v2/${path}`, {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: "{}",
		}),
		params: { path },
		locals: { backendToken: token },
	} as unknown as Parameters<typeof fallback>[0];
}

describe("POST /api/v2/[...path]", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response("{}", { status: 200 })),
		);
	});

	// The proxy attaches a valid backend token to whatever path the caller
	// picked. Without an allowlist, a self-registered user could steer it at
	// the service-to-service RPC surface, which has no user-JWT check of its
	// own.
	//
	// ADR-000954 moved that surface: `services.backend.v1.BackendInternalService`
	// is retired and its capabilities live on `services.datahub.v1.DataHubService`
	// in alt-data-hub. The retired path stays in the table — it must remain
	// refused rather than merely unrouted — and the live one sits beside it.
	// Both gates on this hop are now allowlists: this proxy and the BFF each
	// generate their set from the same `(alt.api.v1.visibility)` proto option
	// (ADR-000955), so an east-west service is denied by construction at both
	// and can only become reachable by annotating its proto. `alt.*` is not a
	// pass: the `alt.preprocessor.v2` row is an alt-rooted name absent from the
	// generated list, and default-deny still refuses it. The assertion pins
	// that none of these is ever added by accident.
	it.each([
		"services.datahub.v1.DataHubService/CreateArticle",
		"services.datahub.v1.DataHubService/ListArticlesWithTags",
		"services.datahub.v1.DataHubService/ListRecapArticles",
		"services.backend.v1.BackendInternalService/CreateArticle",
		"services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
		"alt.preprocessor.v2.PreProcessorService/Summarize",
		"alt.knowledge_home.v1.KnowledgeHomeAdminService/StartReproject",
	])("refuses to proxy %s", async (path) => {
		const res = await fallback(makeEvent(path));

		expect(res.status).toBe(404);
		expect(globalThis.fetch).not.toHaveBeenCalled();
	});

	it.each([
		"alt.feeds.v2.FeedService/GetFeedStats",
		"alt.articles.v2.ArticleService/GetArticle",
		"alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
	])("proxies %s", async (path) => {
		const res = await fallback(makeEvent(path));

		expect(res.status).toBe(200);
		expect(globalThis.fetch).toHaveBeenCalledTimes(1);
	});

	it("still requires a backend token", async () => {
		const res = await fallback(
			makeEvent("alt.feeds.v2.FeedService/GetFeedStats", null),
		);

		expect(res.status).toBe(401);
		expect(globalThis.fetch).not.toHaveBeenCalled();
	});
});

// The allowlist is generated per *service* from `(alt.api.v1.visibility)`, and
// the gate reads only the service half of the path, so a new procedure on an
// already-public service is proxyable the moment its proto lands — nothing to
// regenerate, and nothing to hand-edit. Pinned because the alternative
// (discovering it as a 404 from the browser) is indistinguishable from the RPC
// not being deployed.
describe("procedures on an already-public service", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response("{}", { status: 200 })),
		);
	});

	it("proxies alt.articles.v2.ArticleService/BatchPrefetchArticleContent", async () => {
		const res = await fallback(
			makeEvent("alt.articles.v2.ArticleService/BatchPrefetchArticleContent"),
		);

		expect(res.status).toBe(200);
		expect(globalThis.fetch).toHaveBeenCalledTimes(1);
	});

	it("buffers the request body into an ArrayBuffer instead of streaming request.body with duplex half", async () => {
		const res = await fallback(
			makeEvent("alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction"),
		);

		expect(res.status).toBe(200);
		const [, init] = vi.mocked(globalThis.fetch).mock.calls[0] ?? [];
		expect(init?.body).toBeInstanceOf(ArrayBuffer);
		expect((init?.body as ArrayBuffer).byteLength).toBe(2);
	});
});

describe("keep-alive framing between Connect-RPC proxy and E2E mock backend", () => {
	it("waits for POST request body completion before responding so a reused keep-alive socket does not return HTTP 400 on FetchRandomFeed", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}

		try {
			const secondResponseStatusLine = await new Promise<string>(
				(resolve, reject) => {
					const socket = net.connect(address.port, "127.0.0.1");
					let raw = "";
					let sentSecondRequest = false;
					let bodyTimer: ReturnType<typeof setTimeout> | null = null;

					socket.on("connect", () => {
						// Matches Bun's chunked POST when headers flush before the body stream.
						socket.write(
							"POST /alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction HTTP/1.1\r\n" +
								`Host: 127.0.0.1:${address.port}\r\n` +
								"Content-Type: application/json\r\n" +
								"Transfer-Encoding: chunked\r\n" +
								"Connection: keep-alive\r\n\r\n",
						);
						bodyTimer = setTimeout(() => {
							socket.write("2\r\n{}\r\n0\r\n\r\n");
						}, 20);
					});

					socket.on("data", (chunk) => {
						raw += chunk.toString("utf-8");
						if (!sentSecondRequest && raw.includes("0\r\n\r\n")) {
							// When the server finishes the first response, Bun stops streaming
							// the first request body and reuses the keep-alive socket for SSR FetchRandomFeed.
							if (bodyTimer) {
								clearTimeout(bodyTimer);
								bodyTimer = null;
							}
							sentSecondRequest = true;
							raw = "";
							socket.write(
								"POST /alt.articles.v2.ArticleService/FetchRandomFeed HTTP/1.1\r\n" +
									`Host: 127.0.0.1:${address.port}\r\n` +
									"Content-Type: application/json\r\n" +
									"Content-Length: 2\r\n" +
									"Connection: close\r\n\r\n{}",
							);
							return;
						}
						if (sentSecondRequest && raw.includes("\r\n")) {
							const firstLine = raw.split("\r\n")[0] ?? "";
							socket.destroy();
							resolve(firstLine);
						}
					});

					socket.on("error", reject);
				},
			);

			expect(secondResponseStatusLine).toBe("HTTP/1.1 200 OK");
		} finally {
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});
});
