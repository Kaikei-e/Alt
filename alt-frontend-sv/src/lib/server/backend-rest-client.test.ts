import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/env/private", () => ({
	BACKEND_REST_URL: "http://backend-rest.test",
	BACKEND_CONNECT_URL: "http://backend-connect.test",
}));

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

vi.mock("./auth", () => ({
	getBackendToken,
}));

import { callBackendAPI, callBackendAPIWithBody } from "./backend-rest-client";

describe("backend-rest-client", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("callBackendAPI", () => {
		it("uses options.backendToken directly without calling getBackendToken when provided", async () => {
			const fetchMock = vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ ok: true }), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				}),
			);

			const result = await callBackendAPI<{ ok: boolean }>(
				"/v1/test",
				"ory_kratos_session=abc",
				{
					fetch: fetchMock as unknown as typeof fetch,
					backendToken: "pre-fetched-token",
				},
			);

			expect(result).toEqual({ ok: true });
			expect(getBackendToken).not.toHaveBeenCalled();
			expect(fetchMock).toHaveBeenCalledTimes(1);
			const [url, init] = fetchMock.mock.calls[0] ?? [];
			expect(url).toBe("http://backend-rest.test/v1/test");
			expect(
				(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
			).toBe("pre-fetched-token");
		});

		it("does not call getBackendToken when options.backendToken is explicitly null", async () => {
			const fetchMock = vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ ok: true }), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				}),
			);

			await callBackendAPI<{ ok: boolean }>("/v1/test", "cookie=1", {
				fetch: fetchMock as unknown as typeof fetch,
				backendToken: null,
			});

			expect(getBackendToken).not.toHaveBeenCalled();
			const [, init] = fetchMock.mock.calls[0] ?? [];
			expect(
				(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
			).toBeUndefined();
		});

		it("falls back to getBackendToken(cookie) when options.backendToken is undefined", async () => {
			getBackendToken.mockResolvedValue("fetched-from-auth-hub");
			const fetchMock = vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ count: 5 }), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				}),
			);

			const result = await callBackendAPI<{ count: number }>(
				"/v1/feeds/count",
				"ory_kratos_session=xyz",
				{ fetch: fetchMock as unknown as typeof fetch },
			);

			expect(result).toEqual({ count: 5 });
			expect(getBackendToken).toHaveBeenCalledWith("ory_kratos_session=xyz");
			const [, init] = fetchMock.mock.calls[0] ?? [];
			expect(
				(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
			).toBe("fetched-from-auth-hub");
		});
	});

	describe("callBackendAPIWithBody", () => {
		it("uses options.backendToken directly without calling getBackendToken", async () => {
			const fetchMock = vi
				.fn()
				.mockResolvedValue(new Response(null, { status: 200 }));

			await callBackendAPIWithBody(
				"/v1/feeds/read",
				"ory_kratos_session=abc",
				"POST",
				{ feed_url: "https://example.com/rss" },
				{
					fetch: fetchMock as unknown as typeof fetch,
					backendToken: "cached-token",
				},
			);

			expect(getBackendToken).not.toHaveBeenCalled();
			expect(fetchMock).toHaveBeenCalledTimes(1);
			const [url, init] = fetchMock.mock.calls[0] ?? [];
			expect(url).toBe("http://backend-rest.test/v1/feeds/read");
			expect(init?.method).toBe("POST");
			expect(init?.body).toBe(
				JSON.stringify({ feed_url: "https://example.com/rss" }),
			);
			expect(
				(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
			).toBe("cached-token");
		});

		it("calls getBackendToken when options.backendToken is omitted", async () => {
			getBackendToken.mockResolvedValue("token-from-cookie");
			const fetchMock = vi
				.fn()
				.mockResolvedValue(new Response(null, { status: 204 }));

			await callBackendAPIWithBody(
				"/v1/rss-feed-link/123",
				"ory_kratos_session=abc",
				"DELETE",
				undefined,
				{ fetch: fetchMock as unknown as typeof fetch },
			);

			expect(getBackendToken).toHaveBeenCalledWith("ory_kratos_session=abc");
			const [, init] = fetchMock.mock.calls[0] ?? [];
			expect(
				(init?.headers as Record<string, string>)["X-Alt-Backend-Token"],
			).toBe("token-from-cookie");
		});
	});
});
