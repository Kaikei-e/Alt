import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/env/private", () => ({
	BACKEND_REST_URL: "http://backend-rest.test",
	BACKEND_CONNECT_URL: "http://backend-connect.test",
}));

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

vi.mock("#lib/api.js", () => ({
	getBackendToken,
}));

import { GET } from "./+server";

describe("GET /api/v1/feeds/stats/trends", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("reuses locals.backendToken without calling getBackendToken and prefers BACKEND_REST_URL", async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ points: [] }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		const res = await GET({
			request: new Request("http://localhost/api/v1/feeds/stats/trends"),
			url: new URL("http://localhost/api/v1/feeds/stats/trends?window=24h"),
			locals: { backendToken: "locals-token" },
		} as unknown as Parameters<typeof GET>[0]);

		expect(res.status).toBe(200);
		expect(getBackendToken).not.toHaveBeenCalled();
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [calledUrl, init] = fetchMock.mock.calls[0] ?? [];
		expect(calledUrl).toBe(
			"http://backend-rest.test/v1/feeds/stats/trends?window=24h",
		);
		expect(
			(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
		).toBe("locals-token");
	});

	it("returns 400 on invalid window parameter", async () => {
		const fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);

		const res = await GET({
			request: new Request("http://localhost/api/v1/feeds/stats/trends"),
			url: new URL("http://localhost/api/v1/feeds/stats/trends?window=99d"),
			locals: { backendToken: "locals-token" },
		} as unknown as Parameters<typeof GET>[0]);

		expect(res.status).toBe(400);
		expect(fetchMock).not.toHaveBeenCalled();
	});
});
