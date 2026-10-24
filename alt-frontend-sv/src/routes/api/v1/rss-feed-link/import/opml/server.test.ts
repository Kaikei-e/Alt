import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/env/private", () => ({
	BACKEND_REST_URL: "http://backend-rest.test",
	BACKEND_CONNECT_URL: "http://backend-connect.test",
	AUTH_HUB_INTERNAL_URL: "http://auth-hub:8888",
}));

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

vi.mock("#lib/server/auth.js", async (importOriginal) => {
	const actual = await importOriginal<typeof import("#lib/server/auth.js")>();
	return { ...actual, getBackendToken };
});

import { POST } from "./+server";

describe("POST /api/v1/rss-feed-link/import/opml", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("returns 403 when CSRF validation fails", async () => {
		const res = await POST({
			request: new Request(
				"http://localhost/api/v1/rss-feed-link/import/opml",
				{ method: "POST" },
			),
			cookies: { get: () => "expected-csrf" },
			locals: { backendToken: "locals-token" },
		} as unknown as Parameters<typeof POST>[0]);

		expect(res.status).toBe(403);
	});

	it("reuses locals.backendToken without calling getBackendToken and prefers BACKEND_REST_URL", async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ imported: 3 }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		const formData = new FormData();
		formData.append(
			"file",
			new File(["<opml version='2.0'></opml>"], "feeds.opml", {
				type: "application/xml",
			}),
		);

		const res = await POST({
			request: new Request(
				"http://localhost/api/v1/rss-feed-link/import/opml",
				{
					method: "POST",
					headers: { "X-CSRF-Token": "expected-csrf" },
					body: formData,
				},
			),
			cookies: {
				get: (name: string) =>
					name === "csrf_token" ? "expected-csrf" : undefined,
			},
			locals: { backendToken: "locals-jwt" },
		} as unknown as Parameters<typeof POST>[0]);

		expect(res.status).toBe(200);
		expect(await res.json()).toEqual({ imported: 3 });
		expect(getBackendToken).not.toHaveBeenCalled();
		const [calledUrl, init] = fetchMock.mock.calls[0] ?? [];
		expect(calledUrl).toBe(
			"http://backend-rest.test/v1/rss-feed-link/import/opml",
		);
		expect(
			(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
		).toBe("locals-jwt");
	});
});
