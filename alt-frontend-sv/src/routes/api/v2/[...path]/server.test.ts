import { beforeEach, describe, expect, it, vi } from "vitest";

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
	// is retired and its capabilities live on `alt.datahub.v1.DataHubService`
	// in alt-data-hub. The retired path stays in the table — it must remain
	// refused rather than merely unrouted — and the live one sits beside it.
	// Note the shape of the guard here differs from the BFF's: this proxy is an
	// allowlist (PROXYABLE_SERVICES), so a new east-west service is denied by
	// construction; the assertion pins that it is never added by accident.
	it.each([
		"alt.datahub.v1.DataHubService/CreateArticle",
		"alt.datahub.v1.DataHubService/ListArticlesWithTags",
		"alt.datahub.v1.DataHubService/ListRecapArticles",
		"services.backend.v1.BackendInternalService/CreateArticle",
		"services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
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
