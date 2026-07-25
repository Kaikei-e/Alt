import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { RECAP_WORKER_BASE_URL: "http://recap-worker.test" },
}));

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

// Real verifyCsrfToken runs so this is an end-to-end test of the route's
// double-submit-cookie guard, not just a mocked comparison.
vi.mock("$lib/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/api")>();
	return { ...actual, getBackendToken };
});

import { POST } from "./+server";

function makeEvent(
	body: unknown,
	opts: { csrfHeader?: string; cookieCsrf?: string } = {},
) {
	const csrfHeader = opts.csrfHeader;
	// Distinguish "cookieCsrf explicitly omitted from the option bag" (defaults
	// to a token) from "cookieCsrf explicitly set to undefined" (no cookie was
	// ever issued) — a plain destructure default collapses both to the same
	// case since it also fires on an explicit `undefined` property value.
	const cookieCsrf = "cookieCsrf" in opts ? opts.cookieCsrf : "expected-token";
	const headers = new Headers({ "Content-Type": "application/json" });
	if (csrfHeader !== undefined) headers.set("X-CSRF-Token", csrfHeader);
	headers.set("cookie", "ory_kratos_session=abc");
	return {
		request: new Request("http://localhost/api/v1/generate/recaps/3days", {
			method: "POST",
			headers,
			body: JSON.stringify(body),
		}),
		cookies: {
			get: (name: string) => (name === "csrf_token" ? cookieCsrf : undefined),
		},
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/v1/generate/recaps/3days — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getBackendToken.mockResolvedValue("backend-token");
		vi.stubGlobal(
			"fetch",
			vi
				.fn()
				.mockResolvedValue(
					new Response(JSON.stringify({ job_id: "1" }), { status: 200 }),
				),
		);
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		const res = await POST(makeEvent({}));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the double-submit cookie", async () => {
		const res = await POST(makeEvent({}, { csrfHeader: "wrong-token" }));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when no csrf cookie was ever issued", async () => {
		const res = await POST(
			makeEvent({}, { csrfHeader: "expected-token", cookieCsrf: undefined }),
		);

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the double-submit cookie", async () => {
		const res = await POST(makeEvent({}, { csrfHeader: "expected-token" }));

		expect(res.status).toBe(200);
		expect(fetch).toHaveBeenCalledTimes(1);
	});
});
