import { beforeEach, describe, expect, it, vi } from "vitest";

const { getCSRFToken, issueCsrfCookie } = vi.hoisted(() => ({
	getCSRFToken: vi.fn(),
	issueCsrfCookie: vi.fn(),
}));

vi.mock("$lib/api", () => ({ getCSRFToken, issueCsrfCookie }));

import { GET } from "./+server";

function makeEvent(cookieHeader?: string) {
	const headers = new Headers();
	if (cookieHeader !== undefined) headers.set("cookie", cookieHeader);
	return {
		request: new Request("http://localhost/api/auth/csrf", { headers }),
		cookies: { get: vi.fn(), set: vi.fn(), delete: vi.fn() },
	} as unknown as Parameters<typeof GET>[0];
}

describe("GET /api/auth/csrf", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("returns 401 without issuing a cookie when auth-hub returns no token", async () => {
		getCSRFToken.mockResolvedValue(null);

		const event = makeEvent("ory_kratos_session=abc");
		const res = await GET(event);

		expect(res.status).toBe(401);
		expect(issueCsrfCookie).not.toHaveBeenCalled();
	});

	it("mirrors the auth-hub token into a cookie and returns it in the flat client contract shape", async () => {
		getCSRFToken.mockResolvedValue("tok-789");

		const event = makeEvent("ory_kratos_session=abc");
		const res = await GET(event);
		const body = await res.json();

		expect(res.status).toBe(200);
		expect(body).toEqual({ csrf_token: "tok-789" });
		expect(issueCsrfCookie).toHaveBeenCalledWith(event.cookies, "tok-789");
	});
});
