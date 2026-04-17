import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { BACKEND_CONNECT_URL: "http://backend.test" },
}));

vi.mock("$lib/server/auth", () => ({
	getBackendToken: vi.fn(),
}));

import { getBackendToken } from "$lib/server/auth";
import { GET } from "./+server";

const makeRequestEvent = () =>
	({
		request: new Request("http://localhost/api/v1/rss-feed-link/export/opml"),
	}) as Parameters<typeof GET>[0];

describe("GET /api/v1/rss-feed-link/export/opml", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.mocked(getBackendToken).mockResolvedValue("token");
	});

	it("does not leak error messages to the client when fetch throws", async () => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
		const secret = "internal stack trace with /var/www/app.js:42";
		vi.stubGlobal(
			"fetch",
			vi.fn().mockRejectedValue(new Error(secret)),
		);

		const res = await GET(makeRequestEvent());

		expect(res.status).toBe(500);
		const body = await res.json();
		expect(body).toEqual({ error: "Export failed" });
		expect(JSON.stringify(body)).not.toContain(secret);
		expect(consoleError).toHaveBeenCalled();
	});

	it("does not leak upstream error body on non-OK responses", async () => {
		vi.stubGlobal(
			"fetch",
			vi
				.fn()
				.mockResolvedValue(
					new Response("stack: /internal/path/handler.go:128", {
						status: 503,
					}),
				),
		);

		const res = await GET(makeRequestEvent());

		expect(res.status).toBe(503);
		const body = await res.json();
		expect(body).toEqual({ error: "Export failed" });
	});
});
