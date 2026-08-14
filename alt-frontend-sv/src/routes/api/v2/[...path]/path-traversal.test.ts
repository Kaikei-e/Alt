import { beforeEach, describe, expect, it, vi } from "vitest";

import { fallback } from "./+server";

// SvelteKit hands the rest param already percent-decoded, so `%5C` arrives as a
// literal backslash and `%2E%2E` as a literal `..`. The WHATWG URL parser that
// `fetch` runs then treats `\` as a path separator for http(s) and collapses
// dot segments, so a method name carrying either escapes the service prefix and
// reaches an arbitrary backend path — with the proxy's backend token attached.
// Any negative-form guard (split, blacklist, substring scan) loses this race;
// only a positive method-name format check closes it.
function makeEvent(path: string, token: string | null = "backend-token") {
	return {
		request: new Request("http://localhost/api/v2/proxied", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: "{}",
		}),
		params: { path },
		locals: { backendToken: token },
	} as unknown as Parameters<typeof fallback>[0];
}

describe("POST /api/v2/[...path] method-name validation", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response("{}", { status: 200 })),
		);
	});

	// Every entry names a service that IS on the generated allowlist, so the
	// only thing standing between the caller and an arbitrary backend path is
	// the method-name check.
	it.each([
		["encoded backslash traversal", "Get\\..\\..\\v1\\dashboard\\overview"],
		["bare backslash traversal", "..\\..\\v1\\aggregate"],
		["single backslash segment", "Get\\Other"],
		["dot segment", ".."],
		["query string smuggled into the method", "GetFeedStats?admin=1"],
		["fragment smuggled into the method", "GetFeedStats#/v1/aggregate"],
		["still-encoded slash", "GetFeedStats%2F..%2Fv1"],
		["absolute path escape", "/v1/dashboard/overview"],
	])("refuses %s", async (_label, method) => {
		const res = await fallback(makeEvent(`alt.feeds.v2.FeedService/${method}`));

		expect(res.status).toBe(404);
		expect(globalThis.fetch).not.toHaveBeenCalled();
	});

	it("still proxies a well-formed Connect method", async () => {
		const res = await fallback(
			makeEvent("alt.feeds.v2.FeedService/GetFeedStats"),
		);

		expect(res.status).toBe(200);
		expect(globalThis.fetch).toHaveBeenCalledTimes(1);
		expect(vi.mocked(globalThis.fetch).mock.calls[0]?.[0]).toBe(
			"http://alt-backend:9101/alt.feeds.v2.FeedService/GetFeedStats",
		);
	});
});
