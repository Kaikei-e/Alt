import { describe, it, expect, vi, beforeEach } from "vitest";
import type { Cookies } from "@sveltejs/kit";

vi.mock("$env/dynamic/private", () => ({
	env: { AUTH_HUB_INTERNAL_URL: "http://auth-hub:8888" },
}));

import { getCSRFToken, verifyCsrfToken, issueCsrfCookie } from "./auth";

describe("getCSRFToken", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("returns null without calling fetch when no cookie header is present", async () => {
		const fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);

		const result = await getCSRFToken(null);

		expect(result).toBeNull();
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("POSTs to auth-hub's /csrf endpoint forwarding the session cookie", async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ data: { csrf_token: "tok-123" } }), {
				status: 200,
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		await getCSRFToken("ory_kratos_session=abc");

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0] ?? [];
		expect(url).toBe("http://auth-hub:8888/csrf");
		expect(init?.method).toBe("POST");
		expect((init?.headers as Record<string, string>).Cookie).toBe(
			"ory_kratos_session=abc",
		);
	});

	it("returns the token parsed from the nested data.csrf_token field", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ data: { csrf_token: "tok-456" } }), {
					status: 200,
				}),
			),
		);

		const result = await getCSRFToken("ory_kratos_session=abc");

		expect(result).toBe("tok-456");
	});

	it("returns null when auth-hub responds with a non-2xx status", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(new Response("", { status: 401 })),
		);

		const result = await getCSRFToken("ory_kratos_session=abc");

		expect(result).toBeNull();
	});

	it("returns null when the response body has no data.csrf_token (e.g. the old flat shape)", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ csrf_token: "flat-shape" }), {
					status: 200,
				}),
			),
		);

		const result = await getCSRFToken("ory_kratos_session=abc");

		expect(result).toBeNull();
	});

	it("returns null when fetch throws", async () => {
		vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network")));

		const result = await getCSRFToken("ory_kratos_session=abc");

		expect(result).toBeNull();
	});
});

function makeCookies(store: Record<string, string> = {}): Cookies {
	return {
		get: vi.fn((name: string) => store[name]),
		getAll: vi.fn(() => []),
		set: vi.fn((name: string, value: string) => {
			store[name] = value;
		}),
		delete: vi.fn(),
		serialize: vi.fn(),
	} as unknown as Cookies;
}

describe("verifyCsrfToken", () => {
	it("returns true when the provided token matches the issued cookie", () => {
		const cookies = makeCookies({ csrf_token: "match-me" });

		expect(verifyCsrfToken(cookies, "match-me")).toBe(true);
	});

	it("returns false when the provided token does not match the cookie", () => {
		const cookies = makeCookies({ csrf_token: "match-me" });

		expect(verifyCsrfToken(cookies, "wrong-token")).toBe(false);
	});

	it("returns false when no csrf cookie was ever issued", () => {
		const cookies = makeCookies();

		expect(verifyCsrfToken(cookies, "anything")).toBe(false);
	});

	it("returns false when no token was provided", () => {
		const cookies = makeCookies({ csrf_token: "match-me" });

		expect(verifyCsrfToken(cookies, null)).toBe(false);
	});
});

describe("issueCsrfCookie", () => {
	it("sets an httpOnly, strict, root-scoped cookie mirroring the token", () => {
		const cookies = makeCookies();

		issueCsrfCookie(cookies, "issued-token");

		expect(cookies.set).toHaveBeenCalledWith(
			expect.any(String),
			"issued-token",
			expect.objectContaining({
				httpOnly: true,
				sameSite: "strict",
				path: "/",
			}),
		);
	});

	it("issues a cookie that verifyCsrfToken subsequently accepts", () => {
		const cookies = makeCookies();

		issueCsrfCookie(cookies, "round-trip-token");

		expect(verifyCsrfToken(cookies, "round-trip-token")).toBe(true);
	});
});
