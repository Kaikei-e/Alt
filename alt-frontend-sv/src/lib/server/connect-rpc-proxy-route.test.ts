import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: {
		BACKEND_CONNECT_URL: "http://backend.test",
	},
}));

describe("Connect-RPC proxy route", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it("adds no-buffering headers for application/connect+json responses", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve(
					new Response('{"chunk":"hello"}', {
						status: 200,
						headers: {
							"content-type": "application/connect+json; charset=utf-8",
						},
					}),
				),
			),
		);

		const { fallback } = await import("../../routes/api/v2/[...path]/+server");
		const response = await fallback({
			request: new Request(
				"http://localhost/api/v2/alt.feeds.v2.FeedService/StreamSummarize",
				{
					method: "POST",
				},
			),
			params: { path: "alt.feeds.v2.FeedService/StreamSummarize" },
			locals: { backendToken: "token-123" },
		} as never);

		expect(response.headers.get("content-type")).toContain(
			"application/connect+json",
		);
		expect(response.headers.get("X-Accel-Buffering")).toBe("no");
		expect(response.headers.get("Cache-Control")).toBe("no-cache, no-transform");
		expect(response.headers.get("Alt-Svc")).toBe("clear");
	});

	it("adds no-buffering headers for application/connect+proto responses", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve(
					new Response(new Uint8Array([0x00]), {
						status: 200,
						headers: {
							"content-type": "application/connect+proto",
						},
					}),
				),
			),
		);

		const { fallback } = await import("../../routes/api/v2/[...path]/+server");
		const response = await fallback({
			request: new Request(
				"http://localhost/api/v2/alt.augur.v2.AugurService/StreamChat",
				{
					method: "POST",
				},
			),
			params: { path: "alt.augur.v2.AugurService/StreamChat" },
			locals: { backendToken: "token-123" },
		} as never);

		expect(response.headers.get("X-Accel-Buffering")).toBe("no");
		expect(response.headers.get("Cache-Control")).toBe("no-cache, no-transform");
		expect(response.headers.get("Alt-Svc")).toBe("clear");
	});

	it("does not add no-buffering headers for regular json responses", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve(
					new Response('{"ok":true}', {
						status: 200,
						headers: {
							"content-type": "application/json",
						},
					}),
				),
			),
		);

		const { fallback } = await import("../../routes/api/v2/[...path]/+server");
		const response = await fallback({
			request: new Request(
				"http://localhost/api/v2/alt.feeds.v2.FeedService/GetAllFeeds",
				{
					method: "POST",
				},
			),
			params: { path: "alt.feeds.v2.FeedService/GetAllFeeds" },
			locals: { backendToken: "token-123" },
		} as never);

		expect(response.headers.get("content-type")).toBe("application/json");
		expect(response.headers.get("X-Accel-Buffering")).toBeNull();
		expect(response.headers.get("Cache-Control")).toBeNull();
		expect(response.headers.get("Alt-Svc")).toBeNull();
	});
});
