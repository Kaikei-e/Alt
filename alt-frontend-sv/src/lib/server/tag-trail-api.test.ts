import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: {
		BACKEND_REST_URL: "http://backend.test",
	},
}));

const AUTH_HUB_URL = "http://auth-hub:8888";
const BACKEND_TOKEN = "backend-token-for-the-caller";

// SvelteKit hands `params.id` to the handler already decodeURIComponent'd, so a
// request for `..%2F..%2Fv1%2Fdashboard%2Foverview%3F` arrives here as literal
// dot segments plus a `?` that swallows the rest of the template into a query
// string. This is the raw value the handler sees, not the wire form.
const TRAVERSAL_ID = "../../v1/dashboard/overview?";

/**
 * Stubs global fetch: answers auth-hub's /session with a token so the request
 * to the backend carries one, and records every backend URL that was asked for.
 */
function stubFetch(body: unknown): { backendUrls: string[] } {
	const backendUrls: string[] = [];

	vi.stubGlobal(
		"fetch",
		vi.fn((input: RequestInfo | URL) => {
			const url = typeof input === "string" ? input : String(input);

			if (url.startsWith(AUTH_HUB_URL)) {
				return Promise.resolve(
					new Response(null, {
						status: 200,
						headers: { "X-Alt-Backend-Token": BACKEND_TOKEN },
					}),
				);
			}

			backendUrls.push(url);
			return Promise.resolve(
				new Response(JSON.stringify(body), {
					status: 200,
					headers: { "content-type": "application/json" },
				}),
			);
		}),
	);

	return { backendUrls };
}

function requestEvent(id: string) {
	return {
		request: new Request(`http://localhost/api/articles/${id}/tags`, {
			headers: { cookie: "ory_kratos_session=abc" },
		}),
		params: { id },
	} as never;
}

describe("tag trail route params", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("keeps a traversal article id inside /v1/articles/<id>/tags", async () => {
		const { backendUrls } = stubFetch({ article_id: TRAVERSAL_ID, tags: [] });

		const { GET } = await import("../../routes/api/articles/[id]/tags/+server");
		await GET(requestEvent(TRAVERSAL_ID));

		expect(backendUrls).toHaveLength(1);
		const requested = new URL(backendUrls[0] as string);
		expect(requested.pathname.startsWith("/v1/articles/")).toBe(true);
		expect(requested.pathname.endsWith("/tags")).toBe(true);
		expect(requested.search).toBe("");
	});

	it("keeps a well-formed article id addressing the same endpoint", async () => {
		const articleId = "550e8400-e29b-41d4-a716-446655440000";
		const { backendUrls } = stubFetch({ article_id: articleId, tags: [] });

		const { GET } = await import("../../routes/api/articles/[id]/tags/+server");
		await GET(requestEvent(articleId));

		const requested = new URL(backendUrls[0] as string);
		expect(requested.pathname).toBe(`/v1/articles/${articleId}/tags`);
		expect(requested.search).toBe("");
	});

	it("keeps a traversal feed id inside /v1/feeds/<id>/tags", async () => {
		const { backendUrls } = stubFetch({ feed_id: TRAVERSAL_ID, tags: [] });

		const { GET } = await import("../../routes/api/feeds/[id]/tags/+server");
		await GET({
			request: new Request(`http://localhost/api/feeds/${TRAVERSAL_ID}/tags`, {
				headers: { cookie: "ory_kratos_session=abc" },
			}),
			params: { id: TRAVERSAL_ID },
		} as never);

		expect(backendUrls).toHaveLength(1);
		const requested = new URL(backendUrls[0] as string);
		expect(requested.pathname.startsWith("/v1/feeds/")).toBe(true);
		expect(requested.pathname.endsWith("/tags")).toBe(true);
		expect(requested.search).toBe("?limit=20");
	});

	it("keeps a well-formed feed id addressing the same endpoint", async () => {
		const feedId = "9f2c1ab0-0000-4000-8000-00000000feed";
		const { backendUrls } = stubFetch({ feed_id: feedId, tags: [] });

		const { GET } = await import("../../routes/api/feeds/[id]/tags/+server");
		await GET({
			request: new Request(`http://localhost/api/feeds/${feedId}/tags`, {
				headers: { cookie: "ory_kratos_session=abc" },
			}),
			params: { id: feedId },
		} as never);

		const requested = new URL(backendUrls[0] as string);
		expect(requested.pathname).toBe(`/v1/feeds/${feedId}/tags`);
		expect(requested.search).toBe("?limit=20");
	});
});
